package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// int64IDsToStrings 出站 ID 数组字符串化（2026-10-14 契约审计 B1：standards §3-12
// 发送侧一律字符串——原 []int64 直出为 JSON number，前端按字符串比对全不命中；
// utils jsonutil.Int64Slice 的 MarshalJSON 随 v0.4.2 可用后可迁移至该形态）
func int64IDsToStrings(ids []int64) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = strconv.FormatInt(id, 10)
	}
	return out
}

// setOperationHeaders 回填操作核查头（C05：响应丢失后调用方可按 K 查原结果）。
// 503（投影待恢复）时回执虽已提交，头部状态改报 pending——「已提交、待恢复」
// 是调用方/UI 的唯一关注语义；权威状态可按 K 走受权核查。
func setOperationHeaders(c *gin.Context, res *service.SessionCommandResult, err error) {
	if res == nil {
		return
	}
	status := res.Status
	if err != nil {
		status = "pending"
	}
	c.Header("X-Operation-ID", res.OperationID)
	c.Header("X-Operation-Status", status)
}

// UserHandler 用户处理器
type UserHandler struct {
	userService *service.UserService
	menuService *service.MenuService
}

func NewUserHandler(userService *service.UserService, menuService *service.MenuService) *UserHandler {
	return &UserHandler{userService: userService, menuService: menuService}
}

// List GET /api/v1/users
//
//	@Summary	用户列表
//	@Tags		users
//	@Produce	json
//	@Param		page			query	int		false	"页码"
//	@Param		page_size		query	int		false	"每页条数"
//	@Param		username		query	string	false	"用户名"
//	@Param		employee_no		query	string	false	"工号"
//	@Param		role			query	string	false	"角色编码"
//	@Param		status			query	int		false	"状态（1=启用 0=禁用）"
//	@Success	200				{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/users [get]
func (h *UserHandler) List(c *gin.Context) {
	q := repository.UserListQuery{
		Page:       queryInt(c, "page", 1),
		PageSize:   queryInt(c, "page_size", 20),
		Username:   c.Query("username"),
		EmployeeNo: c.Query("employee_no"),
		RoleCode:   c.Query("role"),
	}
	if st := c.Query("status"); st != "" {
		if v, err := strconv.Atoi(st); err == nil {
			q.Status = &v
		}
	}
	resp, err := h.userService.List(c.Request.Context(), q, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, resp)
}

// Create POST /api/v1/users
//
//	@Summary	创建用户
//	@Tags		users
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.CreateUserRequest	true	"创建用户请求"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/users [post]
func (h *UserHandler) Create(c *gin.Context) {
	var req model.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	user, err := h.userService.Create(c.Request.Context(), &req, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, user)
}

// Get GET /api/v1/users/:id
//
//	@Summary	用户详情
//	@Tags		users
//	@Produce	json
//	@Param		id		path	int	true	"用户 ID"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/users/{id} [get]
func (h *UserHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的用户 ID")
		return
	}
	user, err := h.userService.GetByID(c.Request.Context(), id, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, user)
}

// Update POST /api/v1/users/update
//
//	@Summary	更新用户
//	@Tags		users
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.UpdateUserRequest	true	"更新用户请求"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/users/update [post]
func (h *UserHandler) Update(c *gin.Context) {
	var req model.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	user, err := h.userService.Update(c.Request.Context(), &req, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, user)
}

