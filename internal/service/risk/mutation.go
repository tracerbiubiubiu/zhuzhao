package risk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

var validLevels = map[string]bool{"urgent": true, "high": true, "medium": true, "low": true}
var validTreatments = map[string]bool{"mitigate": true, "accept": true, "transfer": true, "avoid": true}

// riskManualScope 人工源命名空间（详设 §2.1：服务端建立，不接受外部 source_ref）。
const riskManualScope = "manual"

// canonicalHash 写命令请求指纹（同号同参幂等 / 同号异参 409 的判定输入）。
func canonicalHash(v any) []byte {
	sum := sha256.Sum256(jsonBytes(v))
	return sum[:]
}

type createFingerprint struct {
	Command       string                  `json:"command"`
	Title         string                  `json:"title"`
	Description   string                  `json:"description"`
	OrgID         int64                   `json:"org_id"`
	AffectedOrgID *int64                  `json:"affected_org_id,omitempty"`
	OwnerUserID   *int64                  `json:"owner_user_id,omitempty"`
	SystemRef     string                  `json:"system_ref,omitempty"`
	FindingKey    string                  `json:"finding_key,omitempty"`
	AssetRef      *model.RiskAssetRef     `json:"asset_ref,omitempty"`
	EvidenceRefs  []model.RiskEvidenceRef `json:"evidence_refs,omitempty"`
	Attributes    any                     `json:"attributes,omitempty"`
}

// validateManualFindingIdentity 人工登记的稳定漏洞/资产身份成对校验
// （B2 第 6 项 P1：二者成对、后兼容；结构化字段直收，自由 attributes
// 不充当来源）。长度沿 risk_items 列宽：finding_key 200 / provider 100 / external_id 200。
func validateManualFindingIdentity(findingKey string, ref *model.RiskAssetRef) error {
	if findingKey == "" && ref == nil {
		return nil
	}
	if findingKey == "" || ref == nil {
		return errcode.ErrInvalidParams
	}
	if len([]rune(findingKey)) > 200 || strings.TrimSpace(findingKey) == "" ||
		len([]rune(ref.Provider)) > 100 || strings.TrimSpace(ref.Provider) == "" ||
		len([]rune(ref.ExternalID)) > 200 || strings.TrimSpace(ref.ExternalID) == "" {
		return errcode.ErrInvalidParams
	}
	return nil
}

func parseExpectedRevision(s string, allowNew bool) (int64, error) {
	if allowNew && (s == "" || s == "0") {
		return 0, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 1 {
		return 0, errcode.ErrInvalidParams
	}
	return n, nil
}

func normalizeJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return map[string]any{}
	}
	return v
}

func jsonArrayBytes(v []model.RiskEvidenceRef) []byte {
	if len(v) == 0 {
		return []byte("[]")
	}
	return jsonBytes(v)
}

// orgManageOnOrgSQL 主锚管理范围谓词（目标组织别名为 go，actor 绑 $2）。
func orgManageOnOrgSQL() string {
	return `EXISTS (
		SELECT 1 FROM user_orgs m
		JOIN organizations mo ON mo.id = m.org_id
		WHERE m.user_id = $2
		  AND (` + repository.OrgSourceActiveSQL("mo") + `)
		  AND (m.expires_at IS NULL OR m.expires_at > NOW())
		  AND m.org_member_role IN ('admin','owner')
		  AND go.path <@ mo.path
	)`
}

