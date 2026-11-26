# Phase 6 第 11 项参考应用实施与验收

> 范围：[总排期第 11 项](intranet-uaa-release-schedule.md#剩余开发顺序与验收)和 [B1 交付边界](../design/foundation-release-boundaries.md#1-先交哪一版)。基线 Core `9d0019b7`。只用合成数据和独占测试资源。

## 实施顺序

1. 固定远端 main，核对平台和五仓可用入口。复跑已有风险资产查询客户端测试。本轮所需的组织范围列表、参考应用和旧消费者没有可复用制品，先冻结首次可运行的 V1 组合。
2. 分开实现 V1 查询合同、独立参考应用和模拟来源。核心重用本地会话、当前有效角色和有效组织成员关系。启动配置只启用固定参考入口，限定当前角色、应用目的地及两条独立服务身份；核心向应用发短时不透明引用，应用回调复核，再按获权范围读取模拟来源。应用和模拟来源均检查行与字段。
3. 在专属数据库、Redis 和端口上测试逐请求与翻页的撤权、停用、无权限、来源失败、伪身份、替换地址和旧制品。先保存首套可运行组合的二进制与摘要，再用未改的应用对后继核心实测兼容。
4. 主会话持有 Core 接线与状态文档，由不同助手复核固定候选和运行证据。执行候选适用的普通档门禁与专项验收，将正式结论登记到项目控制台。

本轮仅交付合成只读参考路径。第 12–14 项使用各自的工作树、运行资源和交付范围。

## 运行配置和读取权限

从部署工作目录读取专用 `configs/config.yaml`。增加 `reference_assets` 对象，指定 `enabled`、`extension_id: reference.assets`、`profile: synthetic.asset-directory.v1`、`config_revision`、`source_id`、`app_origin`、`outbound_ak`、`outbound_key_file`、`callback_ak`、`callback_key_file`、`reader_roles` 和 `timeout`。出站与回调使用不同身份和绝对密钥文件引用。合成应用地址使用本机回环；来源由应用自己的配置指定。替换应用或停用时用新配置重启 Core。

只读页位于 `/extensions/reference-assets`，以本地会话查询固定 GET `/api/v1/extensions/reference-assets`。该合成 profile 使用 `reader_roles` 显式登记读取资格，与 Core 的 `menu_apis` 分别归属。启动时发现重复登记会拒启。配置列出的角色须是调用人的当前有效角色。数据范围取有效直接组织与所选过滤的交集。应用通过短期授权引用逐页向 Core 回调复核。配置替换、会话或组织范围改变会阻止原游标继续读取。

用 `go build` 分别构建 `./cmd/reference-assets-app` 与 `./cmd/reference-assets-source`，用各自的 `-h` 查看监听、上游和密钥文件参数。专项通过 `scripts/acceptance-reference-assets.py` 对运行组合调用 HTTP。命令行冻结消费者使用应用制品的 `-mode client`，其 Bearer 只发送到 Core。源故障在正常配置下返回查询失败。默认 Core 不启用参考入口。
