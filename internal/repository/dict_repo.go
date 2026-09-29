package repository

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// DictRepo P4-3 字典数据访问。sentinel error 由 service 映射 errcode。
var (
	ErrDictTypeNotFound = errors.New("repository: dict type not found")
	ErrDictTypeDupCode  = errors.New("repository: dict type code duplicated")
	ErrDictItemNotFound = errors.New("repository: dict item not found")
	ErrDictItemDupCode  = errors.New("repository: dict item code duplicated")
)

const dictTypeColumns = `id, code, name, enabled, remark, version, created_at, updated_at`
const dictItemColumns = `id, type_code, code, label, sort_order, enabled, remark, version, created_at, updated_at`

type DictRepo struct {
	db *pgxpool.Pool
}

func NewDictRepo(db *pgxpool.Pool) *DictRepo {
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

// DeleteTypeByCode 按 code 删（items FK ON DELETE CASCADE 随删）
func (r *DictRepo) DeleteTypeByCode(ctx context.Context, code string) error {
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

func (r *DictRepo) DeleteItemByID(ctx context.Context, id int64) error {
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
