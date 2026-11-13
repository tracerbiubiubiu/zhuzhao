# AI、skill与SOC设计参考模型

> 日期：2026-11-13。用途：复验FD03/FD06/FD08候选设计；范围：全部合成数据；不访问网络/模型/Agent平台/SOC，不写主仓业务数据库。不是生产Agent runtime、SDK互操作或全栈验收。
> 权威说明：[领域Agent](../../domain-agent-reference-scenario.md)、[skill](../../skill-reference-package.md)、[SOC协作](../../soc-collaboration-authorization.md)。

## 文件和命令

fixtures.json维护独立业务预期，verify.py运行三种策略及共同守卫。35个Agent场景×3、16个skill场景、46个SOC场景=167条行为检查；另外10个守卫变异必须被预期抓住。临时checkpoint、budget、receipt只在自有临时目录，结束自动清理。

```bash
python3 docs/design/examples/ai-soc-reference/verify.py --output .codex/audits/ai-soc-design-20261113/prototype-results.json
node docs/design/examples/ai-soc-reference/validate-schemas.cjs ../zhuzhao-ui/node_modules/.pnpm/ajv@6.15.0/node_modules/ajv
```

第二条从主仓根目录执行，显式复用已有UI lockfile中的Ajv6.15.0，不安装生产依赖；若该路径因锁文件升级变化，传入已核版本的实际模块目录并登记版本。它校验上级三份draft-07 Schema、86个正样本和9个负样本，不替代行为守卫。

## 模型范围

- fixed_steps是确定性四步骤；enterprise_agent_stub只模拟协议JSON交互；bounded_host_model用脚本化提议模型，不是真实企业Agent/LLM/ADK/LangGraph。
- actor/组织/风险/知识/工单均为demo标识，hidden记录只在夹具中验证不可泄露，不进入受权输出。
- JSON临时文件模型只演示串行失败恢复与预算/receipt，不证明PG事务、多实例竞态或生产时间/token预算。
- [contract-examples.json](contract-examples.json)是精确plan/approval/checkpoint样本；[可信skill](skills/risk-remediation-brief/README.md)是指令资产包，文档本身不授工具权限。

## 本轮结果和维护

当前167条行为、10个变异、86正/9负Schema均通过，真实网络调用0。初轮A14的预期把知识ACL失败与已验证草案混淆，按本场景明确条件修正，原失败留本地；没有把改预期描述成产品修复。

改业务语义先改拥有者合同和独立预期，再修改模型；新能力必须补相应正/负/授权/恢复例。变化后重跑两条命令，结果只证明本目录定义的范围。

后继独立审查发现原plan只存知识ID且apply未重验当前ACL/版本。原57条Agent与16条skill例均没有覆盖preview后的知识竞争窗口。新增A20–A25×3在加守卫前实际18条失败，修正后75条Agent通过；[去敏回归记录](knowledge-recheck-regression.json)分别记录错误新效果、receipt授权与投影失败，不将旧覆盖夸大为已验证这些情况。plan引用现钉id/version/digest，receipt只返回当前仍获权的最小业务效果状态；样包1.0.0的SKILL.md及其digest未改变。

再次按原话核验时，发现A25只保护专门的receipt读取路径，没有覆盖已经提交的操作从apply入口重放。新增A26–A35×3固定预期后，原守卫实际失败21条：同进程/恢复后读权撤销仍返回旧记录，或知识ACL/版本/digest及原执行批准失效后错误阻断合法已有效果查询；原137条行为不包含这些交叉条件。加守卫前源码、固定预期、失败明细和修正后收据保存在本地ignored审计目录`conversation-reaudit-20261113/ai-replay/`，未将其当成产品漏洞或真实IAM验收。

apply遇到已有operation时，先校验原plan绑定，再复用当前操作主体、范围和目标读权限检查，只投影`operation_id/status/effect_id`。该分支不重新执行、不返回plan/hash或actor等旧内部记录；知识和原执行批准的后续变化不抹掉已合法完成的效果，也不授权新效果。相同operation改plan、换actor、撤scope或目标越域仍拒绝；没有已有receipt的新执行继续完整复核知识、批准和业务revision。新增三个变异分别移除apply重放读权、移除plan绑定、错误地重验已完成操作的原执行批准/知识，均由固定预期抓住。新增10个动作只扩Schema的封闭action枚举，未增加业务输入字段。
