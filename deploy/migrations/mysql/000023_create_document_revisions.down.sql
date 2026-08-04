ALTER TABLE project_documents
    DROP COLUMN updated_by,
    DROP COLUMN version;

DROP TABLE IF EXISTS project_document_revisions;
