package handler

import (
	"encoding/json"
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

// DeleteType POST /dicts/delete（code 入 body；可选 id/version 必须成对；items 级联删）
func (h *DictHandler) DeleteType(c *gin.Context) {
	var req struct {
		Code    string          `json:"code" binding:"required,max=64"`
		ID      json.RawMessage `json:"id"`
		Version json.RawMessage `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "code 必填且最多 64 字符")
		return
	}
	var target []service.DictTypeDeleteTarget
	if len(req.ID) != 0 || len(req.Version) != 0 {
		id, idErr := parseDictDeleteID(req.ID)
		version, versionErr := parseDictDeleteVersion(req.Version)
		if idErr != nil || versionErr != nil {
			response.BadRequest(c, "id/version 必须成对，id 为正整数字符串，version 为正整数")
			return
		}
		target = append(target, service.DictTypeDeleteTarget{ID: id, Version: version})
	}
	if err := h.dictService.DeleteType(c.Request.Context(), req.Code, target...); err != nil {
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

// EnabledItems GET /dicts/:code/items（消费面——默认仅启用项，type 停用返回空；
// include_disabled=true 返回含停用项——存量工单回显名称/编辑回显旧值用，RR-HY-02）
func (h *DictHandler) EnabledItems(c *gin.Context) {
	items, err := h.dictService.ItemsForConsumer(c.Request.Context(), c.Param("code"), c.Query("include_disabled") == "true")
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

// DeleteItem POST /dict-items/delete（id 入 body；可选 version 做条件删除）
func (h *DictHandler) DeleteItem(c *gin.Context) {
	var req struct {
		ID      json.RawMessage `json:"id"`
		Version json.RawMessage `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "id 必须为正整数字符串")
		return
	}
	id, err := parseDictDeleteID(req.ID)
	if err != nil {
		response.BadRequest(c, "id 必须为正整数字符串")
		return
	}
	var version []int64
	if len(req.Version) != 0 {
		v, err := parseDictDeleteVersion(req.Version)
		if err != nil {
			response.BadRequest(c, "version 必须为正整数")
			return
		}
		version = append(version, v)
	}
	if err := h.dictService.DeleteItem(c.Request.Context(), id, version...); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OKWithMessage(c, "已删除", nil)
}

func parseDictDeleteID(raw json.RawMessage) (int64, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, err
	}
	if len(value) > 19 {
		return 0, strconv.ErrRange
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, err
	}
	if id <= 0 {
		return 0, strconv.ErrSyntax
	}
	return id, nil
}

func parseDictDeleteVersion(raw json.RawMessage) (int64, error) {
	var version int64
	if err := json.Unmarshal(raw, &version); err != nil {
		return 0, err
	}
	if version <= 0 {
		return 0, strconv.ErrSyntax
	}
	return version, nil
}
