APP_NAME ?= doc-search
APP_DIR := $(shell pwd)
BIN_DIR := $(APP_DIR)/bin
BIN := $(BIN_DIR)/$(APP_NAME)
SERVICE_NAME ?= $(APP_NAME)
MEILI_SERVICE_NAME ?= $(APP_NAME)-meilisearch
APP_USER ?= $(shell whoami)

GO ?= go
TARGET_OS ?= linux
TARGET_ARCH ?= amd64
CGO_ENABLED ?= 0
RELEASE_TAG ?= $(shell git describe --always --dirty --tags 2>/dev/null || date +%Y%m%d%H%M%S)
RELEASE_DIR ?= $(APP_DIR)/release
RELEASE_NAME := $(APP_NAME)-offline-$(TARGET_OS)-$(TARGET_ARCH)-$(RELEASE_TAG)
RELEASE_PATH := $(RELEASE_DIR)/$(RELEASE_NAME)
RELEASE_TAR := $(RELEASE_DIR)/$(RELEASE_NAME).tar.gz

MEILI_VERSION ?= v1.13.3
MEILI_BIN ?= /usr/local/bin/meilisearch
MEILI_HOST ?= 127.0.0.1
MEILI_PORT ?= 7700
MEILI_DB_PATH ?= /var/lib/$(APP_NAME)-meilisearch

.PHONY: help show-config check-go install-deps ensure-env ensure-meili-env build test vet run swagger \
	install-meilisearch create-meili-key register-service register-meili-service deploy \
	start restart stop status logs meili-start meili-restart meili-stop meili-status meili-logs \
	prepare-release build-release release-package release-checksum clean-release

help:
	@printf '%s\n' \
	  'make install-deps     Install Ubuntu runtime tools and download Go modules' \
	  'make build            Cross-compile a Linux release binary into bin/' \
	  'make test             Run Go tests' \
	  'make vet              Run Go static analysis' \
	  'make swagger          Regenerate Swagger documents' \
	  'make deploy           Prepare env files, install Meili, build, register systemd services' \
	  'make restart          Restart the Go search service' \
	  'make meili-restart    Restart the local Meilisearch service' \
	  'make release-package  Build a Jenkins-ready Linux tar.gz artifact' \
	  'make release-checksum Print SHA-256 for the release artifact'

show-config:
	@printf '%s\n' \
	  "APP_DIR=$(APP_DIR)" \
	  "APP_NAME=$(APP_NAME)" \
	  "SERVICE_NAME=$(SERVICE_NAME)" \
	  "APP_USER=$(APP_USER)" \
	  "TARGET_OS=$(TARGET_OS)" \
	  "TARGET_ARCH=$(TARGET_ARCH)" \
	  "RELEASE_TAG=$(RELEASE_TAG)"

check-go:
	@set -eu; \
	version="$$($(GO) env GOVERSION | sed 's/^go//')"; \
	major="$${version%%.*}"; \
	rest="$${version#*.}"; \
	minor="$${rest%%.*}"; \
	if [ "$$major" -lt 1 ] || { [ "$$major" -eq 1 ] && [ "$$minor" -lt 25 ]; }; then \
	  echo "Go 1.25+ is required, found $$version" >&2; exit 1; \
	fi; \
	echo "[make] Go $$version"; \
	$(GO) env GOMOD

install-deps: check-go
	@sudo apt-get update
	@sudo apt-get install -y ca-certificates curl tar
	@GOWORK=off $(GO) mod download
	@GOWORK=off $(GO) mod verify

ensure-env:
	@if [ ! -f "$(APP_DIR)/.env" ]; then \
	  cp "$(APP_DIR)/.env.example" "$(APP_DIR)/.env"; \
	  echo '[make] Created search-service/.env; set DATABASE_URL, MEILI_API_KEY, and SEARCH_SYNC_TOKEN'; \
	else \
	  echo '[make] Existing search-service/.env found, keeping it'; \
	fi

ensure-meili-env:
	@if [ ! -f "$(APP_DIR)/meilisearch.env" ]; then \
	  printf '%s\n' 'MEILI_MASTER_KEY=change_me_to_a_long_random_value' > "$(APP_DIR)/meilisearch.env"; \
	  chmod 600 "$(APP_DIR)/meilisearch.env"; \
	  echo '[make] Created search-service/meilisearch.env; set MEILI_MASTER_KEY before starting Meilisearch'; \
	else \
	  echo '[make] Existing search-service/meilisearch.env found, keeping it'; \
	fi

build: check-go
	@mkdir -p "$(BIN_DIR)"
	@GOWORK=off CGO_ENABLED=$(CGO_ENABLED) GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) \
	  $(GO) build -trimpath -ldflags='-s -w' -o "$(BIN)" ./cmd/search
	@echo "[make] Built $(BIN) for $(TARGET_OS)/$(TARGET_ARCH)"

test: check-go
	@GOWORK=off $(GO) test ./cmd/... ./internal/... ./docs

vet: check-go
	@GOWORK=off $(GO) vet ./cmd/... ./internal/... ./docs

run: check-go
	@GOWORK=off $(GO) run ./cmd/search

