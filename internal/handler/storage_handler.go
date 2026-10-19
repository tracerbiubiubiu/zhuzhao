package handler

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

// StorageHandler 对象存储端点（phase5 S5 B5-1 底座，05 号 A.4）。
// B5-1 权限面 = 登录态自服务（avatar/*_create purpose——A.7）；
// ticket_attachment/ticket_richtext 的 ticket:update+L2 分流随 B5-3 扩。
type StorageHandler struct {
	svc *storageservice.Service
}

func NewStorageHandler(svc *storageservice.Service) *StorageHandler {
	return &StorageHandler{svc: svc}
}

// PresignUpload godoc
//
//	@Summary		对象预签名上传（返回直传 URL——Content-Type 已钉入签名）
//	@Description	限制矩阵预检（MIME 白名单+黑名单/大小）→ 服务端生成对象键 → 签发 PUT 直传 URL。purpose 取 avatar / attachment_create / richtext_create（ticket 态随附件批）
//	@Tags			storage
//	@Accept			json
//	@Produce		json
//	@Param			req	body		model.PresignUploadInput	true	"上传声明"
//	@Success		200	{object}	response.Response{data=model.PresignUploadResponse}
//	@Failure		400	{object}	response.Response	"91001 超限 / 91002 类型不允许 / 10003 参数错误"
//	@Failure		500	{object}	response.Response
//	@Router			/api/v1/storage/presign/upload [post]
func (h *StorageHandler) PresignUpload(c *gin.Context) {
	var in model.PresignUploadInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	resp, err := h.svc.PresignUpload(c.Request.Context(), &in, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, resp)
}

// Confirm godoc
//
//	@Summary		确认上行对象（HEAD 实测→落 confirmed+etag 快照）
//	@Description	校验归属（非本人同形 404 防枚举）与 pending 态 → 对象存储 HEAD 实测（size/类型）→ 翻 confirmed。重复 confirm 409
//	@Tags			storage
//	@Accept			json
//	@Produce		json
//	@Param			req	body		model.ConfirmFileInput	true	"对象键"
//	@Success		200	{object}	response.Response{data=model.FileObject}
//	@Failure		404	{object}	response.Response	"91003 不存在（含非本人防枚举同形）"
//	@Failure		409	{object}	response.Response	"91004 已确认/已关联"
//	@Failure		500	{object}	response.Response
//	@Router			/api/v1/tickets/attachments/confirm [post]
func (h *StorageHandler) Confirm(c *gin.Context) {
	var in model.ConfirmFileInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	f, err := h.svc.Confirm(c.Request.Context(), &in, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, f)
}

// normalizeETag 归一化客户端 If-None-Match（剥 W/ 前缀与引号——与库内快照裸值比对）。
func normalizeETag(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "W/")
	return strings.Trim(v, `"`)
}

// ProxyDownload godoc
//
//	@Summary		文件代理下载（稳定 URL——头像等对象经应用投递）
//	@Description	定位元数据→状态/purpose 面校验→If-None-Match 命中快照 304→条件读流（etag 作 If-Match，被重放替换即 404）。avatar 免审计+Cache-Control private（A.4 #5 收窄）；ticket 态 L2 与审计行随 B5-3
//	@Tags			storage
//	@Produce		octet-stream
//	@Param			key	path string true "对象键（{domain}/{yyyy}/{mm}/{uuid}.{ext}）"
//	@Success		200	{file}	file "对象流（ETag/nosniff/private/分型 disposition）"
//	@Failure		304	"命中 If-None-Match（零传输零审计行）"
//	@Failure		404	{object} response.Response "91003（含 pending/非支持面/替换嫌疑同形）"
//	@Router			/api/v1/files/{key} [get]
func (h *StorageHandler) ProxyDownload(c *gin.Context) {
	key := strings.TrimPrefix(c.Param("key"), "/")
	if key == "" || strings.Contains(key, "..") {
		writeServiceError(c, errcode.ErrFileNotFound)
		return
	}
	dl, err := h.svc.Download(c.Request.Context(), key, normalizeETag(c.GetHeader("If-None-Match")))
	if errors.Is(err, storageservice.ErrNotModified) {
		c.Status(http.StatusNotModified)
		return
	}
	if err != nil {
		writeServiceError(c, err)
		return
	}
	defer dl.Body.Close()
	f := dl.File

	disposition := "attachment"
	if h.svc.InlineDisposition(f.MIMEType) {
		disposition = "inline"
	}
	c.Header("Content-Type", f.MIMEType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private")
	if f.ETag != nil {
		c.Header("ETag", `"`+*f.ETag+`"`)
	}
	c.Header("Content-Disposition", disposition+"; filename*=UTF-8''"+url.PathEscape(f.FileName))
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, dl.Body)
	// 审计（A.4 #5 收窄）：avatar 免审计；ticket_attachment/ticket_richtext
	// 的 handler 级审计行（谁/何时/key/结果，仅 200）随 B5-3 接入
}
