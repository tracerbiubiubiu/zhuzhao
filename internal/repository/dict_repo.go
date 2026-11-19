package repository

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// DictRepo P4-3 字典数据访问。sentinel error 由 service 映射 errcode。
var (
	ErrDictTypeNotFound   = errors.New("repository: dict type not found")
	ErrDictTypeDupCode    = errors.New("repository: dict type code duplicated")
	ErrDictItemNotFound   = errors.New("repository: dict item not found")
	ErrDictItemDupCode    = errors.New("repository: dict item code duplicated")
	ErrDictDeleteConflict = errors.New("repository: dict delete target changed")
	ErrDictDeleteInvalid  = errors.New("repository: invalid dict delete condition")
)

// DictTypeDeleteTarget 绑定调用方确认的稳定身份及版本；不包含子项列表世代。
type DictTypeDeleteTarget struct {
	ID      int64
	Version int64
}

type dictDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

const dictTypeColumns = `id, code, name, enabled, remark, version, created_at, updated_at`
const dictItemColumns = `id, type_code, code, label, sort_order, enabled, remark, version, created_at, updated_at`

type DictRepo struct {
	db dictDB
}

func NewDictRepo(db dictDB) *DictRepo {
	return &DictRepo{db: db}
}

// ─── 类型 ───

func (r *DictRepo) ListTypes(ctx context.Context, page, pageSize int, codeLike string) ([]*model.DictType, int64, error) {
	where, args := "", []any{}
	if codeLike != "" {
		where = " WHERE code ILIKE '%' || $1 || '%' OR name ILIKE '%' || $1 || '%'"
		args = append(args, codeLike)
	}
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM dict_types"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := "SELECT " + dictTypeColumns + " FROM dict_types" + where +
		" ORDER BY id DESC LIMIT $" + strconv.Itoa(len(args)+1) + " OFFSET $" + strconv.Itoa(len(args)+2)
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*model.DictType{}
	for rows.Next() {
		var t model.DictType
		if err := rows.Scan(&t.ID, &t.Code, &t.Name, &t.Enabled, &t.Remark, &t.Version, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, &t)
	}
	return out, total, rows.Err()
}

func (r *DictRepo) CreateType(ctx context.Context, t *model.DictType) error {
	err := r.db.QueryRow(ctx, `
		INSERT INTO dict_types (code, name, enabled, remark) VALUES ($1, $2, $3, $4)
		RETURNING id, version, created_at, updated_at`,
		t.Code, t.Name, t.Enabled, t.Remark,
	).Scan(&t.ID, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	if err != nil && isPgUnique(err) {
		return ErrDictTypeDupCode
	}
	return err
}

func (r *DictRepo) UpdateType(ctx context.Context, t *model.DictType) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE dict_types SET name=$2, enabled=$3, remark=$4, version=version+1, updated_at=NOW()
		WHERE id=$1 AND version=$5`,
		t.ID, t.Name, t.Enabled, t.Remark, t.Version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDictTypeNotFound
	}
	return nil
}

// HasFieldDictRef 字段 schema 是否引用 `dict:<code>`（B1-2 删除守卫——01 号 §3.2
// 拍板点④"补删除守卫"）。引用形态 = ticket_type_fields.field_options JSONB 数组
// 元素精确等于 "dict:<code>"（jsonb_array_elements_text 精确匹配，非 LIKE 模糊）。
func (r *DictRepo) HasFieldDictRef(ctx context.Context, code string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM ticket_type_fields f,
			jsonb_array_elements_text(f.field_options) e
			WHERE e = 'dict:' || $1
		)`, code).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// DeleteTypeByCode 保留旧 code-only 调用；提供 target 时最终 DELETE 必须匹配身份和版本。
// 锁定目标及条件删除在同一语句内区分缺失/冲突，不协调字段引用写方或子项列表世代。
func (r *DictRepo) DeleteTypeByCode(ctx context.Context, code string, target ...DictTypeDeleteTarget) error {
	if len(target) > 1 || len(target) == 1 && (target[0].ID <= 0 || target[0].Version <= 0) {
		return ErrDictDeleteInvalid
	}
	if len(target) == 1 {
		var exists, deleted bool
		err := r.db.QueryRow(ctx, `
			WITH target AS MATERIALIZED (
				SELECT id FROM dict_types WHERE code = $1 FOR UPDATE
			), deleted AS (
				DELETE FROM dict_types d USING target
				WHERE d.code = $1 AND d.id = $2 AND d.version = $3 AND d.id = target.id
				RETURNING d.id
			)
			SELECT EXISTS (SELECT 1 FROM target), EXISTS (SELECT 1 FROM deleted)`,
			code, target[0].ID, target[0].Version).Scan(&exists, &deleted)
		if err != nil {
			return err
		}
		if deleted {
			return nil
		}
		if exists {
			return ErrDictDeleteConflict
		}
		return ErrDictTypeNotFound
	}
	tag, err := r.db.Exec(ctx, "DELETE FROM dict_types WHERE code = $1", code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDictTypeNotFound
	}
	return nil
}

