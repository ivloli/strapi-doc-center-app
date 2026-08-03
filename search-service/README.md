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

The reconciliation first compares a SHA-256 `sourceVersion` calculated from `docId`, document and menu update times, and URL. Meilisearch receives a separate safe primary key derived from the SHA-256 of `docId`, so Chinese or special-character document IDs remain supported. Only new or changed IDs trigger a second query for title and content. Missing IDs are deleted from Meilisearch. Verify the physical table and column names against the production Strapi v5 database before deployment.

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

`/search/list` returns frontend-ready hits. `path` is the relative frontend route derived from `docId`; `url` is the same route with the request's reverse-proxy domain and protocol. `summary` is a cropped plain-text excerpt, while `highlight.title` and `highlight.summary` contain the same fields with Meilisearch `<mark>` tags. Escape all non-highlight content and render only `<mark>` tags as markup.

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
    "list": [
      {
        "docId": "test-kirito",
        "path": "/test-kirito",
        "url": "https://help.test.starviewcloud.com/test-kirito",
        "summary": "域名防封主要用于...",
        "highlight": {
          "title": "test-kirito",
          "summary": "域名防封主要用于统一<mark>管理</mark>..."
        }
      }
    ],
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

The search service has its own Makefile. Do not run these commands from the Strapi repository root: `cd search-service` first. The commands below target Ubuntu and use systemd.

See [MAKE_USAGE.md](MAKE_USAGE.md) for the complete target-host, operations, and Jenkins command reference.

### Target Host

```bash
git pull
cd search-service
make deploy
```

`make deploy` verifies the existing Go 1.25+ installation, installs required Ubuntu tools and Meilisearch, downloads Go module dependencies, compiles the Linux binary, and registers both systemd units. It does not start them until the secret configuration is complete.

Set a unique Meilisearch master key in `meilisearch.env`:

```bash
MEILI_MASTER_KEY=<long-random-secret>
```

For example, generate the two service secrets with `openssl rand -hex 32`; use one as `MEILI_MASTER_KEY` and another as `SEARCH_SYNC_TOKEN`.

Set `DATABASE_URL`, `SEARCH_SYNC_TOKEN`, and the other Search Service settings in `.env`. Then start Meilisearch and create the restricted service API key:

```bash
make meili-start
make create-meili-key
# Copy the returned "key" value into MEILI_API_KEY in .env.
make start
```

Meilisearch listens on `127.0.0.1:7700` and persists data in `/var/lib/doc-search-meilisearch`. Do not expose its port publicly. Point Nginx or the load balancer at the Go service's `/search` route instead.

Create a non-master Meilisearch API key scoped to `docs_public`, then set it as `MEILI_API_KEY` in `.env`. It needs index creation/read, settings update, document add/get/delete, task read, and search permissions. Restart the Go service after updating `.env`.

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

## Jenkins Release

Jenkins should only build and archive an artifact; it must not invoke `deploy`, `register-service`, or any target using `sudo`.

```bash
cd search-service
make test vet swagger
make release-package TARGET_OS=linux TARGET_ARCH=amd64 RELEASE_TAG="$BUILD_TAG"
make release-checksum TARGET_OS=linux TARGET_ARCH=amd64 RELEASE_TAG="$BUILD_TAG"
```

The artifact is written below `search-service/release/` and contains a Linux binary, Swagger files, environment templates, and deployment documentation. A target host can unpack it, set the two environment files, and use the included Makefile's `register-service` and `register-meili-service` commands.

## Consistency

The lifecycle hook never waits for Go, so it cannot slow or fail a Strapi CRUD request. A successful notification synchronizes the affected document immediately. The hourly metadata reconciliation is the fallback for failed notifications, restarts, hard deletes, and data anomalies; unchanged document bodies are not reread or reindexed.
