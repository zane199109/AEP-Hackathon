# AEP 环境配置清单

## 第一步：启动基础设施 (PostgreSQL + Redis)

```bash
cd AEP-Hackathon
docker compose up -d
# 等待几秒后验证
docker compose ps
# Expected: postgres (healthy), redis (healthy)
```

> 数据库表会自动通过 `docker-entrypoint-initdb.d/` 初始化。
> 如需手动初始化：`docker exec -i aep-postgres psql -U postgres -d aep < scripts/schema.sql`

---

## 第二步：配置环境变量

复制模板并填入真实值：

```bash
cp conf/.env.example conf/.env
```

然后在 `conf/.env` 中填写以下值：

### 必需（按使用顺序）

| 变量 | 用于 | 获取方式 |
|------|------|----------|
| `CAW_API_KEY` | Day 2 — CAW Pact 创建 | `curl -X POST https://api-core.agenticwallet.dev.cobo.com/api/v1/principals/provision -H "Content-Type: application/json" -d '{"name":"AEP-Agent"}'` → 取 `result.api_key` |
| `CAW_WALLET_ID` | Day 2 — 指定钱包 | `curl -X POST .../wallets ...` → 取 `result.uuid` |
| `BASE_SEPOLIA_RPC` | Day 1.4 — 合约部署 | Base Sepolia RPC 提供商（如 Infura, Alchemy） |
| `PRIVATE_KEY` | Day 1.4 — 部署签名 | 部署用钱包的私钥（**安全保管**） |
| `OPENAI_API_KEY` | Day 3 — LLM 双轨裁决 | OpenAI 平台 |
| `PINATA_JWT` | Day 3 — 交付物 IPFS 上传 | Pinata 平台 → API Keys → JWT |

---

## 第三步：配置 backend config.yaml

```bash
cp conf/config.demo.yaml conf/config.yaml
```

编辑 `conf/config.yaml`，确认以下项：

```yaml
caw:
  api_key: "caw_..."      # ← 从 .env 读取 (或直接填)
  wallet_id: "uuid..."     # ← 从 .env 读取
  sandbox: true             # dev环境用true

db:
  dsn: "postgres://postgres:password@localhost:5432/aep?sslmode=disable"
  # 如果 Docker compose 的密码改了，同步修改
```

> 配置中的 `${CAW_API_KEY}` 等占位符会自动从环境变量读取。你也可以直接填入明文值。

---

## 第四步：Setup Cobo CAW (分步操作)

按照 `docs/cobo_setup.md` 的完整指引：

1. **创建 API Key** — 上面第二步已做
2. **运行 TSS Node**:
   ```bash
   # 下载
   curl -LO https://download.tss.cobo.com/binary-release/latest/cobo-tss-node-linux-amd64.tar.gz
   tar xzf cobo-tss-node-linux-amd64.tar.gz
   sudo mv cobo-tss-node /usr/local/bin/
   # 初始化 & 启动（保持终端开着）
   cobo-tss-node init
   cobo-tss-node start --caw
   ```
3. **创建钱包** — 调用 API (详见 cobo_setup.md)
4. **创建地址 & 充值** — 获取钱包地址，打测试 ETH

---

## 第五步：验证全套环境

```bash
# 1. 数据库
docker compose ps
docker exec aep-postgres psql -U postgres -d aep -c "\dt"

# 2. Redis
redis-cli ping

# 3. CAW 连通性
curl -s https://api-core.agenticwallet.dev.cobo.com/api/v1/wallets \
  -H "X-API-Key: $CAW_API_KEY" | jq

# 4. 启动后端
cd backend-go
GONOSUMCHECK=* GONOSUMDB=* go run ./cmd/main.go -config ../conf/config.yaml
```

---

## 配置路径总结

| 用途 | 文件路径 | 操作 |
|------|---------|------|
| 数据库 + Redis | `AEP-Hackathon/docker-compose.yml` | ✅ 已创建，`docker compose up -d` 即可 |
| 环境变量 | `AEP-Hackathon/conf/.env` | 复制模板 → 填入真实值 |
| 后端配置 | `AEP-Hackathon/conf/config.yaml` | 复制模板 → 确认配置 |
| CAN 钱包设置 | `AEP-Hackathon/docs/cobo_setup.md` | 按步骤操作 |
