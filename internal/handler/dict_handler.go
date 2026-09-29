package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// DictHandler P4-3 字典管理面（dict:read 页面 / dict:manage 写按钮——000034 词表）。
// 消费端点 GET /dicts/:code/items 挂页面行（登录可读——业务表单选项场景）。
type DictHandler struct {
	dictService *service.DictService
}

func NewDictHandler(s *service.DictService) *DictHandler {
	return &DictHandler{dictService: s}
}

func pageOf(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	return page, pageSize
}

// ListTypes GET /dicts
func (h *DictHandler) ListTypes(c *gin.Context) {
	page, pageSize := pageOf(c)
	resp, err := h.dictService.ListTypes(c.Request.Context(), page, pageSize, c.Query("keyword"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, resp)
}

// CreateType POST /dicts
func (h *DictHandler) CreateType(c *gin.Context) {
	var req struct {
		Code   string `json:"code" binding:"required"`
		Name   string `json:"name" binding:"required"`
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "code/name 必填")
		return
	}
	t := &model.DictType{Code: req.Code, Name: req.Name, Enabled: true, Remark: req.Remark}
	if err := h.dictService.CreateType(c.Request.Context(), t); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, t)
}

// UpdateType POST /dicts/update（version 乐观锁；enabled=启停）
func (h *DictHandler) UpdateType(c *gin.Context) {
	var req struct {
		ID      int64  `json:"id,string" binding:"required"`
		Name    string `json:"name" binding:"required"`
		Enabled *bool  `json:"enabled"`
		Remark  string `json:"remark"`
		Version int64  `json:"version" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "id/name/version 必填")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	t := &model.DictType{ID: req.ID, Name: req.Name, Enabled: enabled, Remark: req.Remark, Version: req.Version}
	if err := h.dictService.UpdateType(c.Request.Context(), t); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OKWithMessage(c, "已更新", nil)
}

// DeleteType POST /dicts/delete（code 入 body——zhuzhao 风格；items 级联删）
func (h *DictHandler) DeleteType(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "code 必填")
		return
	}
	if err := h.dictService.DeleteType(c.Request.Context(), req.Code); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OKWithMessage(c, "已删除", nil)
}

// ListItems GET /dict-items?type_code=（管理面——含停用项）
func (h *DictHandler) ListItems(c *gin.Context) {
	typeCode := c.Query("type_code")
	if typeCode == "" {
		response.BadRequest(c, "type_code 必填")
		return
	}
	page, pageSize := pageOf(c)
	resp, err := h.dictService.ListItems(c.Request.Context(), page, pageSize, typeCode)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, resp)
}

// EnabledItems GET /dicts/:code/items（消费面——仅启用项，type 停用返回空）
func (h *DictHandler) EnabledItems(c *gin.Context) {
	items, err := h.dictService.EnabledItemsByCode(c.Request.Context(), c.Param("code"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"items": items})
}

// CreateItem POST /dict-items
func (h *DictHandler) CreateItem(c *gin.Context) {
	var req struct {
		TypeCode  string `json:"type_code" binding:"required"`
		Code      string `json:"code" binding:"required"`
		Label     string `json:"label" binding:"required"`
		SortOrder int    `json:"sort_order"`
		Enabled   *bool  `json:"enabled"` // 缺省 true（建时即停用场景显式传 false）
		Remark    string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "type_code/code/label 必填")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	it := &model.DictItem{TypeCode: req.TypeCode, Code: req.Code, Label: req.Label, SortOrder: req.SortOrder, Enabled: enabled, Remark: req.Remark}
	if err := h.dictService.CreateItem(c.Request.Context(), it); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, it)
}

// UpdateItem POST /dict-items/update
func (h *DictHandler) UpdateItem(c *gin.Context) {
	var req struct {
		ID        int64  `json:"id,string" binding:"required"`
		Label     string `json:"label" binding:"required"`
		SortOrder int    `json:"sort_order"`
		Enabled   *bool  `json:"enabled"`
		Remark    string `json:"remark"`
		Version   int64  `json:"version" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "id/label/version 必填")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	it := &model.DictItem{ID: req.ID, Label: req.Label, SortOrder: req.SortOrder, Enabled: enabled, Remark: req.Remark, Version: req.Version}
	if err := h.dictService.UpdateItem(c.Request.Context(), it); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OKWithMessage(c, "已更新", nil)
}

// DeleteItem POST /dict-items/delete（id 入 body）
func (h *DictHandler) DeleteItem(c *gin.Context) {
	var req struct {
		ID int64 `json:"id,string" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "id 必填")
		return
	}
	if err := h.dictService.DeleteItem(c.Request.Context(), req.ID); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OKWithMessage(c, "已删除", nil)
}
