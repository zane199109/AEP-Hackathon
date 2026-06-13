# AEP Demo 演示脚本 — 评审向

> ⚠️ 本项目为技术协议演示，仅使用测试网资产，无真实金融服务。

---

## 演示前环境准备

```bash
# 1. 启动基础设施
cd AEP-Hackathon && docker compose up -d

# 2. 启动 3 个 TSS 节点（需要 3 个终端）

# 终端 A：Buyer TSS
cd ~/.cobo-agentic-wallet/profiles/profile_caw_agent_d5f503b472be10db/tss-node
./cobo-tss-node start --caw --dev \
  --key-file .password \
  --db db/secrets.db \
  --config configs/cobo-tss-node-config.yaml

# 终端 B：Provider TSS
cd ~/.cobo-agentic-wallet/profiles/profile_caw_agent_b3848cd699aa9f3f/tss-node
./cobo-tss-node start --caw --dev \
  --key-file .password \
  --db db/secrets.db \
  --config configs/cobo-tss-node-config.yaml

# 终端 C：Sub-Provider TSS
cd ~/.cobo-agentic-wallet/profiles/profile_caw_agent_32440e7663ace850/tss-node
./cobo-tss-node start --caw --dev \
  --key-file .password \
  --db db/secrets.db \
  --config configs/cobo-tss-node-config.yaml

# 终端 D：启动后端
cd AEP-Hackathon
export $(grep -v '^#' conf/.env | xargs)
cd backend-go
GONOSUMCHECK=* GONOSUMDB=* go run ./cmd/main.go -config ../conf/config.yaml

# 终端 E：启动前端
cd frontend-web && npm run dev
```

---

## 演示流程（约 8-10 分钟）

### 开场白（30s）

> "各位评审好，今天展示的是 **AEP（Agent Escrow Protocol）**—— 一个基于 Cobo Agentic Wallet 的 AI Agent 去中心化协作协议。"
>
> "AEP 的核心创新在于：AI Agent 之间的经济活动需要**信任**—— Buyer 担心付了钱拿不到合格交付物，Seller 担心交了货收不到钱。AEP 通过 **CAW 的双层审批机制** 和 **AI 双轨评估** 解决了这个问题。"

---

### Step 1: Buyer 创建 Bounty（1min）

**操作：** 前端页面 → 填写 Bounty（标题、金额 0.01 ETH、最低声誉 60）→ 提交

**讲稿：**
> "Buyer（我手机上这个 CAW App 控制的钱包）发布一个任务：我需要一份 Base 链上 Aave 和 Compound 的流动性分析报告，至少 800 字，包含 TVL 和利率数据。我的预算是0.03ETH,要求接单的agent声誉不能少于70。悬赏发布后的一天之内要提交。"
>
> "注意看 —— 这里的资金不是直接转给合约，而是通过 **CAW Pact（协议）** 锁定在 Buyer 的 MPC 钱包里。只有 Buyer 本人批准后，资金才会释放。"

**前端验证：** ✅ Bounty 状态 `Open`，Pact 状态 `pending_approval`

**手机操作：** 📱 打开 CAW App → 批准 Pact → 资金锁定

**前端验证：** ✅ Pact 状态变 `active`

---

### Step 2: 自动领取与任务分包（1min）

> "Bounty 发布后，Provider Agent 自动检测到任务并领取。它发现这个任务需要数据收集 + 分析报告两部分，所以决定把数据收集部分分包给 Sub-Provider。"

**前端验证：** ✅ Provider 节点出现 → 拓扑图出现 Sub-Provider

---

### Step 3: Sub-Provider 交付（30s）

> "Sub-Provider 自动领取子任务，生成数据收集报告并提交。"

**前端验证：** ✅ Sub-Provider 节点变绿

---

### Step 4: Provider 合并交付物（30s）

> "Provider 拿到 Sub-Provider 的数据后，整合成完整的分析报告，提交给 AEP 评估引擎。"

**前端验证：** ✅ Provider 显示交付中

---

### Step 5: 双轨 AI 评估（1min）

> "这是 AEP 的核心 —— 双轨评估。左侧是规则引擎：检查交付物是否为空、格式是否完整。右侧是 LLM（DeepSeek）：评估内容的相关性和质量。"
>
> "规则引擎有一票否决权 —— 如果交付物是空的，直接拒绝。规则通过后，LLM 的分数就是最终分数，不取平均。这样避免了高分被低分拉低的问题。"

**前端验证：** ✅ 双轨面板显示规则通过 + LLM 分数

---

### Step 6: Buyer 手机审批结算（1min）

> "评估通过，现在 Buyer 需要在 CAW App 上确认 —— 这模拟了人类对 AI Agent 的关键监督。"
>
> "只有在人类批准后，CAW 才会执行 MPC 签名，将 0.01 ETH 从 Buyer 钱包释放到 Provider。"

**手机操作：** 📱 打开 CAW App → 批准转账

**前端验证：** ✅ 资金流向 Buyer → Provider，状态 `Settled`
**链上验证：** https://sepolia.etherscan.io/tx/...

---

### Step 7: Provider 自动结算 Sub-Provider（30s）

> "Provider 收到款项后，自动将 0.002 ETH 结算给 Sub-Provider。注意这里跟 Buyer 的审批不同 —— Provider 是未配对的 MPC 钱包，资金释放通过 API 自动完成，不需要手机确认。"
>
> "这展示了两种模式的对比：**重要决策走人工审批，常规结算走自动流程**。"

**链上验证：** https://sepolia.etherscan.io/tx/...

---

### Step 8: 声誉链上更新（30s）

> "最后，AEPReputation 合约更新双方声誉。Provider 声誉 +10，Sub-Provider 初始化为 65 分。这些声誉数据是跨任务的，以后的 Buyer 可以根据声誉来筛选合作的 Agent。"

**链上验证：** https://sepolia.etherscan.io/address/0x56286C4E051ba476Fe20E69Aec63d712D9835823

---

### 总结（30s）

> "总结一下 AEP 的三个关键价值："
>
> "1️⃣ **CAW 双层审批** —— Pact 锁定资金只在人类批准后释放，AI 不能独自花钱"
>
> "2️⃣ **双轨 AI 评估** —— 规则引擎硬否决 + LLM 质量评分，兼顾安全与灵活性"
>
> "3️⃣ **链上声誉体系** —— Agent 的工作记录和评分永久上链，构建可信的 AI 协作网络"
>
> "谢谢各位评审！"

---

## 关键数据

| 项目 | 值 |
|------|-----|
| AEPReputation 合约 | `0x56286C4E051ba476Fe20E69Aec63d712D9835823` |
| Buyer 钱包 | `0x2fce0a555212fc4adfec0eeb731cd96fea01f93e` |
| Provider 钱包 | `0x12e1aec7224d47376ac3a391f27076ed13df0267` |
| Sub-Provider 钱包 | `0xdf782505c76f2ee59ffbd9f4385feec266c06b99` |
| Buyer→Provider TX | (待补充) |
| Provider→Sub TX | (待补充) |

## 注意事项

- TSS 节点需全部在线（3 个终端）
- Buyer 钱包需提前与 CAW App 配对
- DeepSeek API key 需有效
- 如 live demo 出问题，用录好的视频兜底
