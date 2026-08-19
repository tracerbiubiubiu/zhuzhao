package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

const orgSelectColumns = `
	id, code, name, COALESCE(description, '') AS description,
	parent_id, path::text AS path, org_type, status, is_system,
	sort_order, created_by, tenant_id, version, deleted_at, created_at, updated_at`

// OrgRepo 组织数据访问
type OrgRepo struct {
	db *pgxpool.Pool
}

func NewOrgRepo(db *pgxpool.Pool) *OrgRepo {
	return &OrgRepo{db: db}
}

func (r *OrgRepo) FindByID(ctx context.Context, id int64) (*model.Organization, error) {
	const q = `SELECT` + orgSelectColumns + `
		FROM organizations WHERE id = $1 AND deleted_at IS NULL`
	return r.queryOne(ctx, q, id)
}

func (r *OrgRepo) GetTree(ctx context.Context) ([]*model.Organization, error) {
	const q = `SELECT` + orgSelectColumns + `
		FROM organizations WHERE deleted_at IS NULL
		ORDER BY sort_order ASC, id ASC`
	return r.queryMany(ctx, q)
}

func (r *OrgRepo) GetUserOrgs(ctx context.Context, userID int64) ([]*model.UserOrg, error) {
	rows, err := r.db.Query(ctx, `
		SELECT uo.user_id, uo.org_id, uo.is_primary, uo.joined_at
		FROM user_orgs uo
		INNER JOIN organizations o ON o.id = uo.org_id
		WHERE uo.user_id = $1 AND o.deleted_at IS NULL
		ORDER BY uo.is_primary DESC, uo.org_id ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("get user orgs: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.UserOrg, error) {
		var uo model.UserOrg
		if err := row.Scan(&uo.UserID, &uo.OrgID, &uo.IsPrimary, &uo.JoinedAt); err != nil {
			return nil, err
		}
		return &uo, nil
	})
}

func (r *OrgRepo) IsMember(ctx context.Context, orgID, userID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_orgs uo
			INNER JOIN organizations o ON o.id = uo.org_id
			WHERE uo.org_id = $1 AND uo.user_id = $2 AND o.deleted_at IS NULL
		)`, orgID, userID).Scan(&exists)
	return exists, err
}

// AddMember 添加组织成员（幂等）
func (r *OrgRepo) AddMember(ctx context.Context, orgID, userID int64, isPrimary bool) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if isPrimary {
		if _, err := tx.Exec(ctx, `
			UPDATE user_orgs SET is_primary = false WHERE user_id = $1`, userID); err != nil {
			return err
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO user_orgs (user_id, org_id, is_primary)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, org_id) DO UPDATE SET is_primary = EXCLUDED.is_primary`, userID, orgID, isPrimary)
	if err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return tx.Commit(ctx)
}

// RemoveMember 移除组织成员
func (r *OrgRepo) RemoveMember(ctx context.Context, orgID, userID int64) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM user_orgs WHERE org_id = $1 AND user_id = $2`, orgID, userID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrNotOrgMember
	}
	return nil
}

// SetUserOrgs 全量覆盖用户组织
func (r *OrgRepo) SetUserOrgs(ctx context.Context, userID int64, orgIDs []int64, primaryOrgID *int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := r.SetUserOrgsTx(ctx, tx, userID, orgIDs, primaryOrgID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetUserOrgsTx 在外部事务内全量覆盖用户组织
func (r *OrgRepo) SetUserOrgsTx(ctx context.Context, tx pgx.Tx, userID int64, orgIDs []int64, primaryOrgID *int64) error {
	if _, err := tx.Exec(ctx, `DELETE FROM user_orgs WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, orgID := range orgIDs {
		isPrimary := primaryOrgID != nil && *primaryOrgID == orgID
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_orgs (user_id, org_id, is_primary)
			VALUES ($1, $2, $3)`, userID, orgID, isPrimary); err != nil {
			return fmt.Errorf("insert user org: %w", err)
		}
	}
	return nil
}

func (r *OrgRepo) CountChildren(ctx context.Context, orgID int64) (int64, error) {
	var n int64
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM organizations
		WHERE parent_id = $1 AND deleted_at IS NULL`, orgID).Scan(&n)
	return n, err
}

func (r *OrgRepo) CountMembers(ctx context.Context, orgID int64) (int64, error) {
	var n int64
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM user_orgs uo
		INNER JOIN organizations o ON o.id = uo.org_id
		INNER JOIN users u ON u.id = uo.user_id
		WHERE uo.org_id = $1 AND o.deleted_at IS NULL AND u.deleted_at IS NULL`, orgID).Scan(&n)
	return n, err
}

