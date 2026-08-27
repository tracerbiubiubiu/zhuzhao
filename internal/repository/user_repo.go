package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	Page                   int
	PageSize               int
	Username               string // 模糊匹配
	EmployeeNo             string // 精确匹配
	RoleCode               string
	Status                 *int
	ExcludeSuperadminUsers bool
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

// ListByOrgID 组织成员列表（B4-5：分页——modules/organization.md §4.3 承诺）
func (r *UserRepo) ListByOrgID(ctx context.Context, orgID int64, page, pageSize int) ([]*model.User, int64, error) {
	page, pageSize = normalizePage(page, pageSize)
	var total int64
	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM users u
		INNER JOIN user_orgs uo ON uo.user_id = u.id
		WHERE uo.org_id = $1 AND u.deleted_at IS NULL`, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users by org: %w", err)
	}
	const q = `SELECT` + userSelectColumns + `
		FROM users u
		INNER JOIN user_orgs uo ON uo.user_id = u.id
		WHERE uo.org_id = $1 AND u.deleted_at IS NULL
		ORDER BY u.id ASC
		LIMIT $2 OFFSET $3`
	rows, err := r.db.Query(ctx, q, orgID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list users by org: %w", err)
	}
	defer rows.Close()
	users, err := pgx.CollectRows(rows, scanUserCollectableRow)
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// FindByEmployeeNo 登录用：工号精确匹配；未删除用户
func (r *UserRepo) FindByEmployeeNo(ctx context.Context, employeeNo string) (*model.User, error) {
	// B4-1：补文档承诺的空串防御（与 docs/modules/user.md §5 登录查询一致；
	// 当前入口已保证非空，防御未来新增调用方）
	const q = `SELECT` + userSelectColumns + `
    FROM users WHERE employee_no = $1 AND deleted_at IS NULL
    AND employee_no <> '' AND employee_no IS NOT NULL LIMIT 1`
	return r.queryOne(ctx, q, employeeNo)
}

// FindByEmployeeNoIncludeDeleted 含软删用户的工号查询（D2-27）——
// 审计按工号筛选须覆盖历史（软删用户的历史审计可查，原 404）
func (r *UserRepo) FindByEmployeeNoIncludeDeleted(ctx context.Context, employeeNo string) (*model.User, error) {
	const q = `SELECT` + userSelectColumns + `
    FROM users WHERE employee_no = $1 LIMIT 1`
	return r.queryOne(ctx, q, employeeNo)
}

// FindByID 根据 ID 查询未删除用户
func (r *UserRepo) FindByID(ctx context.Context, id int64) (*model.User, error) {
	const q = `SELECT` + userSelectColumns + `
		FROM users WHERE id = $1 AND deleted_at IS NULL`
	return r.queryOne(ctx, q, id)
}

// rowExec 抽象 pgx.Tx 与 pgxpool.Pool 共同的 QueryRow/Exec 能力，供 Tx 版本复用
type rowExec interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Create 创建用户（password 须为 bcrypt hash）
func (r *UserRepo) Create(ctx context.Context, user *model.User) error {
	return r.createUser(ctx, r.db, user)
}

// CreateTx 在外部事务内创建用户
func (r *UserRepo) CreateTx(ctx context.Context, tx pgx.Tx, user *model.User) error {
	return r.createUser(ctx, tx, user)
}

func (r *UserRepo) createUser(ctx context.Context, exec rowExec, user *model.User) error {
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

	err := exec.QueryRow(ctx, q,
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

// SoftDelete 软删除用户（事务内清理 user_roles/user_orgs 关联）
func (r *UserRepo) SoftDelete(ctx context.Context, id int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := r.SoftDeleteTx(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SoftDeleteTx 在外部事务内软删除用户（含关联清理）
func (r *UserRepo) SoftDeleteTx(ctx context.Context, tx pgx.Tx, id int64) error {
	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, id); err != nil {
		return fmt.Errorf("delete user_roles: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM user_orgs WHERE user_id = $1`, id); err != nil {
		return fmt.Errorf("delete user_orgs: %w", err)
	}

	tag, err := tx.Exec(ctx, `
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

// RunInTx 在事务内执行 fn（失败整体回滚）。
// TOCTOU 修复基础设施：superadmin 保护检查与写入须在同一事务 + advisory lock 下串行化
func (r *UserRepo) RunInTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AcquireSuperadminGuard 事务内获取最后 superadmin 保护锁。
// 相同 lock key 的并发事务在此串行化，消除"检查与写入之间"的时间窗（TOCTOU）
func AcquireSuperadminGuard(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('last_superadmin_guard'))`)
	if err != nil {
		return fmt.Errorf("acquire superadmin guard lock: %w", err)
	}
	return nil
}

// IsSuperadminUserTx 事务内版本（TOCTOU 修复：与写操作同快照）
// 角色禁用语义（B1-1）：仅启用中的角色参与鉴权/保护判断
func (r *UserRepo) IsSuperadminUserTx(ctx context.Context, tx pgx.Tx, userID int64) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_roles ur
			INNER JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = $1 AND r.code = 'superadmin' AND r.deleted_at IS NULL AND r.status = 1
		)`, userID).Scan(&exists)
	return exists, err
}

// CountActiveSuperadminUsersTx 事务内版本（TOCTOU 修复：与写操作同快照）
// 角色禁用语义（B1-1）：活跃 superadmin = 用户启用 ∧ 角色启用
func (r *UserRepo) CountActiveSuperadminUsersTx(ctx context.Context, tx pgx.Tx) (int64, error) {
	var n int64
	err := tx.QueryRow(ctx, `
		SELECT COUNT(DISTINCT u.id) FROM users u
		INNER JOIN user_roles ur ON ur.user_id = u.id
		INNER JOIN roles r ON r.id = ur.role_id
		WHERE r.code = 'superadmin' AND u.deleted_at IS NULL AND u.status = 1 AND r.status = 1`).Scan(&n)
	return n, err
}

// UpdateStatusTx 在外部事务内更新启用/禁用状态
func (r *UserRepo) UpdateStatusTx(ctx context.Context, tx pgx.Tx, id int64, status int) error {
	tag, err := tx.Exec(ctx, `
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

