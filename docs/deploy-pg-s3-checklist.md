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
S3_ACCESS_KEY_ID=<minio-user>
S3_SECRET_ACCESS_KEY=<minio-password>
S3_BUCKET=files
S3_USE_SSL=false
S3_FORCE_PATH_STYLE=true
S3_REGION=us-east-1
S3_ROOT_PATH=strapi
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
        "StringLike": { "s3:prefix": "strapi/*" }
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

## 5) App-level validation

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
