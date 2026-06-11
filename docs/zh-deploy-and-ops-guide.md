# Strapi 项目中文部署与运维手册

本文面向第一次接手该项目的同学，覆盖从 `clone` 到启动、日志、PG/MinIO 账号权限配置，以及 `Makefile` 全量命令和变量用法。

## 1. 部署目标与前置条件

- 目标系统：Ubuntu 20.04+
- 项目目录：`/opt/strapi-doc-center-app`
- 运行方式：`systemd` + `make`
- 外部依赖：
  - PostgreSQL（RDS 或自建）
  - MinIO（S3 协议）

## 2. 克隆项目

```bash
cd /opt
sudo git clone git@github.com:ivloli/strapi-doc-center-app.git /opt/strapi-doc-center-app
cd /opt/strapi-doc-center-app
```

如果服务器没有配置 GitHub SSH Key，可以改用 HTTPS：

```bash
sudo git clone https://github.com/ivloli/strapi-doc-center-app.git /opt/strapi-doc-center-app
```

## 3. 安装依赖工具（Node、mcli、psql/pgcli）

### 3.1 Node.js 20（项目运行必须）

```bash
sudo apt update
sudo apt install -y ca-certificates curl gnupg git make
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt install -y nodejs
node -v
npm -v
```

### 3.2 MinIO 客户端（mcli）

> 注意：避免与系统 `mc` 命令冲突，这里统一安装成 `mcli`。

```bash
curl -fsSL -o mcli https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x mcli
sudo mv mcli /usr/local/bin/mcli
mcli --version
```

### 3.3 PostgreSQL 客户端（psql）

```bash
sudo apt install -y postgresql-client
psql --version
```

### 3.4 pgcli（可选，类似 mycli 的交互体验）

```bash
sudo apt install -y pipx
pipx ensurepath
pipx install pgcli
~/.local/bin/pgcli --version
```

## 4. `.env` 如何配置（新人可照抄）

### 4.1 先生成基础模板

如果项目里已有 `.env.test`：

```bash
cp /opt/strapi-doc-center-app/.env.test /opt/strapi-doc-center-app/.env
```

如果没有，就先执行：

```bash
cd /opt/strapi-doc-center-app
make ensure-env
```

### 4.2 必填项说明（按行理解）

```env
HOST=0.0.0.0
PORT=1337
STRAPI_RUN_MODE=develop
NODE_ENV=development

APP_KEYS=change1,change2,change3,change4
API_TOKEN_SALT=change_me_api_token_salt
ADMIN_JWT_SECRET=change_me_admin_jwt_secret
TRANSFER_TOKEN_SALT=change_me_transfer_token_salt
JWT_SECRET=change_me_jwt_secret

DATABASE_CLIENT=postgres
DATABASE_URL=postgresql://<db_user>:<db_password>@<db_host>:5432/<db_name>?search_path=public&connect_timeout=20&timezone=Asia/Shanghai
DATABASE_SSL=true
DATABASE_SSL_REJECT_UNAUTHORIZED=false

S3_ENDPOINT=http://<minio_host>:9100
S3_ACCESS_KEY_ID=<minio_access_key>
S3_SECRET_ACCESS_KEY=<minio_secret_key>
S3_BUCKET=files
S3_USE_SSL=false
S3_FORCE_PATH_STYLE=true
S3_SIGNED_URL_EXPIRES=600
S3_REGION=us-east-1
S3_ROOT_PATH=strapi

ADMIN_PATH=/help-apis/v1/admin
CORS_ORIGIN=https://help.test.starviewcloud.com,https://help.dev.starviewcloud.com,http://localhost:4006,http://127.0.0.1:4006
```

### 4.3 新人最容易踩的点

- **数据库 SSL 报错**：
  - 若出现 `self-signed certificate in certificate chain`，通常需要：
    - `DATABASE_SSL=true`
    - `DATABASE_SSL_REJECT_UNAUTHORIZED=false`
- **不要把生产密钥提交到 git**：`.env` 已在 `.gitignore` 内。
- **ADMIN_PATH 要和 nginx 前缀一致**，否则后台页面会能开但接口 404/405。

## 5. PostgreSQL 账号、数据库、权限

### 5.1 连通性检查

```bash
nc -vz <pg-host> 5432
PGPASSWORD='<db-pass>' psql -h <pg-host> -p 5432 -U <db-user> -d postgres -c 'select now();'
```

### 5.2 创建 Strapi 专用账号与库（管理员执行）

```bash
PGPASSWORD='<postgres-admin-pass>' psql -h <pg-host> -p 5432 -U postgres -d postgres -c "CREATE ROLE strapi_admin WITH LOGIN PASSWORD 'YourStrongPass123!';"
PGPASSWORD='<postgres-admin-pass>' psql -h <pg-host> -p 5432 -U postgres -d postgres -c "CREATE DATABASE strapi_docs;"
PGPASSWORD='<postgres-admin-pass>' psql -h <pg-host> -p 5432 -U postgres -d postgres -c "GRANT ALL PRIVILEGES ON DATABASE strapi_docs TO strapi_admin;"
PGPASSWORD='<postgres-admin-pass>' psql -h <pg-host> -p 5432 -U postgres -d strapi_docs -c "GRANT ALL ON SCHEMA public TO strapi_admin; GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO strapi_admin; GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO strapi_admin; ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO strapi_admin; ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO strapi_admin;"
```

### 5.3 验证新账号

```bash
PGPASSWORD='YourStrongPass123!' psql -h <pg-host> -p 5432 -U strapi_admin -d strapi_docs -c "SELECT current_user, current_database(), now();"
```

