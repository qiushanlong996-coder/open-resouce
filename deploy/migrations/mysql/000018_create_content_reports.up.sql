CREATE TABLE content_reports (
    id VARCHAR(64) NOT NULL,
    reporter_id VARCHAR(64) NOT NULL,
    target_type VARCHAR(16) NOT NULL,
    target_id VARCHAR(191) NOT NULL,
    reason VARCHAR(64) NOT NULL,
    detail VARCHAR(1000) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'open',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    resolved_at DATETIME(6) NULL,
    resolver_id VARCHAR(64) NULL,
    PRIMARY KEY (id),
    INDEX idx_content_reports_status (status, created_at),
    INDEX idx_content_reports_target (target_type, target_id),
    CONSTRAINT fk_content_reports_reporter
        FOREIGN KEY (reporter_id) REFERENCES users (id)
        ON DELETE CASCADE ON UPDATE RESTRICT
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_unicode_ci;
