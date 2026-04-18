DEPLOY_CONFIG_FILE ?= .deploy.mk

-include $(DEPLOY_CONFIG_FILE)

APP_BRANCH ?= main
APP_DIR := $(shell pwd)
APP_PORT ?= 1337
APP_USER ?= $(shell whoami)
SERVICE_NAME ?= strapi-doc-center

.PHONY: help show-config install-node ensure-env install-deps build write-service restart status logs deploy update switch-dev switch-prod run uninstall clean

help:
	@printf '%s\n' \
	  'make deploy          Install Node.js, deps, build, write systemd service' \
	  'make update          Pull latest code, rebuild, restart service' \
	  'make restart         Restart systemd service' \
	  'make switch-dev      Switch to develop mode and restart' \
	  'make switch-prod     Build and switch to production mode' \
	  'make status          Show systemd service status' \
	  'make logs            Tail systemd logs' \
	  'make show-config     Show current config values' \
	  'make uninstall       Stop service, remove service file'

show-config:
	@printf '%s\n' \
	  "APP_DIR=$(APP_DIR)" \
	  "APP_BRANCH=$(APP_BRANCH)" \
	  "APP_PORT=$(APP_PORT)" \
	  "APP_USER=$(APP_USER)" \
	  "SERVICE_NAME=$(SERVICE_NAME)"

install-node:
	@if command -v node >/dev/null 2>&1 && [ "$$(node -p "process.versions.node.split('.')[0]")" -ge 20 ]; then \
	  echo '[make] Node.js already installed:' $$(node -v); \
	else \
	  echo '[make] Installing Node.js 20'; \
	  curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -; \
	  sudo apt-get install -y nodejs build-essential; \
	fi

ensure-env:
	@if [ ! -f "$(APP_DIR)/.env" ]; then \
	  echo '[make] Creating $(APP_DIR)/.env'; \
	  printf '%s\n' \
	    'HOST=0.0.0.0' \
	    'PORT=$(APP_PORT)' \
	    'NODE_ENV=production' \
	    'STRAPI_RUN_MODE=start' \
	    '' \
	    'APP_KEYS=change1,change2,change3,change4' \
	    'API_TOKEN_SALT=change_me_api_token_salt' \
	    'ADMIN_JWT_SECRET=change_me_admin_jwt_secret' \
	    'TRANSFER_TOKEN_SALT=change_me_transfer_token_salt' \
	    'JWT_SECRET=change_me_jwt_secret' \
	    '' \
	    'DATABASE_CLIENT=postgres' \
	    'DATABASE_URL=postgresql://strapi_u:your_password@your_db_host:5432/strapi_docs?sslmode=disable&search_path=public&connect_timeout=20&timezone=Asia/Shanghai' \
	    'DATABASE_SSL=false' \
	    '' \
	    'S3_ENDPOINT=http://your_minio_host:9100' \
	    'S3_ACCESS_KEY_ID=your_access_key' \
	    'S3_SECRET_ACCESS_KEY=your_secret_key' \
	    'S3_BUCKET=files' \
	    'S3_USE_SSL=false' \
	    'S3_FORCE_PATH_STYLE=true' \
	    'S3_SIGNED_URL_EXPIRES=600' \
	    'S3_REGION=us-east-1' \
	    'S3_ROOT_PATH=strapi' \
	    > "$(APP_DIR)/.env"; \
	  echo '[make] Fill real values in $(APP_DIR)/.env and rerun make deploy'; \
	else \
	  echo '[make] Existing .env found, keeping it'; \
	fi

install-deps:
	@echo '[make] Installing npm dependencies'
	@cd "$(APP_DIR)" && npm install && npm install pg @strapi/provider-upload-aws-s3 --save

build:
	@echo '[make] Building Strapi app'
	@cd "$(APP_DIR)" && npm run build

