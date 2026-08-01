# OpenResource Open Write API

A machine-friendly guide for an external AI agent (skill / MCP tool) to publish a
project to OpenResource programmatically, including uploading a parsed code archive.

All Open API endpoints are under `/api/v1/open/*` and authenticate with a Bearer
API key. They act **as the key's owner user** — the admin who issued the key. Any
project or document you create is owned by that user and shows up in that user's
author dashboard.

## Authentication

- Obtain an API key from the **admin console** (Admin → API Keys → issue). The
  plaintext key (format `ork_<hex>`) is shown **once** at creation; store it
  securely. Only the SHA-256 digest is persisted server-side.
- Send it on every request:

  ```
  Authorization: Bearer ork_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
  ```

- Missing/malformed header or an unknown/revoked key → `401`
  (`api_key_required` / `api_key_invalid`).
- If the key owner is banned, all write endpoints → `403` (`user_banned`).

Errors follow the platform envelope:

```json
{ "error": { "code": "invalid_project", "message": "..." }, "request_id": "..." }
```

Successful responses wrap the payload as `{ "data": ..., "request_id": "..." }`.

## Publish flow

```
presign  →  PUT archive to OSS  →  create draft  →  submit for review  →  (admin approves) → published
```

Publishing always passes through the platform's **admin review gate**. Submitting
only moves the project to `pending_review`; it is never auto-published. Statuses:
`draft → pending_review → published` (or `rejected`, which you may edit/resubmit).

Code parsing/packaging is done client-side by the agent. The platform just accepts
the uploaded archive (`.zip`, `.gz`, `.tgz`, `.tar`) and references it by object key.

### 1. Presign an upload — `POST /api/v1/open/files/presign`

Request one presigned PUT per file. `kind` is `image` (cover), `document`
(pdf/md/txt), or `code` (zip/gz/tgz/tar).

```bash
curl -sS https://api.openresource.cn/api/v1/open/files/presign \
  -H "Authorization: Bearer $ORK_KEY" \
  -H "Content-Type: application/json" \
  -d '{"filename":"parsed.zip","content_type":"application/zip","size":4096,"kind":"code"}'
```

Response:

```json
{
  "data": {
    "object_key": "uploads/user-<id>/2026/08/<random>.zip",
    "method": "PUT",
    "url": "https://<bucket>.<endpoint>/uploads/...?<v4-signature>",
    "headers": { "Content-Type": "application/zip" },
    "expires_at": "2026-08-01T12:10:00Z"
  },
  "request_id": "..."
}
```

Size limits: `image` ≤ 10 MiB, `document` ≤ 50 MiB, `code` ≤ 500 MiB. The
`content_type` and file extension must match the `kind` or you get `422`
(`invalid_file`). The URL expires in 10 minutes and forbids overwrite.

### 2. Upload the archive to OSS

PUT the file bytes directly to the presigned `url`, echoing the returned `headers`:

```bash
curl -sS -X PUT "$PRESIGNED_URL" \
  -H "Content-Type: application/zip" \
  --data-binary @parsed.zip
```

Keep the `object_key` — you reference it when creating the project.

### 3. Create the draft — `POST /api/v1/open/projects`

```bash
curl -sS https://api.openresource.cn/api/v1/open/projects \
  -H "Authorization: Bearer $ORK_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "slug": "agent-demo",
    "name": "Agent Demo",
    "summary": "A project published programmatically via the Open API.",
    "description": "# Overview\n\nMarkdown body, at least 20 characters long ...",
    "category": "Coding Agent",
    "tags": ["Agent", "MCP"],
    "tech_stack": ["Go", "React"],
    "license": "MIT",
    "repository_url": "https://github.com/example/agent",
    "current_version": "0.1.0",
    "code_object_key": "uploads/user-<id>/2026/08/<random>.zip",
    "document_object_key": "",
    "cover_object_key": ""
  }'
```

Returns `201` with the created project (`"status": "draft"`, `owner_id` = key owner).

### 4. Submit for review — `POST /api/v1/open/projects/{id}/submit`

```bash
curl -sS https://api.openresource.cn/api/v1/open/projects/$PROJECT_ID/submit \
  -X POST -H "Authorization: Bearer $ORK_KEY"
```

Returns `200` with `"status": "pending_review"`. Only the key owner can submit
their own project (otherwise `404`). An admin then approves (→ `published`) or
rejects (→ `rejected`, editable + resubmittable).

### 5. (Optional) Add a knowledge-base document — `POST /api/v1/open/projects/{id}/documents`

```bash
curl -sS https://api.openresource.cn/api/v1/open/projects/$PROJECT_ID/documents \
  -H "Authorization: Bearer $ORK_KEY" \
  -H "Content-Type: application/json" \
  -d '{"slug":"getting-started","title":"Getting Started","markdown":"# Hi\n..."}'
```

## Field constraints (create draft)

| Field                 | Rule                                                        |
| --------------------- | ----------------------------------------------------------- |
| `slug`                | `^[a-z0-9]+(?:-[a-z0-9]+)*$`, ≤ 80, unique across platform   |
| `name`                | 2–120 chars                                                 |
| `summary`             | 10–300 chars                                                |
| `description`         | 20–50000 chars (Markdown)                                   |
| `category`            | non-empty, ≤ 80                                             |
| `tags`, `tech_stack`  | ≤ 10 items each                                             |
| `license`             | non-empty, ≤ 40                                             |
| `current_version`     | non-empty, ≤ 40                                             |
| `repository_url`      | ≤ 500                                                       |
| `*_object_key`        | empty, or an `uploads/user-<owner-id>/...` key you presigned |

Invalid input → `422` (`invalid_project`). An object key not owned by the key
owner → `422` (`invalid_project_file`). Duplicate slug → `409`
(`project_slug_exists`). All string fields are trimmed; `slug` is lowercased.

Document fields: `slug` (`^[a-z0-9-]+$`, ≤ 160), `title` (1–200), `markdown`
(≤ 200000), optional `parent_id`.

## Endpoint summary

| Method | Path                                     | Purpose                    |
| ------ | ---------------------------------------- | -------------------------- |
| GET    | `/api/v1/open/projects`                  | List published projects    |
| POST   | `/api/v1/open/projects`                  | Create a draft project     |
| POST   | `/api/v1/open/projects/{id}/submit`      | Submit draft for review    |
| POST   | `/api/v1/open/projects/{id}/documents`   | Add a knowledge-base doc   |
| POST   | `/api/v1/open/files/presign`             | Presign an OSS upload      |

See `api/openapi/openapi.json` (the `/api/v1/open/*` paths) for the machine-readable
contract.
