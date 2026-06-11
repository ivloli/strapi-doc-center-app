# Deploy Checklist: PostgreSQL + MinIO(S3)

## Scope

This note is for deploying this project to a new machine while reusing existing PostgreSQL and MinIO services.

## 1) Required `.env` values

At minimum, confirm these values in deployment `.env`:

```bash
DATABASE_CLIENT=postgres
DATABASE_URL=postgresql://<user>:<password>@<pg-host>:5432/<db>?sslmode=disable&search_path=public&connect_timeout=20&timezone=Asia/Shanghai
DATABASE_SSL=false

S3_ENDPOINT=http://<minio-host>:9100
S3_BASE_URL=https://help.test.starviewcloud.com/help-apis/v1/doc-center-files
S3_ACCESS_KEY_ID=<minio-user>
S3_SECRET_ACCESS_KEY=<minio-password>
S3_BUCKET=files
S3_USE_SSL=false
S3_FORCE_PATH_STYLE=true
S3_REGION=us-east-1
S3_ROOT_PATH=strapi
ADMIN_PATH=/admin
```

## 2) PostgreSQL connectivity checks

Run these commands on the new machine.

### 2.1 Check port reachability

```bash
nc -vz <pg-host> 5432
```

### 2.2 Verify login/database access

```bash
PGPASSWORD='<pg-password>' psql \
  -h <pg-host> -p 5432 -U <pg-user> -d <pg-db> \
  -c 'select now();'
```

Expected: query returns a timestamp.

## 3) MinIO(S3) connectivity checks

### 3.1 Check port reachability

```bash
nc -vz <minio-host> 9100
```

### 3.2 Check MinIO health endpoint

```bash
curl -i 'http://<minio-host>:9100/minio/health/live'
```

Expected: `HTTP/1.1 200 OK`.

### 3.3 Verify S3 credential + bucket access

```bash
AWS_ACCESS_KEY_ID='<minio-user>' \
AWS_SECRET_ACCESS_KEY='<minio-password>' \
AWS_DEFAULT_REGION='us-east-1' \
aws --endpoint-url 'http://<minio-host>:9100' s3 ls s3://files
```

Expected: bucket listing succeeds.

## 4) Create dedicated MinIO user/policy (least privilege)

If you need a fresh account for this app:

### 4.1 Configure MinIO root alias

```bash
mcli alias set myminio-root http://<minio-host>:9100 <root-user> <root-password>
```

### 4.2 Create policy file

```bash
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
```

### 4.3 Create user and attach policy

```bash
mcli admin policy create myminio-root strapi-policy /tmp/strapi-policy.json
mcli admin user add myminio-root strapi-user '<new-strong-password>'
mcli admin policy attach myminio-root strapi-policy --user strapi-user
```

Then update `.env` with this user/password and keep `S3_ROOT_PATH=strapi`.

Note: keeping only `strapi/*` may break `ls s3://files/strapi` style requests from some clients because they send `strapi` or `strapi/` as the list prefix.

## 5) App-level validation

### 5.0 Reverse-proxy validation for same-domain admin and media

If `9100` is not publicly exposed, keep `S3_ENDPOINT` on the MinIO LAN address and publish files via HTTPS reverse proxy instead. Recommended `nginx` locations:

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
    proxy_pass http://<minio-host>:9100/files/;
    proxy_http_version 1.1;
    proxy_set_header Host <minio-host>:9100;
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

Open the admin from `https://help.test.starviewcloud.com/admin`, not `http://<public-ip>:1337`, otherwise browser-side media preview/download requests become cross-origin.

After `npm ci`, start Strapi and verify no DB/S3 initialization errors:

```bash
npm run build
npm run start
```

Check logs for:
- database connection failure
- upload provider initialization failure

## 6) Security notes

- Do not paste real DB passwords or S3 secrets in tickets/chat.
- Rotate credentials immediately if exposed.
- Use dedicated MinIO users per environment.
