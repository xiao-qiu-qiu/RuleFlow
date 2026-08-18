-- 将自适应作为一等配置策略目标，按请求客户端选择实际输出格式。
ALTER TABLE config_policies
    DROP CONSTRAINT IF EXISTS config_policies_target_check;

ALTER TABLE config_policies
    ADD CONSTRAINT config_policies_target_check
    CHECK (target IN ('clash-mihomo', 'stash', 'surge', 'sing-box', 'adaptive'));
