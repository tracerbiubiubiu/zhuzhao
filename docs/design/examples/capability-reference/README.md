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

当前命令验证143个Schema样本（39正、104反）、6个规范输入性质、9个已冻结正命令hash、6个原始JSON输入、原27个语义样本及23个get读绑定样本；读绑定另验command/receipt/event形状及输入hash。原80样例（含最初49例）及预期保持不变；新增IAM63例只扩严格同步profile，不证明其HTTP适配/事务/真实授权。运行前后比对Schema字节/hash，读失败/依赖版本不符/样本错误/文件被改均非零。原语义oracle仍只验其具名合成状态，不冒充新增IAM行为oracle。

## 本地IAM的形状补全

2026-11-15添加角色授予/角色菜单两个strict profile、最小同步causality、0起始集合版本、领域提交与运行时policy_reload分轨及受权receipt选择器。新reference未改verify.cjs/verify_semantics.py或产品源；使用工具进程Node22.14.0＋既有AJV6.15.0/Python，无新增依赖，不作为项目目标Node26的产品门禁。

新增首批55例先在旧Schema上有效红（14个新正例拒绝、4个禁止descriptor接受）；独审发现并新增8个初稿能错误放行的I/O/profile错绑、pending零策略代际和noop反例，收紧后共143形状全绿。原AL/report、admission及订阅定义和既有80样例不因IAM放宽。前后红绿与独审反例收据由调用者留在ignored审计目录，本页不声称新业务已验。

IAM不变量仍须实施C批独立证明：集合与ETag同快照、pending效果的提交head、保留锚不归零、key-only重放不改hash、读权先于回放/收据、同事务审计/意图失败回滚、未知提交及策略恢复。Shape与命令hash不能替代这些PG/HTTP/进程恢复断言。

[C05秘密绑定向量](c05-secret-binding-vector.json)只使用合成口令及零测试key，封闭消息编码与HMAC已分别由Python/Node独立重算一致；它不由verify.cjs自动覆盖，也不证明运行key托管/轮换或改密链。该文件不可作为运行配置、账号资料或日志样板使用。

## 公共模块与HTTP消息的选定参考

2026-11-15新增[模块向量](module-profile-vectors.json)/[Schema](module-profile-vectors.schema.json)与[消息向量](message-http-vectors.json)/[Schema](message-http-vectors.schema.json)，各10例（6正4反）；[参考manifest](module-reference-manifest.json)记录profile、文件摘要和验证边界。模块覆盖有界Query、首期设备无凭据投影、单ticket.list工具及风险序列/观察投影；消息固定send/query/异步回执形状，并有12字段payload hash正例。两者各在空Schema上抓到4个反例后收紧，根会话独立复跑20/20；消息Unicode/换行/末尾空格载荷由Node/Python独立重算相同478字节/hash。

这些文件**不被原verify.cjs自动执行**，用现有AJV可单独复跑：

```bash
AJV_MODULE=/path/to/existing/ajv node <<'JS'
const fs = require('node:fs');
const p = 'docs/design/examples/capability-reference/';
const m = process.env.AJV_MODULE;
if (require(m + '/package.json').version !== '6.15.0') throw Error('AJV version');
const Ajv = require(m);
for (const name of ['module-profile-vectors', 'message-http-vectors']) {
  const s = JSON.parse(fs.readFileSync(p + name + '.schema.json'));
  const d = JSON.parse(fs.readFileSync(p + name + '.json'));
  const a = new Ajv({allErrors: true, format: 'full'});
  if (!a.validateSchema(s)) throw Error(name + ': schema');
  a.addSchema(s);
  if (!a.getSchema(s.$id)(d)) throw Error(name + ': envelope');
  for (const c of d.cases)
    if (a.getSchema(s.$id + '#/definitions/' + c.definition)(c.value) !== c.expected_valid)
      throw Error(name + ': ' + c.id);
  console.log(name + ': ' + d.cases.length + ' PASS');
}
JS
```

命令只验shape；hash正例另按message向量的hash_reference.projection_fields规范化重算。manifest是参考索引，不是可装载运行配置，也不证明旧adapter×新core兼容已经实现。当前权限、sequence比较/关闭checkpoint、跨字段匹配、回执去重/CAS、先受理再断连及重启查询仍属产品验收。原[UAA离线参考](../uaa-reference-mock/README.md)57行为/16去守卫亦已重跑通过，网络/企业调用0；它不是OIDC/SCIM服务或真实UAA联调。公司字段、地址、密钥及运行制品不在这些公开合成文件中。

## 独立AL只读后继的必要补全

2026-11-14补`activelist.document.get@1`，复用已有AL查询能力，支持没有工单的实际AL→TR只读后继。get成功用core保存的query_result，observation_owner与AL data_owner分开，禁止伪造写effect；原update/report成功仍必须effect。get订阅只支持受控target_from_event、登记类型/世代与字段名常量数组，原update映射没有因此放宽。

新增形状例覆盖无票/手动get、独立观察receipt、按事件逐行取目标、字段缺失/空/重复/超额、未知版本、目标owner、写patch混入、伪写effect、结果嵌套/超额/来源配对、动态与静态目标混用、伪字段数组、错event pointer及update不受扩宽。原20个绑定模型例覆盖第二条事件文档而非固定第一行、手动无事件、type/generation/事件/operation/hash一致性、相同event_id不同绑定source的拒绝、当前字段投影、无权行、当前值与触发revision区分及UTF-8字节预算。

2026-11-14先加入样本，原Schema对4个新正例失败；随后读绑定入口未实现时16项具名预期失败，再补严格分支与oracle得到绿色。原始红绿和准确候选摘要由调用者保存在本地ignored审计目录，未将运行日志加入Git。读绑定守卫可显式变异：

```bash
python3 docs/design/examples/capability-reference/verify_semantics.py --disable-read-contract-guard
```

该命令当前预期18个拒绝样本失败、非零退出；3个允许样本仍通过。11月14日的17项连续8次稳定抓到该变异，原初15项红记录仍保留；相同event_id冒认另一source的新增反例也先红后绿。它证明**设计模型**的约束确实参与判断，不证明AL/TR产品没有副作用、真实权限或并发正确。旧replay守卫变异仍单独恰好抓3例。

## 可选字段未设置的空投影

2026-11-15复核发现AL允许可选字段缺失，但`alReadResult.values`最少1属性会拒绝合法空投影。本次仅把该约束改为允许0属性；记录存在且请求字段均未设置时返回`values={}`，显式清值保留`{"status":null}`，不把未设置伪造成null。不存在仍404，无读资格、依赖失联仍是错误，不能统一变成空成功。

新增2个形状正例（空投影、显式null）和3个读绑定例（未设置、显式null、空投影但无权拒绝）。旧Schema对空投影形状及两条相应绑定形状共3项有效变红，窄修后80形状/23读绑定全部通过；已有语义oracle原本就支持空投影并拒绝无权，该部分没有虚称新行为先红。新增无权例与其余17个负例在读守卫移除后连续8次稳定失败。旧fixtures、原例预期与产品代码均不改。

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
