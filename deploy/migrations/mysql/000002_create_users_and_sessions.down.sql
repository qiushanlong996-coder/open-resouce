ALTER TABLE document_comments
    DROP FOREIGN KEY fk_document_comments_author;

DROP TABLE IF EXISTS auth_sessions;
DROP TABLE IF EXISTS users;
