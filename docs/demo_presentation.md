# AEP Demo 演示脚本 — 评审向

> ⚠️ 本项目为技术协议演示，仅使用测试网资产，无真实金融服务。

---

## 准备工作（演示前）

```bash
# 终端 1：启动基础设施
cd AEP-Hackathon && docker compose up -d

# 终端 2：启动后端
bash /tmp/start-backend.sh

# 终端 3：启动前端
cd frontend-web && npm run dev

# 终端 4：跑 Go 集成测试
cd backend-go && go test ./cmd/ -run TestDemo_ -v -count=1 -timeout=120s
```

---

## 演示流程（约 8 分钟）

### 开场白（30s）

> "各位评审好，今天展示的是 **AEP（Agent Escrow Protocol）**——面向 AI Agent 的去中心化任务协作协议。"
>
> "核心流程：**Buyer 发榜**（含声誉要求）→ **多个 Provider 检查自身声誉后抢单** → 胜出者提交交付物 → **AI 双轨裁决** → Buyer 确认 → **付款并更新 Provider 声誉**。"

---

### Scene 1: 系统架构总览（1min）

**操作：** 打开浏览器 → http://localhost:3000

**评审看到：**
- 6 节点拓扑图：Buyer → AEP Backend → CAW Pact → Seller → Evaluator → Settlement
- 顶部免责声明（合规红线）
- 右下角 SSE 连接状态 🟢

**讲解重点：**
```
1. Buyer 发榜（含声誉门槛）→ CAW Pact 锁定资金
2. 多个 Provider 检查 FluxA 声誉 → 自查是否符合要求
3. 符合者抢单 → Redis + PG 双保险锁防并发
4. 胜出者提交交付物 → Rule 规则引擎（一票否决）+ LLM 旁轨（可降级）
5. Buyer 确认 → CAW Release 释放资金 → 更新 Provider 声誉
```

---

### Scene 2: Happy Path 全流程（4min）

**操作：** 分两步演示——先发榜（展示 CAW App 审批），再跑完剩余流程。

**Step 2a — 发榜 & 人类审批 CAW App**

```bash
curl -s -X POST http://localhost:8080/api/bounty \
  -H "Content-Type: application/json" \
  -d '{"buyer":"0xBuyer","amount":"5000000000000000","deadline":"2026-06-10T00:00:00Z"}'
```

**评审看到后端返回：**
```json
{
  "job_id": 1780680999452,
  "pact_id": "8934037e-eb9c-4b64-87d2-0b97da2caebc",
  "pact_status": "pending_approval",
  "status": "Open"
}
```

**讲解：**
> "注意 `pact_status`——当前演示用 Custodial 钱包，Pact 自动激活。"
> "生产环境使用 MPC 钱包时，pact_status 为 `pending_approval`，人类必须在 CAW App 上批准后才能继续。"
> "这就是 **人类意愿兜底**——AI 不能自己决定花钱。"
> "（如需展示 App 审批流程，可以现场打开 CAW App 演示配对和批准）"

**操作：** 拿起手机 → 打开 CAW App → 展示待审批的 Pact → 点击批准

> "现在评审可以拿出手机，在 CAW App 上批准这个 Pact。批准后 Pact 变成 active 状态，后续结算才能进行。"

**Step 2b — 全自动流程（Claim → Submit → Confirm）**

```bash
# 等 Pact 批准后，跑集成测试
cd backend-go && go test ./cmd/ -run TestDemo_HappyPath -v -count=1
```

**评审看到输出：**
```
✅ Post Bounty   → pact=8934037e..., status=Open
✅ Claim         → status=Assigned
✅ Submit        → verdict=verified, passed=true
✅ Confirm       → settlement=settled
```

**同步在前端观察：** 拓扑节点依次脉冲变色

**讲解每步：**

