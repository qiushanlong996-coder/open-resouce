CREATE TABLE api_keys (
    id VARCHAR(64) NOT NULL,
    owner_id VARCHAR(64) NOT NULL,
    name VARCHAR(120) NOT NULL,
    key_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    prefix VARCHAR(16) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    revoked_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_api_keys_key_hash (key_hash),
    INDEX idx_api_keys_owner (owner_id, created_at),
    CONSTRAINT fk_api_keys_owner
        FOREIGN KEY (owner_id) REFERENCES users (id)
        ON DELETE CASCADE ON UPDATE RESTRICT
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_unicode_ci;
