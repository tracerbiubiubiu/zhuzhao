# api/errcode.md — 业务错误码对外契约登记

> **定位（standards §3-4 对账位）**：错误码定义权威 = `internal/pkg/errcode/errcode.go`，
> HTTP 映射权威 = `internal/handler/errors.go`（表驱动 + 全码清单测试双锁）。本文件登记
> 各段对外契约语义（code/含义/HTTP），供前端与联调对表——段随启用批登记，不预登记。
> 全码清单测试（`errors_test.go`）锁定「定义↔映射↔登记」三处一致。

## 91000-91999 对象存储（phase5 S5 B5-1 启用；蓝本语义 = phase2/10-storage §10）

| code  | 常量                    | 含义             | HTTP | 备注                                       |
|-------|-------------------------|------------------|------|--------------------------------------------|
| 91001 | ErrFileTooLarge         | 文件大小超过限制 | 400  | presign 声明值预检 + confirm HEAD 实测双卡点 |
| 91002 | ErrFileTypeNotAllowed   | 文件类型不允许   | 400  | 显式枚举白名单（支持尾部 `.*` 通配）+ html/svg 黑名单硬底线 |
| 91003 | ErrFileNotFound         | 文件不存在       | 404  | 含防枚举归属遮蔽（非本人对象同形 404）       |
| 91004 | ErrFileAlreadyBound     | 文件已关联其它资源 | 409  | 重复 confirm / 已绑定（B5-3 关联表语义）    |
