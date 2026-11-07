# B26 / G-14：字段选项变更软提示边界

## 范围与基线

承接Phase5债台账G-14（P3），候选原问题基线 `996c167f4567e39362226937b25368b55b7f44bd`，正式推广前B23基线 `82bda27e5a46c22d3d57daab4387dc1aa5dbae93`；只改`internal/service/ticket/type_admin.go`与新增专项集成测试，保留B23求值Context接线。UI/AL/TR/utils无对应实现改动，不合main、不启动Phase6。

## B26-F01 / P3：比较长度不足以发现选项约束变化

原[softFieldWarnings](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/service/ticket/type_admin.go#L331)只处理select/multi_select，且先要求旧集合长度更大，漏radio、等基数替换与扩容但删除旧值；dict引用字符串比较也不等于实际启用值集合。此功能是保存前影响提示，不是阻断许可，本批不将提示增强改成硬锁。

## 修复与限制

完整比较旧值与新值，支持radio、静态/不同dict引用切换。查询当前启用字典项与G2同源，但把读取/解析不确定作为“影响无法评估”软提示，不把错误冒充合法空集；相同dict引用无旧历史快照，不借格式空白差异伪造变更。已有G2空集表示不限制：有限→空放宽不告警，空→有限新增约束提示，空→空不告警；静态空列表仍按原保存期400校验。

字典严格读取显式接受RowQuerier并检查Query/Scan/rows.Err；既有G2包装仍按原故障策略回空集，不改为硬拒绝。软提示读取在配置替换事务前，运行校验继续复用传入tx/池，不引入事务内第二连接。required提示、配置CAS、硬锁与合法保存行为保持。外部直接修改同一字典项的历史影响另需字典写侧合同，本批没有快照或自动预警能力，不能将该面销账。

## 验证证据

- 真实PG Service.Replace的15项选择集合矩阵，FieldImpact仅计两张含该字段的在途单，排除closed和缺键单；验证提示后配置确实保存。涵盖radio/multi、同基数/扩容丢旧值、标签重排/只增加、静态↔dict/A↔B、空集约束语义。
- 真实只读角色模拟dict_items读取拒绝，合法配置仍保存并提示无法评估；坏旧配置不静默漏报。查询/扫描/中途迭代错误不返回部分集合；单连接事务、持锁及并发CAS对照通过。
- 候选原实现8轮稳定红、修复8轮绿；7种定向变异各8轮命中对应断言，非编译/准备/panic假失败。正式两文件推广后与候选的唯一生产差异为保留已提交B23 Context，不回退该修复。
- 正式354源固定：Go1.26.10原样unit/cover/integration均exit0（10.22s / 5.62s / 716.53s），集成25包、526主测/718子测，0失败/跳过；14个testutil容器已清理。默认单测覆盖2051/10099=20.309%，ticket包12.0%→11.9%，新增分支由integration标签回归覆盖，不用默认unit比例宣称全面覆盖。
- root原样lint/wire exit0（6.754s），真实Swagger/Wire再生零漂移。最终G14组合实际ELF`60adf44c45f1868a499790c1d5cc6746bae9a48bbe8cef2d11d6add002372f9b`，同354源构建；隔离栈原样迁移/253/0/0与81对往返零漂移（40.845s），schema始终81 clean。
- 首个runtime镜像因构建目录copyfile丢执行位而app启动permission denied，未将其计为验收通过；保留失败镜像268e及attempt4。改为显式COPY chmod0755后镜像`8cfe9ec7cbb0a4663915dc261c30fc486e8e8f7f831a67adb4b60b81bdd9d000`通过非root执行权限检查及attempt5真实启动/验收。ELF源与字节不变，未改正式Dockerfile或常驻环境。

UI B20完整门禁、准确提交CI与常驻承接另核，当前代码出口通过不等于生产已部署。正式完整修复hash提交后补绑；源码、矩阵、七变异与原始失败/成功日志只在ignored `.codex/audits/ECO-20261104/phase5-closeout-20261107/{field-soft-warnings,g14-runtime,joint-b19-b20-b23}/`。

## 固定提交与最终组合补证

本批主仓完整提交 `f87ab1aefcbf1843bd8714c5accf245033f99d8b`。最终运行源码354项逐字绑定主仓 `f87ab1aefcbf1843bd8714c5accf245033f99d8b`，原样253/81及29文件230单测、41/41 E2E均通过；[组合补证与保留限制](./zhuzhao-ecosystem-audit-b20-ui-feedback-2026-11-07.md#验证证据与已纠正的初版误判)保留初版失败、wrapper缺checker与未执行附加control，不扩大本批原覆盖。后续主4db/UI152准确CI与dev/test承接已核，当前映射及Chrome代表走查见[B27最终收口](./zhuzhao-ecosystem-audit-b27-phase5-closeout-2026-11-07.md)；main/生产未变。上文保留本批原轮次与未执行路径，不用新时点倒补旧证据。
