APP_DIR := $(shell pwd)
BIN_DIR := $(APP_DIR)/bin
BIN := $(BIN_DIR)/doc-search
SERVICE_NAME ?= doc-search
MEILI_SERVICE_NAME ?= doc-search-meilisearch
APP_USER ?= $(shell whoami)
GO_VERSION ?= 1.25.0
MEILI_VERSION ?= v1.13.3
MEILI_BIN ?= /usr/local/bin/meilisearch
MEILI_HOST ?= 127.0.0.1
MEILI_PORT ?= 7700
MEILI_DB_PATH ?= /var/lib/doc-search-meilisearch

.PHONY: build test run swagger install-go install-meilisearch create-meili-key write-service write-meili-service deploy restart status logs meili-restart meili-status meili-logs

build:
	@mkdir -p "$(BIN_DIR)"
	go build -o "$(BIN)" ./cmd/search

test:
	go test ./...

run:
	go run ./cmd/search

swagger:
	bash ./scripts/generate-swagger.sh

install-go:
	@set -eu; \
	arch="$$(uname -m)"; \
	case "$$arch" in x86_64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; *) echo "Unsupported Go architecture: $$arch" >&2; exit 1 ;; esac; \
	tmp="$$(mktemp -d)"; \
	curl -fsSL "https://go.dev/dl/go$(GO_VERSION).linux-$$arch.tar.gz" -o "$$tmp/go.tar.gz"; \
	sudo rm -rf /usr/local/go; \
	sudo tar -C /usr/local -xzf "$$tmp/go.tar.gz"; \
	sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go; \
	rm -rf "$$tmp"; \
	/usr/local/go/bin/go version

install-meilisearch:
	@set -eu; \
	arch="$$(uname -m)"; \
	case "$$arch" in x86_64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; *) echo "Unsupported Meilisearch architecture: $$arch" >&2; exit 1 ;; esac; \
	sudo curl -fsSL "https://github.com/meilisearch/meilisearch/releases/download/$(MEILI_VERSION)/meilisearch-linux-$$arch" -o "$(MEILI_BIN)"; \
	sudo chmod 0755 "$(MEILI_BIN)"; \
	"$(MEILI_BIN)" --version

create-meili-key:
	@set -eu; \
	set -a; . "$(APP_DIR)/meilisearch.env"; set +a; \
	curl -fsS -X POST "http://$(MEILI_HOST):$(MEILI_PORT)/keys" \
	  -H "Authorization: Bearer $$MEILI_MASTER_KEY" \
	  -H 'Content-Type: application/json' \
	  --data '{"description":"doc-search service","actions":["search","documents.add","documents.delete","documents.get","indexes.create","indexes.get","settings.update","tasks.get"],"indexes":["docs_public"],"expiresAt":null}'

write-service: build
	@printf '%s\n' \
	  '[Unit]' \
	  'Description=Documentation Search Service' \
	  'After=network.target' \
	  '' \
	  '[Service]' \
	  'Type=simple' \
	  'User=$(APP_USER)' \
	  'WorkingDirectory=$(APP_DIR)' \
	  'EnvironmentFile=$(APP_DIR)/.env' \
	  'ExecStart=$(BIN)' \
	  'Restart=always' \
	  'RestartSec=5' \
	  '' \
	  '[Install]' \
	  'WantedBy=multi-user.target' \
	  | sudo tee "/etc/systemd/system/$(SERVICE_NAME).service" >/dev/null
	@sudo systemctl daemon-reload
	@sudo systemctl enable "$(SERVICE_NAME)"

write-meili-service:
	@sudo install -d -o "$(APP_USER)" -g "$$(id -gn "$(APP_USER)")" -m 0750 "$(MEILI_DB_PATH)"
	@printf '%s\n' \
	  '[Unit]' \
	  'Description=Documentation Search Meilisearch' \
	  'After=network.target' \
	  '' \
	  '[Service]' \
	  'Type=simple' \
	  'User=$(APP_USER)' \
	  'EnvironmentFile=$(APP_DIR)/meilisearch.env' \
	  'ExecStart=$(MEILI_BIN) --env production --http-addr $(MEILI_HOST):$(MEILI_PORT) --db-path $(MEILI_DB_PATH)' \
	  'Restart=always' \
	  'RestartSec=5' \
	  '' \
	  '[Install]' \
	  'WantedBy=multi-user.target' \
	  | sudo tee "/etc/systemd/system/$(MEILI_SERVICE_NAME).service" >/dev/null
	@sudo systemctl daemon-reload
	@sudo systemctl enable "$(MEILI_SERVICE_NAME)"

deploy: install-meilisearch write-meili-service write-service
	@printf '%s\n' 'Set MEILI_MASTER_KEY in meilisearch.env, then run make meili-restart and make restart.'

restart:
	sudo systemctl restart "$(SERVICE_NAME)"

status:
	sudo systemctl --no-pager status "$(SERVICE_NAME)"

logs:
	journalctl -u "$(SERVICE_NAME)" -f

meili-restart:
	sudo systemctl restart "$(MEILI_SERVICE_NAME)"

meili-status:
	sudo systemctl --no-pager status "$(MEILI_SERVICE_NAME)"

meili-logs:
	journalctl -u "$(MEILI_SERVICE_NAME)" -f
