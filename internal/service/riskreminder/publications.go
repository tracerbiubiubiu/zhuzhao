// publications.go — 接单提醒发布的真实存储读取面。
//
// 发布源 = 工单钉版的不可变定义快照（ticket_type_definitions.snapshot）内
// transition_meta[待接单状态].first_accept_reminder 显式条目；发布键 =
// type_code + 精确钉版 definition_version + definition_hash，每次读取经
// repository.TicketRepo.BoundTicketDefinitionTx 在 SQL 内校验快照哈希。
//
// 三态合同：
//   - 有效发布       → (*AcceptancePolicy, nil)
//   - 无显式条目     → (nil, ErrReminderPolicyAbsent)——唯一合法的「无提醒事实」路径
//   - 条目存在但失配 → (nil, err)——钉版读取失败/哈希失配/条目损坏/多状态重复发布，
//     全部显式报错：不静默跳过、不伪投递、不从节点 SLA
//     （timeout_minutes/sla_due_hours/escalation）外推。
//
// 时限只取发布值（accept_within_minutes），生产时限不写死在本包。
package riskreminder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// 发布条目的 JSON 键域（state 级 transition_meta 内的显式子对象）。
const acceptReminderMetaKey = "first_accept_reminder"

// ErrReminderPolicyAbsent 钉版定义未发布接单提醒策略——合法路径而非故障，
// 调用方据此不落任何提醒事实。与 ErrPolicyUnavailable（源已接但失配/不可用，
// 可见错误且意图留 pending 可恢复）严格区分。
var ErrReminderPolicyAbsent = errors.New("first-acceptance reminder policy not published")

// acceptancePublication 解析后的发布条目（state 键即待接单状态）。
type acceptancePublication struct {
	AwaitingState       string
	RuleID              string
	RuleVersion         int64
	AcceptWithinMinutes int64
}

