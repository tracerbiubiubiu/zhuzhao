package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
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
func (h *UserHandler) GetProfile(c *gin.Context) {
	user, err := h.userService.GetProfile(c.Request.Context(), c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, user)
}

// UpdateProfile POST /api/v1/user/profile/update
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
func (h *UserHandler) GetMenus(c *gin.Context) {
	menus, err := h.menuService.GetUserMenus(c.Request.Context(), c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"menus": menus})
}

// GetPermissions GET /api/v1/user/permissions
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
