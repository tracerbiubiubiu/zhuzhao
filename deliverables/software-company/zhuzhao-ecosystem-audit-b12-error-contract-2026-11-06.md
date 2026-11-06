# 生态全面审计 B12：错误码登记完整性与守卫（2026-11-06）

## 范围与基线

本批由root占号，确认1项P1与同根P2守卫缺口；修复已提交，专项通过，完整联合门禁仍待验证。原缺陷以主仓 `7389ed79bdf409e466542992d9f1befa0c1a8529` 冻结源实核；`73a76b09d998ff9df6930508031793829f255fdc` 为L6归一提交。独立修复 `4da89fad82b70d85bdda0830aca9e79ec5ce42da`（parent738），分支 `codex/fix-error-contract-registry`，工作树 `/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-fix/error-contract`。仅 `api/errcode.md` 与 `internal/handler/errors_test.go` 两文件，无生产码/HTTP/依赖/迁移变化，未push/合main。

跨仓只读核对AL `19a4c1d5cd84cc892b82ad52554db320bfad8f7f`、taskrunner `5e3f5f3720a54fad9f5cec4a08481364693b8f7d` / port修复 `eb3c0c07615c9646da9a9ada673de16843c75564`，utils正式v0.4.3定义。没有新增环境/部署，B11自有资源已清。

## 发现与分级

**ECO-20261104-B12-F01 / P1：L6内容完整性销账失真。** L6将旧 `docs/api/errcode.md` 303行改为指针后，根登记只有57行/21个当前码，而真实主仓77定义中56项缺失，含通用/认证/用户/角色/组织/菜单/权限/工单与PAT。AL的100000–100008九项契约亦不在新根登记，而AL文档仍引旧§4。旧表共91表码，当前根比旧缺70项，但其中含退役/预留，不能直接把旧表整份恢复。原证据：`error-config-review/registry-gap-before.json`、`L6-content-comparison.json` 及保留旧源全文。该发现不意味着生产返回错误码被L6修改。

**ECO-20261104-B12-F02 / P2：单向文案门禁不验证完整性。** 原 `TestErrcodeDocMessageSync` 从文档已有行出发，匹配到至少一行即接受；原“全码计数”只锁测试清单自身77，不能发现新增定义未入清单。固定738全码HTTP/非业务错误/文案测试各八轮绿，而删去活跃91001行后文案测试仍8/8绿。`doc-row-mutant-eight.json` 保留命令/终态与完整日志。P2与P1合根处理，不重复统计为第二个数据损坏问题。

另有 **P2旧稿口径**：40003 `ErrRoleInUse` 运行时403，旧modules/role与RBAC设计稿409。B12登记按当前403，不为迁就旧稿改生产行为；该项归既有L5/旧稿校准，不扩源码修复。

## 修复与挂点

- `api/errcode.md` 从当前Owning Source补齐77定义，逐字保留默认文案，按真实所属出口核HTTP。20005/70002只保留定义、没有当前生产发出点，明确其默认500+10000降级，未凭旧303行捏造现役401等出口。92303等未定义预留码不进有效码表。
- AL跨仓段列明有效九码并指向固定apperr与handler源码，不假定一码唯一HTTP：VALIDATION_ERROR实际有400/413/422。TR101xxx段仍未启用，明确当前通用码映射及固定handler权威。未声称主仓单测能自动验证兄弟仓契约。
- `errors_test.go` 通过Go AST核对实际导出Err定义与编译后的测试清单，再双向核登记、默认文案、实际HTTP出口；缺定义/漏清单/漏行/错文案/错HTTP/幽灵登记均不能靠“尚有一行”通过。auth和中间件直写与service默认路径分开核。
- L6仅结构归一部分已完成，完整性原判已更正。17债独立manifest状态现为闭4/开4/未来9，L6在root联合最终门禁前保留分支修复/待承接，不直接改主设计代理共享索引。

## 验证证据与边界

候选：make lint0、make test-unit0、错误码三项专项-count8全绿。四类变异（删20001行、50012错文案、40003错HTTP、漏ErrPasswordChanged测试值清单）各8个独立串行go test进程稳定红，失败原因匹配断言且排除编译错误。

固定4da提交重新git archive后：lint0/5.24s、全unit0/12.10s、专项-count8 0/3.71s，697原文件逐字等价true。原失败/假绿轮保留。证据根 `.codex/audits/ECO-20261104/evidence/ECO-20261104-b07/error-config-review/`：`fixed-results.json`、`contract-mutations.json`、逐轮日志、`candidate-results.json`。

本批纯文档/测试，完整integration/coverage/acceptance尚待root按下一联合冻结执行，不把本批局部门禁当完整交付。没有新增生产接口、授权或资源配置。

配置同类核对：主仓5c→738、ALaf0→19a、TR1dcc→eb3的configs/config.yaml均逐字相同，`frozen-config-comparison.json`记录三组SHA；TR默认18080的已知F11修复在env-only代码，未改冻结yaml。该静态事实不替代每个生产拓扑的动态验收。

## 可点击锚点与主审独立复验

原738锚点：[api/errcode.md 的主仓登记](https://github.com/tracerbiubiubiu/zhuzhao/blob/7389ed79bdf409e466542992d9f1befa0c1a8529/api/errcode.md#L1)、[errcode.go 的实际Err变量](https://github.com/tracerbiubiubiu/zhuzhao/blob/7389ed79bdf409e466542992d9f1befa0c1a8529/internal/pkg/errcode/errcode.go#L1)、[errors_test.go 的 TestErrcodeDocMessageSync / TestWriteServiceError_FullCodeTable](https://github.com/tracerbiubiubiu/zhuzhao/blob/7389ed79bdf409e466542992d9f1befa0c1a8529/internal/handler/errors_test.go#L155)。

固定修复4da89fa挂点：[api/errcode.md 当前定义/跨仓边界](/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-fix/error-contract/api/errcode.md:1)、[errors_test.go 的 errcodeNames / readErrcodeRegistry / TestErrcodeDocMessageSync](/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-fix/error-contract/internal/handler/errors_test.go:157)。源码符号为主，行号仅辅助。

root另以固定4da89fa的detached checkout独立执行三项错误码测试 `-race -count=8`，6.367秒exit0、工作树干净，见同目录root-fixed-results.json及root-fixed-three-eight.log。原始日志与变异源码仅本地.codex，不入Git/Docker。没有新增真实环境或账号。

仍未执行此修复组合的完整integration/coverage/acceptance及远端CI，不标完整关单，不把主仓守卫当成跨仓自动契约测试，不修改生产HTTP行为去迁就旧稿。控制台同步修复与承接边界，全面审计继续。
