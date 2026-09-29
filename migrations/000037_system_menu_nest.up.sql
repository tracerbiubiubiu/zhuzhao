-- 审计修复（2026-09-30 P1-1）：000002 种子 INSERT 列清单遗漏 parent_id——
-- system_user/role/menu/org 四页 parent=NULL 平铺顶层（system 目录成空目录）。
-- 裁定=嵌套意图（code 前缀 system_ 与目录存在强暗示；audit_log/system_dict
-- 的 NULL 是显式书写属顶层意图——前端组件名另改 *_page）。
UPDATE menus SET parent_id = (SELECT id FROM menus WHERE code = 'system')
WHERE code IN ('system_user', 'system_role', 'system_menu', 'system_org')
  AND parent_id IS NULL AND deleted_at IS NULL;
