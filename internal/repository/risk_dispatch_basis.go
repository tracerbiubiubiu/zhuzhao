package repository

import (
	"context"
	"fmt"
)

// B2 第 6–7 项 P1（2026-11-23）：派单依据的现势计算。
// DDL 沿用 000089/000090 既有列（risk_items.finding_key/asset_*、
// risk_families.fingerprint_scope、risk_source_records.mirror/content_hash、
// risk_events.assessed），无新迁移。

// dispatchBasisSQL 逐风险计算当前有效派单依据的稳定核实事实版本：
//   - 前置（两源通用）：已评估、选择整改策略且携带稳定漏洞-资产身份；
//     缺任一 → ”（不可派）。
//   - 人工源（family scope=manual）：man1:<risk_id>:<最近 assessed 事件
//     aggregate_revision>。评估事件即核实留证（assessment_note 随事件 payload），
//     owner 责任确认不推进该版本；重新核实才换版本。
//   - 合成来源：src1:<hex(现势镜像 content_hash)>。镜像缺身份时，允许最近
//     有效的主锚补核提供缺项，并将补核修订钉入 src2 依据。
//     镜像 kind 非 observed（withdrawn/deleted）→ ”（撤回失效），
//     停用来源、映射变更、身份不匹配、指定期限已过或无镜像 → 空（无法核实）。
//
// 只做纯快照读（无 FOR UPDATE）：report 应用路径按 advisory(source item) →
// risk_source_records → risk_families → risk_items 顺序加锁，派单锁路径已持有
// risk 行锁，若在此再锁来源镜像即构成 report/source→risk 锁序反转；
// 并发撤回与派单由派单已持有的 risk 行锁串行（详见 dispatch_port.go）。
const dispatchBasisSQL = `
SELECT ri.id,
	CASE
		WHEN ri.level IS NULL
			OR ri.treatment IS DISTINCT FROM 'mitigate'
			OR NULLIF(btrim(ri.finding_key), '') IS NULL
			OR NULLIF(btrim(ri.asset_provider), '') IS NULL
			OR NULLIF(btrim(ri.asset_external_id), '') IS NULL
		THEN ''
		WHEN fam.fingerprint_scope = 'manual' THEN
			CASE WHEN (assessment.payload ->> 'note') ~ '[^[:space:]]'
				AND ((assessment.payload ->> 'dispatch_basis_valid_until') IS NULL
					OR (assessment.payload ->> 'dispatch_basis_valid_until')::timestamptz > statement_timestamp())
				THEN 'man1:' || ri.id::text || ':' || assessment.aggregate_revision::text ELSE '' END
		WHEN source.enabled IS DISTINCT FROM TRUE
			OR NOT (ri.org_id = ANY(source.allowed_org_ids))
			OR source.fingerprint_scope <> fam.fingerprint_scope
			OR source.key_schema_version <> fam.key_schema_version
			OR source.mapping_ref <> current_source.mapping_ref
			OR current_source.mirror ->> 'kind' IS DISTINCT FROM 'observed'
			OR (current_source.mirror ->> 'valid_until')::timestamptz <= statement_timestamp()
			OR (NULLIF(btrim(current_source.mirror ->> 'finding_key'), '') IS NOT NULL
				AND current_source.mirror ->> 'finding_key' <> ri.finding_key)
			OR (NULLIF(btrim(current_source.mirror #>> '{asset_ref,provider}'), '') IS NOT NULL
				AND current_source.mirror #>> '{asset_ref,provider}' <> ri.asset_provider)
			OR (NULLIF(btrim(current_source.mirror #>> '{asset_ref,external_id}'), '') IS NOT NULL
				AND current_source.mirror #>> '{asset_ref,external_id}' <> ri.asset_external_id)
		THEN ''
		WHEN current_source.mirror ->> 'finding_key' = ri.finding_key
			AND current_source.mirror #>> '{asset_ref,provider}' = ri.asset_provider
			AND current_source.mirror #>> '{asset_ref,external_id}' = ri.asset_external_id
		THEN 'src1:' || encode(current_source.content_hash, 'hex')
		WHEN assessment.payload #>> '{identity_supplement,finding_key}' = ri.finding_key
			AND assessment.payload #>> '{identity_supplement,asset_ref,provider}' = ri.asset_provider
			AND assessment.payload #>> '{identity_supplement,asset_ref,external_id}' = ri.asset_external_id
			AND (assessment.payload ->> 'dispatch_basis_valid_until')::timestamptz > statement_timestamp()
		THEN 'src2:' || encode(current_source.content_hash, 'hex') || ':' || assessment.aggregate_revision::text
		ELSE ''
	END
FROM risk_items ri
JOIN risk_families fam ON fam.id = ri.family_id
` + riskCurrentSourceJoin + `
LEFT JOIN risk_sources source ON source.source_ref = current_source.source_ref
LEFT JOIN LATERAL (
	SELECT e.payload, e.aggregate_revision
	FROM risk_events e
	WHERE e.risk_id = ri.id AND e.event_kind = 'assessed'
	ORDER BY e.aggregate_revision DESC
	LIMIT 1
) assessment ON TRUE
WHERE ri.id = ANY($1)`

// DispatchBases 批量读取风险当前派单依据（纯快照读，不加来源侧锁）。
// 返回 map 缺项 = ”（防御：行消失按不可派处理）。
func DispatchBases(ctx context.Context, q IAMDBTX, ids []int64) (map[int64]string, error) {
	bases := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return bases, nil
	}
	rows, err := q.Query(ctx, dispatchBasisSQL, ids)
	if err != nil {
		return nil, fmt.Errorf("read dispatch bases: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var basis string
		if err := rows.Scan(&id, &basis); err != nil {
			return nil, fmt.Errorf("scan dispatch basis: %w", err)
		}
		bases[id] = basis
	}
	return bases, rows.Err()
}
