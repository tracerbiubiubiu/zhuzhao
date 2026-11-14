# 能力互通合同的只读参考验证

> 日期：2026-11-13。范围：选定三链Schema、规范输入、严格JSON与有限语义oracle；状态：设计参考模型，不是产品runtime、真实权限或三链集成验收。合同权威见[互通详设](../../capability-interconnect-detail.md)，样本在[候选Schema](../../capability-interconnect-detail.schema.json)。

## 复跑

从主仓根目录执行，使用既有Node/Python和兄弟UI已安装的AJV6.15.0；不安装依赖、不连接数据库或网络、不读取凭据，不写Schema/fixtures或报告文件。JSON报告仅写stdout，调用者可自行重定向到ignored审计目录。

```bash
node docs/design/examples/capability-reference/verify.cjs
```

默认按仓库相对位置发现`../zhuzhao-ui/node_modules/.pnpm/ajv@6.15.0/node_modules/ajv`。不同布局时设置`AJV_MODULE`为已有AJV6.15.0的模块目录；可用`PYTHON`选择已有Python3解释器。这些参数不会下载模块或改变系统工具链。

```bash
AJV_MODULE=/path/to/existing/ajv node docs/design/examples/capability-reference/verify.cjs
```

命令验证49个Schema样本（16正、33反）、6个规范输入性质、3个已冻结正命令hash、6个原始JSON输入、27个语义样本。前后比对Schema字节/hash，读失败、依赖版本不符、样本不符或文件被改均非零退出。Schema是形状合同，跨字段相等/实时授权/事务效果不能仅由AJV证明；本oracle只对具名合成状态作决定。

## 执行入口重放也要检查读权

S25–S27故意把`phase=execute`、`allowed=true`与`read_allowed=false`组合：相同operation已为succeeded、held、unknown。写/执行资格不自动授予旧结果读权；都应统一not_found、无新效果且不返回旧content。不能只保护专门receipt GET路径。

[首次红例](replay-read-regression.json)记录旧oracle实际分别返回原receipt、held及unknown对账的3项失败，保留原函数摘要。当前[固定oracle](verify_semantics.py)在任何旧记录重放前检查当前主体/读资格；[只读入口](verify.cjs)不再重写样本hash或生成cases。

可以单独运行语义/原始JSON验证，或主动移除该守卫验其确实抓到目标错误：

```bash
python3 docs/design/examples/capability-reference/verify_semantics.py
python3 docs/design/examples/capability-reference/verify_semantics.py --disable-replay-read-guard
```

第二条是明确的设计模型变异，预期S25–S27恰好失败、进程非零；不是普通验证命令。首次真实红、绿色及变异不声称产品TDD或真实权限验收。正常执行不因为user写权限仍在就泄露其已失读权的旧状态/内容，也不因拒读创建新operation重做效果。

## PostgreSQL实验的独立边界

最初8项候选SQL实验的[去敏收据](pg-candidate-result.json)单独保留：完整提交水位锁序、原子回滚、同key同输入、同key异输入、noop、16并发去重、stage唯一性，以及plain sequence不是完整提交水位的反例。那是最小候选表/锁实验，未执行AL真实service/产品三链；与本次只读模型验证不同，本次没有重跑PG。

原实验源码封存本地ignored审计区，避免`.go`工具扫描到临时程序。不能用默认业务DSN运行它：复现实验必须由协调者先验证自有容器标签，创建唯一且空的专属`ce_review_*`数据库，设置单独数据库写权限，执行后drop自有DB并核计数0。不得连接业务DB/public、共享账号数据或让此只读入口自动建库/迁移。未具备独占环境时将PG实验记NOT_RUN，不影响本参考命令的无数据库范围，也不把旧8项收据冒称本轮新跑。

仓内样例尚未接入通用CI质量门。产品实现仍须按既有测试先行跑真实共同路径、权限/撤权、写主事务、故障/多实例与三链验收。