// parseAcceptancePublication 从钉版快照解析唯一发布条目。按 meta 键序遍历保证
// 错误路径确定性；transition 级键（"from>to"）是另一元数据域——不构成发布；
// 节点 SLA 字段（timeout_minutes/sla_due_hours/escalation）一律忽略——绝不
// 由此外推提醒时限；状态域之外的显式条目按可见错误拒绝。
func parseAcceptancePublication(snapshot *model.TicketDefinitionSnapshot) (*acceptancePublication, error) {
	if snapshot == nil {
		return nil, ErrPolicyUnavailable
	}
	var states []string
	if json.Unmarshal(snapshot.Type.States, &states) != nil {
		return nil, fmt.Errorf("reminder publication states unreadable")
	}
	var meta map[string]json.RawMessage
	if json.Unmarshal(snapshot.Type.TransitionMeta, &meta) != nil {
		return nil, fmt.Errorf("reminder publication transition_meta unreadable")
	}
	inStates := make(map[string]bool, len(states))
	for _, state := range states {
		inStates[state] = true
	}
	keys := make([]string, 0, len(meta))
	for key := range meta {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var found *acceptancePublication
	for _, key := range keys {
		raw := meta[key]
		if len(raw) == 0 {
			continue
		}
		var seg struct {
			Entry json.RawMessage `json:"first_accept_reminder"`
		}
		if json.Unmarshal(raw, &seg) != nil || seg.Entry == nil {
			continue // 该键未发布本条目（键级损坏由 BoundTicketDefinitionTx 的图校验拦截）
		}
		trimmed := trimJSONSpace(seg.Entry)
		if string(trimmed) == "null" {
			continue // 显式置 null 视为未发布
		}
		if !inStates[key] {
			if strings.Contains(key, ">") {
				continue // transition 级键是另一元数据域——不构成发布
			}
			return nil, fmt.Errorf("reminder publication under unknown state key %q (not in states)", key)
		}
		if found != nil {
			return nil, fmt.Errorf("reminder publication duplicated across states %q and %q", found.AwaitingState, key)
		}
		var fields model.RiskFirstAcceptRule
		if json.Unmarshal(trimmed, &fields) != nil {
			return nil, fmt.Errorf("reminder publication transition_meta[%q].%s unreadable", key, acceptReminderMetaKey)
		}
		if !fields.Valid() {
			return nil, fmt.Errorf("reminder publication transition_meta[%q].%s has an invalid rule ID, version, or duration", key, acceptReminderMetaKey)
		}
		found = &acceptancePublication{
			AwaitingState:       key,
			RuleID:              fields.RuleID,
			RuleVersion:         fields.RuleVersion,
			AcceptWithinMinutes: fields.AcceptWithinMinutes,
		}
	}
	if found == nil {
		return nil, ErrReminderPolicyAbsent
	}
	return found, nil
}

// BoundAcceptancePolicyTx 在调用方事务上读取工单钉版的接单提醒发布（三态合同见
// 文件头）。只读：不取任何锁、不另取连接、无外部 I/O；db 仅用于构造真实存储
// repo，读取全部走传入 tx——未提交的钉版/票面事实对本读可见，隐藏连接读不可达。
func BoundAcceptancePolicyTx(ctx context.Context, db *pgxpool.Pool, tx pgx.Tx, ticketID int64) (*AcceptancePolicy, error) {
	policy, _, err := boundAcceptancePublicationTx(ctx, db, tx, ticketID)
	return policy, err
}

// boundAcceptancePublicationTx 三态读并带回钉版快照（接单边校验需要图）。
func boundAcceptancePublicationTx(ctx context.Context, db *pgxpool.Pool, tx pgx.Tx, ticketID int64) (*AcceptancePolicy, *model.TicketDefinitionSnapshot, error) {
	if db == nil || tx == nil || ticketID <= 0 {
		return nil, nil, errcode.ErrInvalidParams
	}
	var typeCode string
	var version *int64
	err := tx.QueryRow(ctx, `SELECT type_code, definition_version FROM tickets WHERE id=$1`, ticketID).
		Scan(&typeCode, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read reminder publication ticket: %w", err)
	}
	// 真实存储 BoundTicketDefinitionTx：SQL 内定义 definition_hash=快照哈希——
	// 钉版缺失/哈希失配/读失败在此显式报错，不降级为「无发布」。
	snapshot, ref, err := repository.NewTicketRepo(db).BoundTicketDefinitionTx(ctx, tx, typeCode, version)
	if err != nil {
		return nil, nil, err
	}
	pub, err := parseAcceptancePublication(snapshot)
	if err != nil {
		return nil, nil, err
	}
	var transitions map[string][]string
	if json.Unmarshal(snapshot.Type.Transitions, &transitions) != nil || pub.AwaitingState != "assigned" ||
		!slices.Contains(transitions["assigned"], "in_progress") {
		return nil, nil, ErrPolicyUnavailable
	}
	policy := &AcceptancePolicy{
		DefinitionRef: *ref,
		RuleID:        pub.RuleID,
		RuleVersion:   pub.RuleVersion,
		AwaitingState: pub.AwaitingState,
		AcceptWithin:  time.Duration(pub.AcceptWithinMinutes) * time.Minute,
	}
	return policy, snapshot, nil
}

// NewBoundPolicyResolver 生产装配面：把真实存储发布读适配为内核 PolicyResolver，
// 交付 NewRuntime。无发布 → (nil, nil)——checker 收敛为 ErrPolicyUnavailable
// （投递留 pending 可恢复，不抑止不入箱）；失配/读失败原样上抛。
// 测试的合成策略不是生产授权：生产装配必须用本构造器。
func NewBoundPolicyResolver(db *pgxpool.Pool) PolicyResolver {
	return func(ctx context.Context, tx pgx.Tx, ticketID int64) (*AcceptancePolicy, error) {
		policy, err := BoundAcceptancePolicyTx(ctx, db, tx, ticketID)
		if errors.Is(err, ErrReminderPolicyAbsent) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return policy, nil
	}
}

// trimJSONSpace 去除 JSON 前导空白（RawMessage 值比较用）。
func trimJSONSpace(raw json.RawMessage) json.RawMessage {
	start := 0
	for start < len(raw) && (raw[start] == ' ' || raw[start] == '\t' || raw[start] == '\n' || raw[start] == '\r') {
		start++
	}
	end := len(raw)
	for end > start && (raw[end-1] == ' ' || raw[end-1] == '\t' || raw[end-1] == '\n' || raw[end-1] == '\r') {
		end--
	}
	return raw[start:end]
}
