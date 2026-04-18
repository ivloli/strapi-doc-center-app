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
| `S3_ACCESS_KEY_ID` | MinIO access key |
| `S3_SECRET_ACCESS_KEY` | MinIO secret key |

Then start the service:

```bash
make restart
```

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
        "StringLike": { "s3:prefix": "strapi/*" }
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
