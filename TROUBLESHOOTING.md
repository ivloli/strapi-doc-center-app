# Troubleshooting & Operations Log

## 2026-04-18

### 1. Makefile heredoc 语法错误

**问题**：`make` 报错，`remember-config`、`ensure-env`、`write-service` 三个 target 里使用了 heredoc（`<<-EOF`），但 heredoc 内容行没有 tab 缩进，Make 解析器将其识别为新 target，导致语法错误。

**修复**：将所有 heredoc 替换为 `printf '%s\n'` 多行写法。

---

### 2. `make status` 报错 Error 3

**问题**：`systemctl status` 在服务未运行时返回 exit code 3，Make 将其视为命令失败。

**修复**：在 `status` 和 `restart` target 末尾加 `|| true`，忽略非零退出码。同时 `restart` 加了 `sleep 2`，避免查到旧状态。

---

### 3. deploy 后服务用占位符 .env 直接启动

**问题**：`deploy` 链末尾直接执行 `restart`，但此时 `.env` 里还是模板占位符（`your_db_host` 等），导致 Strapi 启动失败并不断重启。

**修复**：从 `deploy` 链里去掉 `restart`，改为在完成后打印提示，引导用户手动填写 `.env` 再执行 `make restart`。

---

### 4. copy-config 覆盖 APP_REPO 里的配置文件

**问题**：`deploy` 链包含 `copy-config`，每次部署都会用 `strapi-dev/templates/config/` 里的模板覆盖 `APP_DIR/config/database.js` 和 `plugins.js`，职责混乱。

**修复**：从 `deploy` 链里去掉 `copy-config`，config 文件由 `APP_REPO` 自己维护。`deploy` 完成提示里加入检查这两个文件是否存在的提醒。

---

### 5. 图片上传后只显示占位符

**问题**：Strapi 是 TypeScript 项目，`config/plugins.ts` 优先级高于 `plugins.js`。`plugins.ts` 返回空对象 `{}`，导致 S3 配置完全被忽略，上传走了默认本地存储。

**修复**：将 S3 配置迁移到 `plugins.ts`，删除冗余的 `plugins.js`。

---

### 6. 清理数据库中的脏 media 记录

上传失败或文件已删除但数据库记录残留时，在 Strapi admin 无法删除，直接操作数据库清理：

```bash
# 查看 files 表
psql "$DATABASE_URL" -c "SELECT id, name, provider, url FROM files LIMIT 20;"

# 按条件删除
psql "$DATABASE_URL" -c "DELETE FROM files WHERE provider = 'local';"
```

---

### 7. MinIO 用户权限排查与配置

**问题**：`dnps-upload-user` 的 policy `dnps-upload-policy` 只有 `s3:GetObject` 和 `s3:PutObject`，缺少 `s3:DeleteObject` 和 `s3:ListBucket`，导致删除报 `AccessDenied`，列举失败导致图片 URL 无法正确生成。

**排查过程**：

```bash
# 安装 MinIO Client（避免与 Midnight Commander 冲突，改名 mcli）
curl -O https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x mc
sudo mv mc /usr/local/bin/mcli

# 查找 MinIO root 账号（从进程环境变量）
cat /proc/$(pgrep minio)/environ | tr '\0' '\n' | grep -i minio

# 用 root 登录
mcli alias set myminio-root http://127.0.0.1:9100 minioadmin minioadmin123

# 查看用户 policy
mcli admin user info myminio-root dnps-upload-user
mcli admin policy info myminio-root dnps-upload-policy
```

**解决方案**：新建独立用户 `strapi-user`，policy 限制在 `files/strapi/` 前缀下：

```bash
# 创建 policy
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
mcli admin user add myminio-root strapi-user 'your_password'
mcli admin policy attach myminio-root strapi-policy --user strapi-user
```

**验证**：

```bash
mcli alias set myminio-strapi http://127.0.0.1:9100 strapi-user 'your_password'
mcli cp /etc/hostname myminio-strapi/files/strapi/test.txt   # 上传
mcli ls myminio-strapi/files/strapi/                          # 查看
mcli cp myminio-strapi/files/strapi/test.txt /tmp/test.txt   # 下载
mcli rm myminio-strapi/files/strapi/test.txt                  # 删除
```

**`.env` 对应配置**：

```
S3_ACCESS_KEY_ID=strapi-user
S3_SECRET_ACCESS_KEY=your_password
S3_ROOT_PATH=strapi
```

---

### 8. 新增 make update

代码更新后重新部署（拉代码 + 构建 + 重启）：

```bash
make update
```

---

### 9. 图片 URL 使用 127.0.0.1 导致浏览器无法访问

**问题**：`S3_ENDPOINT=http://127.0.0.1:9100`，Strapi 生成的图片 URL 也是 `http://127.0.0.1:9100/...`，浏览器访问的是自己本机，无法加载图片。

**修复**：将 `.env` 里 `S3_ENDPOINT` 改为服务器内网 IP：

```
S3_ENDPOINT=http://172.31.36.140:9100
```

同时修复数据库中已有的脏记录：

```bash
psql "$DATABASE_URL" -c "UPDATE files SET url = REPLACE(url, 'http://127.0.0.1:9100', 'http://172.31.36.140:9100');"
```

---

### 10. 冗余的 .js config 文件

**问题**：`config/` 下同时存在 `database.js` + `database.ts`、`plugins.js` + `plugins.ts`，Strapi TypeScript 项目优先加载 `.ts`，`.js` 完全被忽略。

**修复**：删除 `database.js` 和 `plugins.js`，只保留 `.ts` 文件。

---

### 11. 图片上传成功但界面显示不出来（CSP + Referrer-Policy）

**问题**：图片 URL 直接打开正常，但在 Strapi admin 界面显示不出来。浏览器 Console 报 `Referrer-Policy: same-origin`，Strapi 默认的 CSP 也没有放开外部图片来源。

**修复**：在 `config/middlewares.ts` 里覆盖 `strapi::security` 默认配置：

```ts
{
  name: 'strapi::security',
  config: {
    referrerPolicy: {
      policy: 'no-referrer-when-downgrade',
    },
    contentSecurityPolicy: {
      useDefaults: true,
      directives: {
        'img-src': ["'self'", 'data:', 'blob:', '*'],
      },
    },
  },
},
```

同时将 `strapi::cors` 改为允许所有来源（内网测试环境）：

```ts
{
  name: 'strapi::cors',
  config: {
    origin: '*',
  },
},
```
