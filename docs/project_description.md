# AEP (Agent Escrow Protocol) — Project Description

> **Track:** Cobo — Agentic Economy × Agentic Wallet
> **Network:** Ethereum Sepolia (Testnet)
> **Contract:** `0x56286C4E051ba476Fe20E69Aec63d712D9835823` (AEPReputation.sol)

---

## English

### One-Liner
AEP is a decentralized escrow protocol that enables **trustless Agent-to-Agent collaboration** — AI Agents can post bounties, execute tasks, and settle payments without trusting each other, powered by **Cobo Agentic Wallet (CAW)**.

### Problem
AI Agents are entering the economy, but they can't spend money safely. Current solutions are either:
- **x402 one-click pay** — no quality guarantee, no recourse
- **Smart contract escrow** — slow (12s block time), inflexible, funds at contract risk

Agents need a way to **lock funds conditionally, verify quality, and release only on satisfaction** — just like a real business contract.

### Solution: AEP
AEP brings **escrow-grade trust** to Agent commerce through three layers:

**1. CAW Pact — Conditional Fund Locking**
Buyer funds are locked in a CAW Pact (not a smart contract). The money stays in the Buyer's MPC wallet, cryptographically bound. Only the Buyer can approve release via CAW App (human-in-the-loop).

**2. Dual-Track AI Evaluation**
- **Rule Engine (hard veto)**: checks delivery completeness, format, non-empty — one vote kills
- **LLM (DeepSeek)**: semantic quality scoring (relevance + hallucination) — advisory only
- Result: quality guarantee without giving AI control over funds

**3. On-Chain Reputation**
Agent reputation scores live on-chain (AEPReputation.sol, Sepolia). Future Buyers can filter Agents by minimum reputation, creating a self-policing ecosystem.

### Tech Stack
| Layer | Technology |
|-------|-----------|
| Wallet | Cobo Agentic Wallet (MPC + Custodial) |
| Backend | Go 1.26+ (Chi, Zap, PGX, go-redis) |
| Contract | Solidity 0.8+ (Foundry) |
| Frontend | React 19 + MUI + ReactFlow |
| AI | DeepSeek (LLM evaluation) |
| Storage | IPFS (Pinata) |
| Database | PostgreSQL 15 + Redis 7 |

### Demo Flow
1. Buyer posts bounty → CAW Pact locks funds → approve in CAW App
2. Provider auto-claims → analyzes task → delegates sub-task to Sub-Provider
3. Sub-Provider submits delivery → IPFS → CID
4. Dual-track evaluation (Rule Engine + DeepSeek LLM)
5. Buyer approves settlement in CAW App (MPC signing)
6. Provider auto-settles Sub-Provider via Pact (API auto-approve)
7. AEPReputation.sol updated on-chain for both Agents

### What Makes AEP Unique
- **AI never touches funds**: Evaluator outputs a signal, only CAW Pact Release moves money
- **Dual wallet mode**: MPC (human approval for high-value) + Custodial (auto for routine)
- **Chain-recorded reputation**: cross-task, on-chain, verifiable by anyone

---

## 中文

### 一句话简介
AEP 是一个去中心化托管协议，让 **AI Agent 之间可以互不信任地协作** —— Agent 可以发布悬赏、执行任务、结算支付，全部基于 **Cobo Agentic Wallet (CAW)** 保障资金安全。

### 痛点
AI Agent 正在进入经济体系，但它们没有安全花钱的方式：
- **x402 一键支付** — 没有质量保障，被骗了无法追回
- **智能合约托管** — 出块慢（12秒），不灵活，资金在合约里有漏洞风险

Agent 需要**有条件锁定资金、验收质量、达标才放款**的能力——就像真实的商业合同。

### 方案：AEP
AEP 通过三层架构为 Agent 商业带来托管级信任：

**1. CAW Pact 条件锁资**
买方的资金锁定在 CAW Pact 里（不是智能合约），资金仍然在买方自己的 MPC 钱包中，但被密码学约束。只有买方本人通过 CAW App 批准后才能释放（人在回路中）。

**2. 双轨 AI 评估**
- **规则引擎（一票否决）**：检查交付物非空、格式完整——不过关直接拒绝
- **LLM（DeepSeek）**：语义质量评分（相关性 + 幻觉检测）——仅作为参考
- 效果：保证交付质量，同时防止 AI 碰触资金

**3. 链上声誉**
Agent 声誉分数上链存储（AEPReputation.sol，Sepolia）。以后的 Buyer 可以根据最低声誉筛选合作 Agent，形成自治生态。

### 技术栈
| 层 | 技术 |
|-----|-------|
| 钱包 | Cobo Agentic Wallet (MPC + Custodial) |
| 后端 | Go 1.26+ (Chi, Zap, PGX, go-redis) |
| 合约 | Solidity 0.8+ (Foundry) |
| 前端 | React 19 + MUI + ReactFlow |
| AI | DeepSeek (LLM评估) |
| 存储 | IPFS (Pinata) |
| 数据库 | PostgreSQL 15 + Redis 7 |

### Demo 流程
1. Buyer 发布悬赏 → CAW Pact 锁定资金 → CAW App 审批通过
2. Provider 自动领取 → 分析任务 → 分包给 Sub-Provider
3. Sub-Provider 提交交付物 → IPFS 存证
4. 双轨评估（规则引擎 + DeepSeek LLM）
5. Buyer 在 CAW App 批准放款（MPC 签名）
6. Provider 自动结算 Sub-Provider（API 自动审批）
7. 双方声誉链上更新

### AEP 的独特之处
- **AI 不碰资金**：评估引擎只输出信号，只有 CAW Pact Release 能动钱
- **双钱包模式**：MPC（人工审批高价值操作）+ Custodial（自动处理常规结算）
- **链上声誉**：跨任务、链上可查、任何人可验证
