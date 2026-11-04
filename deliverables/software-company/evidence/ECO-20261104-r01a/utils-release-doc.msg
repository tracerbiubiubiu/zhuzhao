docs(release): 更新 v0.4.3 版本与纪元接入口径

改动摘要：
将 README 待发标记及 AT 密码纪元接入说明更新为 v0.4.3，配套所有者已授权的发布。

影响面：
仅 README，不改实现或依赖；消费方仍须显式升级并实现权威纪元比对。

验证证据：
f6dbe69 已经新 git archive 独立全量 race 测试通过，三节提交格式检查通过。
本笔只改版本文档，git diff --check 通过；发布后另核 main/tag CI 及正式模块解析。

Refs: ECO-20261104-B02-F01
