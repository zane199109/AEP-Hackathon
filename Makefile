# AEP-Hackathon Makefile
.PHONY: build-contract test-contract build-backend test-backend run-backend verify-day0

# ======== 合约 ========
build-contract:
	cd contract-foundry && forge build

test-contract:
	cd contract-foundry && forge test -vvv

deploy-contract:
	cd contract-foundry && forge script script/DeployAll.s.sol \
		--rpc-url $(BASE_SEPOLIA_RPC) \
		--broadcast \
		--verify

# ======== 后端 ========
build-backend:
	cd backend-go && go build -o ../bin/aep-backend ./cmd/

test-backend:
	cd backend-go && go test ./... -v

run-backend:
	cd backend-go && go run ./cmd/main.go

# ======== 验证 ========
verify-day0:
	cd verification_reports && bash verify_day0.sh

# ======== Demo ========
demo:
	@echo "=== AEP Demo ==="
	@echo "1. Start backend: make run-backend"
	@echo "2. Open frontend: cd frontend-web && npm run dev"
	@echo "3. See docs/start.md for full instructions"

# ======== 打包 ========
package:
	tar -czf aep-demo.tar.gz \
		backend-go/ contract-foundry/ frontend-web/ \
		scripts/ conf/ docs/ \
		CONTEXT.yaml Makefile docker-compose.yml README.md
