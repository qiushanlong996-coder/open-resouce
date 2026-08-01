ALTER TABLE users
    ADD COLUMN experience INT NOT NULL DEFAULT 0;

CREATE TABLE experience_events (
    id VARCHAR(64) NOT NULL,
    user_id VARCHAR(64) NOT NULL,
    action VARCHAR(40) NOT NULL,
    source_key VARCHAR(191) NOT NULL,
    points INT NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_experience_events_source (user_id, action, source_key),
    INDEX idx_experience_events_user (user_id, created_at),
    CONSTRAINT fk_experience_events_user
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE CASCADE ON UPDATE RESTRICT
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_unicode_ci;
