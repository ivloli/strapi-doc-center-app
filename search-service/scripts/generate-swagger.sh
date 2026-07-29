#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Run the pinned generator through Go so developers do not need a global swag binary.
go run -mod=readonly github.com/swaggo/swag/cmd/swag@v1.16.6 init \
  --generalInfo main.go \
	--dir cmd/search \
  --output docs \
  --parseInternal \
  --generatedTime=false
