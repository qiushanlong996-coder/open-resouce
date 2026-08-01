-- 文档标识变更后的旧地址重定向。
-- 修改文档 slug 会让已分享出去的阅读链接失效，这里记录历史标识，
-- 让旧地址能 301 到当前地址，而不是直接 404。
CREATE TABLE document_slug_aliases (
    project_id VARCHAR(64) NOT NULL,
    -- 历史 slug。与 project_documents 的唯一键同构，保证同一项目内不重复。
    slug VARCHAR(160) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    document_id VARCHAR(64) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (project_id, slug),
    INDEX idx_document_slug_aliases_document (document_id),
    CONSTRAINT fk_document_slug_aliases_project
        FOREIGN KEY (project_id) REFERENCES managed_projects (id)
        ON DELETE CASCADE ON UPDATE RESTRICT,
    -- 文档被删除时别名一并清除，避免指向不存在的文档。
    CONSTRAINT fk_document_slug_aliases_document
        FOREIGN KEY (document_id) REFERENCES project_documents (id)
        ON DELETE CASCADE ON UPDATE RESTRICT
) ENGINE = InnoDB DEFAULT CHARACTER SET = utf8mb4 COLLATE = utf8mb4_unicode_ci;
