# 设计合同与合成样本

本目录只放可复验的候选合同与有限设计模型，不是可调用产品模块。人工设计在上级design目录；样例无公司数据、真实身份、地址或凭据。

- [AI/skill/SOC参考模型](ai-soc-reference/README.md)：FD03、FD06、FD08同场景对比、可信样包与职责授权正反例；真实提供方仍单独验收。

- [MCP官方SDK互操作样例](mcp-sdk-interop/README.md)：独立Go module、两只读发布/一只读消费、14个语义负例及预算/撤权测试；AS和业务授权仍为合成，未代替核心IAM/企业联调。

- [共同能力合同只读验证](capability-reference/README.md)：49个Schema样本、规范输入6项、严格JSON6项和27个语义正反例；包括执行入口重放也必须验读权，PG实验单独说明而不默认运行。
