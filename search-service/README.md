# Documentation Search Service

This is a standalone Go service. It does not call or modify Strapi: PostgreSQL remains the source of truth, and Meilisearch is a rebuildable search read model.

## Scope

- Indexes only documents with a non-null `docs.published_at` and an associated published menu.
- Uses `menus.value` as the document URL.
- Strapi lifecycle notifications trigger an idempotent single-document sync.
- Reconciles source and index metadata on startup and at `SYNC_INTERVAL`.
- Provides public `GET /search`, `GET /healthz`, and a private `POST /internal/sync` endpoint.

The source query deliberately models the current Strapi schema:

```sql
SELECT d.doc_id, d.updated_at, m.value, m.updated_at
FROM docs d
JOIN menus m ON m.doc_id = d.doc_id
WHERE d.published_at IS NOT NULL
  AND m.published_at IS NOT NULL;
```

The reconciliation first compares a SHA-256 `sourceVersion` calculated from `docId`, document and menu update times, and URL. Only new or changed IDs trigger a second query for title and content. Missing IDs are deleted from Meilisearch. Verify the physical table and column names against the production Strapi v5 database before deployment.

## Local Run

```bash
cp .env.example .env
# Set DATABASE_URL, MEILI_URL, and MEILI_API_KEY.
make run
```

Run tests and build the service independently from Strapi:

```bash
make test
make build
```

## Search API

```text
POST /search/list
GET /healthz
POST /internal/sync
```

`/search/list` accepts a camelCase JSON request body and returns Meilisearch-formatted hits. Highlighted `title` and cropped `content` live in each hit's `_formatted` object. Treat the `<mark>` tags as the only allowed markup and escape all other content before rendering.

```json
{
  "keyword": "快速开始",
  "pagination": {
    "page": 1,
    "pageSize": 20
  }
}
```

All endpoints use the platform response envelope. The search result shape is:

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "list": [],
    "pagination": {
      "page": 1,
      "pageSize": 20,
      "total": "0"
    }
  }
}
```

## Swagger

Generate OpenAPI documents and update the embedded Swagger metadata:

```bash
make swagger
```

The script runs the pinned `swaggo/swag` generator without requiring a globally installed binary. Generated files are written to `docs/swagger.json`, `docs/swagger.yaml`, and `docs/docs.go`. When the service is running, browse the interactive API documentation at `/swagger/index.html`.

## Deployment

Deploy the search service and Meilisearch separately from Strapi. The commands below target Ubuntu and use systemd.

### 1. Install Go and Meilisearch

```bash
cd search-service
cp .env.example .env
make install-go
make install-meilisearch
```

Create `meilisearch.env` beside the Makefile with a unique master key:

```bash
MEILI_MASTER_KEY=<long-random-secret>
```

For example, generate the two service secrets with `openssl rand -hex 32`; use one as `MEILI_MASTER_KEY` and another as `SEARCH_SYNC_TOKEN`.

Then install and start both services:

```bash
make write-meili-service
make meili-restart
make build
make write-service
make restart
```

Meilisearch listens on `127.0.0.1:7700` and persists data in `/var/lib/doc-search-meilisearch`. Do not expose its port publicly. Point Nginx or the load balancer at the Go service's `/search` route instead.

Create a non-master Meilisearch API key scoped to `docs_public`, then set it as `MEILI_API_KEY` in `.env`. It needs index creation/read, settings update, document add/get/delete, task read, and search permissions. Restart the Go service after updating `.env`.

```bash
make create-meili-key
# Copy the returned "key" value into MEILI_API_KEY in .env.
```

Set the same random `SEARCH_SYNC_TOKEN` in both `search-service/.env` and the root Strapi `.env`. Set the root `SEARCH_SYNC_URL` to `http://127.0.0.1:8080/internal/sync` when both services share a host, then restart Strapi.

Expose only the Go endpoint through Nginx or the load balancer:

```nginx
location /search {
  proxy_pass http://127.0.0.1:8080;
  proxy_http_version 1.1;
  proxy_set_header Host $host;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}
```

Use a dedicated PostgreSQL role. It needs `SELECT` access to `docs` and `menus`; it does not need access to Strapi write tables.

## Consistency

The lifecycle hook never waits for Go, so it cannot slow or fail a Strapi CRUD request. A successful notification synchronizes the affected document immediately. The hourly metadata reconciliation is the fallback for failed notifications, restarts, hard deletes, and data anomalies; unchanged document bodies are not reread or reindexed.
