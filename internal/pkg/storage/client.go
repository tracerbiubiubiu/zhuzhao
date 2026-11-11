// Package storage 提供 S3 兼容对象存储的端口抽象（phase5 S5，05 号 A.1）。
// 端口面刻意收窄为四方法：presign GET 仅 B5-5 备份与 302 升级用，业务链路
// 不出现预签名下载（代理下载已拍）。实现为 minio-go v7（对 RustFS/云 OSS
// 同构，换后端=配置级切换——RK-P5-13 保底；B5-1 spike 六判据 2026-10-19 预验全绿）。
//
// 本包属 pkg 横切层：禁止 import service/repository/handler/app/gateway
// （架构守护 TestArchitecture_PkgNoBusinessDependency 在案）。
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrObjectNotFound 对象不存在（confirm 判定与 GC 的哨兵；实现层
// NoSuchKey 映射至此）。
var ErrObjectNotFound = errors.New("storage: object not found")

// ErrObjectReplaced 对象已被替换（存储侧 If-Match 412——presign TTL 内
// 重放覆盖已 confirm 对象的防重放检测，拍板 1b；代理侧映射 404+告警）。
var ErrObjectReplaced = errors.New("storage: object etag mismatch (replaced)")

// PresignedPut 预签名上行。Headers 为客户端直传必须原样携带的请求头
// （Content-Type 已钉入签名——篡改即 403 SignatureDoesNotMatch，spike 判据④实证）。
type PresignedPut struct {
	URL       string
	ExpiresAt time.Time
	Headers   map[string]string
}

// ObjectInfo 对象元数据——confirm HEAD 实测与 file_objects.etag 列快照的
// 数据源（05 号 A.3 修正③/A.4 #2，2026-10-19 拍板 1b）。
type ObjectInfo struct {
	Key         string
	Size        int64
	ETag        string
	ContentType string
}

// Client S3 兼容客户端端口（05 号 A.1；B5-2 扩至 5 方法——代理投递需读流，
// 原四方法收窄对代理形态为设计缺口，随批登记 A.1）。
type Client interface {
	// PresignPut 生成钉死 Content-Type 的预签名上传 URL（Content-Type 入签）。
	PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (*PresignedPut, error)
	// PresignGet 生成短 TTL 预签名下载 URL（B5-5 备份与 302 升级路径专用）。
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	// StatObject 对象元数据（含 ETag）。
	StatObject(ctx context.Context, key string) (*ObjectInfo, error)
	// GetObject 读取对象流：matchETag 非空时作 S3 If-Match 条件读
	//（不匹配→ErrObjectReplaced——防重放替换检测，拍板 1b）。
	GetObject(ctx context.Context, key, matchETag string) (io.ReadCloser, *ObjectInfo, error)
	// RemoveObject 删除对象（GC 链路用）。
	RemoveObject(ctx context.Context, key string) error
}