// Create 人工登记（POST /risk-items/create；详设 §2.1）。
// 人工源命名空间由服务端建立（family scope=manual，键为服务端生成身份），
// 不接受外部 source_ref 伪装扫描结论；L3 要求主锚管理范围。
func (s *Service) Create(ctx context.Context, req model.RiskCreateRequest, actor int64) (*model.RiskMutationResult, error) {
	if req.Title == "" || req.OrgID <= 0 {
		return nil, errcode.ErrInvalidParams
	}
	if err := validateManualFindingIdentity(req.FindingKey, req.AssetRef); err != nil {
		return nil, err
	}
	if _, err := parseExpectedRevision(req.ExpectedRevision, true); err != nil {
		return nil, err
	}
	hash := canonicalHash(createFingerprint{
		Command: "create", Title: req.Title, Description: req.Description,
		OrgID: req.OrgID, AffectedOrgID: req.AffectedOrgID, OwnerUserID: req.OwnerUserID,
		SystemRef: req.SystemRef, FindingKey: req.FindingKey, AssetRef: req.AssetRef,
		EvidenceRefs: req.EvidenceRefs, Attributes: normalizeJSON(req.Attributes),
	})
	var result *model.RiskMutationResult
	humanKey, allocationErr := s.humanKeys.Next(ctx)
	if allocationErr != nil {
		return nil, fmt.Errorf("reserve risk number: %w", allocationErr)
	}
	err := s.withHTTPCommand(ctx, riskCreatePath, actor, func(ctx context.Context, tx pgx.Tx) error {
		receipt, replayed, err := repository.ClaimOperationTx(ctx, tx, actor, req.OperationID, hash, "create")
		if err != nil {
			return err
		}
		if replayed {
			// 同号重放先复核当前对象（详设 §7）——失权者不得从重放读业务元数据。
			if err := replayAuthorized(ctx, tx, receipt, actor); err != nil {
				return err
			}
			if !bytes.Equal(receipt.RequestHash, hash) {
				return ErrRiskOperationConflict
			}
			result = replayResult(receipt)
			return nil
		}
		// L3：登记是管理动作——要求对目标管理组织的主锚管理范围。
		allowed, err := repository.TxBool(ctx, tx, `SELECT `+orgManageOnOrgSQL()+`
			FROM organizations go WHERE go.id=$1`, req.OrgID, actor)
		if err != nil {
			return err
		}
		if !allowed {
			return errcode.ErrNoPermission
		}
		if req.AffectedOrgID != nil {
			if *req.AffectedOrgID <= 0 {
				return errcode.ErrInvalidParams
			}
			ok, err := repository.TxBool(ctx, tx,
				`SELECT EXISTS(SELECT 1 FROM organizations WHERE id=$1)`, *req.AffectedOrgID)
			if err != nil {
				return err
			}
			if !ok {
				return errcode.ErrInvalidParams
			}
		}
		if req.OwnerUserID != nil {
			active, err := repository.UserActive(ctx, tx, *req.OwnerUserID)
			if err != nil {
				return err
			}
			if !active {
				return ErrRiskOwnerInvalid
			}
		}
		manualKey := uuid.NewString()
		familyID, err := repository.UpsertFamilyTx(ctx, tx, riskManualScope, manualKey, 1)
		if err != nil {
			return err
		}
		row := &repository.RiskItemRow{
			HumanKey: humanKey, FamilyID: familyID, Status: "new",
			OrgID: req.OrgID, AffectedOrgID: req.AffectedOrgID, OwnerUserID: req.OwnerUserID,
			Title: req.Title, Description: nilIfEmpty(req.Description), SystemRef: nilIfEmpty(req.SystemRef),
			FindingKey:   nilIfEmpty(req.FindingKey),
			EvidenceRefs: jsonArrayBytes(req.EvidenceRefs), Attributes: jsonBytes(normalizeJSON(req.Attributes)),
			CreatedBy: actor,
		}
		if req.AssetRef != nil {
			row.AssetProvider = &req.AssetRef.Provider
			row.AssetExternalID = &req.AssetRef.ExternalID
		}
		itemID, err := repository.InsertRiskItemTx(ctx, tx, row)
		if err != nil {
			return err
		}
		createdPayload := map[string]any{"title": req.Title, "origin": "manual"}
		if req.FindingKey != "" && req.AssetRef != nil {
			// 申报的漏洞/资产身份随创建留痕（仍为人工源，不落来源镜像/台账）。
			createdPayload["finding_key"] = req.FindingKey
			createdPayload["asset_ref"] = map[string]any{
				"provider": req.AssetRef.Provider, "external_id": req.AssetRef.ExternalID,
			}
		}
		if err := repository.InsertEventTx(ctx, tx, itemID, 1, "created", 0, actor,
			req.OperationID, jsonBytes(createdPayload)); err != nil {
			return err
		}
		if err := repository.CompleteOperationTx(ctx, tx, actor, req.OperationID,
			"new", &itemID, int64Ptr(1)); err != nil {
			return err
		}
		result = &model.RiskMutationResult{
			Risk:    model.RiskRef{ID: itemID, HumanKey: humanKey, Revision: 1},
			Status:  "new",
			Receipt: model.RiskReceipt{OperationID: req.OperationID, Command: "create", Status: "new", RiskID: itemID, Revision: 1},
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Assess 核实/评估（POST /risk-items/assess；详设 §2.2）。
// 允许 new/assessed；L3 = 主锚管理范围 ∨ owner 本人；deadline 首次写入即 init_deadline。
func (s *Service) Assess(ctx context.Context, req model.RiskAssessRequest, actor int64) (*model.RiskMutationResult, error) {
	if req.ID <= 0 || !validLevels[req.Level] || !validTreatments[req.Treatment] {
		return nil, errcode.ErrInvalidParams
	}
	expected, err := parseExpectedRevision(req.ExpectedRevision, false)
	if err != nil {
		return nil, err
	}
	deadline := req.Deadline
	if deadline != nil && deadline.IsZero() {
		deadline = nil
	}
	hash := canonicalHash(map[string]any{
		"command": "assess", "id": req.ID, "expected_revision": expected,
		"level": req.Level, "treatment": req.Treatment,
		"note": req.AssessmentNote, "deadline": deadline,
	})
	var result *model.RiskMutationResult
	err = s.withHTTPCommand(ctx, riskAssessPath, actor, func(ctx context.Context, tx pgx.Tx) error {
		receipt, replayed, err := repository.ClaimOperationTx(ctx, tx, actor, req.OperationID, hash, "assess")
		if err != nil {
			return err
		}
		if replayed {
			// 同号重放先复核当前对象（详设 §7）——失权者不得从重放读业务元数据。
			if err := replayAuthorized(ctx, tx, receipt, actor); err != nil {
				return err
			}
			if !bytes.Equal(receipt.RequestHash, hash) {
				return ErrRiskOperationConflict
			}
			result = replayResult(receipt)
			return nil
		}
		row, err := repository.RiskItemAuthorizedTx(ctx, tx, req.ID, actor, false)
		if err != nil {
			return err
		}
		if row.Status != "new" && row.Status != "assessed" {
			return ErrRiskInvalidState
		}
		updated, err := repository.AssessRiskItemTx(ctx, tx, req.ID, expected, req.Level, req.Treatment, deadline)
		if err != nil {
			return err
		}
		if err := repository.InsertEventTx(ctx, tx, updated.ID, updated.Revision, "assessed", 0, actor,
			req.OperationID, jsonBytes(map[string]any{
				"level": req.Level, "treatment": req.Treatment,
				"deadline": deadline, "note": req.AssessmentNote,
			})); err != nil {
			return err
		}
		if err := repository.CompleteOperationTx(ctx, tx, actor, req.OperationID,
			updated.Status, &updated.ID, &updated.Revision); err != nil {
			return err
		}
		result = &model.RiskMutationResult{
			Risk:         riskRefFromRow(updated),
			Status:       updated.Status,
			Level:        updated.Level,
			Deadline:     updated.Deadline,
			InitDeadline: updated.InitDeadline,
			Receipt: model.RiskReceipt{
				OperationID: req.OperationID, Command: "assess", Status: updated.Status,
				RiskID: updated.ID, Revision: updated.Revision,
			},
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Assign 责任确认（POST /risk-items/assign；详设 §2.2——主锚管理者专属；
// owner 不能仅凭拥有本风险就把责任移给任意人）。
func (s *Service) Assign(ctx context.Context, req model.RiskAssignRequest, actor int64) (*model.RiskMutationResult, error) {
	if req.ID <= 0 || req.OwnerUserID <= 0 || req.Reason == "" {
		return nil, errcode.ErrInvalidParams
	}
	expected, err := parseExpectedRevision(req.ExpectedRevision, false)
	if err != nil {
		return nil, err
	}
	hash := canonicalHash(map[string]any{
		"command": "assign", "id": req.ID, "expected_revision": expected,
		"owner_user_id": req.OwnerUserID, "reason": req.Reason,
	})
	var result *model.RiskMutationResult
	err = s.withHTTPCommand(ctx, riskAssignPath, actor, func(ctx context.Context, tx pgx.Tx) error {
		receipt, replayed, err := repository.ClaimOperationTx(ctx, tx, actor, req.OperationID, hash, "assign")
		if err != nil {
			return err
		}
		if replayed {
			// 同号重放先复核当前对象（详设 §7）——失权者不得从重放读业务元数据。
			if err := replayAuthorized(ctx, tx, receipt, actor); err != nil {
				return err
			}
			if !bytes.Equal(receipt.RequestHash, hash) {
				return ErrRiskOperationConflict
			}
			result = replayResult(receipt)
			return nil
		}
		row, err := repository.RiskItemAuthorizedTx(ctx, tx, req.ID, actor, true)
		if err != nil {
			return err
		}
		if row.Status == "closed" {
			return ErrRiskInvalidState
		}
		// A gate-on submission freezes both owner and risk revision for review.
		// Even reconfirming the same owner advances that revision. Rejection may
		// permit rework, but does not grant a new owner the published review edge.
		// The risk row already serializes all proposal writers; do not acquire
		// group/ticket/link locks here in the reverse workflow lock order.
		reviewStarted, err := repository.TxBool(ctx, tx, `SELECT EXISTS(
			SELECT 1 FROM risk_ticket_links l
			WHERE l.risk_id=$1 AND l.ended_at IS NULL
			AND (l.remediation_state='pending_verify' OR l.proposal_version>0))`, req.ID)
		if err != nil {
			return err
		}
		if reviewStarted {
			return ErrRiskInvalidState
		}
		active, err := repository.UserActive(ctx, tx, req.OwnerUserID)
		if err != nil {
			return err
		}
		if !active {
			return ErrRiskOwnerInvalid
		}
		updated, err := repository.AssignRiskItemTx(ctx, tx, req.ID, expected, req.OwnerUserID)
		if err != nil {
			return err
		}
		if err := repository.InsertEventTx(ctx, tx, updated.ID, updated.Revision, "owner_assigned", 0, actor,
			req.OperationID, jsonBytes(map[string]any{"owner_user_id": req.OwnerUserID, "reason": req.Reason})); err != nil {
			return err
		}
		if err := repository.CompleteOperationTx(ctx, tx, actor, req.OperationID,
			updated.Status, &updated.ID, &updated.Revision); err != nil {
			return err
		}
		result = &model.RiskMutationResult{
			Risk:   riskRefFromRow(updated),
			Status: updated.Status,
			Receipt: model.RiskReceipt{
				OperationID: req.OperationID, Command: "assign", Status: updated.Status,
				RiskID: updated.ID, Revision: updated.Revision,
			},
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func int64Ptr(v int64) *int64 { return &v }
