package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

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
	if err := h.userService.Delete(c.Request.Context(), req.UserID, c.GetInt64("userID")); err != nil {
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
	if err := h.userService.UpdateStatus(c.Request.Context(), &req, c.GetInt64("userID")); err != nil {
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
//	@Security	BearerAuth
//	@Router		/api/v1/users/roles [post]
func (h *UserHandler) SetRoles(c *gin.Context) {
	var req model.SetUserRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	if err := h.userService.SetRoles(c.Request.Context(), &req, c.GetInt64("userID")); err != nil {
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
	if err := h.userService.ResetPassword(c.Request.Context(), &req, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
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
