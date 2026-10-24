// S3a B3-2 自动分派规则仓库（03 号附录 A.2 DDL——CRUD + 命中查询）。
package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

type AssignmentRuleRepo struct {
	db *pgxpool.Pool
}

func NewAssignmentRuleRepo(db *pgxpool.Pool) *AssignmentRuleRepo {
	return &AssignmentRuleRepo{db: db}
}

// List 全量规则（管理面——按 type_code+priority 排序）。
func (r *AssignmentRuleRepo) List(ctx context.Context) ([]*model.AssignmentRule, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ar.id, ar.type_code, ar.priority, COALESCE(ar.keyword, ''), ar.target_org,
		       ar.target_user, ar.enabled, ar.created_by, ar.created_at
		FROM assignment_rules ar
		ORDER BY ar.type_code, ar.priority, ar.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.AssignmentRule, error) {
		var a model.AssignmentRule
		err := row.Scan(&a.ID, &a.TypeCode, &a.Priority, &a.Keyword, &a.TargetOrg,
			&a.TargetUser, &a.Enabled, &a.CreatedBy, &a.CreatedAt)
		return &a, err
	})
}

// Create 建规则（target_user 属 target_org 校验在 service 层——repo 只管库）。
func (r *AssignmentRuleRepo) Create(ctx context.Context, a *model.AssignmentRule) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO assignment_rules (type_code, priority, keyword, target_org, target_user, enabled, created_by)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7)
		RETURNING id, created_at`,
		a.TypeCode, a.Priority, a.Keyword, a.TargetOrg, a.TargetUser, a.Enabled, a.CreatedBy,
	).Scan(&a.ID, &a.CreatedAt)
}

// Delete 删规则（幂等）。
func (r *AssignmentRuleRepo) Delete(ctx context.Context, id int64) error {
	ct, err := r.db.Exec(ctx, `DELETE FROM assignment_rules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return errcode.New(errcode.ErrNotFound.Code, "规则不存在")
	}
	return nil
}

// UpdateEnabled 启停（管理面最小 patch——优先级/关键词等改走删+建）。
func (r *AssignmentRuleRepo) UpdateEnabled(ctx context.Context, id int64, enabled bool) error {
	ct, err := r.db.Exec(ctx, `UPDATE assignment_rules SET enabled = $2 WHERE id = $1`, id, enabled)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return errcode.New(errcode.ErrNotFound.Code, "规则不存在")
	}
	return nil
}

// MatchOnCreate OnAfterCreate 命中查询（03 号决议：按 priority 升序→首个命中）。
// keyword=NULL/空=不限（类型兜底）；非空=标题 LIKE 包含匹配（决议①不分词）。
// 返回 nil=无命中（调用方走 dispatch_missed 分支）。
func (r *AssignmentRuleRepo) MatchOnCreate(ctx context.Context, typeCode, title string) (*model.AssignmentRule, error) {
	var a model.AssignmentRule
	err := r.db.QueryRow(ctx, `
		SELECT id, type_code, priority, COALESCE(keyword, ''), target_org, target_user, enabled, created_by, created_at
		FROM assignment_rules
		WHERE type_code = $1 AND enabled
		  AND (keyword IS NULL OR keyword = '' OR $2 LIKE '%' || keyword || '%')
		ORDER BY priority, id
		LIMIT 1`, typeCode, title).Scan(
		&a.ID, &a.TypeCode, &a.Priority, &a.Keyword, &a.TargetOrg,
		&a.TargetUser, &a.Enabled, &a.CreatedBy, &a.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// UserBelongsToOrg 决议③ target_user 属 target_org 校验。
func (r *AssignmentRuleRepo) UserBelongsToOrg(ctx context.Context, userID, orgID int64) (bool, error) {
	var ok bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_orgs WHERE user_id = $1 AND org_id = $2)`,
		userID, orgID).Scan(&ok)
	return ok, err
}