func (r *OrgRepo) Create(ctx context.Context, org *model.Organization) error {
	tenantID := org.TenantID
	if tenantID == 0 {
		tenantID = 1
	}
	status := org.Status
	if status == 0 {
		status = 1
	}
	const q = `
		INSERT INTO organizations (
			code, name, description, parent_id, path, org_type, status, is_system,
			sort_order, created_by, tenant_id
		) VALUES (
			$1, $2, NULLIF($3, ''), $4, $5::ltree, $6, $7, false,
			$8, $9, $10
		)
		RETURNING id, version, created_at, updated_at`
	err := r.db.QueryRow(ctx, q,
		org.Code, org.Name, org.Description, org.ParentID, org.Path,
		org.OrgType, status, org.SortOrder, org.CreatedBy, tenantID,
	).Scan(&org.ID, &org.Version, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		if ec := mapUniqueViolation(err); ec != nil {
			return ec
		}
		return fmt.Errorf("create org: %w", err)
	}
	org.Status = status
	org.TenantID = tenantID
	return nil
}

func (r *OrgRepo) Update(ctx context.Context, org *model.Organization) error {
	const q = `
		UPDATE organizations SET
			name = $2,
			description = NULLIF($3, ''),
			status = $4,
			sort_order = $5,
			version = version + 1,
			updated_at = NOW()
		WHERE id = $1 AND version = $6 AND deleted_at IS NULL
		RETURNING version, updated_at`
	err := r.db.QueryRow(ctx, q,
		org.ID, org.Name, org.Description, org.Status, org.SortOrder, org.Version,
	).Scan(&org.Version, &org.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrConcurrentModification
		}
		return fmt.Errorf("update org: %w", err)
	}
	return nil
}

func (r *OrgRepo) Delete(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE organizations SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("delete org: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrOrgNotFound
	}
	return nil
}

func (r *OrgRepo) Move(ctx context.Context, id int64, newParentID *int64, newRootPath, oldPath string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		SELECT id FROM organizations WHERE path <@ $1::ltree FOR UPDATE`, oldPath); err != nil {
		return fmt.Errorf("lock org subtree: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE organizations
		SET path = CASE
			WHEN nlevel(path) = nlevel(text2ltree($2)) THEN text2ltree($1)
			ELSE text2ltree($1) || subpath(path, nlevel(text2ltree($2)))
		END,
		    parent_id = CASE WHEN id = $3 THEN $4 ELSE parent_id END,
		    updated_at = NOW(),
		    version = version + 1
		WHERE path <@ text2ltree($2)`,
		newRootPath, oldPath, id, newParentID,
	); err != nil {
		return fmt.Errorf("move org subtree: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *OrgRepo) IsDescendant(ctx context.Context, ancestorID, descendantID int64) (bool, error) {
	var ok bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM organizations child
			INNER JOIN organizations ancestor ON ancestor.id = $1
			WHERE child.id = $2 AND child.deleted_at IS NULL
			  AND child.path <@ ancestor.path
		)`, ancestorID, descendantID).Scan(&ok)
	return ok, err
}

func (r *OrgRepo) queryOne(ctx context.Context, q string, args ...any) (*model.Organization, error) {
	row := r.db.QueryRow(ctx, q, args...)
	org, err := scanOrgRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errcode.ErrOrgNotFound
		}
		return nil, err
	}
	return org, nil
}

func (r *OrgRepo) queryMany(ctx context.Context, q string, args ...any) ([]*model.Organization, error) {
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, scanOrgCollectableRow)
}

func scanOrgRow(row pgx.Row) (*model.Organization, error) {
	var o model.Organization
	err := row.Scan(
		&o.ID, &o.Code, &o.Name, &o.Description, &o.ParentID, &o.Path,
		&o.OrgType, &o.Status, &o.IsSystem, &o.SortOrder, &o.CreatedBy,
		&o.TenantID, &o.Version, &o.DeletedAt, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func scanOrgCollectableRow(row pgx.CollectableRow) (*model.Organization, error) {
	return scanOrgRow(row)
}
