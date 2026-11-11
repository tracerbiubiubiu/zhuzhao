# audit_archive cron 收归切换 checklist（P4，2026-09-29）

收归形态：zhuzhao 进程内 ticker + PG advisory lock（`audit.archive.cron_enabled=true`）
——审计数据与归档内务全在 zhuzhao 本地，闹钟外置 taskrunner 属既成依赖（拍板）。

切换步骤（部署侧二选一防双跑）：
1. **先停用 taskrunner 侧 audit_archive cron job**，通过当前任务管理更新入口设置 `enabled=false`，记录原配置并确认在途执行已结束；不直接修改数据库绕过管理路径。
2. zhuzhao 开 `APP_AUDIT_ARCHIVE_CRON_ENABLED=true`（tick 默认 30s，单批一执行），按部署流程重启目标实例并确认旧触发器不再调度。
3. 验证：zhuzhao 日志见 `cron scheduler started`（job=audit_archive）+
   归档目录出现新 JSONL（有超期行时）；
4. 回滚：先关进程内开关并确认在途结束，再恢复原 taskrunner job，避免同时触发。

「长任务×并发重试」taskrunner 侧集成测试：归档链路已单批化（zhuzhao 侧根治
——60s WriteTimeout 窗口对秒级单批不再触发），taskrunner 通用长回调窗口的
测试属触发驱动（非归档链路），登记待办不随本批。