| 步骤 | 技术亮点 | 评审关注点 |
|------|---------|-----------|
| **Post** | `min_reputation` 字段；`BizID = SHA256(Buyer+Block+Nonce)` 幂等；CAW Pact 锁定 | Buyer 设置声誉门槛 |
| **Claim** | FluxA 声誉自查 → Redis SETNX(120s TTL+defer) + PG `SELECT FOR UPDATE` | Provider 先自查再抢单 |
| **Submit** | Pinata IPFS 存证 + Rule 规则引擎 + DeepSeek LLM 评估 | LLM 异常可降级 |
| **Confirm** | `BuyerApproval` 强校验 + CAW Release + 声誉更新 | 付款+声誉联动 |

---

### Scene 3: Fail Path — 规则拦截（1min）

**操作：**
```bash
go test ./cmd/ -run TestDemo_FailPath -v -count=1
```

**评审看到：**
```
✅ Empty delivery → status=slashed, passed=false
```

**讲解：** 空交付直接被规则引擎拦截，不走 LLM。
> "规则引擎一票否决，这是最常见的安全防线——LLM 再强，碰到硬规则也得让路。"

---

### Scene 4: 并发抢单演示（1min）

**操作：**
```bash
go test ./cmd/ -run TestDemo_ConcurrentClaim -v -count=1
```

**评审看到：**
```
✅ Concurrent claim: 1 success, 4 conflicts
```

**讲解：**
> "5 个卖家同时抢同一单，Redis 锁 + PG 行锁双重保险下，只有 1 个成功，4 个获 409 冲突。"
> "锁 TTL 强制 120 秒，defer 释放，防止死锁。"

---

### Scene 5: L2 幂等去重（1min）

**操作：**
```bash
go test ./cmd/ -run TestDemo_L2Dedup -v -count=1
```

**评审看到：**
```
✅ First confirm: settled
✅ Second confirm: already_settled
```

**讲解：**
> "第一次确认正常结算。第二次确认——即使重启服务——被 PG `processed_events` 表拦截，返回 `already_settled`，防止重复放款。"

---

### Scene 6: Admin 应急（30s）

**操作：**
```bash
go test ./cmd/ -run TestDemo_AdminRetryAuth -v -count=1
```

**评审看到：**
```
✅ No token → 401
✅ With token → 200, settlement=failed
```

**讲解：**
> "Admin 接口有 Token 鉴权保护，无 Token 返回 401。这是防止越权篡改的最后一道防线。"

---

### 技术亮点总结（1min）

**评审可能问的问题 & 回答：**

| 问题 | 回答 |
|------|------|
| **资金安全怎么保证？** | 资金在用户自己的 CAW 钱包中，通过 Pact 锁定。AEP 后端只能按 Pact 规则释放，且必须 Buyer 确认。 |
| **AI 裁决出错怎么办？** | 双轨制：规则引擎一票否决。LLM 超时/异常直接降级忽略，以规则引擎为准。 |
| **Provider 声誉怎么保证？** | FluxA 集成，发榜时 Buyer 设置 `min_reputation` 门槛。Provider 抢单前自查，不达标无法抢单。 |
| **并发抢单怎么防？** | Redis SETNX（120s TTL + defer 强制释放）+ PG `SELECT FOR UPDATE` 行锁。 |
| **重启导致重复结算？** | L1 Redis（24h TTL）+ L2 PG `processed_events` 联合主键联合去重。 |
| **离线怎么演示？** | `--offline` 模式 + `mock_events.jsonl`（从测试网 `cast` 导出），结构 100% 一致。 |

---

### 红线合规展示（30s）

前端大屏首屏始终显示：
```
⚠️ 本项目为技术协议演示，仅使用测试网资产，无真实金融服务。
```

代码中全量替换"资金托管"为「密码学资金锁定调度」。

---

## 附录：一键跑全量测试

```bash
cd AEP-Hackathon/backend-go
go test ./cmd/ -run TestDemo_ -v -count=1 -timeout=120s
```

全部场景在 15s 内跑完，无需手动操作。
