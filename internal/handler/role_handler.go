package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// RoleHandler 角色处理器
type RoleHandler struct {
	rbacService *service.RBACService
}

func NewRoleHandler(rbacService *service.RBACService) *RoleHandler {
	return &RoleHandler{rbacService: rbacService}
}

// List GET /api/v1/roles
//
//	@Summary	角色列表
//	@Tags		roles
//	@Produce	json
//	@Success	200	{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/roles [get]
func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.rbacService.ListRoles(c.Request.Context(), !isSuperadminActor(c))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, roles)
}

// Create POST /api/v1/roles
//
//	@Summary	创建角色
//	@Tags		roles
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.CreateRoleRequest	true	"创建角色请求"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/roles [post]
func (h *RoleHandler) Create(c *gin.Context) {
	var req model.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	role, err := h.rbacService.CreateRole(c.Request.Context(), &req, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, role)
}

// Get GET /api/v1/roles/:id
//
//	@Summary	角色详情
//	@Tags		roles
//	@Produce	json
//	@Param		id		path	int	true	"角色 ID"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/roles/{id} [get]
func (h *RoleHandler) Get(c *gin.Context) {
	roleID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的角色 ID")
		return
	}
	role, err := h.rbacService.GetRole(c.Request.Context(), roleID, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, role)
}

// Update POST /api/v1/roles/update
//
//	@Summary	更新角色
//	@Tags		roles
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.UpdateRoleRequest	true	"更新角色请求"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/roles/update [post]
func (h *RoleHandler) Update(c *gin.Context) {
	var req model.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	role, err := h.rbacService.UpdateRole(c.Request.Context(), &req, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, role)
}

// Delete POST /api/v1/roles/delete
//
//	@Summary	删除角色
//	@Tags		roles
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.RoleIDRequest	true	"删除角色请求"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/roles/delete [post]
func (h *RoleHandler) Delete(c *gin.Context) {
	var req model.RoleIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	if err := h.rbacService.DeleteRole(c.Request.Context(), req.RoleID, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// AssignMenus POST /api/v1/roles/menus
//
//	@Summary	分配角色菜单
//	@Tags		roles
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.AssignMenusRequest	true	"分配菜单请求"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/roles/menus [post]
func (h *RoleHandler) AssignMenus(c *gin.Context) {
	var req model.AssignMenusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// B4-4：字段级校验详情（原固定文案「参数错误」，无法定位是 role_id 还是 menu_ids 格式问题）
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	if err := h.rbacService.AssignMenus(c.Request.Context(), &req, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// GetMenus GET /api/v1/roles/:id/menus
//
//	@Summary	查询角色菜单
//	@Tags		roles
//	@Produce	json
//	@Param		id		path	int	true	"角色 ID"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/roles/{id}/menus [get]
func (h *RoleHandler) GetMenus(c *gin.Context) {
	roleID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的角色 ID")
		return
	}
	menuIDs, err := h.rbacService.GetRoleMenuIDs(c.Request.Context(), roleID, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"menu_ids": menuIDs})
}

// GetPermissions GET /api/v1/roles/:id/permissions
//
//	@Summary	查询角色权限
//	@Tags		roles
//	@Produce	json
//	@Param		id		path	int	true	"角色 ID"
//	@Success	200		{object}	response.Response
//	@Security	BearerAuth
//	@Router		/api/v1/roles/{id}/permissions [get]
func (h *RoleHandler) GetPermissions(c *gin.Context) {
	roleID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的角色 ID")
		return
	}
	policies, err := h.rbacService.GetRolePermissions(c.Request.Context(), roleID, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"policies": policies})
}

func isSuperadminActor(c *gin.Context) bool {
	roles, ok := c.Get("roles")
	if !ok {
		return false
	}
	codes, ok := roles.([]string)
	if !ok {
		return false
	}
	for _, code := range codes {
		if code == "superadmin" {
			return true
		}
	}
	return false
}