## 6. MinIO 账号、目录（前缀）、权限

### 6.1 连通性检查

```bash
nc -vz <minio-host> 9100
curl -i http://<minio-host>:9100/minio/health/live
```

### 6.2 用 root/admin 创建 Strapi 最小权限账号

```bash
mcli alias set myminio-root http://<minio-host>:9100 <root-user> '<root-password>'

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

mcli admin policy create myminio-root strapi-policy /tmp/strapi-policy.json || true
mcli admin user add myminio-root strapi-user 'YourStrongPass123!'
mcli admin policy attach myminio-root strapi-policy --user strapi-user
```

### 6.3 验证目录（前缀）权限

```bash
mcli alias set myminio-strapi http://<minio-host>:9100 strapi-user 'YourStrongPass123!'
echo ok > /tmp/.keep
mcli cp /tmp/.keep myminio-strapi/files/strapi/.keep
mcli ls myminio-strapi/files/strapi/
mcli cat myminio-strapi/files/strapi/.keep
mcli rm myminio-strapi/files/strapi/.keep
```

> S3/MinIO 的“目录”是前缀，不是实体目录。没有对象时前缀可不显示，这是正常现象。

补充说明：如果策略里只写 `strapi/*`，部分客户端在执行 `ls files/strapi` 时会携带 `strapi` 或 `strapi/` 作为前缀，导致 `ListBucket` 被拒绝；上面的三种前缀写法兼容性更好。

## 7. 使用 Make 部署与运行

### 7.1 首次部署

```bash
cd /opt/strapi-doc-center-app
sudo make deploy
```

`make deploy` 会依次执行：
- `install-node`
- `ensure-env`
- `install-deps`
- `build`
- `write-service`

### 7.2 启动/重启

```bash
cd /opt/strapi-doc-center-app
sudo make restart
```

### 7.3 切换开发模式（可热更新）

```bash
cd /opt/strapi-doc-center-app
sudo make switch-dev
```

### 7.4 切换生产模式（构建后启动）

```bash
cd /opt/strapi-doc-center-app
sudo make switch-prod
```

### 7.5 查看状态与日志

```bash
cd /opt/strapi-doc-center-app
sudo make status
sudo make logs
```

### 7.6 拉取更新并重启

```bash
cd /opt/strapi-doc-center-app
sudo make update
```

## 8. Makefile 所有命令说明

### 8.1 命令列表

- `make help`：打印命令帮助
- `make show-config`：显示当前变量值（`APP_DIR`/`APP_PORT`/`APP_USER`/`SERVICE_NAME`）
- `make install-node`：安装 Node.js 20（若未安装）
- `make ensure-env`：若 `.env` 不存在则生成默认模板
- `make install-deps`：安装 npm 依赖（含 `pg` 与 S3 provider）
- `make build`：构建 Strapi Admin
- `make write-service`：写入并启用 systemd 服务
- `make restart`：重启服务
- `make status`：查看服务状态
- `make logs`：跟随日志
- `make switch-dev`：切到开发模式并重启
- `make switch-prod`：切到生产模式、构建并重启
- `make run`：前台执行 `npm run start`（不走 systemd）
- `make update`：`git pull + build + restart`
- `make deploy`：首次部署组合命令
- `make uninstall`：卸载服务（停止、disable、删 service 文件）
- `make clean`：等同 `uninstall`

### 8.2 变量说明与示例

Makefile 支持以下变量：

- `APP_DIR`：项目目录（默认当前路径）
- `APP_PORT`：`.env` 模板中的默认端口（默认 `1337`）
- `APP_USER`：systemd 运行用户（默认当前用户）
- `SERVICE_NAME`：systemd 服务名（默认 `strapi-doc-center`）

示例 1：自定义服务名

```bash
cd /opt/strapi-doc-center-app
sudo make SERVICE_NAME=strapi-doc-center-test write-service
sudo make SERVICE_NAME=strapi-doc-center-test restart
```

示例 2：部署前查看变量

```bash
cd /opt/strapi-doc-center-app
make show-config
```

示例 3：首次部署时覆盖端口/用户

```bash
cd /opt/strapi-doc-center-app
sudo make APP_PORT=14000 APP_USER=www-data deploy
```

> 注意：`APP_PORT` 只影响 `ensure-env` 生成模板时的端口；若 `.env` 已存在，以 `.env` 实际内容为准。

## 9. 常见故障速查

### 9.1 `no pg_hba.conf entry ... no encryption`

- 原因：数据库要求 SSL，连接串用了非 SSL。
- 处理：
  - `DATABASE_SSL=true`
  - `DATABASE_SSL_REJECT_UNAUTHORIZED=false`
  - `DATABASE_URL` 不要强制 `sslmode=disable`

### 9.2 `self-signed certificate in certificate chain`

- 原因：数据库证书链为私有/自签，Node 默认校验失败。
- 处理：`DATABASE_SSL_REJECT_UNAUTHORIZED=false`

### 9.3 Admin 页面打开但接口 404/405

- 原因：`ADMIN_PATH` 与 nginx 路径前缀不一致，或请求落到前端兜底路由。
- 处理：
  - 统一 `ADMIN_PATH` 与代理前缀
  - Strapi 路由规则放在 `location /` 前

## 10. 推荐交接清单

交接前请确认：

- `.env` 已按目标环境填完
- PG：可连接、库存在、账号有 schema 权限
- MinIO：账号可 `list/get/put/delete` 到 `files/strapi/*`
- `make switch-dev` / `make switch-prod` 可正常切换
- `make logs` 无持续报错
