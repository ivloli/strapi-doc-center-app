# Search Service Make Usage

所有命令必须在 `search-service/` 目录执行。该 Makefile 与仓库根目录的 Strapi Makefile 完全独立。

```bash
cd search-service
make help
```

## Target Host Deployment

目标机已安装 Go 1.25+ 时，拉取代码后执行：

```bash
git pull
cd search-service
make deploy
```

`make deploy` 会：

- 校验 Go 版本。
- 安装 Ubuntu 所需工具：`ca-certificates`、`curl`、`tar`。
- 下载并校验 Go 模块依赖。
- 创建 `.env` 与 `meilisearch.env` 模板文件，已有文件不会覆盖。
- 安装 Meilisearch 二进制。
- 交叉编译 Linux/amd64 的 Go 服务二进制。
- 写入并启用 Go 与 Meilisearch 的 systemd 服务，但不会立即启动。

编辑 `meilisearch.env`：

```bash
MEILI_MASTER_KEY=<long-random-secret>
```

编辑 `.env`，至少填写：

```bash
DATABASE_URL=<search-readonly-postgresql-url>
MEILI_API_KEY=<created-after-meili-starts>
SEARCH_SYNC_TOKEN=<same-value-as-strapi>
```

先启动 Meilisearch 并创建受限 API Key：

```bash
make meili-start
make create-meili-key
```

将返回 JSON 中的 `key` 写入 `.env` 的 `MEILI_API_KEY`，再启动 Go 服务：

```bash
make start
```

Strapi 根目录 `.env` 需要使用相同的同步 Token：

```bash
SEARCH_SYNC_URL=http://127.0.0.1:8080/internal/sync
SEARCH_SYNC_TOKEN=<same-value-as-search-service>
```

修改 Strapi 配置后，回到仓库根目录重启 Strapi 服务。

## Daily Operations

```bash
make status          # Go 服务状态
make logs            # 跟踪 Go 服务日志
make restart         # 重启 Go 服务
make stop            # 停止 Go 服务

make meili-status    # Meilisearch 状态
make meili-logs      # 跟踪 Meilisearch 日志
make meili-restart   # 重启 Meilisearch
make meili-stop      # 停止 Meilisearch
```

重新生成 API 文档：

```bash
make swagger
```

服务运行后，可通过 `/swagger/index.html` 查看 Swagger UI。

## Local Validation

```bash
make test
make vet
make swagger
make build
```

`make build` 默认输出 Linux/amd64 二进制到 `bin/doc-search`。可通过变量指定其他目标平台：

```bash
make build TARGET_OS=linux TARGET_ARCH=arm64
```

## Jenkins Release

Jenkins 只负责测试、编译和归档，不执行任何使用 `sudo` 的部署命令。

```bash
make test vet swagger
make release-package TARGET_OS=linux TARGET_ARCH=amd64 RELEASE_TAG="$BUILD_TAG"
make release-checksum TARGET_OS=linux TARGET_ARCH=amd64 RELEASE_TAG="$BUILD_TAG"
```

发布包位于：

```text
release/doc-search-offline-linux-amd64-<release-tag>.tar.gz
```

其中包含 Linux 二进制、Swagger 文件、环境模板、README 和 Makefile。部署机解压后，可设置环境文件并执行：

```bash
make register-meili-service
make register-service
make meili-start
make start
```

发布包内不包含 Go 源码，因此不要在解压后的发布包中执行 `make build` 或 `make deploy`。

## Troubleshooting

```bash
make check-go
make show-config
make meili-status
make status
```

若 Go 服务无法启动，先确认 `.env` 中的 `DATABASE_URL`、`MEILI_URL`、`MEILI_API_KEY` 和 `SEARCH_SYNC_TOKEN`。若 Meilisearch 无法启动，确认 `meilisearch.env` 中的 `MEILI_MASTER_KEY` 已设置，且 `127.0.0.1:7700` 未被占用。
