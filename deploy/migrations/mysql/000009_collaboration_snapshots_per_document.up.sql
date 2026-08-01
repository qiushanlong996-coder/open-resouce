-- 协作快照下沉到文档级：同一项目下每篇文档需要独立的 Yjs 房间与快照，
-- 否则多篇文档同时协作会共用一份快照而互相串内容。
-- document_id 为空串表示项目正文（尚未建文档的旧项目仍走这条路径），
-- 因此已有数据无需迁移即可继续使用。
ALTER TABLE project_collaboration_snapshots
    ADD COLUMN document_id VARCHAR(64) NOT NULL DEFAULT '' AFTER project_id,
    DROP PRIMARY KEY,
    ADD PRIMARY KEY (project_id, document_id);
