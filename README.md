# Strapi Doc Center

A Strapi v5 CMS for documentation, with PostgreSQL and MinIO (S3-compatible) storage.

## Requirements

- Ubuntu 20.04+
- Node.js 20+ (auto-installed by `make deploy`)
- PostgreSQL
- MinIO or S3-compatible storage

## Quick Start

Clone the repo directly to your target directory, then deploy:

```bash
git clone git@github.com:ivloli/strapi-doc-center-app.git /opt/strapi-doc-center-app
cd /opt/strapi-doc-center-app
make deploy
```

`make deploy` will:
1. Install Node.js 20 if not present
2. Generate a `.env` template if none exists
3. Install npm dependencies
4. Build the admin panel
5. Register and enable a systemd service

After deploy completes, edit `.env` with real values:

```bash
nano .env
```

Key values to fill in:

| Variable | Description |
|---|---|
| `DATABASE_URL` | PostgreSQL connection string |
| `APP_KEYS` | Comma-separated random strings |
| `ADMIN_JWT_SECRET` | Random secret |
| `API_TOKEN_SALT` | Random secret |
| `TRANSFER_TOKEN_SALT` | Random secret |
| `JWT_SECRET` | Random secret |
| `S3_ENDPOINT` | MinIO endpoint (use server LAN IP, not 127.0.0.1) |
| `S3_BASE_URL` | Public file base URL, usually the HTTPS reverse-proxy path |
| `S3_ACCESS_KEY_ID` | MinIO access key |
| `S3_SECRET_ACCESS_KEY` | MinIO secret key |

Then start the service:

```bash
make restart
```

If you front Strapi with `nginx`, use `nginx/strapi-doc-center.conf` as the template. For same-domain admin and MinIO file access, keep `ADMIN_PATH=/admin` and proxy the admin, API, and file paths separately.

Recommended `.env` values for MinIO behind HTTPS reverse proxy:

```bash
ADMIN_PATH=/admin
S3_ENDPOINT=http://<minio-lan-ip>:9100
S3_BASE_URL=https://help.test.starviewcloud.com/help-apis/v1/doc-center-files
S3_BUCKET=files
S3_ROOT_PATH=strapi
```

Recommended `nginx` locations in the `help.test.starviewcloud.com` server:

```nginx
location ^~ /help-apis/v1/doc-center/ {
    proxy_pass http://127.0.0.1:1337/api/;
    proxy_http_version 1.1;
    include /etc/nginx/proxy_params;
    proxy_set_header X-Forwarded-Host $host;
    proxy_set_header X-Forwarded-Port $server_port;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_redirect off;
}

location ^~ /help-apis/v1/doc-center-files/ {
    proxy_pass http://<minio-lan-ip>:9100/files/;
    proxy_http_version 1.1;
    proxy_set_header Host <minio-lan-ip>:9100;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Connection "";
    proxy_connect_timeout 5s;
    proxy_send_timeout 60s;
    proxy_read_timeout 60s;
    proxy_redirect off;
}

location = /admin {
    proxy_pass http://127.0.0.1:1337/admin;
    proxy_http_version 1.1;
    include /etc/nginx/proxy_params;
    proxy_set_header X-Forwarded-Host $host;
    proxy_set_header X-Forwarded-Port $server_port;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_redirect off;
}

location ^~ /admin/ {
    proxy_pass http://127.0.0.1:1337/admin/;
    proxy_http_version 1.1;
    include /etc/nginx/proxy_params;
    proxy_set_header X-Forwarded-Host $host;
    proxy_set_header X-Forwarded-Port $server_port;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_redirect off;
}

location ^~ /content-manager/ {
    proxy_pass http://127.0.0.1:1337/content-manager/;
    proxy_http_version 1.1;
    include /etc/nginx/proxy_params;
    proxy_set_header X-Forwarded-Host $host;
    proxy_set_header X-Forwarded-Port $server_port;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_redirect off;
}

location ^~ /upload/ {
    proxy_pass http://127.0.0.1:1337/upload/;
    proxy_http_version 1.1;
    include /etc/nginx/proxy_params;
    proxy_set_header X-Forwarded-Host $host;
    proxy_set_header X-Forwarded-Port $server_port;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_redirect off;
}

location ^~ /users-permissions/ {
    proxy_pass http://127.0.0.1:1337/users-permissions/;
    proxy_http_version 1.1;
    include /etc/nginx/proxy_params;
    proxy_set_header X-Forwarded-Host $host;
    proxy_set_header X-Forwarded-Port $server_port;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_redirect off;
}

location ^~ /content-type-builder/ {
    proxy_pass http://127.0.0.1:1337/content-type-builder/;
    proxy_http_version 1.1;
    include /etc/nginx/proxy_params;
    proxy_set_header X-Forwarded-Host $host;
    proxy_set_header X-Forwarded-Port $server_port;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_redirect off;
}
```

Open the admin from the domain URL, not `http://<public-ip>:1337`, to avoid browser-side cross-origin requests for media preview/download.

## Make Commands

| Command | Description |
|---|---|
| `make deploy` | First-time setup: install deps, build, register systemd service |
| `make update` | Pull latest code, rebuild, restart |
| `make restart` | Restart the systemd service |
| `make status` | Show service status |
| `make logs` | Tail service logs |
| `make switch-dev` | Switch to develop mode (hot reload) |
| `make switch-prod` | Build and switch to production mode |
| `make show-config` | Show current config values |
| `make uninstall` | Stop and remove the systemd service |

## MinIO Setup

The app uses a dedicated MinIO user scoped to the `files/strapi/` prefix.

```bash
# Install mcli (MinIO Client)
curl -O https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x mc && sudo mv mc /usr/local/bin/mcli

# Connect as root
mcli alias set myminio-root http://<minio-host>:9100 <root-user> <root-password>

# Create policy
cat > /tmp/strapi-policy.json << 'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
      "Resource": "arn:aws:s3:::files/strapi/*"
    },
    {
      "Effect": "Allow",
      "Action": "s3:ListBucket",
      "Resource": "arn:aws:s3:::files",
      "Condition": {
        "StringLike": {
          "s3:prefix": ["strapi", "strapi/", "strapi/*"]
        }
      }
    }
  ]
}
EOF

mcli admin policy create myminio-root strapi-policy /tmp/strapi-policy.json
mcli admin user add myminio-root strapi-user '<your-password>'
mcli admin policy attach myminio-root strapi-policy --user strapi-user
```

Set `S3_ROOT_PATH=strapi` in `.env` to scope uploads to this prefix.

If MinIO is not publicly reachable, also set `S3_BASE_URL` to the reverse-proxy file path and keep `S3_ENDPOINT` pointing to the LAN MinIO address.

Use all three `s3:prefix` variants above because some S3 clients list with `strapi` or `strapi/`, not only `strapi/*`.

## Development Mode

```bash
make switch-dev   # enables hot reload
make switch-prod  # builds and switches back to production
```

## Updating

After pushing new code to the repo:

```bash
make update
```

This pulls the latest code, rebuilds the admin panel, and restarts the service.

## Uninstall

```bash
make uninstall
```

Stops and removes the systemd service. The app directory and `.env` are preserved.

## Troubleshooting Notes

- CORS / same-domain API gateway case: `docs/cors-resolution-2026-05.md`
- New machine deploy checks (PostgreSQL + MinIO/S3): `docs/deploy-pg-s3-checklist.md`
- Chinese full deploy/ops guide: `docs/zh-deploy-and-ops-guide.md`
