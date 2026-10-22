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
		&t.Subject, &t.Body, &t.VariableSchema, &t.Enabled, &t.Version,
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
			enabled, version, created_by, created_at, updated_at
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
			enabled, version, created_by, created_at, updated_at
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
