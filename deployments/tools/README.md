# 专用运维工具镜像

[migrate](migrate/README.md)与[rclone](rclone/README.md)固定官方源码与依赖锁后重新构建，不维护业务 fork、不扩充平台能力。使用同一已验证Go/Alpine版本，保留可扫描符号、模块清单和许可证；migrate仅保留本项目使用的PostgreSQL驱动，rclone保留其工具能力并修复已发现依赖公告。

公共上游镜像摘要仍在[组件清单](../component-lock.json)中用于追溯比较，实际Compose使用其中`local_tool_builds`定义的镜像与构建目录。已有环境迁移、验证和回退先读[数据契约](../../docs/design/component-upgrade-data-contract.md)。本地标签不能替代正式发布时的内容摘要。

```bash
docker build -t zhuzhao-migrate:4.20.1-go1.27.4 deployments/tools/migrate
docker build -t zhuzhao-rclone:1.76.0-go1.27.4 deployments/tools/rclone
```

运行环境可以只导入已验收镜像；构建环境需要固定SDK基础镜像与已校验Go模块。内部缓存可以更换下载端点，不能关闭sumdb或改成未锁定源码。CI从真实main package和实际镜像ELF分别扫描，禁止扫描空wrapper的`./...`后宣称安全通过，也不能依据JSON模式退出0放行。
