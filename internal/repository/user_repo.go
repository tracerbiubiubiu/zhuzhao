package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

const userSelectColumns = `
	id, username,
	COALESCE(employee_no, '') AS employee_no,
	COALESCE(domain_account, '') AS domain_account,
	COALESCE(user_domain, '') AS user_domain,
	password,
	COALESCE(real_name, '') AS real_name,
	COALESCE(email, '') AS email,
	COALESCE(phone, '') AS phone,
	COALESCE(avatar, '') AS avatar,
	status, must_change_password,
	last_login_at,
	COALESCE(last_login_ip, '') AS last_login_ip,
	is_system, tenant_id, version, deleted_at, created_at, updated_at`

// UserListQuery 用户列表筛选
type UserListQuery struct {
	Page       int
	PageSize   int
	Username   string // 模糊匹配
	EmployeeNo string // 精确匹配
	RoleCode   string
	Status     *int
}

// UserRepo 用户数据访问
type UserRepo struct {
	db *pgxpool.Pool
}

// NewUserRepo 创建 UserRepo
func NewUserRepo(db *pgxpool.Pool) *UserRepo {
	return &UserRepo{db: db}
}

// FindByUsername 按 username 精确匹配（非登录键）
func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	const q = `SELECT` + userSelectColumns + `
		FROM users WHERE username = $1 AND deleted_at IS NULL LIMIT 1`
	return r.queryOne(ctx, q, username)
}

// FindByEmployeeNo 登录用：工号精确匹配；未删除用户
func (r *UserRepo) FindByEmployeeNo(ctx context.Context, employeeNo string) (*model.User, error) {
	const q = `SELECT` + userSelectColumns + `
		FROM users WHERE employee_no = $1 AND deleted_at IS NULL LIMIT 1`
	return r.queryOne(ctx, q, employeeNo)
}

// FindByID 根据 ID 查询未删除用户
func (r *UserRepo) FindByID(ctx context.Context, id int64) (*model.User, error) {
	const q = `SELECT` + userSelectColumns + `
		FROM users WHERE id = $1 AND deleted_at IS NULL`
	return r.queryOne(ctx, q, id)
}

// Create 创建用户（password 须为 bcrypt hash）
func (r *UserRepo) Create(ctx context.Context, user *model.User) error {
	const q = `
		INSERT INTO users (
			username, employee_no, domain_account, user_domain, password,
			real_name, email, phone, avatar, status, must_change_password,
			is_system, tenant_id
		) VALUES (
			$1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5,
			NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), $10, $11,
			$12, $13
		)
		RETURNING id, version, created_at, updated_at`

	tenantID := user.TenantID
	if tenantID == 0 {
		tenantID = 1
	}
	status := user.Status
	if status == 0 {
		status = 1
	}

	err := r.db.QueryRow(ctx, q,
		user.Username,
		user.EmployeeNo,
		user.DomainAccount,
		user.UserDomain,
		user.Password,
		user.RealName,
		user.Email,
		user.Phone,
		user.Avatar,
		status,
		user.MustChangePassword,
		user.IsSystem,
		tenantID,
	).Scan(&user.ID, &user.Version, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if ec := mapUniqueViolation(err); ec != nil {
			return ec
		}
		return fmt.Errorf("create user: %w", err)
	}
	user.Status = status
	user.TenantID = tenantID
	return nil
}

// List 分页查询用户列表（仅未软删）
func (r *UserRepo) List(ctx context.Context, q UserListQuery) ([]*model.User, int64, error) {
	page, pageSize := normalizePage(q.Page, q.PageSize)

	where, args := buildUserListWhere(q)

	countSQL := `SELECT COUNT(*) FROM users u` + where
	var total int64
	if err := r.db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	listArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	listSQL := `SELECT` + userSelectColumns + `
		FROM users u` + where + `
		ORDER BY u.id ASC
		LIMIT $` + fmt.Sprint(len(args)+1) + ` OFFSET $` + fmt.Sprint(len(args)+2)

	rows, err := r.db.Query(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users, err := pgx.CollectRows(rows, scanUserCollectableRow)
	if err != nil {
		return nil, 0, fmt.Errorf("collect users: %w", err)
	}
	return users, total, nil
}

// Update 更新用户资料（乐观锁 version）
func (r *UserRepo) Update(ctx context.Context, user *model.User) error {
	const q = `
		UPDATE users SET
			username = $2,
			employee_no = NULLIF($3, ''),
			domain_account = NULLIF($4, ''),
			user_domain = NULLIF($5, ''),
			real_name = NULLIF($6, ''),
			email = NULLIF($7, ''),
			phone = NULLIF($8, ''),
			avatar = NULLIF($9, ''),
			status = $10,
			must_change_password = $11,
			version = version + 1,
			updated_at = NOW()
		WHERE id = $1 AND version = $12 AND deleted_at IS NULL
		RETURNING version, updated_at`

	err := r.db.QueryRow(ctx, q,
		user.ID,
		user.Username,
		user.EmployeeNo,
		user.DomainAccount,
		user.UserDomain,
		user.RealName,
		user.Email,
		user.Phone,
		user.Avatar,
		user.Status,
		user.MustChangePassword,
		user.Version,
	).Scan(&user.Version, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrConcurrentModification
		}
		if ec := mapUniqueViolation(err); ec != nil {
			return ec
		}
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

// UpdateStatus 更新启用/禁用状态
func (r *UserRepo) UpdateStatus(ctx context.Context, id int64, status int) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE users SET status = $2, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id, status)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrUserNotFound
	}
	return nil
}

