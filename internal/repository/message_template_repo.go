// MessageTemplateRepo 消息模板仓库（S6 B6-3）。
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao-utils/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

var (
	ErrTemplateNotFound = errcode.New(92001, "消息模板不存在")
	ErrTemplateDupCode  = errcode.New(92002, "模板 code 已存在")
)

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
		return nil, ErrTemplateNotFound
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
			return ErrTemplateDupCode
		}
	}
	return err
}

// Update 更新模板（version CAS）。
func (r *MessageTemplateRepo) Update(ctx context.Context, t *model.MessageTemplate) error {
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
			return ErrTemplateNotFound
		}
		return ErrTemplateVersionConflict
	}
	return nil
}

// ErrTemplateSystem 系统模板不可删（B6-3 增强批——A.8.1 种子只读语义）。
var ErrTemplateSystem = errcode.New(92003, "系统模板不可修改或删除")

// ErrTemplateReferenced 模板被 message_rules 引用中（B6-3 增强批删除守卫）。
var ErrTemplateVersionConflict = errcode.New(92005, "模板版本冲突——请刷新后重试")

var ErrTemplateReferenced = errcode.New(92004, "模板被订阅规则引用中，不可删除")

// Delete 删除模板（守卫：is_system 不可删 + 被 message_rules 引用不可删）。
func (r *MessageTemplateRepo) Delete(ctx context.Context, code string) error {
	// 守卫① 种子模板只读
	var isSys bool
	if err := r.db.QueryRow(ctx,
		`SELECT is_system FROM message_templates WHERE code = $1`, code).Scan(&isSys); err != nil {
		if err == pgx.ErrNoRows {
			return ErrTemplateNotFound
		}
		return err
	}
	if isSys {
		return ErrTemplateSystem
	}
	// 守卫② 被 message_rules 引用
	var refCount int
	if err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM message_rules WHERE template_code = $1 AND enabled = true`, code).Scan(&refCount); err != nil {
		return err
	}
	if refCount > 0 {
		return ErrTemplateReferenced
	}
	tag, err := r.db.Exec(ctx, `DELETE FROM message_templates WHERE code = $1`, code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrTemplateNotFound
	}
	return nil
}
