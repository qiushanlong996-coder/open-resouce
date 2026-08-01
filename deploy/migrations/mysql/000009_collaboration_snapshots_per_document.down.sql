-- 回滚前需保证每个项目只剩一条快照，否则恢复单列主键会因重复键失败。
DELETE FROM project_collaboration_snapshots WHERE document_id <> '';

ALTER TABLE project_collaboration_snapshots
    DROP PRIMARY KEY,
    ADD PRIMARY KEY (project_id),
    DROP COLUMN document_id;
