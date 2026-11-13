# 风险整改摘要skill样包

本目录为合成可信样包：SKILL.md使用可移植指令格式，manifest.json使用本平台候选分发/信任格式。版本1.0.0及SKILL.md的SHA256不可就地修改；合法语义变更发布新版本/digest并重新评测。README仅为仓内说明，不属于发布负载；加载器只向模型提供manifest明确列出的文件，忽略目录中未列的说明文件。

- [SKILL.md](SKILL.md)：指令/元数据；其中allowed-tools不是授权。
- [manifest.json](manifest.json)：受控来源、digest、能力依赖及无脚本/无网络执行策略。

加载/撤销/兼容patch/原版缺失/破坏性合同/主体撤权/旧批准等例子归上级[参考模型](../../README.md)。真实包签名、企业知识权限及生产loader均未验收。
