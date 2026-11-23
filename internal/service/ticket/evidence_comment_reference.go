package ticket

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	commentref "github.com/tracerbiubiubiu/zhuzhao/internal/model/comment"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// ticketCommentEvidenceMaxRefs 单次可核引用数量上限（与
// storage.BoundTicketEvidenceValidator、riskremediation.validEvidence 的 50 对齐）。
const ticketCommentEvidenceMaxRefs = 50

// ValidateTicketCommentEvidenceTx 校验 ticket-comment 可核证据引用（工单域 owner
// 的独立校验方法，Phase6 B2 验收卡二 R1 唯一实现；与
// storage.BoundTicketEvidenceValidator 同一核验边界，写端点按 kind 分流）。
//
// 与 file 校验器同款事务纪律：消费调用方自有的 IAM 事务（裸 tx、父事务上下文
// 携带的 savepoint、另一 pool 的守卫一律拒绝）；调用方必须先完成当前工单的
// 读取/行动授权并按组/风险/链接锁序持有工单行锁；此处按 ID 升序锁定评论行后
// 读取归属与指纹，事务须保持开启到证据写完成，任何错误交回调用方回滚。
// 成功不隐含任何授权能力。
//
// 每条引用要求：Ref 恰为 ticket-comment:<正十进制ID>；Version 恰为
// comment.EvidenceVersionToken 的规范形（公开评论内容 SHA256 + created_at），
// 且与库内该评论重算结果逐字节一致。评论不存在、属于其他工单、为内部备注
// 或指纹不符统一按资源不存在拒绝——不向无内部备注可见性的一方泄露备注行
// 存在性；引用与版本解析失败（非规范形）按参数错误拒绝，同引用携带两个
// 不同版本同样拒绝。校验只读，不写绑定或业务效果。
func (s *Service) ValidateTicketCommentEvidenceTx(ctx context.Context, tx pgx.Tx, ticketID int64, refs []commentref.EvidenceRef) error {
	if s == nil || s.db == nil || tx == nil || !repository.IAMGuardActive(ctx) || repository.IAMDB(ctx, s.db) != tx {
		return errcode.ErrNoPermission
	}
	if ticketID <= 0 || len(refs) > ticketCommentEvidenceMaxRefs {
		return errcode.ErrInvalidParams
	}
	versions := make(map[int64]string, len(refs))
	ids := make([]int64, 0, len(refs))
	for _, ref := range refs {
		id, err := commentref.ParseEvidenceRef(ref.Ref)
		if err != nil {
			return errcode.ErrInvalidParams
		}
		if _, _, err := commentref.ParseEvidenceVersion(ref.Version); err != nil {
			return errcode.ErrInvalidParams
		}
		if old, exists := versions[id]; exists {
			if old != ref.Version {
				return errcode.ErrInvalidParams
			}
			continue
		}
		versions[id] = ref.Version
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT id, content, created_at FROM ticket_comments
		WHERE id=ANY($1) AND ticket_id=$2 AND NOT is_internal ORDER BY id FOR UPDATE`, ids, ticketID)
	if err != nil {
		return err
	}
	type commentRow struct {
		content   string
		createdAt time.Time
	}
	found := make(map[int64]commentRow, len(ids))
	for rows.Next() {
		var id int64
		var row commentRow
		if err := rows.Scan(&id, &row.content, &row.createdAt); err != nil {
			rows.Close()
			return err
		}
		found[id] = row
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	if len(found) != len(ids) {
		// 缺行 / 跨票 / 内部备注不可区分，统一不存在。
		return errcode.ErrNotFound
	}
	for id, submitted := range versions {
		row := found[id]
		if commentref.EvidenceVersionToken(row.createdAt, row.content) != submitted {
			return errcode.ErrNotFound
		}
	}
	return nil
}
