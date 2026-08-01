CREATE TABLE project_documents (
    id VARCHAR(64) NOT NULL,
    project_id VARCHAR(64) NOT NULL,
    parent_id VARCHAR(64) NULL,
    slug VARCHAR(160) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    title VARCHAR(200) NOT NULL,
    content_markdown MEDIUMTEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_by VARCHAR(64) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
        ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    -- 同一项目内文档 slug 唯一，用于稳定的阅读页地址。
    UNIQUE KEY uq_project_documents_slug (project_id, slug),
    INDEX idx_project_documents_tree (project_id, parent_id, sort_order),
    INDEX idx_project_documents_deleted (project_id, deleted_at),
    CONSTRAINT fk_project_documents_project
        FOREIGN KEY (project_id) REFERENCES managed_projects (id)
        ON DELETE CASCADE ON UPDATE RESTRICT,
    -- 自引用父级，删除父文档时其子文档一并移除。
    CONSTRAINT fk_project_documents_parent
        FOREIGN KEY (parent_id) REFERENCES project_documents (id)
        ON DELETE CASCADE ON UPDATE RESTRICT,
    CONSTRAINT fk_project_documents_author
        FOREIGN KEY (created_by) REFERENCES users (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE = InnoDB DEFAULT CHARACTER SET = utf8mb4 COLLATE = utf8mb4_unicode_ci;
