v0.4.3

- JWT AccessClaims 增加 Pwe，新增 GenerateAccessTokenWithEpoch；旧签发 API 保持纪元 0。
- 调用方负责读取、递增并校验权威密码纪元，升级工具库本身不等于完成应用会话撤销修复。
- AK/SK 默认响应码引用公共常量，wire 码值不变；jwt/redis/postgres/jsonutil 错误文本补包名前缀，按字符串匹配错误的调用方需同步，errors.Is 不受影响。
- 增补 crypto/errcode/logger 与 JWT 测试；未新增依赖。

验证：本地独立 git archive 全量 race 测试通过，main CI 41800001957 通过。消费仓须 pin 正式版本后自行复验。

Refs: ECO-20261104-B02-F01

CI：[main](https://github.com/tracerbiubiubiu/zhuzhao-utils/actions/runs/41800001957)、[v0.4.3](https://github.com/tracerbiubiubiu/zhuzhao-utils/actions/runs/41800452762) 均通过。
