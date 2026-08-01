CREATE TABLE admin_audit (
    id VARCHAR(64) NOT NULL,
    actor_id VARCHAR(64) NULL,
    actor_email VARCHAR(254) NOT NULL DEFAULT '',
    action VARCHAR(64) NOT NULL,
    target VARCHAR(191) NOT NULL DEFAULT '',
    detail VARCHAR(500) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    INDEX idx_admin_audit_created (created_at),
    INDEX idx_admin_audit_action (action, created_at),
    CONSTRAINT fk_admin_audit_actor
        FOREIGN KEY (actor_id) REFERENCES users (id)
        ON DELETE SET NULL ON UPDATE RESTRICT
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_unicode_ci;
