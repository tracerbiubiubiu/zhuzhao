package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// OrgHandler 组织处理器
type OrgHandler struct {
	orgService *service.OrgService
}

func NewOrgHandler(orgService *service.OrgService) *OrgHandler {
	return &OrgHandler{orgService: orgService}
}

// GetTree GET /api/v1/orgs
func (h *OrgHandler) GetTree(c *gin.Context) {
	tree, err := h.orgService.GetTree(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, tree)
}

// Create POST /api/v1/orgs
func (h *OrgHandler) Create(c *gin.Context) {
	var req model.CreateOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	org, err := h.orgService.Create(c.Request.Context(), &req, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, org)
}

// Get GET /api/v1/orgs/:id
func (h *OrgHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的组织 ID")
		return
	}
	org, err := h.orgService.GetByID(c.Request.Context(), id)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, org)
}

// Update POST /api/v1/orgs/update
func (h *OrgHandler) Update(c *gin.Context) {
	var req model.UpdateOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	org, err := h.orgService.Update(c.Request.Context(), &req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, org)
}

// Delete POST /api/v1/orgs/delete
func (h *OrgHandler) Delete(c *gin.Context) {
	var req model.OrgIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	if err := h.orgService.DeleteOrgDelegated(c.Request.Context(), req.OrgID, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// Move POST /api/v1/orgs/move
func (h *OrgHandler) Move(c *gin.Context) {
	var req model.MoveOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, errcodeInvalidParams(c))
		return
	}
	if err := h.orgService.Move(c.Request.Context(), &req); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// GetMembers GET /api/v1/orgs/:id/members（B4-5：支持 page/page_size 查询参数）
func (h *OrgHandler) GetMembers(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的组织 ID")
		return
	}
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)
	resp, err := h.orgService.GetMembers(c.Request.Context(), id, page, pageSize)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, resp)
}

// AddMember POST /api/v1/orgs/members
func (h *OrgHandler) AddMember(c *gin.Context) {
	var req model.OrgMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.orgService.AddMember(c.Request.Context(), &req, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// ListOrgRoles GET /api/v1/orgs/roles/list?org_id=（IW3/BK-12）
//
//	@Summary	组织已绑定角色列表
//	@Tags		org
//	@Produce	json
//	@Param		org_id	query	string	true	"组织 ID"
//	@Success	200		{object}	response.Response
//	@Router		/api/v1/orgs/roles/list [get]
func (h *OrgHandler) ListOrgRoles(c *gin.Context) {
	orgID, err := strconv.ParseInt(c.Query("org_id"), 10, 64)
	if err != nil || orgID <= 0 {
		response.BadRequest(c, "无效的组织 ID")
		return
	}
	roles, err := h.orgService.ListOrgRoles(c.Request.Context(), orgID, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, roles)
}

// BindOrgRole POST /api/v1/orgs/roles/bind（IW3/BK-12：仅全局管理员）
//
//	@Summary	组织绑定角色
//	@Tags		org
//	@Accept		json
//	@Produce	json
//	@Param		req	body	model.BindOrgRoleRequest	true	"组织与角色"
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/orgs/roles/bind [post]
func (h *OrgHandler) BindOrgRole(c *gin.Context) {
	var req model.BindOrgRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.orgService.BindOrgRole(c.Request.Context(), &req, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// UnbindOrgRole POST /api/v1/orgs/roles/delete（IW3/BK-12：仅全局管理员）
//
//	@Summary	组织解绑角色
//	@Tags		org
//	@Accept		json
//	@Produce	json
//	@Param		req	body	model.BindOrgRoleRequest	true	"组织与角色"
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/orgs/roles/delete [post]
func (h *OrgHandler) UnbindOrgRole(c *gin.Context) {
	var req model.BindOrgRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.orgService.UnbindOrgRole(c.Request.Context(), &req, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// SetMemberScope POST /api/v1/orgs/members/scope（IW1/BK-14：成员数据范围变更）
func (h *OrgHandler) SetMemberScope(c *gin.Context) {
	var req model.SetMemberScopeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.orgService.SetMemberScope(c.Request.Context(), &req, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// RemoveMember POST /api/v1/orgs/members/delete
func (h *OrgHandler) RemoveMember(c *gin.Context) {
	var req model.OrgMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.orgService.RemoveMember(c.Request.Context(), &req, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// SetOwners POST /api/v1/orgs/owners（2c，04 §3.2）
//
//	@Summary	设置组织负责人
//	@Tags		org
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.SetOrgOwnersRequest	true	"设置负责人请求"
//	@Success	200		{object}	response.Response
//	@Router		/api/v1/orgs/owners [post]
func (h *OrgHandler) SetOwners(c *gin.Context) {
	var req model.SetOrgOwnersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	org, err := h.orgService.SetOwners(c.Request.Context(), &req, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, org)
}

// SetMemberRole POST /api/v1/orgs/members/role（2c，04 §3.3）
//
//	@Summary	任命/变更组内角色
//	@Tags		org
//	@Accept		json
//	@Produce	json
//	@Param		request	body	model.SetOrgMemberRoleRequest	true	"变更组内角色请求"
//	@Success	200		{object}	response.Response
//	@Router		/api/v1/orgs/members/role [post]
func (h *OrgHandler) SetMemberRole(c *gin.Context) {
	var req model.SetOrgMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.orgService.SetMemberRole(c.Request.Context(), &req, c.GetInt64("userID")); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}
