package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinioClient minio-go v7 实现（B5-1 spike 判据②③④实测兼容面全绿 2026-10-19：
// PresignHeader 钉 Content-Type/StatObject 含 etag/未签名 403 fail-closed）。
// 桶初始化随首次 presign 幂等确保（ensureBucket 内联，不占端口四方法面——
// 05 号 A.1 刻意收窄；MakeBucket 撞并发已存在视为成功）。
type MinioClient struct {
	cli    *minio.Client
	bucket string
	region string
}

var _ Client = (*MinioClient)(nil)

// NewMinioClient 构造（不建连——minio 客户端懒连接，首请求才触达存储端）。
func NewMinioClient(cfg *StorageConfig) (*MinioClient, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, fmt.Errorf("storage: 未启用（enabled=false）")
	}
	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: init minio client: %w", err)
	}
	return &MinioClient{cli: cli, bucket: cfg.Bucket, region: cfg.Region}, nil
}

func (m *MinioClient) ensureBucket(ctx context.Context) error {
	exists, err := m.cli.BucketExists(ctx, m.bucket)
	if err != nil {
		return fmt.Errorf("storage: bucket exists: %w", err)
	}
	if exists {
		return nil
	}
	if err := m.cli.MakeBucket(ctx, m.bucket, minio.MakeBucketOptions{Region: m.region}); err != nil {
		if exists2, err2 := m.cli.BucketExists(ctx, m.bucket); err2 == nil && exists2 {
			return nil
		}
		return fmt.Errorf("storage: make bucket: %w", err)
	}
	return nil
}

// PresignPut 生成钉死 Content-Type 的预签名 PUT URL。
func (m *MinioClient) PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (*PresignedPut, error) {
	if err := m.ensureBucket(ctx); err != nil {
		return nil, err
	}
	u, err := m.cli.PresignHeader(ctx, http.MethodPut, m.bucket, key, ttl, nil,
		http.Header{"Content-Type": []string{contentType}})
	if err != nil {
		return nil, fmt.Errorf("storage: presign put: %w", err)
	}
	return &PresignedPut{
		URL:       u.String(),
		ExpiresAt: time.Now().Add(ttl),
		Headers:   map[string]string{"Content-Type": contentType},
	}, nil
}

// PresignGet 生成短 TTL 预签名 GET URL。
func (m *MinioClient) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := m.cli.PresignedGetObject(ctx, m.bucket, key, ttl, nil)
	if err != nil {
		return "", fmt.Errorf("storage: presign get: %w", err)
	}
	return u.String(), nil
}

// StatObject 对象元数据（NoSuchKey → ErrObjectNotFound 哨兵）。
func (m *MinioClient) StatObject(ctx context.Context, key string) (*ObjectInfo, error) {
	st, err := m.cli.StatObject(ctx, m.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, ErrObjectNotFound
		}
		return nil, fmt.Errorf("storage: stat object: %w", err)
	}
	return &ObjectInfo{
		Key:         key,
		Size:        st.Size,
		ETag:        strings.Trim(st.ETag, `"`),
		ContentType: st.ContentType,
	}, nil
}

// RemoveObject 删除对象（GC 链路）。
func (m *MinioClient) RemoveObject(ctx context.Context, key string) error {
	if err := m.cli.RemoveObject(ctx, m.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("storage: remove object: %w", err)
	}
	return nil
}

// GetObject 读取对象流（代理投递段）。matchETag 非空时 SetMatchETag 条件读——
// 对象被重放替换（etag 漂移）时存储侧 412 → ErrObjectReplaced（拍板 1b 防重放）。
func (m *MinioClient) GetObject(ctx context.Context, key, matchETag string) (io.ReadCloser, *ObjectInfo, error) {
	opts := minio.GetObjectOptions{}
	if matchETag != "" {
		if err := opts.SetMatchETag(matchETag); err != nil {
			return nil, nil, fmt.Errorf("storage: set match etag: %w", err)
		}
	}
	obj, err := m.cli.GetObject(ctx, m.bucket, key, opts)
	if err != nil {
		if minio.ToErrorResponse(err).Code == "PreconditionFailed" {
			return nil, nil, ErrObjectReplaced
		}
		return nil, nil, fmt.Errorf("storage: get object: %w", err)
	}
	st, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		if minio.ToErrorResponse(err).Code == "PreconditionFailed" {
			return nil, nil, ErrObjectReplaced
		}
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, nil, ErrObjectNotFound
		}
		return nil, nil, fmt.Errorf("storage: stat object: %w", err)
	}
	return obj, &ObjectInfo{Key: key, Size: st.Size, ETag: strings.Trim(st.ETag, `"`), ContentType: st.ContentType}, nil
}
