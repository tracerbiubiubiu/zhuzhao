-- 000083 down：撤销 DB 安全代际与私有 operation/intent 存储。
-- ⚠ 回滚合同（底座 §4.3）：down 删除安全代际列后，旧代码不能按旧代继续接受
-- 已推进代际的会话——降版前须受控停会话/轮换 JWT 签名密钥并要求重新登录，
-- 禁止只回滚 DB 后让旧 schema 重新接受旧 token。
DROP TABLE IF EXISTS session_projection_intents;
DROP TABLE IF EXISTS user_session_operations;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_session_projection_epoch_ck;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_session_epoch_ck;
ALTER TABLE users
    DROP COLUMN IF EXISTS session_projection_epoch,
    DROP COLUMN IF EXISTS session_epoch;
