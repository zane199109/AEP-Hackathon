import React, { createContext, useContext, useState, useCallback, useRef, useEffect } from 'react'
import useStore from '../store/useStore'

const AppContext = createContext(null)

const API = (path, options = {}) => {
  const opts = { headers: { 'Content-Type': 'application/json' }, ...options }
  return fetch(`/api${path}`, opts)
    .then(r => r.json().catch(() => ({ status: r.status })))
}

// Wallet addresses for Buyer and Provider (from backend config)
const BUYER_ADDR = '0xa115523ac8f1391075c0f0d74418a4f159df53fe'
const PROVIDER_ADDR = '0x276e8c07f3c140d6f894ee5567df146d58db3c56'

// Reverse reputation calculation: given reward_eth, return score delta
function calcRepReward(rewardEth) {
  return Math.max(1, Math.floor((rewardEth || 0) * 10))
}

export function AppProvider({ children }) {
  // Phase state machine: idle → pending_approval → published → claimed → submitted → evaluated → settling → settled
  const phase = useStore(s => s.phase)
  const pollingDone = useRef(false)
  const [rerender, forceRerender] = useState(0)
  useEffect(() => { document.title = `AEP | ${phase.toUpperCase()}` }, [phase])
  // Bounty list (open bounties from SSE)
  const [bounties, setBounties] = useState([])
  // Currently claimed bounty — only one at a time for demo
  const [activeBounty, setActiveBounty] = useState(null)
  // Reputation
  const [reputation, setReputation] = useState(50)
  const [repHistory, setRepHistory] = useState([])
  // Evaluation
  const [evaluationResult, setEvaluationResult] = useState(null)
  // Rule results (template-based rule engine)
  const [ruleResults, setRuleResults] = useState(null)
  // Approval modal state
  const [showApproval, setShowApproval] = useState(false)
  // Settlement flag for topology phase detection
  const [settled, setSettled] = useState(false)
  // Terminal events
  const [terminalEvents, setTerminalEvents] = useState([])
  // SSE status
  const [sseConnected, setSseConnected] = useState(false)
  // Loading
  const [loading, setLoading] = useState(false)
  // Latest created pact_id for polling (persisted across reloads)
  const [lastPactId, setLastPactId] = useState(() => sessionStorage.getItem('aep_lastPactId') || null)
  const [pactStatus, setPactStatus] = useState(null)
  const [lastJobId, setLastJobId] = useState(null) // for pipeline polling
  const [pipelineStep, setPipelineStep] = useState('')

  // Persist lastPactId across page reloads
  useEffect(() => {
    if (lastPactId) {
      sessionStorage.setItem('aep_lastPactId', lastPactId)
    } else {
      sessionStorage.removeItem('aep_lastPactId')
    }
  }, [lastPactId])
  



  const addTerminal = useCallback((msg, type = 'info') => {
    setTerminalEvents(prev => [{ time: new Date().toLocaleTimeString(), text: msg, type }, ...prev].slice(0, 50))
  }, [])

// Poll pact status every 2s while waiting (fallback to SSE)
  useEffect(() => {
    if (!lastPactId || phase !== 'pending_approval') return
    const interval = setInterval(async () => {
      try {
        const res = await fetch(`/api/pact/${lastPactId}`)
        const data = await res.json()
        if (data && data.status === 'active') {
          clearInterval(interval)
          addTerminal(`✅ Pact approved: ${data.pact_id?.slice(0,8)}...`, 'release')
          setLastPactId(null)
          setPactStatus('active')
          useStore.setState(state => ({
            phase: 'published',
            pactStatus: 'active',
            pendingApprovals: [],
            lastPactId: null,
          }))
          forceRerender(n => n + 1)
        }
      } catch (e) {}
    }, 2000)
    return () => clearInterval(interval)
  }, [lastPactId, phase, addTerminal])
// Poll pipeline status after pact approval
  useEffect(() => {
    if (!lastJobId || phase === 'idle' || phase === 'pending_approval') return
    const interval = setInterval(async () => {
      try {
        const res = await fetch(`/api/bounty/${lastJobId}/pipeline`)
        const data = await res.json()
        if (data && data.step && data.step !== pipelineStep) {
          setPipelineStep(data.step)
          // Store full pipeline data in Zustand for UI consumption
          useStore.setState({ pipelineData: data })
          const labels = {
            claiming: '⏳ Provider 正在接单...', claimed: '✅ Provider 已接单',
            analyzing: '💭 Provider LLM 分析中...', decided: '🤔 Provider 已决策',
            creating_sub_bounty: '📋 Provider 发起子任务...',
            sub_claiming: '⏳ Sub-Provider 接单中...', sub_claimed: '✅ Sub-Provider 已接单',
            generating_sub_delivery: '✍️ Sub-Provider 生成交付物...',
            evaluating_sub: '🛡️ AEP 评估子任务...', sub_verified: '✅ 子任务通过',
            submitted: '📦 Provider 最终交付已提交',
            evaluating_final: '🛡️ AEP 评估主任务...',
            evaluated_verified: '✅ 评估通过，等待 Buyer 确认',
            evaluated_slashed: '❌ 评估不通过，进入争议',
            awaiting_confirmation: '⏳ 等待 Buyer 在 CAW 确认放款',
            settling: '💰 评估通过，自动放款中...',
            settled: '✅ 放款完成，全链路结束',
            settle_failed: '❌ 放款失败',
            release_pending: '🔐 CAW 放款待审批 — 请在 Cobo 钱包确认',
            pact_approved: '✅ Pact 已审批',
          }
          if (labels[data.step]) {
            addTerminal(labels[data.step], data.step.includes('verified') ? 'release' : data.step.includes('slash') ? 'slash' : 'info')
          }
          // Show reasoning when available
          if (data.reasoning) {
            addTerminal(`💡 推理: ${data.reasoning.slice(0, 120)}`, 'info')
          }
          // Show evaluation details when available
          if (data.eval_status && data.eval_score > 0) {
            addTerminal(`📊 评估结果: ${data.eval_status} | 分数: ${(data.eval_score * 100).toFixed(0)}分 | ${data.eval_summary?.slice(0, 80) || ''}`, 'info')
          }
          // Show delivery available
          if (data.sub_delivery && data.step === 'generating_sub_delivery') {
            addTerminal(`📄 Sub-Provider 交付物已生成 (${data.sub_delivery.length}字符)`, 'info')
          }
          if (data.final_delivery && data.step === 'submitted') {
            addTerminal(`📄 Provider 最终交付物已提交 (${data.final_delivery.length}字符)`, 'info')
          }
          // Set phase based on pipeline step
          if (data.step === 'evaluated_slashed') {
            useStore.setState({ phase: 'disputed' })
          } else if (data.step === 'awaiting_confirmation') {
            useStore.setState({ phase: 'verified' })
          } else if (data.step === 'settled') {
            useStore.setState({ phase: 'settled' })
          }
        }
        if (data.step === 'awaiting_confirmation' || data.step?.startsWith('evaluated_') || data.step === 'settled' || data.step === 'settle_failed') {
          clearInterval(interval)
        }
      } catch (e) {}
    }, 2000)
    return () => clearInterval(interval)
  }, [lastJobId, phase, pipelineStep, addTerminal])
// SSE connection
  useEffect(() => {
    let es, timer
    function connect() {
      es = new EventSource('/api/events')
      es.addEventListener('connected', () => setSseConnected(true))
      es.onerror = () => { setSseConnected(false); es.close(); timer = setTimeout(connect, 3000) }

      es.addEventListener('bounty_posted', e => {
        try {
          const data = JSON.parse(e.data)
          const bounty = {
            id: data.job_id,
            title: `Bounty #${data.job_id}`,
            reward: '0.001',
            deadline: '2026-06-10',
            status: data.status,
            pactId: data.pact_id || lastPactId,
            pactStatus: pactStatus || (lastPactId ? 'pending_approval' : 'active'),
          }
          setBounties(prev => [...prev, bounty])
          addTerminal(`📌 Bounty Posted — Job #${data.job_id}`, 'lock')
        } catch (err) {}
      })

      es.addEventListener('claimed', e => {
        try {
          const data = JSON.parse(e.data)
          addTerminal(`🤝 Bounty Claimed — Job #${data.job_id}`, 'claim')
        } catch (err) {}
      })

      es.addEventListener('submitted', e => {
        try {
          const data = JSON.parse(e.data)
          addTerminal(`📦 Delivery Submitted — Job #${data.job_id} | Status: ${data.status}`, 'submit')
        } catch (err) {}
      })

      es.addEventListener('settled', e => {
        try {
          const data = JSON.parse(e.data)
          addTerminal(`✅ Funds Settled — Job #${data.job_id}`, 'release')
        } catch (err) {}
      })


    }
    connect()
    return () => { if (es) es.close(); clearTimeout(timer) }
  }, [addTerminal])

  // Create bounty
  const createBounty = useCallback(async (params) => {
    pollingDone.current = false
    setLoading(true)
    useStore.setState({ phase: 'pending_approval' })
    setPactStatus('pending_approval')
    const data = await API('/bounty', {
      method: 'POST',
      body: JSON.stringify({
        buyer: BUYER_ADDR,
        amount: params.amount || '1000000000000000',
        deadline: params.deadline || '2026-06-10T00:00:00Z',
        min_reputation: params.minReputation || 60,
        intent: params.intent || '',
        demo_slash: params.demoSlash || false,
      })
    })
    setLoading(false)
    if (data.error) { useStore.setState({ phase: 'idle' }); return { error: data.error } }
    // Start polling pact status
    if (data.pact_id) {
      setLastPactId(data.pact_id)
      }
    if (data.job_id) {
      setLastJobId(data.job_id)
      useStore.setState({ lastConfirmJob: data.job_id })
    }
    // Update bounty list with real info
    setBounties(prev => prev.map(b =>
      b.id === data.job_id
        ? { ...b, title: params.title || `Bounty #${data.job_id}`, reward: params.amount ? (parseInt(params.amount) / 1e18).toString() : '0.001', deadline: params.deadline || '2026-06-10' }
        : b
    ))
    return data
  }, [])

  // Claim bounty
  const claimBounty = useCallback(async (bountyId) => {
    setLoading(true)
    const data = await API(`/bounty/${bountyId}/claim`, {
      method: 'POST',
      body: JSON.stringify({ seller: PROVIDER_ADDR })
    })
    setLoading(false)
    if (data.error) return { error: data.error }
    // Move from open list to active
    const claimed = bounties.find(b => b.id === bountyId)
    setBounties(prev => prev.filter(b => b.id !== bountyId))
    setActiveBounty(claimed ? { ...claimed, ...data } : { id: bountyId, ...data })
    useStore.setState({ phase: 'claimed' })
    addTerminal(`🤝 Bounty Claimed — Job #${bountyId}`, 'claim')
    return data
  }, [bounties, addTerminal])

  // Submit delivery
  const submitDelivery = useCallback(async (deliveryText) => {
    if (!activeBounty) return { error: 'No active bounty' }
    setLoading(true)
    const data = await API(`/bounty/${activeBounty.id}/submit`, {
      method: 'POST',
      body: JSON.stringify({
        seller: PROVIDER_ADDR,
        data: deliveryText || 'Complete solution with code, tests, and documentation meeting all requirements.'
      })
    })
    setLoading(false)
    if (data.error) return { error: data.error }
    useStore.setState({ phase: 'evaluated' })
    setEvaluationResult(data.verdict)
    setRuleResults(data.rule_results || null)
    // Trigger approval modal for all results (even slashed allows reject)
    if (data.verdict) {
      setShowApproval(true)
    }
    addTerminal(`📦 Delivery Submitted — Job #${activeBounty.id} | Score: ${((data.verdict?.score || 0) * 100).toFixed(0)}%`, 'submit')
    return data
  }, [activeBounty, addTerminal])

  // Confirm payment
  const confirmPayment = useCallback(async () => {
    if (!activeBounty) return { error: 'No active bounty' }
    setLoading(true)
    useStore.setState({ phase: 'settling' })
    const data = await API(`/confirm/${activeBounty.id}`, { method: 'POST' })
    setLoading(false)
    if (data.error) { useStore.setState({ phase: 'evaluated' }); return { error: data.error } }
    if (data.settlement === 'settled') {
      const rewardEth = parseFloat(activeBounty.reward || '0.001')
      const delta = calcRepReward(rewardEth)
      setReputation(prev => prev + delta)
      setRepHistory(prev => [{ time: new Date().toLocaleTimeString(), delta: `+${delta}`, reason: `Job #${activeBounty.id} settlement` }, ...prev].slice(0, 10))
      addTerminal(`🔐 CAW_Release | Job: ${activeBounty.id} | BuyerApproval: true`, 'release')
      addTerminal(`⛓️ On-chain Reputation +${delta} | Job: ${activeBounty.id}`, 'reputation')
      setSettled(true)
      useStore.setState({ phase: 'settled' })
    }
    return data
  }, [activeBounty, addTerminal])

  // Reject bounty — calls arbitration API for on-chain slash
  const rejectBounty = useCallback(async () => {
    if (!activeBounty) return { error: 'No active bounty' }
    setLoading(true)
    const data = await API(`/arbitrate/${activeBounty.id}`, { method: 'POST' })
    setLoading(false)
    if (data.error) return { error: data.error }
    const penalty = 20
    setReputation(prev => Math.max(0, prev - penalty))
    setRepHistory(prev => [{ time: new Date().toLocaleTimeString(), delta: `-${penalty}`, reason: `Job #${activeBounty.id} arbitration — on-chain slash` }, ...prev].slice(0, 10))
    addTerminal(`❌ Arbitration Slash | Job: ${activeBounty.id} | Penalty: -${penalty} rep`, 'slash')
    addTerminal(`⛓️ On-chain reputation updated | Job: ${activeBounty.id}`, 'slash')
    setSettled(true)
    useStore.setState({ phase: 'slashed' })
    return data
  }, [activeBounty, addTerminal])

  // Reset demo
  const resetDemo = useCallback(() => {
    setBounties([])
    setActiveBounty(null)
    setEvaluationResult(null)
    setRuleResults(null)
    setShowApproval(false)
    setSettled(false)
    setTerminalEvents([])
    setReputation(50)
    setRepHistory([])
    useStore.setState({ phase: 'idle' })
    setPactStatus(null)
    setLastPactId(null)
    sessionStorage.removeItem('aep_lastPactId')
  }, [])

  const value = {
    bounties, activeBounty, reputation, repHistory,
    evaluationResult, ruleResults, showApproval, setShowApproval, settled, terminalEvents, sseConnected, loading,
    createBounty, claimBounty, submitDelivery, confirmPayment, rejectBounty,
    resetDemo, setEvaluationResult,
    phase, pactStatus, lastPactId, BUYER_ADDR, PROVIDER_ADDR,
  }

  return <AppContext.Provider value={value}>{children}</AppContext.Provider>
}

export function useApp() {
  const ctx = useContext(AppContext)
  if (!ctx) throw new Error('useApp must be used within AppProvider')
  return ctx
}
