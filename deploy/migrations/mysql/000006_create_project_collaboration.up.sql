CREATE TABLE IF NOT EXISTS project_collaborators (
    project_id VARCHAR(64) NOT NULL,
    user_id VARCHAR(64) NOT NULL,
    role VARCHAR(16) NOT NULL DEFAULT 'editor',
    invited_by VARCHAR(64) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
        ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (project_id, user_id),
    INDEX idx_project_collaborators_user (user_id, updated_at),
    CONSTRAINT fk_project_collaborators_project FOREIGN KEY (project_id)
        REFERENCES managed_projects (id) ON DELETE CASCADE ON UPDATE RESTRICT,
    CONSTRAINT fk_project_collaborators_user FOREIGN KEY (user_id)
        REFERENCES users (id) ON DELETE CASCADE ON UPDATE RESTRICT,
    CONSTRAINT fk_project_collaborators_inviter FOREIGN KEY (invited_by)
        REFERENCES users (id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT chk_project_collaborators_role CHECK (role IN ('viewer', 'editor'))
) ENGINE = InnoDB DEFAULT CHARACTER SET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS project_collaboration_snapshots (
    project_id VARCHAR(64) NOT NULL,
    yjs_snapshot LONGBLOB NOT NULL,
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    updated_by VARCHAR(64) NOT NULL,
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
        ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (project_id),
    CONSTRAINT fk_project_collaboration_snapshots_project FOREIGN KEY (project_id)
        REFERENCES managed_projects (id) ON DELETE CASCADE ON UPDATE RESTRICT,
    CONSTRAINT fk_project_collaboration_snapshots_user FOREIGN KEY (updated_by)
        REFERENCES users (id) ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE = InnoDB DEFAULT CHARACTER SET = utf8mb4 COLLATE = utf8mb4_unicode_ci;