write-service:
	@echo '[make] Writing systemd service /etc/systemd/system/$(SERVICE_NAME).service'
	@printf '%s\n' \
	  '[Unit]' \
	  'Description=Strapi Doc Center' \
	  'After=network.target' \
	  '' \
	  '[Service]' \
	  'Type=simple' \
	  'User=$(APP_USER)' \
	  'WorkingDirectory=$(APP_DIR)' \
	  'EnvironmentFile=$(APP_DIR)/.env' \
	  'ExecStart=/bin/bash -lc '"'"'if [ "$$STRAPI_RUN_MODE" = "develop" ]; then npm run develop; else npm run start; fi'"'"'' \
	  'Restart=always' \
	  'RestartSec=5' \
	  '' \
	  '[Install]' \
	  'WantedBy=multi-user.target' \
	  | sudo tee "/etc/systemd/system/$(SERVICE_NAME).service" >/dev/null
	@sudo systemctl daemon-reload
	@sudo systemctl enable "$(SERVICE_NAME)"

restart:
	@echo '[make] Restarting $(SERVICE_NAME)'
	@sudo systemctl restart "$(SERVICE_NAME)"
	@sleep 2
	@sudo systemctl --no-pager status "$(SERVICE_NAME)" || true

status:
	@sudo systemctl --no-pager status "$(SERVICE_NAME)" || true

logs:
	@journalctl -u "$(SERVICE_NAME)" -f

switch-dev:
	@echo '[make] Switching app to develop mode'
	@grep -q '^STRAPI_RUN_MODE=' "$(APP_DIR)/.env" && \
	  sudo sed -i 's/^STRAPI_RUN_MODE=.*/STRAPI_RUN_MODE=develop/' "$(APP_DIR)/.env" || \
	  echo 'STRAPI_RUN_MODE=develop' | sudo tee -a "$(APP_DIR)/.env" >/dev/null
	@grep -q '^NODE_ENV=' "$(APP_DIR)/.env" && \
	  sudo sed -i 's/^NODE_ENV=.*/NODE_ENV=development/' "$(APP_DIR)/.env" || \
	  echo 'NODE_ENV=development' | sudo tee -a "$(APP_DIR)/.env" >/dev/null
	@$(MAKE) restart

switch-prod:
	@echo '[make] Switching app to production mode'
	@grep -q '^STRAPI_RUN_MODE=' "$(APP_DIR)/.env" && \
	  sudo sed -i 's/^STRAPI_RUN_MODE=.*/STRAPI_RUN_MODE=start/' "$(APP_DIR)/.env" || \
	  echo 'STRAPI_RUN_MODE=start' | sudo tee -a "$(APP_DIR)/.env" >/dev/null
	@grep -q '^NODE_ENV=' "$(APP_DIR)/.env" && \
	  sudo sed -i 's/^NODE_ENV=.*/NODE_ENV=production/' "$(APP_DIR)/.env" || \
	  echo 'NODE_ENV=production' | sudo tee -a "$(APP_DIR)/.env" >/dev/null
	@$(MAKE) build
	@$(MAKE) restart

run:
	@cd "$(APP_DIR)" && npm run start

update:
	@echo '[make] Pulling latest code'
	@git -C "$(APP_DIR)" pull origin "$(APP_BRANCH)"
	@$(MAKE) build
	@$(MAKE) restart

deploy: install-node ensure-env install-deps build write-service
	@printf '%s\n' \
	  '' \
	  '[make] Deploy complete. Next steps:' \
	  '  1. Edit $(APP_DIR)/.env with real values (DATABASE_URL, APP_KEYS, S3_*, etc.)' \
	  '  2. Run: make restart' \
	  ''

uninstall:
	@echo '[make] Uninstalling $(SERVICE_NAME)'
	@sudo systemctl stop "$(SERVICE_NAME)" 2>/dev/null || true
	@sudo systemctl disable "$(SERVICE_NAME)" 2>/dev/null || true
	@sudo rm -f "/etc/systemd/system/$(SERVICE_NAME).service"
	@sudo systemctl daemon-reload
	@echo '[make] Uninstall complete'

clean: uninstall

