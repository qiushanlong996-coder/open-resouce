CREATE TABLE document_comments (
    id VARCHAR(64) NOT NULL,
    document_id VARCHAR(64) NOT NULL,
    parent_id VARCHAR(64) NULL,
    block_id VARCHAR(128) NOT NULL,
    author_id VARCHAR(64) NULL,
    author_name VARCHAR(80) NOT NULL,
    quote_text VARCHAR(500) NOT NULL DEFAULT '',
    body_text VARCHAR(2000) NOT NULL,
    status ENUM('open', 'resolved') NOT NULL DEFAULT 'open',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
        ON UPDATE CURRENT_TIMESTAMP(6),
    resolved_at DATETIME(6) NULL,
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_document_comments_parent
        FOREIGN KEY (parent_id) REFERENCES document_comments (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    INDEX idx_document_comments_document_created
        (document_id, created_at, id),
    INDEX idx_document_comments_document_status
        (document_id, status, created_at, id),
    INDEX idx_document_comments_parent_created
        (parent_id, created_at, id),
    INDEX idx_document_comments_author
        (author_id, created_at, id)
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_unicode_ci;
