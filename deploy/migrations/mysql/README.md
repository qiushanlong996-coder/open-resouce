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

## Integration test database

Integration tests write and delete real rows. They must never run against the
production database: doing so previously left 25 test accounts and 2 fake
projects behind, and the fake projects appeared in the public catalog because
they carried `published` status.

Two independent safeguards are in place:

1. **Code guard** — `requireTestDatabase` refuses to run when the database name
   does not end with `_test`, so a misconfigured DSN fails loudly instead of
   silently polluting production data.
2. **Credential isolation** — the test account is granted privileges only on
   the test schema, so even a wrong DSN cannot write to production.

Set up the test database once per environment:

```sql
CREATE DATABASE open_resouce_test
  CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

CREATE USER 'open_resouce_test'@'<app-server-ip>'
  IDENTIFIED BY '<test-only-secret>';
GRANT ALL PRIVILEGES ON `open_resouce_test`.*
  TO 'open_resouce_test'@'<app-server-ip>';
FLUSH PRIVILEGES;
```

Then apply every `*.up.sql` file in ascending order to `open_resouce_test`,
exactly as for production.

Run the integration tests with:

```sh
MYSQL_TEST_DATABASE_URL='mysql://open_resouce_test:<secret>@<host>:3306/open_resouce_test' \
REDIS_TEST_URL='<redis-url>' \
go test ./services/gateway/
```

`TEST_MYSQL_DSN` is the former variable name and is still accepted, but
`MYSQL_TEST_DATABASE_URL` takes precedence. Both the `mysql://` URL form and the
native go-sql-driver DSN form are supported.

Tests skip (rather than fail) when no test database is configured, so unit tests
still run in environments without MySQL.
