# MySQL migrations

Migration files are ordered by their six-digit prefix. Apply each `*.up.sql`
file once, in ascending order. Rollbacks use the matching `*.down.sql` file in
descending order.

The first migration creates `document_comments`. The second migration creates
`users` and `auth_sessions`, then adds the deferred comment author foreign key.
The third migration creates user-scoped persistent project favorites.
The fourth migration creates single-use password reset tokens. Only token hashes
are stored.
The fifth migration creates author-managed project drafts, review event history,
and OSS object keys for project covers, documents, and code packages.
The document foreign key remains deferred until the document catalog moves out
of the seed repository.

The migrations support the production MySQL 5.7 baseline and MySQL 8.0. State
and timestamp consistency is enforced by repository transactions because MySQL
5.7 parses but does not enforce `CHECK` constraints.

For local verification:

```bash
mysql --protocol=TCP -h 127.0.0.1 -u "$MYSQL_USER" -p \
  "$MYSQL_DATABASE" < deploy/migrations/mysql/000001_create_document_comments.up.sql

mysql --protocol=TCP -h 127.0.0.1 -u "$MYSQL_USER" -p \
  "$MYSQL_DATABASE" < deploy/migrations/mysql/000002_create_users_and_sessions.up.sql

mysql --protocol=TCP -h 127.0.0.1 -u "$MYSQL_USER" -p \
  "$MYSQL_DATABASE" < deploy/migrations/mysql/000003_create_project_favorites.up.sql

mysql --protocol=TCP -h 127.0.0.1 -u "$MYSQL_USER" -p \
  "$MYSQL_DATABASE" < deploy/migrations/mysql/000004_create_password_reset_tokens.up.sql

mysql --protocol=TCP -h 127.0.0.1 -u "$MYSQL_USER" -p \
  "$MYSQL_DATABASE" < deploy/migrations/mysql/000005_create_managed_projects.up.sql

mysql --protocol=TCP -h 127.0.0.1 -u "$MYSQL_USER" -p \
  "$MYSQL_DATABASE" -e "SHOW CREATE TABLE document_comments"
```

Do not put database passwords in migration files, shell history, or committed
environment files.
