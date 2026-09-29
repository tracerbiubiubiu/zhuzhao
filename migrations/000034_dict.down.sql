-- 还原（复合条件精确删；role_menus 补绑一并清——含 000033 两行）
DELETE FROM role_menus WHERE menu_id IN (90001, 90002, 90003, 90004);

DELETE FROM menu_apis WHERE (api_path, api_method) IN ((
    '/api/v1/dicts', 'GET'), (
    '/api/v1/dicts', 'POST'), (
    '/api/v1/dicts/update', 'POST'), (
    '/api/v1/dicts/delete', 'POST'), (
    '/api/v1/dict-items', 'GET'), (
    '/api/v1/dict-items', 'POST'), (
    '/api/v1/dict-items/update', 'POST'), (
    '/api/v1/dict-items/delete', 'POST'), (
    '/api/v1/dicts/:code/items', 'GET'));

DELETE FROM menus WHERE id IN (90003, 90004);

DROP TABLE IF EXISTS dict_items;
DROP TABLE IF EXISTS dict_types;
