package handler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

// TicketAuthorizer ticket 态 purpose 的 L2 工单可见性判定（B5-3——A.7：
// ticket:update+L2 面）。实现方=ticket.Service（结构性满足，域间零 import——
// handler 为编排层，A.7「L2 resource.Authorize 调用点」先例）。
type TicketAuthorizer interface {
	AuthorizeTicketAttachment(ctx context.Context, userID, ticketID int64) error
	AuthorizeTicketFileRead(ctx context.Context, userID, ticketID int64) error
}

// StorageHandler 对象存储端点（phase5 S5，05 号 A.4）。
// 权限面：avatar/*_create=登录态自服务；ticket 态=ticket:update+L2（B5-3）。
type StorageHandler struct {
	svc        *storageservice.Service
	ticketAuth TicketAuthorizer
	auditLog   middleware.AuditLogger // A.4 #5 收窄：仅 ticket 态 200 投递落行
}

func NewStorageHandler(svc *storageservice.Service, ticketAuth TicketAuthorizer, auditLog middleware.AuditLogger) *StorageHandler {
	return &StorageHandler{svc: svc, ticketAuth: ticketAuth, auditLog: auditLog}
}

// authorizeTicketPurpose ticket 态 purpose 的 L2 编排（无 ticket_id 400；
// L2 失败透传 404/403/500 由 writeServiceError 分流）。
func (h *StorageHandler) authorizeTicketPurpose(c *gin.Context, purpose string, ticketID *int64) bool {
	if purpose != "ticket_attachment" && purpose != "ticket_richtext" {
		return true
	}
	if ticketID == nil {
		response.BadRequest(c, "参数错误")
		return false
	}
	if err := h.ticketAuth.AuthorizeTicketAttachment(c.Request.Context(), c.GetInt64("userID"), *ticketID); err != nil {
		writeServiceError(c, err)
		return false
	}
	return true
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
	if !h.authorizeTicketPurpose(c, in.Purpose, in.TicketID) {
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
	// confirm 分流按 TicketID：带 ticket_id 即 ticket 态落绑（建单前态不带）
	if in.TicketID != nil {
		if err := h.ticketAuth.AuthorizeTicketAttachment(c.Request.Context(), c.GetInt64("userID"), *in.TicketID); err != nil {
			writeServiceError(c, err)
			return
		}
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
	dl, err := h.svc.Download(c.Request.Context(), key, normalizeETag(c.GetHeader("If-None-Match")),
		func(ticketID int64) error {
			return h.ticketAuth.AuthorizeTicketFileRead(c.Request.Context(), c.GetInt64("userID"), ticketID)
		})
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
	if f.SizeBytes != nil {
		// 实测 size 在手——补 Content-Length（进度条/长度校验；S5 评审 P3）
		c.Header("Content-Length", strconv.FormatInt(*f.SizeBytes, 10))
	}
	c.Header("Content-Disposition", disposition+"; filename*=UTF-8''"+url.PathEscape(f.FileName))
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, dl.Body)
	// 审计（A.4 #5 收窄）：仅 ticket 态 200 全量投递落行——正向白名单
	//（avatar 免——热路径写放大；304/404 不落——防写放大；未来新 purpose
	// 默认不落不误伤——S5 评审 P2-5）；Insert 失败留痕不再静默吞
	if (f.Purpose == "ticket_attachment" || f.Purpose == "ticket_richtext") && h.auditLog != nil {
		if err := h.auditLog.Insert(c.Request.Context(), middleware.AuditLogEntry{
			UserID: c.GetInt64("userID"), Method: http.MethodGet,
			Path: c.Request.URL.Path, StatusCode: http.StatusOK,
			RequestID: c.GetString("request_id"), CreatedAt: time.Now(),
		}); err != nil {
			slog.Warn("storage proxy download audit insert failed",
				"key", key, "request_id", c.GetString("request_id"), "err", err)
		}
	}
}

// ListAttachments godoc
//
//	@Summary		工单附件列表（JOIN 视图）
//	@Description	L2 read 面（工单可见性——不可见单 404 同形）；条目含 object_key/file_name/mime/size
//	@Tags			storage
//	@Produce		json
//	@Param			id	path int true "工单 ID"
//	@Success		200	{object}	response.Response{data=[]model.TicketAttachmentView}
//	@Failure		404	{object}	response.Response
//	@Router			/api/v1/tickets/{id}/attachments [get]
func (h *StorageHandler) ListAttachments(c *gin.Context) {
	ticketID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || ticketID <= 0 {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.ticketAuth.AuthorizeTicketFileRead(c.Request.Context(), c.GetInt64("userID"), ticketID); err != nil {
		writeServiceError(c, err)
		return
	}
	list, err := h.svc.ListTicketAttachments(c.Request.Context(), ticketID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, list)
}

// DeleteAttachmentInput 删附件请求。
type DeleteAttachmentInput struct {
	TicketID  int64  `json:"ticket_id,string" binding:"required"`
	ObjectKey string `json:"object_key" binding:"required"`
}

// DeleteAttachment godoc
//
//	@Summary		删除工单附件（删关联+对象弃旧）
//	@Description	L2 update 面；删关联行+对象本体 best-effort 删+标 deleted（A.4 #4）
//	@Tags			storage
//	@Accept			json
//	@Produce		json
//	@Param			req	body DeleteAttachmentInput true "工单 ID+对象键"
//	@Success		200	{object}	response.Response
//	@Failure		404	{object}	response.Response	"91003 不存在/非绑定"
//	@Router			/api/v1/tickets/attachments/delete [post]
func (h *StorageHandler) DeleteAttachment(c *gin.Context) {
	var in DeleteAttachmentInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.ticketAuth.AuthorizeTicketAttachment(c.Request.Context(), c.GetInt64("userID"), in.TicketID); err != nil {
		writeServiceError(c, err)
		return
	}
	if err := h.svc.DeleteTicketAttachment(c.Request.Context(), in.TicketID, in.ObjectKey); err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// BindAvatar godoc
//
//	@Summary		头像回写（确认对象绑定为当前用户头像）
//	@Description	校验对象（confirmed+本人+avatar 面，非皆同形 404）→ users.avatar 落对象键 → 旧头像弃旧（对象 best-effort 删除+标 deleted）；同键重复幂等。前端渲染 src=/api/v1/files/{avatar}
//	@Tags			storage
//	@Accept			json
//	@Produce		json
//	@Param			req	body		model.ConfirmFileInput	true	"对象键"
//	@Success		200	{object}	response.Response{data=model.FileObject}
//	@Failure		404	{object}	response.Response	"91003（含非本人/pending/非 avatar 面防枚举同形）"
//	@Failure		500	{object}	response.Response
//	@Router			/api/v1/storage/avatar/bind [post]
func (h *StorageHandler) BindAvatar(c *gin.Context) {
	var in model.ConfirmFileInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	f, err := h.svc.BindAvatar(c.Request.Context(), in.ObjectKey, c.GetInt64("userID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, f)
}
