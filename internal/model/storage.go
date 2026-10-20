package model

import "time"

// FileObject 对象元数据（000045，phase5 S5——对象本体在 RustFS，本表只存元数据）。
type FileObject struct {
	ID          int64      `json:"id"`
	Bucket      string     `json:"bucket"`
	ObjectKey   string     `json:"object_key"`
	Purpose     string     `json:"purpose"`
	Status      string     `json:"status"`
	FileName    string     `json:"file_name"`
	MIMEType    string     `json:"mime_type"`
	SizeBytes   *int64     `json:"size_bytes,string,omitempty"`
	ETag        *string    `json:"etag,omitempty"`
	CreatedBy   int64      `json:"created_by,string"`
	CreatedAt   time.Time  `json:"created_at"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
	// 通用归属引用（000047 随行件）：ticket 态 confirm/建单落绑填 ticket、avatar bind 填 user；
	// NULL = 未归属（pending / *_create 悬挂态 / 存量行）。元数据面，不作判定依据。
	OwnerKind *string `json:"owner_kind,omitempty"`
	OwnerID   *string `json:"owner_id,omitempty"`
}

// file_objects.owner_kind 取值（000047）；后续 script_package / run / ingest / export 按需扩。
const (
	FileOwnerTicket = "ticket"
	FileOwnerUser   = "user"
)

// file_objects.status 状态机（05 号 A.3 修正②：presign 即写 pending）。
const (
	FileStatusPending   = "pending"
	FileStatusConfirmed = "confirmed"
	FileStatusDeleted   = "deleted"
)

// PresignUploadInput 预签名上行声明（A.4 #1；ticket 态必带 ticket_id——B5-3）。
type PresignUploadInput struct {
	Purpose   string `json:"purpose" binding:"required"`
	FileName  string `json:"file_name" binding:"required"`
	MIMEType  string `json:"mime_type" binding:"required"`
	SizeBytes int64  `json:"size_bytes,string" binding:"required"`
	TicketID  *int64 `json:"ticket_id,string,omitempty"`
}

// PresignUploadResponse 预签名上行（Headers 须客户端直传原样携带——CT 已钉入签名）。
type PresignUploadResponse struct {
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ObjectKey string            `json:"object_key"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// ConfirmFileInput confirm 声明（ticket 态必带 ticket_id 落绑——B5-3）。
type ConfirmFileInput struct {
	ObjectKey string `json:"object_key" binding:"required"`
	TicketID  *int64 `json:"ticket_id,string,omitempty"`
}

// TicketAttachmentView 附件列表视图（关联行 JOIN file_objects——B5-3 批3）。
type TicketAttachmentView struct {
	ID         int64     `json:"id"`
	TicketID   int64     `json:"ticket_id,string"`
	ObjectKey  string    `json:"object_key"`
	FileName   string    `json:"file_name"`
	MIMEType   string    `json:"mime_type"`
	SizeBytes  *int64    `json:"size_bytes,string,omitempty"`
	UploadedBy int64     `json:"uploaded_by,string"`
	CreatedAt  time.Time `json:"created_at"`
}

// TicketAttachment 工单附件关联行（000046）。
type TicketAttachment struct {
	ID           int64     `json:"id"`
	TicketID     int64     `json:"ticket_id,string"`
	FileObjectID int64     `json:"file_object_id,string"`
	UploadedBy   int64     `json:"uploaded_by,string"`
	CreatedAt    time.Time `json:"created_at"`
}
