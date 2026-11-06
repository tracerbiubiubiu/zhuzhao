# 生态全面审计 B14：资料写入覆盖账号安全状态（2026-11-06）

## 结论、范围与基线

**ECO-20261104-B14-F01 / P0，开放：在途个人资料更新能覆盖管理员刚写入的禁用状态或强制改密标记，使后续新登录重新通过。** 采用本项目“安全/门禁失效”分级；不是匿名访问、密码回滚或R01旧令牌再次复活。两场景同根记一项。

固定受审实现 **`b4435022d00ee789b9a62f74036824bd7d3791b7`**，包含R01纪元修复。UserService/UserRepo相关路径与原主仓一致，Auth/JWT采用该完整固定源码，不能混用主工作树的修复前认证。随后联合候选b306eed的相关路径未变。实验使用独立PG全81迁移和Redis，全部账号为合成E8段；未写共享dev/test、E000001或真实业务数据。

root从只读数据层复核发现候选后占用B14批，要求实际服务交错与认证结果验证；原始静态推断不单独计作P0证据。当前修复在独立codex分支准备，未合main、未部署或推送。

## 根因与锚点

原固定hash如上；可点击代码：[UserService.UpdateProfile](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/internal/service/user_service.go:375)、[UserRepo.Update](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/internal/repository/user_repo.go:259)、[UserRepo.UpdateStatus / UpdateStatusTx / UpdatePassword](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/internal/repository/user_repo.go:304)。代码符号为主、行号辅助，认证固定源另见本地raw source-manifest。

个人资料先FindByID读取整份User，再调用通用Update。通用SQL不只写请求中的姓名/联系方式，还把旧User.status与must_change_password一并回写，WHERE仅以User.version做CAS。管理员禁用、重置密码使用的安全专用写没有推进该version；因此它们发生在资料读取和最终UPDATE之间时，旧资料写仍成功，把安全字段恢复成旧值。

其他调用方包括管理面资料更新；修复必须对共用写入口做同类扫描，不能只修一个handler或要求个人资料客户端携带原本没有的安全字段。密码hash不在这次通用回写中，实验没有把重置后的密码改回旧口令。

## 真实复现与对照

pgx QueryTracer只在生产UserRepo.Update的SQL即将发送前设置屏障，保留真实FindByID、UpdateProfile与SQL条件，不用仓储桩替代判断。流程为：

1. 合成目标用户初始status=1、must_change_password=false、version=1，正常AuthService.Login取得旧AT。
2. 启动真实UpdateProfile；完成旧读后、实际UPDATE前暂停。
3. 真实管理员UserService.UpdateStatus或ResetPassword成功提交，核DB安全字段已变，version仍1；管理员使用合成superadmin角色账号。
4. 放行旧资料UPDATE，真实DB version变2，资料写成功；再用实际AuthService.Login及真实JWT中间件的受保护HTTP `/probe` 验证后果。

**禁用场景8/8**：管理员后status=0，旧资料写后status=1；原口令重新登录成功，新AT的受保护HTTP为200。**重置场景8/8**：管理员后must_change_password=true，旧资料写后false；使用重置后的新口令登录返回false，新AT直接200而非强改限制403。两个问题都不是仅看数据库字段后推测可登录。

六个顺序/版本控制均正常：每场景分别验证“管理员先完成再读取资料”“资料先完成再管理员”“先有另一资料写推进version，再管理员与旧资料写交错”。前两者安全状态保留；第三者旧资料写返回10006 CAS冲突。六个控制各执行一次，不冒充八轮控制全覆盖。

**所有旧AT仍拒绝，22/22**：16个交错场景为401，禁用控制为403，重置控制为401；R01纪元守卫仍有效。新问题是DB安全状态被另一写入路径覆盖后，后续新登录获得新纪元令牌。HTTP探针仅验证JWT/强改闸门，不把它扩大成全业务L1/L2/L3遍历。

## 修复建议与承接边界

建议将资料写限定为资料字段，彻底移除通用Update对status和强改标记的旧值回写；如需返回User，应回填当前数据库安全字段。保留现有资料version CAS、管理员状态/密码专用入口及全端吊销，不以“只让安全写增version”代替字段所有权约束。

独立修复分支codex/fix-user-security-state-consistency基于b306eed；修复验证须覆盖个人/管理员两资料调用方、禁用及强改保持、正常资料和状态/密码更新、旧AT拒绝与后续新登录，逐项移除修复后稳定失败。代码提交及固定门禁结果到位后另补本报告，当前不标代码层关单。

## 证据、环境与未覆盖项

证据仅本地忽略 `.codex/audits/ECO-20261104/data-coverage/b14/`：完整固定source-manifest、81迁移记录、main.go实际服务/PG/Redis探针、probe.raw.log的22条结构记录、run-result.json及cleanup.json。root已核探针和逐条结果；原两类各八次复现、六个控制、旧AT22次拒绝均有记录，不以描述替代日志。探针Go运行正常退出不表示产品通过，其明确输出16/16违规；原样证据为被测结果断言。

首轮PG临时初始化服务器曾使健康探针过早成功，迁移未连接最终数据库；已保留失败日志并清理两只容器。正式轮改用TCP+SELECT1核最终库就绪；两轮四只资源均按精确ID+owner清理、inspect不存在。没有把初始化失败计作产品缺陷。

该证明是实际服务写入和真实登录/JWT HTTP探针，不是完整HTTP资料请求并发压测。未来完整联合integration、acceptance、R01十三步及本批真实回归仍待执行；当前主线/开发原版/测试部署均未因此自动修复。全面审计继续，控制台和会话台账同步P0开放状态。