swagger: check-go
	@bash ./scripts/generate-swagger.sh

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

register-service:
	@test -x "$(BIN)" || { echo "Build binary first: make build" >&2; exit 1; }
	@printf '%s\n' \
	  '[Unit]' \
	  'Description=Documentation Search Service' \
	  'After=network-online.target $(MEILI_SERVICE_NAME).service' \
	  'Wants=network-online.target $(MEILI_SERVICE_NAME).service' \
	  '' \
	  '[Service]' \
	  'Type=simple' \
	  'User=$(APP_USER)' \
	  'WorkingDirectory=$(APP_DIR)' \
	  'EnvironmentFile=$(APP_DIR)/.env' \
	  'ExecStart=$(BIN)' \
	  'Restart=always' \
	  'RestartSec=5' \
	  'NoNewPrivileges=true' \
	  'PrivateTmp=true' \
	  '' \
	  '[Install]' \
	  'WantedBy=multi-user.target' \
	  | sudo tee "/etc/systemd/system/$(SERVICE_NAME).service" >/dev/null
	@sudo systemctl daemon-reload
	@sudo systemctl enable "$(SERVICE_NAME)"

register-meili-service:
	@sudo install -d -o "$(APP_USER)" -g "$$(id -gn "$(APP_USER)")" -m 0750 "$(MEILI_DB_PATH)"
	@printf '%s\n' \
	  '[Unit]' \
	  'Description=Documentation Search Meilisearch' \
	  'After=network-online.target' \
	  'Wants=network-online.target' \
	  '' \
	  '[Service]' \
	  'Type=simple' \
	  'User=$(APP_USER)' \
	  'EnvironmentFile=$(APP_DIR)/meilisearch.env' \
	  'ExecStart=$(MEILI_BIN) --env production --http-addr $(MEILI_HOST):$(MEILI_PORT) --db-path $(MEILI_DB_PATH)' \
	  'Restart=always' \
	  'RestartSec=5' \
	  'NoNewPrivileges=true' \
	  'PrivateTmp=true' \
	  '' \
	  '[Install]' \
	  'WantedBy=multi-user.target' \
	  | sudo tee "/etc/systemd/system/$(MEILI_SERVICE_NAME).service" >/dev/null
	@sudo systemctl daemon-reload
	@sudo systemctl enable "$(MEILI_SERVICE_NAME)"

deploy: install-deps ensure-env ensure-meili-env install-meilisearch build register-meili-service register-service
	@printf '%s\n' \
	  '[make] Search services registered.' \
	  'Set MEILI_MASTER_KEY and service .env values, run make meili-restart, create the Meili API key, then run make restart.'

start:
	@sudo systemctl start "$(SERVICE_NAME)"
	@sudo systemctl --no-pager status "$(SERVICE_NAME)" || true

restart:
	@sudo systemctl restart "$(SERVICE_NAME)"
	@sudo systemctl --no-pager status "$(SERVICE_NAME)" || true

stop:
	@sudo systemctl stop "$(SERVICE_NAME)"

status:
	@sudo systemctl --no-pager status "$(SERVICE_NAME)" || true

logs:
	@journalctl -u "$(SERVICE_NAME)" -f

meili-start:
	@sudo systemctl start "$(MEILI_SERVICE_NAME)"
	@sudo systemctl --no-pager status "$(MEILI_SERVICE_NAME)" || true

meili-restart:
	@sudo systemctl restart "$(MEILI_SERVICE_NAME)"
	@sudo systemctl --no-pager status "$(MEILI_SERVICE_NAME)" || true

meili-stop:
	@sudo systemctl stop "$(MEILI_SERVICE_NAME)"

meili-status:
	@sudo systemctl --no-pager status "$(MEILI_SERVICE_NAME)" || true

meili-logs:
	@journalctl -u "$(MEILI_SERVICE_NAME)" -f

prepare-release:
	@rm -rf "$(RELEASE_PATH)"
	@mkdir -p "$(RELEASE_PATH)/bin" "$(RELEASE_PATH)/docs"

build-release: prepare-release build
	@install -m 0755 "$(BIN)" "$(RELEASE_PATH)/bin/$(APP_NAME)"
	@install -m 0644 ".env.example" "$(RELEASE_PATH)/.env.example"
	@install -m 0644 "README.md" "$(RELEASE_PATH)/README.md"
	@install -m 0644 "Makefile" "$(RELEASE_PATH)/Makefile"
	@cp -R "docs/." "$(RELEASE_PATH)/docs/"
	@printf '%s\n' 'MEILI_MASTER_KEY=change_me_to_a_long_random_value' > "$(RELEASE_PATH)/meilisearch.env.example"
	@echo "[make] Prepared $(RELEASE_PATH)"

release-package: build-release
	@mkdir -p "$(RELEASE_DIR)"
	@tar -czf "$(RELEASE_TAR)" -C "$(RELEASE_DIR)" "$(RELEASE_NAME)"
	@echo "[make] Created $(RELEASE_TAR)"

release-checksum: release-package
	@sha256sum "$(RELEASE_TAR)"

clean-release:
	@rm -rf "$(RELEASE_DIR)"
