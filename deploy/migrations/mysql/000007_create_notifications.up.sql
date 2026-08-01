CREATE TABLE notifications (
    id VARCHAR(64) NOT NULL,
    recipient_id VARCHAR(64) NOT NULL,
    actor_id VARCHAR(64) NOT NULL DEFAULT '',
    actor_name VARCHAR(80) NOT NULL DEFAULT '',
    type VARCHAR(40) NOT NULL,
    title VARCHAR(200) NOT NULL,
    body VARCHAR(500) NOT NULL DEFAULT '',
    project_slug VARCHAR(80) NOT NULL DEFAULT '',
    document_slug VARCHAR(160) NOT NULL DEFAULT '',
    comment_id VARCHAR(64) NOT NULL DEFAULT '',
    read_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    INDEX idx_notifications_recipient_created (recipient_id, created_at),
    INDEX idx_notifications_recipient_read (recipient_id, read_at),
    CONSTRAINT fk_notifications_recipient FOREIGN KEY (recipient_id)
        REFERENCES users (id) ON DELETE CASCADE ON UPDATE RESTRICT
) ENGINE = InnoDB DEFAULT CHARACTER SET = utf8mb4 COLLATE = utf8mb4_unicode_ci;
