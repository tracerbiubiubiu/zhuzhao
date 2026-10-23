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

## 92300-92399 字典（P4-3 启用；92304 随 phase5 B1-2 删除守卫增补——本段 2026-10-22 补登记）

| code  | 常量                    | 含义                     | HTTP | 备注                                              |
|-------|-------------------------|--------------------------|------|---------------------------------------------------|
| 92301 | ErrDictTypeNotFound     | 字典类型不存在或版本冲突 | 404  | 含版本冲突（乐观锁）同形                           |
| 92302 | ErrDictItemNotFound     | 字典项不存在或版本冲突   | 404  | 同上                                               |
| 92303 | （预留未分配——01 号 A.6 原登记已删注，号位保持空洞防误占） | — | — | — | — |
| 92304 | ErrDictTypeInUse        | 字典类型被字段引用中，不可删除 | 400 | B1-2 删除守卫：ticket_type_fields 引用 `dict:<code>` 时拒删；清引用后可删 |
| 92004 | ErrNotificationNotConfigured   | 通知渠道未配置（生产态 fail-fast） | 500 | 06 号 §3 两态桩防线——S6 B6-2 |

## 92500-92599 消息模板段（S6 B6-3——外审 F-1-4 修复：从 92000 通知段迁移避免撞码）
| 码 | 名称 | 含义 | HTTP | 备注 |
|---|---|---|---|---|
| 92501 | ErrMsgTplNotFound | 消息模板不存在 | 404 | |
| 92502 | ErrMsgTplDupCode | 模板 code 已存在 | 409 | |
| 92503 | ErrMsgTplSystem | 系统模板不可修改或删除 | 403 | 种子模板只读（is_system=true） |
| 92504 | ErrMsgTplReferenced | 模板被订阅规则引用中 | 409 | 须先解引 |
| 92505 | ErrMsgTplVersionConflict | 模板版本冲突 | 409 | CAS 乐观锁 |
| 92506 | ErrMsgRuleVersionConflict | 订阅规则版本冲突 | 409 | CAS 乐观锁（000060——微批 A） |
