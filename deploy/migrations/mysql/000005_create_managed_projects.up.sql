CREATE TABLE managed_projects (
    id VARCHAR(64) NOT NULL,
    owner_id VARCHAR(64) NOT NULL,
    slug VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    name VARCHAR(120) NOT NULL,
    summary VARCHAR(300) NOT NULL,
    description TEXT NOT NULL,
    category VARCHAR(80) NOT NULL,
    tags JSON NOT NULL,
    tech_stack JSON NOT NULL,
    license VARCHAR(40) NOT NULL,
    repository_url VARCHAR(500) NOT NULL DEFAULT '',
    cover_object_key VARCHAR(500) NOT NULL DEFAULT '',
    document_object_key VARCHAR(500) NOT NULL DEFAULT '',
    code_object_key VARCHAR(500) NOT NULL DEFAULT '',
    current_version VARCHAR(40) NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'draft',
    review_reason VARCHAR(500) NOT NULL DEFAULT '',
    submitted_at DATETIME(6) NULL,
    published_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
        ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_managed_projects_slug (slug),
    INDEX idx_managed_projects_owner (owner_id, updated_at),
    INDEX idx_managed_projects_status (status, submitted_at),
    CONSTRAINT fk_managed_projects_owner
        FOREIGN KEY (owner_id) REFERENCES users (id)
        ON DELETE CASCADE ON UPDATE RESTRICT
) ENGINE = InnoDB DEFAULT CHARACTER SET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE project_review_events (
    id VARCHAR(64) NOT NULL,
    project_id VARCHAR(64) NOT NULL,
    actor_id VARCHAR(64) NOT NULL,
    action VARCHAR(24) NOT NULL,
    reason VARCHAR(500) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    INDEX idx_project_review_events_project (project_id, created_at),
    CONSTRAINT fk_project_review_events_project FOREIGN KEY (project_id)
        REFERENCES managed_projects (id) ON DELETE CASCADE ON UPDATE RESTRICT,
    CONSTRAINT fk_project_review_events_actor FOREIGN KEY (actor_id)
        REFERENCES users (id) ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE = InnoDB DEFAULT CHARACTER SET = utf8mb4 COLLATE = utf8mb4_unicode_ci;
