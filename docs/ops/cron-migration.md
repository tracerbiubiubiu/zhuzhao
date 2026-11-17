# audit_archive cron 收归切换 checklist（P4，2026-09-29）

收归形态：zhuzhao 进程内 ticker + PG advisory lock（`audit.archive.cron_enabled=true`）
——审计数据与归档内务全在 zhuzhao 本地，闹钟外置 taskrunner 属既成依赖（拍板）。

当前准入使用[审计模块§3.1](../modules/audit.md#31-维护动作的准入2026-11-17候选)：固定进程内cron与人工维护、固定TR job是三个可信来源。进程内由部署者受审启动配置批准，绑定固定服务主体及定义，不能靠人工维护人名单或外部actor字符串取得该能力；普通归档Handler无私有准入即拒绝。

切换步骤（部署侧二选一防双跑）：
1. **先停用 taskrunner 侧 audit_archive cron job**，通过当前任务管理更新入口设置 `enabled=false`，记录原配置并确认在途执行已结束；不直接修改数据库绕过管理路径。
2. 审查固定服务主体、库内/文件保留期、归档持久卷、batch和tick，记录环境及批准责任后，zhuzhao 开 `APP_AUDIT_ARCHIVE_CRON_ENABLED=true`（tick默认30s，单批一执行）。配置非法会拒启；按部署流程先确认旧进程退出，再启动目标实例并确认旧触发器不再调度。tick需正duration、库内保留期1–36500天、batch/文件保留期正数、目录非空。
3. 验证：先见固定后台维护准入日志（principal=`service:zhuzhao/audit_archive`），再见 `cron scheduler started`（job=audit_archive）+
   归档目录出现新 JSONL（有超期行时）；
4. 回滚：先关进程内开关并确认在途结束，再恢复原 taskrunner job，避免同时触发。

缺省`cron_enabled=false`不启用后台能力；人工维护者名单空只表示人工维护不开放，不自动否决已受审的固定系统cron。临时缩短保留期走独立人工批准，不从ticker参数注入。固定TR恢复还须重新核当前job身份、定义摘要和有效窗，不能仅把旧enabled改回true；开通和撤销步骤见[runbook §6.4](runbook.md#64-审计维护开通与核查)。

「长任务×并发重试」taskrunner 侧集成测试：归档链路已单批化（zhuzhao 侧根治
——60s WriteTimeout 窗口对秒级单批不再触发），taskrunner 通用长回调窗口的
测试属触发驱动（非归档链路），登记待办不随本批。