// Delete POST /api/v1/users/delete
//
//	@Summary	删除用户
//	@Tags		users
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.UserIDRequest	true	"删除用户请求"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/users/delete [post]
func (h *UserHandler) Delete(c *gin.Context) {
	var req model.UserIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	res, err := h.userService.Delete(c.Request.Context(), req.UserID, c.GetInt64("userID"), c.GetHeader("X-Operation-ID"))
	setOperationHeaders(c, res, err)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// UpdateStatus POST /api/v1/users/status
//
//	@Summary	启用/禁用用户
//	@Tags		users
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.UpdateUserStatusRequest	true	"更新用户状态请求"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/users/status [post]
func (h *UserHandler) UpdateStatus(c *gin.Context) {
	var req model.UpdateUserStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	res, err := h.userService.UpdateStatus(c.Request.Context(), &req, c.GetInt64("userID"), c.GetHeader("X-Operation-ID"))
	setOperationHeaders(c, res, err)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// SetRoles POST /api/v1/users/roles
//
//	@Summary	设置用户角色
//	@Tags		users
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.SetUserRolesRequest	true	"设置用户角色请求"
//	@Success	200		{object}	response.Response
//	@Param Idempotency-Key header string false "改权原键，重试必须保持原输入"
//	@Param X-Operation-ID header string false "原操作号，可替代 Idempotency-Key"
//	@Header 200 {string} X-Operation-ID "原操作号"
//	@Security	BearerAuth
//	@Router		/api/v1/users/roles [post]
func (h *UserHandler) SetRoles(c *gin.Context) {
	var req model.SetUserRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	ctx, ok := iamOperationContext(c)
	if !ok {
		return
	}
	if err := h.userService.SetRoles(ctx, &req, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// ResetPassword POST /api/v1/users/password/reset
//
//	@Summary	重置用户密码
//	@Tags		users
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.ResetPasswordRequest	true	"重置密码请求"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/users/password/reset [post]
func (h *UserHandler) ResetPassword(c *gin.Context) {
	var req model.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	res, err := h.userService.ResetPassword(c.Request.Context(), &req, c.GetInt64("userID"), c.GetHeader("X-Operation-ID"))
	setOperationHeaders(c, res, err)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// OperationResult GET /api/v1/users/status/result?operation_id=...
//
//	@Summary	按原操作号核查账号安全操作结果（停用/删除/重置/自助改密）
//	@Tags		users
//	@Produce	json
//	@Param		operation_id	query	string	true	"原操作号（X-Operation-ID）"
//	@Success	200	{object}	response.Response{data=service.UserOperationResultView}	"原操作回执及对应代际的投影状态"
//	@Security	BearerAuth
//	@Router		/api/v1/users/status/result [get]
func (h *UserHandler) OperationResult(c *gin.Context) {
	opID := c.Query("operation_id")
	if opID == "" {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	view, err := h.userService.GetOperationResult(c.Request.Context(), opID, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.Header("X-Operation-ID", view.OperationID)
	c.Header("X-Operation-Status", view.Status)
	response.OK(c, view)
}

// GetUserOrgs GET /api/v1/users/:id/orgs
//
//	@Summary	查询用户所属组织
//	@Tags		users
//	@Produce	json
//	@Param		id		path	int	true	"用户 ID"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/users/{id}/orgs [get]
func (h *UserHandler) GetUserOrgs(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的用户 ID")
		return
	}
	orgs, err := h.userService.GetUserOrgs(c.Request.Context(), id, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"orgs": orgs})
}

// GetUserRoles GET /api/v1/users/:id/roles
//
//	@Summary	查询用户直接绑定的角色 ID（含停用绑定，不含组织或继承来源）
//	@Tags		users
//	@Produce	json
//	@Param		id		path	int	true	"用户 ID"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/users/{id}/roles [get]
func (h *UserHandler) GetUserRoles(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的用户 ID")
		return
	}
	roleIDs, err := h.userService.GetUserRoleIDs(c.Request.Context(), id, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"role_ids": int64IDsToStrings(roleIDs)})
}

// SetUserOrgs POST /api/v1/users/orgs
//
//	@Summary	设置用户所属组织
//	@Tags		users
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.SetUserOrgsRequest	true	"设置用户组织请求"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/users/orgs [post]
func (h *UserHandler) SetUserOrgs(c *gin.Context) {
	var req model.SetUserOrgsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	if err := h.userService.SetUserOrgs(c.Request.Context(), &req, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// GetProfile GET /api/v1/user/profile
//
//	@Summary	当前用户信息
//	@Tags		users
//	@Produce	json
//	@Success	200	{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/user/profile [get]
func (h *UserHandler) GetProfile(c *gin.Context) {
	user, err := h.userService.GetProfile(c.Request.Context(), c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, user)
}

// UpdateProfile POST /api/v1/user/profile/update
//
//	@Summary	更新当前用户信息
//	@Tags		users
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.UpdateProfileRequest	true	"更新个人信息请求"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/user/profile/update [post]
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	var req model.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	user, err := h.userService.UpdateProfile(c.Request.Context(), c.GetInt64("userID"), &req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, user)
}

// GetMenus GET /api/v1/user/menus
//
//	@Summary	当前用户菜单
//	@Tags		users
//	@Produce	json
//	@Success	200	{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/user/menus [get]
func (h *UserHandler) GetMenus(c *gin.Context) {
	menus, err := h.menuService.GetUserMenus(c.Request.Context(), c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"menus": menus})
}

// GetPermissions GET /api/v1/user/permissions
//
//	@Summary	当前用户权限码
//	@Tags		users
//	@Produce	json
//	@Success	200	{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/user/permissions [get]
func (h *UserHandler) GetPermissions(c *gin.Context) {
	perms, err := h.menuService.GetUserPermissions(c.Request.Context(), c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"permissions": perms})
}

func queryInt(c *gin.Context, key string, def int) int {
	v := c.Query(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func errcodeInvalidParams(c *gin.Context) string {
	_ = c
	return "参数错误"
}

// IAMReceipt returns the original, currently authorized IAM command outcome.
//
// @Summary 按原操作号或幂等键核查两条 IAM 改权命令的本地提交结果
// @Tags users
// @Produce json
// @Param operation_id query string false "原操作号，与 idempotency_key 二选一"
// @Param idempotency_key query string false "原幂等键"
// @Success 200 {object} response.Response{data=model.IAMReceipt}
// @Security BearerAuth
// @Router /api/v1/operations/receipt [get]
func (h *UserHandler) IAMReceipt(c *gin.Context) {
	q := c.Request.URL.Query()
	ids, keys := q["operation_id"], q["idempotency_key"]
	if len(ids)+len(keys) != 1 {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	id := q.Get("operation_id")
	if len(keys) == 1 {
		id = keys[0]
	}
	receipt, err := h.userService.IAMReceipt(c.Request.Context(), c.GetInt64("userID"), id)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.Header("X-Operation-ID", receipt.OperationID)
	response.OK(c, receipt)
}
