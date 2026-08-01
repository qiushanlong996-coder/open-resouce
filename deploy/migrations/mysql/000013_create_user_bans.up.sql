CREATE TABLE user_bans (
    user_id VARCHAR(64) NOT NULL,
    reason VARCHAR(500) NOT NULL DEFAULT '',
    banned_by VARCHAR(64) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (user_id),
    INDEX idx_user_bans_created (created_at),
    CONSTRAINT fk_user_bans_user
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE CASCADE ON UPDATE RESTRICT,
    CONSTRAINT fk_user_bans_admin
        FOREIGN KEY (banned_by) REFERENCES users (id)
        ON DELETE SET NULL ON UPDATE RESTRICT
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_unicode_ci;
