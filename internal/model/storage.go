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
}

// file_objects.status 状态机（05 号 A.3 修正②：presign 即写 pending）。
const (
	FileStatusPending   = "pending"
	FileStatusConfirmed = "confirmed"
	FileStatusDeleted   = "deleted"
)

// PresignUploadInput 预签名上行声明（A.4 #1；ticket 态的 ticket_id 字段随 B5-3 扩）。
type PresignUploadInput struct {
	Purpose   string `json:"purpose"`
	FileName  string `json:"file_name"`
	MIMEType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes,string"`
}

// PresignUploadResponse 预签名上行（Headers 须客户端直传原样携带——CT 已钉入签名）。
type PresignUploadResponse struct {
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ObjectKey string            `json:"object_key"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// ConfirmFileInput confirm 声明（B5-1 底座态；ticket 关联落绑参数随 B5-3 扩）。
type ConfirmFileInput struct {
	ObjectKey string `json:"object_key"`
}
