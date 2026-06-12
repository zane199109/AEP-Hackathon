# Cobo Agentic Wallet (CAW) Setup Guide

## Overview

CAW (Cobo Agentic Wallet) is a TSS (Threshold Signature Scheme) wallet designed for AI agents.
It provides a REST API for creating wallets, managing funds, and submitting pacts (funding approvals).

- **Dev API**: `https://api-core.agenticwallet.dev.cobo.com/api/v1`
- **Prod API**: `https://api-core.agenticwallet.cobo.com/api/v1`
- **Auth**: `X-API-Key` header
- **Full API docs**: https://api-core.agenticwallet.dev.cobo.com/api/v1/docs

## Step 1: Create an API Key

```bash
curl -s -X POST https://api-core.agenticwallet.dev.cobo.com/api/v1/principals/provision \
  -H "Content-Type: application/json" \
  -d '{"name": "AEP-Agent"}'
```

Expected response:
```json
{
  "success": true,
  "result": {
    "agent_id": "caw_agent_...",
    "api_key": "caw_...",
    "status": "active"
  }
}
```

Save the `api_key` value — this is your `CAW_API_KEY`.

## Step 2: Run a TSS Node

Download and run the Cobo TSS Node binary. This is required for wallet creation and signing.

```bash
# Download TSS Node
curl -LO https://download.tss.cobo.com/binary-release/latest/cobo-tss-node-linux-amd64.tar.gz
tar xzf cobo-tss-node-linux-amd64.tar.gz
sudo mv cobo-tss-node /usr/local/bin/

# Initialize (set a password of at least 16 characters)
cobo-tss-node init

# Start (must use --caw flag)
cobo-tss-node start --caw
```

Keep the TSS Node running in a separate terminal. Note the **Node ID** displayed during init.

## Step 3: Create a Wallet

```bash
# Replace <NODE_ID> with the Node ID from Step 2
curl -s -X POST https://api-core.agenticwallet.dev.cobo.com/api/v1/wallets \
  -H "Content-Type: application/json" \
  -H "X-API-Key: <CAW_API_KEY>" \
  -d '{
    "wallet_type": "MPC",
    "name": "AEP Wallet",
    "group_type": "agent",
    "main_node_id": "<NODE_ID>",
    "for_owner": false
  }'
```

Expected response includes `uuid` — this is your `CAW_WALLET_ID`.

Check wallet status:
```bash
curl -s https://api-core.agenticwallet.dev.cobo.com/api/v1/wallets \
  -H "X-API-Key: <CAW_API_KEY>" | jq '.result[] | {uuid, status}'
```
Wait until status is `"active"`.

## Step 4: Create an Address

```bash
curl -s -X POST https://api-core.agenticwallet.dev.cobo.com/api/v1/wallets/<CAW_WALLET_ID>/addresses \
  -H "Content-Type: application/json" \
  -H "X-API-Key: <CAW_API_KEY>" \
  -d '{"chain_type": "ETH"}'
```

Expected response includes `address` — this is the wallet's address on EVM chains.

## Step 5: Fund the Wallet

1. Copy the address from Step 4
2. Get Base Sepolia test ETH from a faucet (e.g. https://base-sepolia-faucet.tools/)
3. Send test ETH to that address

## Step 6: (Optional) Pair with CAW App

For human approval of pacts, pair the wallet with the CAW App:

```bash
curl -s -X POST https://api-core.agenticwallet.dev.cobo.com/api/v1/wallets/pairs/initiate \
  -H "Content-Type: application/json" \
  -H "X-API-Key: <CAW_API_KEY>" \
  -d '{"wallet_id": "<CAW_WALLET_ID>"}'
```

Enter the returned token in the CAW App (download from App Store / TestFlight).

## Step 7: Configure Environment Variables

```bash
export CAW_API_KEY="caw_..."       # from Step 1
export CAW_WALLET_ID="uuid"         # from Step 3
```

Add to `~/.bashrc` or use the `.env` file.

## Testing the Integration

Start the backend:
```bash
cd AEP-Hackathon/backend-go
GONOSUMCHECK=* GONOSUMDB=* go run ./cmd/main.go -config ../conf/config.demo.yaml
```

In another terminal, post a bounty:
```bash
curl -s -X POST http://localhost:8080/api/bounty \
  -H "Content-Type: application/json" \
  -d '{"buyer":"0xYourAddress","amount":"1000000000000000","deadline":"2026-06-10T00:00:00Z"}'
```

## Troubleshooting

| Error | Likely Cause | Fix |
|-------|-------------|-----|
| `CAW wallet_id not configured` | `CAW_WALLET_ID` not set | Check env vars |
| `caw api error (status 401)` | Invalid or missing `CAW_API_KEY` | Re-provision from Step 1 |
| `caw api error (status 404)` | Wrong wallet_id | Verify wallet exists via `GET /wallets` |
| `TSS Node not running` | `cobo-tss-node` not started | Run `cobo-tss-node start --caw` |
| `pact status: pending_approval` | Pact needs human approval | Check CAW App |

## Reference: Key API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/principals/provision` | Create API key |
| POST | `/wallets` | Create wallet |
| GET | `/wallets` | List wallets |
| POST | `/wallets/{id}/addresses` | Create address |
| POST | `/wallets/pairs/initiate` | Pair with CAW App |
| POST | `/pacts/submit` | Submit a pact |
| GET | `/pacts/{id}` | Get pact status |