// ─── 项 ───

func (r *DictRepo) ListItems(ctx context.Context, page, pageSize int, typeCode string) ([]*model.DictItem, int64, error) {
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM dict_items WHERE type_code = $1", typeCode).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx,
		"SELECT "+dictItemColumns+" FROM dict_items WHERE type_code = $1 ORDER BY sort_order, code LIMIT $2 OFFSET $3",
		typeCode, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*model.DictItem{}
	for rows.Next() {
		var it model.DictItem
		if err := rows.Scan(&it.ID, &it.TypeCode, &it.Code, &it.Label, &it.SortOrder, &it.Enabled, &it.Remark, &it.Version, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, &it)
	}
	return out, total, rows.Err()
}

// ListItemsByCodeIncludingDisabled 显示面（RR-HY-02）：含停用项（停用项在详情/
// 编辑回显名称用），type 停用仍返回空——type 级开关是"整个字典下线"语义。
func (r *DictRepo) ListItemsByCodeIncludingDisabled(ctx context.Context, code string) ([]*model.DictItem, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+dictItemColumns+` FROM dict_items
		WHERE type_code = $1 AND EXISTS (SELECT 1 FROM dict_types t WHERE t.code = $1 AND t.enabled)
		ORDER BY sort_order, code`, code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.DictItem{}
	for rows.Next() {
		var it model.DictItem
		if err := rows.Scan(&it.ID, &it.TypeCode, &it.Code, &it.Label, &it.SortOrder, &it.Enabled, &it.Remark, &it.Version, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &it)
	}
	return out, rows.Err()
}

// ListEnabledItemsByCode 消费面：按 type 拉取启用项（sort+code 序）——type 停用时返回空
func (r *DictRepo) ListEnabledItemsByCode(ctx context.Context, code string) ([]*model.DictItem, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+dictItemColumns+` FROM dict_items
		WHERE type_code = $1 AND enabled AND EXISTS (SELECT 1 FROM dict_types t WHERE t.code = $1 AND t.enabled)
		ORDER BY sort_order, code`, code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.DictItem{}
	for rows.Next() {
		var it model.DictItem
		if err := rows.Scan(&it.ID, &it.TypeCode, &it.Code, &it.Label, &it.SortOrder, &it.Enabled, &it.Remark, &it.Version, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &it)
	}
	return out, rows.Err()
}

func (r *DictRepo) CreateItem(ctx context.Context, it *model.DictItem) error {
	err := r.db.QueryRow(ctx, `
		INSERT INTO dict_items (type_code, code, label, sort_order, enabled, remark)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, version, created_at, updated_at`,
		it.TypeCode, it.Code, it.Label, it.SortOrder, it.Enabled, it.Remark,
	).Scan(&it.ID, &it.Version, &it.CreatedAt, &it.UpdatedAt)
	if err != nil {
		if isPgUnique(err) {
			return ErrDictItemDupCode
		}
		if isPgFK(err) {
			return ErrDictTypeNotFound
		}
	}
	return err
}

func (r *DictRepo) UpdateItem(ctx context.Context, it *model.DictItem) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE dict_items SET label=$2, sort_order=$3, enabled=$4, remark=$5, version=version+1, updated_at=NOW()
		WHERE id=$1 AND version=$6`,
		it.ID, it.Label, it.SortOrder, it.Enabled, it.Remark, it.Version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDictItemNotFound
	}
	return nil
}

func (r *DictRepo) DeleteItemByID(ctx context.Context, id int64, version ...int64) error {
	if len(version) > 1 || len(version) == 1 && version[0] <= 0 {
		return ErrDictDeleteInvalid
	}
	if len(version) == 1 {
		var exists, deleted bool
		err := r.db.QueryRow(ctx, `
			WITH target AS MATERIALIZED (
				SELECT id FROM dict_items WHERE id = $1 FOR UPDATE
			), deleted AS (
				DELETE FROM dict_items d USING target
				WHERE d.id = $1 AND d.version = $2 AND d.id = target.id
				RETURNING d.id
			)
			SELECT EXISTS (SELECT 1 FROM target), EXISTS (SELECT 1 FROM deleted)`, id, version[0]).Scan(&exists, &deleted)
		if err != nil {
			return err
		}
		if deleted {
			return nil
		}
		if exists {
			return ErrDictDeleteConflict
		}
		return ErrDictItemNotFound
	}
	tag, err := r.db.Exec(ctx, "DELETE FROM dict_items WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDictItemNotFound
	}
	return nil
}

// isPgUnique/isPgFK 唯一/外键冲突判定（*pgconn.PgError 形态）
func isPgUnique(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}

func isPgFK(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23503"
	}
	return false
}