// SetRoles 全量覆盖用户角色（事务内先删后插）
func (r *UserRepo) SetRoles(ctx context.Context, userID int64, roleIDs []int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := r.SetRolesTx(ctx, tx, userID, roleIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetRolesTx 在外部事务内全量覆盖用户角色。
// D2-26：去重 + 行数核对——原 INSERT...SELECT 静默跳过软删角色（TOCTOU 窗口
// 内角色被删 → 创建成功但角色未绑、无提示）；重复 roleID 也会与行数核对冲突，先去重
func (r *UserRepo) SetRolesTx(ctx context.Context, tx pgx.Tx, userID int64, roleIDs []int64) error {
	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete user roles: %w", err)
	}
	// 去重（保序）
	seen := make(map[int64]struct{}, len(roleIDs))
	deduped := make([]int64, 0, len(roleIDs))
	for _, id := range roleIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}
	for _, roleID := range deduped {
		// B4-3：INSERT...SELECT 带活跃性条件——消灭「service 预检通过后、
		// 写入前」角色被软删的 TOCTOU 残余窗口（软删行 FK 仍通过，原会写入幽灵绑定）
		tag, err := tx.Exec(ctx, `
			INSERT INTO user_roles (user_id, role_id)
			SELECT $1, id FROM roles
			WHERE id = $2 AND deleted_at IS NULL
			ON CONFLICT DO NOTHING`, userID, roleID)
		if err != nil {
			if ec := MapForeignKeyViolation(err); ec != nil {
				return ec
			}
			return fmt.Errorf("insert user role: %w", err)
		}
		// D2-26：0 行 = 角色 TOCTOU 窗口内被软删 → 显式 404 回滚整个事务
		//（Create 场景此前会静默丢角色创建成功）
		if tag.RowsAffected() == 0 {
			return errcode.ErrRoleNotFound
		}
	}
	return nil
}

// GetRoleCodes 查询用户绑定的角色 code（供 RoleFetcher / 业务层）。
// 角色禁用语义（B1-1）：禁用角色（status=0）不参与鉴权——Casbin enforce、
// 菜单下发、priority 档位均不将其计入，禁用自下次请求起生效。
func (r *UserRepo) GetRoleCodes(ctx context.Context, userID int64) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT r.code FROM roles r
		INNER JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1 AND r.deleted_at IS NULL AND r.status = 1
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

// GetRoles 查询用户绑定的角色实体（仅启用中，B1-1：禁用角色不计入 priority 档位与 superadmin 判断）
func (r *UserRepo) GetRoles(ctx context.Context, userID int64) ([]*model.Role, error) {
	rows, err := r.db.Query(ctx, `
		SELECT r.id, r.code, r.name, COALESCE(r.description, ''), r.status, r.priority,
			r.sort_order, r.is_system, r.tenant_id, r.version, r.deleted_at, r.created_at, r.updated_at
		FROM roles r
		INNER JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1 AND r.deleted_at IS NULL AND r.status = 1
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

// IsSuperadminUser 用户是否绑定 superadmin 角色（仅角色启用时，B1-1）
func (r *UserRepo) IsSuperadminUser(ctx context.Context, userID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_roles ur
			INNER JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = $1 AND r.code = 'superadmin' AND r.deleted_at IS NULL AND r.status = 1
		)`, userID).Scan(&exists)
	return exists, err
}

// CountActiveSuperadminUsers 统计启用中的 superadmin 用户数（含角色启用条件，B1-1）
func (r *UserRepo) CountActiveSuperadminUsers(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(DISTINCT u.id) FROM users u
		INNER JOIN user_roles ur ON ur.user_id = u.id
		INNER JOIN roles r ON r.id = ur.role_id
		WHERE r.code = 'superadmin' AND u.deleted_at IS NULL AND u.status = 1 AND r.status = 1`).Scan(&n)
	return n, err
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
		// D2-21：转义 ILIKE 通配符（%/_/\）——未转义时用户名含 % 即全表匹配，
		// 含 _ 单字符通配（语义不符 + 可探测）；ESCAPE '\' 为 PG 默认，显式声明
		args = append(args, "%"+escapeLike(q.Username)+"%")
		conds = append(conds, fmt.Sprintf("u.username ILIKE $%d ESCAPE '\\'", len(args)))
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
	if q.ExcludeSuperadminUsers {
		conds = append(conds, `u.id NOT IN (
			SELECT ur.user_id FROM user_roles ur
			INNER JOIN roles r ON r.id = ur.role_id
			WHERE r.code = 'superadmin' AND r.deleted_at IS NULL
		)`)
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// escapeLike 转义 ILIKE 模式元字符（D2-21），配合 ESCAPE '\' 使用
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// normalizePage 用户/组织成员分页规范化。
// D2-13：page 上限 10000——对齐 B4-6 审计分页；原无上限时超大 page 经
// (page-1)*pageSize 溢出回绕为负 OFFSET → SQL 500（且巨量 OFFSET 扫描放大 DB 压力）
func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if page > 10000 {
		page = 10000
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
