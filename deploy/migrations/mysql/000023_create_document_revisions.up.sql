-- 文章版本历史。
--
-- 编辑器每 1.2s 自动保存一次正文，如果每次保存都开一个新版本，历史会被自动保存刷爆。
-- 因此同一作者在合并窗口内的连续编辑会原地更新最新那条版本记录（见 Gateway 侧
-- recordDocumentRevision），只有换人编辑、超出窗口或执行回滚时才递增版本号。
CREATE TABLE project_document_revisions (
    id VARCHAR(64) NOT NULL,
    document_id VARCHAR(64) NOT NULL,
    project_id VARCHAR(64) NOT NULL,
    version INT NOT NULL,
    slug VARCHAR(160) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    title VARCHAR(200) NOT NULL,
    content_markdown MEDIUMTEXT NOT NULL,
    -- 版本作者。历史记录必须能在用户注销后继续存在，所以这里不建外键。
    author_id VARCHAR(64) NULL,
    -- create 首次建档，edit 正文编辑，restore 回滚到历史版本。
    source VARCHAR(16) NOT NULL DEFAULT 'edit',
    -- 回滚版本记录它是从哪个版本号还原来的，便于在历史列表里说明来源。
    restored_from INT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
        ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    -- 同一文档内版本号唯一，保证 v1、v2 是稳定可引用的标识。
    UNIQUE KEY uq_document_revisions_version (document_id, version),
    INDEX idx_document_revisions_list (document_id, version),
    INDEX idx_document_revisions_project (project_id, created_at),
    CONSTRAINT fk_document_revisions_document
        FOREIGN KEY (document_id) REFERENCES project_documents (id)
        ON DELETE CASCADE ON UPDATE RESTRICT,
    CONSTRAINT fk_document_revisions_project
        FOREIGN KEY (project_id) REFERENCES managed_projects (id)
        ON DELETE CASCADE ON UPDATE RESTRICT
) ENGINE = InnoDB DEFAULT CHARACTER SET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

-- 阅读页要展示「v3 · 最后由 X 更新」，为避免每次读文档都联表查历史，
-- 把当前版本号与最后更新人反范式冗余到文档表；写入失败时下一次保存会自愈。
ALTER TABLE project_documents
    ADD COLUMN version INT NOT NULL DEFAULT 1,
    ADD COLUMN updated_by VARCHAR(64) NULL;

-- 存量文档没有历史记录，统一视为 v1，最后更新人回填为创建人。
UPDATE project_documents SET version = 1, updated_by = created_by;
