package repository

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// NotificationRepo 通知渠道配置数据访问（P4-2 本体）。
// 错误形态对齐仓内范式：sentinel error 由 service 映射 errcode。
var (
	ErrNotificationNotFound = errors.New("repository: notification config not found")
	ErrNotificationDupCode  = errors.New("repository: notification code duplicated")
)

const notificationColumns = `
	id, code, name, channel, webhook_url, enabled, created_by, version, created_at, updated_at`

type NotificationRepo struct {
	db *pgxpool.Pool
}

func NewNotificationRepo(db *pgxpool.Pool) *NotificationRepo {
	return &NotificationRepo{db: db}
}

func scanNotification(row pgx.Row) (*model.NotificationConfig, error) {
	var c model.NotificationConfig
	err := row.Scan(&c.ID, &c.Code, &c.Name, &c.Channel, &c.WebhookURL, &c.Enabled,
		&c.CreatedBy, &c.Version, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// List 分页（code 模糊过滤可选）
func (r *NotificationRepo) List(ctx context.Context, page, pageSize int, codeLike string) ([]*model.NotificationConfig, int64, error) {
	where, arg := "", []any{}
	if codeLike != "" {
		where = " WHERE code ILIKE '%' || $1 || '%'"
		arg = append(arg, codeLike)
	}
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM notification_configs"+where, arg...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := "SELECT " + notificationColumns + " FROM notification_configs" + where +
		" ORDER BY id DESC LIMIT $" + strconv.Itoa(len(arg)+1) +
		" OFFSET $" + strconv.Itoa(len(arg)+2)
	arg = append(arg, pageSize, (page-1)*pageSize)
	rows, err := r.db.Query(ctx, q, arg...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*model.NotificationConfig{}
	for rows.Next() {
		c, err := scanNotification(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// FindByID 单查
func (r *NotificationRepo) FindByID(ctx context.Context, id int64) (*model.NotificationConfig, error) {
	c, err := scanNotification(r.db.QueryRow(ctx,
		"SELECT "+notificationColumns+" FROM notification_configs WHERE id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotificationNotFound
	}
	return c, err
}

// Create 新建（code 唯一冲突 → ErrNotificationDupCode）
func (r *NotificationRepo) Create(ctx context.Context, c *model.NotificationConfig) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO notification_configs (code, name, channel, webhook_url, enabled, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, version, created_at, updated_at`,
		c.Code, c.Name, c.Channel, c.WebhookURL, c.Enabled, c.CreatedBy,
	).Scan(&c.ID, &c.Version, &c.CreatedAt, &c.UpdatedAt)
}

// Update 乐观锁（version 不符 → ErrNotificationNotFound——语义同「已被他人变更」）
func (r *NotificationRepo) Update(ctx context.Context, c *model.NotificationConfig) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE notification_configs SET
			name = $2, channel = $3, webhook_url = $4, enabled = $5,
			version = version + 1, updated_at = NOW()
		WHERE id = $1 AND version = $6`,
		c.ID, c.Name, c.Channel, c.WebhookURL, c.Enabled, c.Version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

// DeleteByCode 按业务码删（管理 API 以 code 为标识——zhuzhao 风格）
func (r *NotificationRepo) DeleteByCode(ctx context.Context, code string) error {
	tag, err := r.db.Exec(ctx, "DELETE FROM notification_configs WHERE code = $1", code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

// ListEnabled 查全量启用配置（分发用——enabled 部分索引命中）
func (r *NotificationRepo) ListEnabled(ctx context.Context) ([]*model.NotificationConfig, error) {
	rows, err := r.db.Query(ctx,
		"SELECT "+notificationColumns+" FROM notification_configs WHERE enabled ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.NotificationConfig{}
	for rows.Next() {
		c, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
