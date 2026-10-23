// MessageTemplateRepo 消息模板仓库（S6 B6-3）。
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

var ()

type MessageTemplateRepo struct {
	db *pgxpool.Pool
}

func NewMessageTemplateRepo(db *pgxpool.Pool) *MessageTemplateRepo {
	return &MessageTemplateRepo{db: db}
}

func scanTemplate(row pgx.Row) (*model.MessageTemplate, error) {
	var t model.MessageTemplate
	err := row.Scan(&t.ID, &t.Code, &t.Name, &t.Channel, &t.BizType,
		&t.Subject, &t.Body, &t.VariableSchema, &t.Enabled, &t.IsSystem, &t.Version,
		&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetByCode 按 code 取启用模板（B6-5 触发挂接消费面）。
func (r *MessageTemplateRepo) GetByCode(ctx context.Context, code string) (*model.MessageTemplate, error) {
	t, err := scanTemplate(r.db.QueryRow(ctx,
		`SELECT id, code, name, channel, biz_type, subject, body, variable_schema,
			enabled, is_system, version, created_by, created_at, updated_at
		 FROM message_templates WHERE code = $1 AND enabled = true`, code))
	if err == pgx.ErrNoRows {
		return nil, errcode.ErrMsgTplNotFound
	}
	return t, err
}

// List 分页列表（管理面）。
func (r *MessageTemplateRepo) List(ctx context.Context, page, pageSize int, codeLike string) ([]*model.MessageTemplate, int64, error) {
	where := "WHERE 1=1"
	args := []any{}
	if codeLike != "" {
		args = append(args, "%"+codeLike+"%")
		where += fmt.Sprintf(" AND code ILIKE $%d", len(args))
	}
	var total int64
	if err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM message_templates `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.Query(ctx,
		`SELECT id, code, name, channel, biz_type, subject, body, variable_schema,
			enabled, is_system, version, created_by, created_at, updated_at
		 FROM message_templates `+where+fmt.Sprintf(` ORDER BY id LIMIT $%d OFFSET $%d`, len(args)-1, len(args)),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*model.MessageTemplate
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, nil
}

// Create 创建模板。
func (r *MessageTemplateRepo) Create(ctx context.Context, t *model.MessageTemplate) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO message_templates (code, name, channel, biz_type, subject, body, variable_schema, enabled, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		t.Code, t.Name, t.Channel, t.BizType, t.Subject, t.Body, t.VariableSchema, t.Enabled, t.CreatedBy)
	if err != nil {
		if isPgUnique(err) {
			return errcode.ErrMsgTplDupCode
		}
	}
	return err
}

// Update 更新模板（version CAS）。
func (r *MessageTemplateRepo) Update(ctx context.Context, t *model.MessageTemplate) error {
	// A-4 审计修复：is_system 模板不可 UPDATE（只读语义补全——Delete 已拦）
	var isSys bool
	if err := r.db.QueryRow(ctx,
		`SELECT is_system FROM message_templates WHERE code = $1`, t.Code).Scan(&isSys); err != nil {
		if err == pgx.ErrNoRows {
			return errcode.ErrMsgTplNotFound
		}
		return err
	}
	if isSys {
		return errcode.ErrMsgTplSystem
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE message_templates SET name=$2, subject=$3, body=$4, variable_schema=$5,
			enabled=$6, version=version+1, updated_at=NOW()
		WHERE code=$1 AND version=$7`,
		t.Code, t.Name, t.Subject, t.Body, t.VariableSchema, t.Enabled, t.Version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// CAS 区分：不存在→404 / version 不匹配→409（B6-3 增强批）
		var exists bool
		_ = r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM message_templates WHERE code=$1)`, t.Code).Scan(&exists)
		if !exists {
			return errcode.ErrMsgTplNotFound
		}
		return errcode.ErrMsgTplVersionConflict
	}
	return nil
}

// Delete 删除模板（守卫：is_system 不可删 + 被 message_rules 引用不可删）。
func (r *MessageTemplateRepo) Delete(ctx context.Context, code string) error {
	// 守卫① 种子模板只读
	var isSys bool
	if err := r.db.QueryRow(ctx,
		`SELECT is_system FROM message_templates WHERE code = $1`, code).Scan(&isSys); err != nil {
		if err == pgx.ErrNoRows {
			return errcode.ErrMsgTplNotFound
		}
		return err
	}
	if isSys {
		return errcode.ErrMsgTplSystem
	}
	// 守卫②+F-1-8 修复：引用即拦[含禁用规则] + 单语句原子化
	tag, err := r.db.Exec(ctx,
		"DELETE FROM message_templates WHERE code = $1 AND NOT EXISTS (SELECT 1 FROM message_rules WHERE template_code = $1)",
		code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		_ = r.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM message_templates WHERE code=$1)", code).Scan(&exists)
		if !exists {
			return errcode.ErrMsgTplNotFound
		}
		return errcode.ErrMsgTplReferenced
	}
	return nil
}