// UpdateLastLogin 记录最后登录时间与 IP
func (r *UserRepo) UpdateLastLogin(ctx context.Context, id int64, ip string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE users SET last_login_at = NOW(), last_login_ip = $2, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id, ip)
	if err != nil {
		return fmt.Errorf("update last login: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrUserNotFound
	}
	return nil
}

// UpdatePassword 更新密码 hash
func (r *UserRepo) UpdatePassword(ctx context.Context, id int64, passwordHash string, mustChange bool) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE users SET password = $2, must_change_password = $3, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id, passwordHash, mustChange)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrUserNotFound
	}
	return nil
}

// SoftDelete 软删除用户
func (r *UserRepo) SoftDelete(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE users SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("soft delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrUserNotFound
	}
	return nil
}

// SetRoles 全量覆盖用户角色（事务内先删后插）
func (r *UserRepo) SetRoles(ctx context.Context, userID int64, roleIDs []int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete user roles: %w", err)
	}
	for _, roleID := range roleIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, userID, roleID); err != nil {
			return fmt.Errorf("insert user role: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// GetRoleCodes 查询用户绑定的角色 code（供 RoleFetcher / 业务层）
func (r *UserRepo) GetRoleCodes(ctx context.Context, userID int64) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT r.code FROM roles r
		INNER JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1 AND r.deleted_at IS NULL
		ORDER BY r.priority ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("get role codes: %w", err)
	}
	defer rows.Close()

	codes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (string, error) {
		var code string
		if err := row.Scan(&code); err != nil {
			return "", err
		}
		return code, nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect role codes: %w", err)
	}
	return codes, nil
}

// GetRoles 查询用户绑定的角色实体
func (r *UserRepo) GetRoles(ctx context.Context, userID int64) ([]*model.Role, error) {
	rows, err := r.db.Query(ctx, `
		SELECT r.id, r.code, r.name, COALESCE(r.description, ''), r.status, r.priority,
			r.sort_order, r.is_system, r.tenant_id, r.version, r.deleted_at, r.created_at, r.updated_at
		FROM roles r
		INNER JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1 AND r.deleted_at IS NULL
		ORDER BY r.priority ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("get roles: %w", err)
	}
	defer rows.Close()

	roles, err := pgx.CollectRows(rows, scanRoleRow)
	if err != nil {
		return nil, fmt.Errorf("collect roles: %w", err)
	}
	return roles, nil
}

func (r *UserRepo) queryOne(ctx context.Context, q string, args ...any) (*model.User, error) {
	row := r.db.QueryRow(ctx, q, args...)
	user, err := scanUserRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errcode.ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

func scanUserRow(row pgx.Row) (*model.User, error) {
	var u model.User
	err := row.Scan(
		&u.ID, &u.Username, &u.EmployeeNo, &u.DomainAccount, &u.UserDomain, &u.Password,
		&u.RealName, &u.Email, &u.Phone, &u.Avatar, &u.Status, &u.MustChangePassword,
		&u.LastLoginAt, &u.LastLoginIP, &u.IsSystem, &u.TenantID, &u.Version, &u.DeletedAt,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func scanUserCollectableRow(row pgx.CollectableRow) (*model.User, error) {
	var u model.User
	err := row.Scan(
		&u.ID, &u.Username, &u.EmployeeNo, &u.DomainAccount, &u.UserDomain, &u.Password,
		&u.RealName, &u.Email, &u.Phone, &u.Avatar, &u.Status, &u.MustChangePassword,
		&u.LastLoginAt, &u.LastLoginIP, &u.IsSystem, &u.TenantID, &u.Version, &u.DeletedAt,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func scanRoleRow(row pgx.CollectableRow) (*model.Role, error) {
	var role model.Role
	err := row.Scan(
		&role.ID, &role.Code, &role.Name, &role.Description, &role.Status, &role.Priority,
		&role.SortOrder, &role.IsSystem, &role.TenantID, &role.Version, &role.DeletedAt,
		&role.CreatedAt, &role.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func buildUserListWhere(q UserListQuery) (string, []any) {
	var conds []string
	var args []any
	conds = append(conds, "u.deleted_at IS NULL")

	if q.Username != "" {
		args = append(args, "%"+q.Username+"%")
		conds = append(conds, fmt.Sprintf("u.username ILIKE $%d", len(args)))
	}
	if q.EmployeeNo != "" {
		args = append(args, q.EmployeeNo)
		conds = append(conds, fmt.Sprintf("u.employee_no = $%d", len(args)))
	}
	if q.Status != nil {
		args = append(args, *q.Status)
		conds = append(conds, fmt.Sprintf("u.status = $%d", len(args)))
	}
	if q.RoleCode != "" {
		args = append(args, q.RoleCode)
		conds = append(conds, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM user_roles ur
			INNER JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = u.id AND r.code = $%d AND r.deleted_at IS NULL
		)`, len(args)))
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
