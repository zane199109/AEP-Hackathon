#!/bin/bash
# Source env vars from .env file, then run the backend
set -a
source "$(dirname "$0")/../conf/.env"
set +a

cd "$(dirname "$0")/.."
cd backend-go
GONOSUMCHECK=* GONOSUMDB=* exec go run ./cmd/main.go "$@"
