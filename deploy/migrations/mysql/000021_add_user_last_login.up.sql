-- 用户最近一次登录的 IP 归属地与时间。
--
-- 与评论归属地同样的原则：只存归属地（省级/国家级），不存原始 IP。
-- 差别在于用途与可见范围——评论归属地是公开展示，这两个字段只在管理后台
-- 的用户管理里对管理员可见，用于识别异常登录来源。
--
-- 归属地与时间成对存在才有意义：只有「广东」而不知道是什么时候，
-- 对判断异常登录几乎没有帮助。
--
-- 存量用户在下次登录时才会填上，此前为空/NULL，管理后台显示为「暂无记录」。
ALTER TABLE users
    ADD COLUMN last_login_region VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN last_login_at DATETIME(6) NULL;
