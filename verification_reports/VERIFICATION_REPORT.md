# AEP (Agent Escrow Protocol) - Verification Report

> **Date:** 2026-06-13
> **Network:** Ethereum Sepolia (Testnet)
> **Protocol:** AEP Multi-Agent Bounty System

---

## 1. Smart Contract

| Item | Value |
|------|-------|
| Contract | AEPReputation.sol |
| Address | `0x56286C4E051ba476Fe20E69Aec63d712D9835823` |
| Explorer | [Sepolia Etherscan](https://sepolia.etherscan.io/address/0x56286C4E051ba476Fe20E69Aec63d712D9835823#code) |
| Verified | ✅ |

---

## 2. Wallet Addresses

| Role | Address | Type | Balance |
|------|---------|------|---------|
| **Buyer** | `0x2fce0a555212fc4adfec0eeb731cd96fea01f93e` | MPC (paired with CAW App) | ~0.058 ETH |
| **Provider** | `0x12e1aec7224d47376ac3a391f27076ed13df0267` | MPC (unpaired) | ~0.034 ETH |
| **Sub-Provider** | `0xdf782505c76f2ee59ffbd9f4385feec266c06b99` | MPC (unpaired) | ~0.009 ETH |
| **Gas Key** | `0x8EB3Fe3dDe56Cab0CDf32db3e6E5bA865596BE2C` | EOA | ~0.032 ETH |

---

## 3. On-Chain Reputation (Current)

| Agent | Score | Source |
|-------|-------|--------|
| Provider | **87** | On-chain (no fallback) |
| Sub-Provider | **89** | On-chain (no fallback) |

---

## 4. Transaction Hashes

| Description | Tx Hash | Explorer |
|-------------|---------|----------|
| Buyer→Provider release | `0xacb4fbf04a4b0dd1c5a2a1eff9eb7fb503aee1cfed430e3af3d4bdd381bbeb29` | [Sepolia Etherscan](https://sepolia.etherscan.io/tx/0xacb4fbf04a4b0dd1c5a2a1eff9eb7fb503aee1cfed430e3af3d4bdd381bbeb29) |
| Provider→SubProvider settlement | `0xa343a18f9299a5e2888d52ed7d6b3d81455ed78a96eb2c7eae1e39a3a7379557` | [Sepolia Etherscan](https://sepolia.etherscan.io/tx/0xa343a18f9299a5e2888d52ed7d6b3d81455ed78a96eb2c7eae1e39a3a7379557) |
| Gas key→Buyer (0.05 ETH top-up) | `0xceb7936d09a28568233fd2129832d97c0765fd9b194ceddf4a2028e2a3e16614` | [Sepolia Etherscan](https://sepolia.etherscan.io/tx/0xceb7936d09a28568233fd2129832d97c0765fd9b194ceddf4a2028e2a3e16614) |
| Provider reputation update (+3) | `0x5a69e26d305a81e802b0af11139463875c302998823b5213cb3d397406c67712` | [Sepolia Etherscan](https://sepolia.etherscan.io/tx/0x5a69e26d305a81e802b0af11139463875c302998823b5213cb3d397406c67712) |

---

## 5. CAW MPC Wallet Info

| Wallet | UUID | Status |
|--------|------|--------|
| Buyer | `60f71ce2-fa32-4409-b296-618d85d79817` | ✅ Paired with CAW App |
| Provider | `0ceacced-632f-43d6-9769-e8fedbd29507` | ✅ Unpaired (auto-approve) |
| Sub-Provider | `4262024f-4a4f-4bfb-aa8a-ce9bad2420b5` | ✅ Unpaired (auto-approve) |

---

## 6. Demo Flow Summary

```
1. User posts bounty → CAW lock pact created
2. Approve in CAW App → Pact becomes active
3. Provider auto-claims → LLM analyzes task
4. LLM decides: needs sub-task → creates sub-bounty
5. Sub-Provider auto-claims → generates delivery
6. AEP evaluates sub-delivery (LLM + Rule engine)
7. Provider merges results → submits final delivery
8. AEP evaluates final delivery
9. ⏳ Waiting for Buyer confirmation
10. Buyer clicks "确认放款" → reputation updated on-chain
11. Provider→SubProvider settled via Pact (auto-approved)
12. ✅ Full chain complete
```

---

## 7. Screenshots

| # | Screenshot | File |
|---|------------|------|
| 1 | **Frontend Topology** — 7-node topology (Buyer, AEP Engine, CAW Vault, Provider, Sub-Provider, Evaluator, Settlement) | `01_topology.png` |
| 2 | **Terminal Logs** — CAW Approval → Pact → Provider analysis → Sub-Provider → Evaluation → Settlement | `02_terminal_logs.png` |
| 3 | **Chain Records** — Buyer→Provider + Provider→SubProvider transfer records with Etherscan links | `03_chain_records.png` |
| 4 | **CAW Lock Pact** — Mobile CAW App, lock pact approval page | `04_caw_lock_pact.png` |
| 5 | **CAW Release Pact** — Mobile CAW App, release pact approval page | `05_caw_release_pact.png` |
| 6 | **Etherscan Reputation Tx** — `0x564d644298e406b6f5801bef83575e878ca2ce5309b6a9d3e348ed02f9b02a0d` | `06_etherscan_reputation.png` |
| 7 | **Etherscan Transfer Tx** — `0xa343a18f9299a5e2888d52ed7d6b3d81455ed78a96eb2c7eae1e39a3a7379557` | `07_etherscan_transfer.png` |
| 8 | **Contract Verification** — AEPReputation.sol verified on Etherscan | `08_contract_verified.png` |

> All screenshots are located in `verification_reports/` directory next to this file.

