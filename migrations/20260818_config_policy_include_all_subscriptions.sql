ALTER TABLE config_policies
    ADD COLUMN IF NOT EXISTS include_all_subscriptions BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN config_policies.include_all_subscriptions IS '是否自动包含所有启用的订阅源';

-- 兼容旧版通用订阅路由创建的自动策略。升级后新增订阅会自动进入这些策略。
UPDATE config_policies
SET include_all_subscriptions = true
WHERE LEFT(name, 7) = '__auto_';
