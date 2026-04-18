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

help:
	@printf '%s\n' \
	  'make deploy APP_REPO=<repo>         Clone/pull app repo, remember config, build, write systemd, restart' \
	  'make show-config                    Show current deploy config and defaults' \
	  'make clone-or-update APP_REPO=<repo> Pull only app code into APP_DIR' \
	  'make switch-dev                     Set STRAPI_RUN_MODE=develop and restart service' \
	  'make switch-prod                    Build app, set STRAPI_RUN_MODE=start and restart service' \
	  'make uninstall                      Stop service, remove service file and APP_DIR, clear .deploy.mk' \
	  'make clean                          Alias of uninstall' \
	  'make logs                           Tail systemd logs' \
	  'make status                         Show systemd status'

show-config:
	@printf '%s\n' \
	  "APP_REPO=$(APP_REPO)" \
	  "APP_BRANCH=$(APP_BRANCH)" \
	  "APP_DIR=$(APP_DIR)" \
	  "APP_PORT=$(APP_PORT)" \
	  "APP_USER=$(APP_USER)" \
	  "SERVICE_NAME=$(SERVICE_NAME)" \
	  "DEPLOY_CONFIG_FILE=$(DEPLOY_CONFIG_FILE)"

remember-config:
	@test -n "$(APP_REPO)" || (echo 'Set APP_REPO=<your business repo>' && exit 1)
	@echo '[make] Saving deploy config to $(DEPLOY_CONFIG_FILE)'
	@printf 'APP_REPO=%s\nAPP_BRANCH=%s\nAPP_DIR=%s\nAPP_PORT=%s\nAPP_USER=%s\nSERVICE_NAME=%s\n' \
	  "$(APP_REPO)" "$(APP_BRANCH)" "$(APP_DIR)" "$(APP_PORT)" "$(APP_USER)" "$(SERVICE_NAME)" \
	  > "$(DEPLOY_CONFIG_FILE)"

install-node:
	@if command -v node >/dev/null 2>&1 && [ "$$(node -p "process.versions.node.split('.')[0]")" -ge 20 ]; then \
	  echo '[make] Node.js already installed:' $$(node -v); \
	else \
	  echo '[make] Installing Node.js 20'; \
	  curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -; \
	  sudo apt-get install -y nodejs build-essential; \
	fi

clone-or-update:
	@test -n "$(APP_REPO)" || (echo 'Set APP_REPO=<your business repo>' && exit 1)
	@echo '[make] Preparing app repo at $(APP_DIR)'
	@sudo mkdir -p "$(APP_DIR)"
	@sudo chown -R "$(APP_USER)":"$(APP_USER)" "$(APP_DIR)"
	@if [ -d "$(APP_DIR)/.git" ]; then \
	  git -C "$(APP_DIR)" fetch --all --prune && \
	  git -C "$(APP_DIR)" checkout "$(APP_BRANCH)" && \
	  git -C "$(APP_DIR)" pull origin "$(APP_BRANCH)"; \
	else \
	  rm -rf "$(APP_DIR)" && git clone -b "$(APP_BRANCH)" "$(APP_REPO)" "$(APP_DIR)"; \
	fi

copy-config:
	@echo '[make] Copying Postgres and S3/MinIO config templates'
	@mkdir -p "$(APP_DIR)/config"
	@cp templates/config/database.js "$(APP_DIR)/config/database.js"
	@cp templates/config/plugins.js "$(APP_DIR)/config/plugins.js"

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
	@$(MAKE) restart APP_REPO="$(APP_REPO)" APP_BRANCH="$(APP_BRANCH)" APP_DIR="$(APP_DIR)" APP_PORT="$(APP_PORT)" APP_USER="$(APP_USER)" SERVICE_NAME="$(SERVICE_NAME)"

switch-prod:
	@echo '[make] Switching app to production mode'
	@grep -q '^STRAPI_RUN_MODE=' "$(APP_DIR)/.env" && \
	  sudo sed -i 's/^STRAPI_RUN_MODE=.*/STRAPI_RUN_MODE=start/' "$(APP_DIR)/.env" || \
	  echo 'STRAPI_RUN_MODE=start' | sudo tee -a "$(APP_DIR)/.env" >/dev/null
	@grep -q '^NODE_ENV=' "$(APP_DIR)/.env" && \
	  sudo sed -i 's/^NODE_ENV=.*/NODE_ENV=production/' "$(APP_DIR)/.env" || \
	  echo 'NODE_ENV=production' | sudo tee -a "$(APP_DIR)/.env" >/dev/null
	@$(MAKE) build APP_REPO="$(APP_REPO)" APP_BRANCH="$(APP_BRANCH)" APP_DIR="$(APP_DIR)" APP_PORT="$(APP_PORT)" APP_USER="$(APP_USER)" SERVICE_NAME="$(SERVICE_NAME)"
	@$(MAKE) restart APP_REPO="$(APP_REPO)" APP_BRANCH="$(APP_BRANCH)" APP_DIR="$(APP_DIR)" APP_PORT="$(APP_PORT)" APP_USER="$(APP_USER)" SERVICE_NAME="$(SERVICE_NAME)"

run:
	@cd "$(APP_DIR)" && npm run start

update: clone-or-update build restart

deploy: remember-config install-node clone-or-update ensure-env install-deps build write-service
	@printf '%s\n' \
	  '' \
	  '[make] Deploy complete. Next steps:' \
	  '  1. Verify $(APP_DIR)/config/database.js exists and is configured' \
	  '  2. Verify $(APP_DIR)/config/plugins.js exists and is configured' \
	  '  3. Edit $(APP_DIR)/.env with real values (DATABASE_URL, APP_KEYS, S3_*, etc.)' \
	  '  4. Run: make restart' \
	  ''

uninstall:
	@echo '[make] Uninstalling deployed app from $(APP_DIR)'
	@sudo systemctl stop "$(SERVICE_NAME)" 2>/dev/null || true
	@sudo systemctl disable "$(SERVICE_NAME)" 2>/dev/null || true
	@sudo rm -f "/etc/systemd/system/$(SERVICE_NAME).service"
	@sudo systemctl daemon-reload
	@sudo rm -rf "$(APP_DIR)"
	@rm -f "$(DEPLOY_CONFIG_FILE)"
	@echo '[make] Uninstall complete'

clean: uninstall
