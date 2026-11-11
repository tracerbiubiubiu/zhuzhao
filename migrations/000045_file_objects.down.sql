-- 000045 down: 独立新表，直接删除（对象本体在 RustFS，元数据表删除不影响存量功能）
DROP TABLE IF EXISTS file_objects;
