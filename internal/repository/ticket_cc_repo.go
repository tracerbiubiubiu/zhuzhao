// TicketCCRepo 工单抄送仓库（S10 B10-1——10 号附录 A.3 ① 表族 CRUD）。
package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

type TicketCCRepo struct {
	db *pgxpool.Pool
}

func NewTicketCCRepo(db *pgxpool.Pool) *TicketCCRepo {
	return &TicketCCRepo{db: db}
}

// Add 批量挂抄送（幂等：UNIQUE(ticket_id,user_id,role) ON CONFLICT DO NOTHING）。
// 返回**新增**的 user_id 列表（事件只对新挂的人生效——重复挂载不重发通知）；
// user_id 不存在 → FK 23503 → 400（MapForeignKeyViolation）。
func (r *TicketCCRepo) Add(ctx context.Context, ticketID, createdBy int64, userIDs []int64) ([]int64, error) {
	rows, err := r.db.Query(ctx, `
		INSERT INTO ticket_cc (ticket_id, user_id, role, created_by)
		SELECT $1, u, 'cc', $2 FROM unnest($3::bigint[]) AS u
		ON CONFLICT (ticket_id, user_id, role) DO NOTHING
		RETURNING user_id`, ticketID, createdBy, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var added []int64
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		added = append(added, uid)
	}
	// FK 违规在 RETURNING 流上延迟到 rows.Err() 浮出（Query 本体不报）——
	// 幻影 user_id 在此映射为 400
	if err := rows.Err(); err != nil {
		if ec := MapForeignKeyViolation(err); ec != nil {
			return nil, errcode.New(ec.Code, "抄送用户不存在")
		}
		return nil, err
	}
	return added, nil
}

// Remove 摘除抄送（幂等：不存在=0 行不算错）。返回是否实际删除。
func (r *TicketCCRepo) Remove(ctx context.Context, ticketID, userID int64) (bool, error) {
	ct, err := r.db.Exec(ctx,
		`DELETE FROM ticket_cc WHERE ticket_id = $1 AND user_id = $2 AND role = 'cc'`, ticketID, userID)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// ListByTicket 单的抄送行（repo 层 join users 富化——软删用户保留 ID 行不回填姓名）。
func (r *TicketCCRepo) ListByTicket(ctx context.Context, ticketID int64) ([]model.TicketCCRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT cc.user_id,
		       CASE WHEN u.deleted_at IS NULL THEN COALESCE(NULLIF(u.real_name, ''), u.username) ELSE '' END,
		       CASE WHEN u.deleted_at IS NULL THEN COALESCE(u.employee_no, '') ELSE '' END,
		       cc.role, cc.created_at
		FROM ticket_cc cc
		JOIN users u ON u.id = cc.user_id
		WHERE cc.ticket_id = $1
		ORDER BY cc.id`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.TicketCCRow, error) {
		var cc model.TicketCCRow
		err := row.Scan(&cc.UserID, &cc.Name, &cc.EmployeeNo, &cc.Role, &cc.CreatedAt)
		return cc, err
	})
}

// IsCC 单用户是否该单抄送人（L2 特例通道 Authorize 读支——10 号附录 A.2）。
func (r *TicketCCRepo) IsCC(ctx context.Context, ticketID, userID int64) (bool, error) {
	var ok bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM ticket_cc WHERE ticket_id = $1 AND user_id = $2 AND role = 'cc')`,
		ticketID, userID).Scan(&ok)
	return ok, err
}
