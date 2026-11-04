# zhuzhao 全面审计 R01A：utils v0.4.3 发布与正式依赖验证

日期：2026-11-04。执行：Codex 当前会话。性质：R01 已完成库侧工作的独立归档，非新的全量审计窗口。

**库发布已完成，父发现 ECO-20261104-B02-F01/P0 仍开放。主仓鉴权修复候选未提交、未部署。**

## 范围、授权与提交

所有者明确回复「授权发布 utils v0.4.3」，范围为修复合入 main、推送、发布该版本；不含主仓推送/部署。推送前 SSH fetch 核对 main 为 `5a3900fa6013750e912eca958666831f0a6f2c9d`、v0.4.3 不存在、工作区及索引无他人改动。

新增窗口两笔（修复分支快进 main，不改写历史）：

- `f6dbe69c668ee16a3c60f552539c03e52134a1eb` / `fix(jwt): 为访问令牌增加密码纪元绑定`：仅 jwt 实现、回归测试及 README 三文件。
- `9296ae170b71b06c623c54b6010a107406903346` / `docs(release): 更新 v0.4.3 版本与纪元接入口径`：仅 README，退役候选/待发标记。

远端 main 与 v0.4.3 解引用均实核指向 `9296ae170b71b06c623c54b6010a107406903346`。[GitHub Release](https://github.com/tracerbiubiubiu/zhuzhao-utils/releases/tag/v0.4.3) 已发布，非 draft、非 prerelease；精确发布信息、tag 对象与 CI 回执见 [发布核验](evidence/ECO-20261104-r01a/utils-release-verification.json)。本次没有推送主仓或变更常驻应用。

v0.4.3 同时包含 B01 已登记的 v0.4.2 后四笔清欠变更，未把当前库 HEAD 冒充旧消费版本。错误文案新增包名前缀的消费影响已在版本说明披露，`errors.Is` 保持可用。

## 能力与验证

`jwt.AccessClaims.Pwe` 与 `GenerateAccessTokenWithEpoch` 已发布；旧签发入口保持纪元 0。工具库不访问业务会话存储，消费方仍须读取/递增权威纪元并在请求时比较；仅升级库不等于修复应用撤权链。

- 本地 `make lint` 与全量 race 测试已过；修复提交另从全新 git archive、`GOWORK=off` 独立全量 race 复验通过，见 [回执](evidence/ECO-20261104-r01a/utils-postcommit-independent.json)/[日志](evidence/ECO-20261104-r01a/utils-postcommit-independent.log)。两笔提交格式检查通过。
- 实际发布提交的 [main CI](https://github.com/tracerbiubiubiu/zhuzhao-utils/actions/runs/41800001957) 与 [tag CI](https://github.com/tracerbiubiubiu/zhuzhao-utils/actions/runs/41800452762) 均 success，含 vet/gofmt/test-race。
- 主仓**独立修复工作树**已通过正式 `go get ...@v0.4.3` 升级，仅改变一行 go.mod 与新增两行 go.sum；未改根工作树或另两消费者的 pin。`GOWORK=off`、无 Replace，模块缓存的 31 个 Go 源文件与发布提交逐件一致，见 [模块解析/源对拍](evidence/ECO-20261104-r01a/formal-module-resolution.json)。
- 正式 pin 下 `make lint`、全量 `go test -race -count=1 -timeout=120s ./internal/...` 均 exit 0，见 [lint](evidence/ECO-20261104-r01a/formal-pin-lint.json)、[单元测试](evidence/ECO-20261104-r01a/formal-pin-unit.json)。
- 首次通过 goproxy.cn 验证新版本时，其 sumdb 镜像返回 temporarily unavailable 404；完整失败记录保留。切换官方 `proxy.golang.org` 后成功，未关闭 sumdb 校验，非本地伪造版本或工作区替换；两次回执分别在证据目录。

本批证据：[SHA-256 清单](evidence/ECO-20261104-r01a/manifest.json)。未完成项不混入提交：父 R01 主仓源码、候选报告/证据仍本地保留；前轮完整集成、四档与运行态验证使用的是外部 go.work 候选，不能冒充正式 pin 的整套重验。

## 剩余出口

先按 [B05](zhuzhao-ecosystem-audit-b05-acceptance-isolation-2026-11-04.md) 修复验收连接目标与临时文件隔离，再对主仓正式 pin 跑完整集成及原样完整 acceptance，路径提交并独立复验后才关闭 P0。整体审计仍处阶段 1，阶段 2 仅认证定向初核；工单、兄弟仓、前端功能/视觉/使用习惯等全面审计尚待后续批次。
