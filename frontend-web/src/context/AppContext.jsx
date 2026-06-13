import React, { createContext, useContext, useState, useCallback, useRef, useEffect } from 'react'
import useStore from '../store/useStore'

const AppContext = createContext(null)

const API = (path, options = {}) => {
  const opts = { headers: { 'Content-Type': 'application/json' }, ...options }
  return fetch(`/api${path}`, opts)
    .then(r => r.json().catch(() => ({ status: r.status })))
}

// Wallet addresses for Buyer and Provider (from backend config, fetched on init)
let BUYER_ADDR = ''
let PROVIDER_ADDR = ''

// Fetch agent addresses from backend
fetch('/api/agents').then(r => r.json()).then(data => {
  if (data) {
    BUYER_ADDR = data.buyer?.address || BUYER_ADDR
    PROVIDER_ADDR = data.provider?.address || PROVIDER_ADDR
  }
}).catch(() => {})

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
  // Sync bounties from Store (updated by useSSE)
  const storeBounties = useStore(s => s.bounties)
  useEffect(() => { setBounties(storeBounties) }, [storeBounties])
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
  
  // Pact polling effects...
  const addTerminal = useCallback((msg, type = 'info') => {
    console.log('[ADD_TERMINAL]', msg, type)
    setTerminalEvents(prev => [{ time: new Date(Date.now()).toLocaleTimeString(), text: msg, type }, ...prev].slice(0, 50))
  }, [])
  
  // Expose addTerminal globally so Store actions can use it
  useEffect(() => {
    window.__addTerminal = addTerminal
  }, [addTerminal])

// Poll pact status every 2s while waiting (fallback to SSE)
  useEffect(() => {
    if (!lastPactId || phase !== 'pending_approval') return
    const interval = setInterval(async () => {
      try {
        const res = await fetch(`/api/pact/${lastPactId}`)
        const data = await res.json()
        if (data && data.status === 'active') {
          clearInterval(interval)
          addTerminal('✅ CAW pact approved — bounty published!', 'release')
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
  const loggedStepsRef = useRef(new Set())
  useEffect(() => {
    if (!lastJobId || phase === 'idle' || phase === 'pending_approval') return
    const interval = setInterval(async () => {
      try {
        const res = await fetch(`/api/bounty/${lastJobId}/pipeline`)
        const data = await res.json()
        if (!data || !data.step) return
        
        // Store full pipeline data in Zustand for UI consumption
        useStore.setState({ pipelineData: data })
        
        // Check if step changed
        if (data.step !== pipelineStep) {
          setPipelineStep(data.step)
        }
        
        // Iterate through all steps_reached (new steps since last poll) and log each
        const steps = data.steps_reached || [data.step]
        for (const s of steps) {
          if (loggedStepsRef.current.has(s)) continue
          loggedStepsRef.current.add(s)
          
          if (s === 'pact_approved') {
            addTerminal('✅ CAW pact approved — bounty published!', 'release')
            useStore.setState({ phase: 'published' })
          } else if (s === 'claiming') {
            addTerminal('⏳ Provider 正在接单...', 'info')
          } else if (s === 'claimed') {
            addTerminal('✅ Provider 已接单', 'claim')
            useStore.setState({ phase: 'claimed' })
          } else if (s === 'analyzing') {
            addTerminal('💭 Provider 分析任务中...', 'info')
          } else if (s === 'decided') {
            addTerminal('🤔 Provider 推理: ' + (data.reasoning || '').slice(0, 50), 'info')
          } else if (s === 'creating_sub_bounty') {
            addTerminal('📋 Provider 发起子任务...', 'info')
            useStore.setState({ phase: 'creating_sub_bounty' })
          } else if (s === 'sub_claiming') {
            addTerminal('⏳ Sub-Provider 正在接单...', 'info')
          } else if (s === 'sub_claimed') {
            addTerminal('✅ Sub-Provider 已接单', 'claim')
            useStore.setState({ phase: 'sub_claimed' })
          } else if (s === 'generating_sub_delivery') {
            addTerminal('💭 Sub-Provider 生成子任务交付物...', 'info')
          } else if (s === 'evaluating_sub') {
            addTerminal('🛡️ AEP 评估子任务...', 'info')
          } else if (s === 'sub_verified') {
            const score = data.sub_eval_score ? (data.sub_eval_score * 100).toFixed(0) : ''
            addTerminal(`📊 子任务评估通过${score ? ' | 分数: ' + score + '分' : ''}`, 'info')
          } else if (s === 'submitted') {
            addTerminal('📦 Provider 最终交付已提交', 'submit')
          } else if (s === 'evaluating_final') {
            addTerminal('🛡️ AEP 评估最终交付...', 'info')
            useStore.setState({ phase: 'evaluated' })
          } else if (s === 'evaluated_verified') {
            const score = data.eval_score ? (data.eval_score * 100).toFixed(0) : ''
            addTerminal(`📊 最终评估通过${score ? ' | 分数: ' + score + '分' : ''}`, 'info')
          } else if (s === 'evaluated_slashed') {
            addTerminal('⚠️ 评估不通过，悬赏进入争议状态', 'slash')
            useStore.setState({ phase: 'disputed' })
          } else if (s === 'awaiting_confirmation') {
            addTerminal('⏳ 等待 Buyer 在 CAW 确认放款', 'info')
            useStore.setState({ phase: 'verified' })
          } else if (s === 'settled') {
            addTerminal('⏳ 结算已提交，等待链上确认...', 'info')
            useStore.setState({ phase: 'settled' })
            // Fallback: populate chain records when SSE drops
            const st = useStore.getState()
            if (!st.repTxHashes.some(e => e.type === 'transfer' && e.from === 'provider')) {
              st.addRepTxHash({
                agent: 'sub_provider', oldScore: '', newScore: '',
                delta: '', txHash: data.child_tx_hash || '',
                type: 'transfer', from: 'provider', to: 'sub_provider', amount: data.child_amount || '',
              })
            }
            if (!st.repTxHashes.some(e => e.type === 'transfer' && e.from === 'buyer')) {
              st.addRepTxHash({
                agent: 'provider', oldScore: '', newScore: '',
                delta: '', txHash: data.parent_tx_hash || '',
                type: 'transfer', from: 'buyer', to: 'provider', amount: data.parent_amount || '',
              })
            }
            // Check if both tx hashes already available (from pipeline)
            if (data.parent_tx_hash && data.child_tx_hash) {
              addTerminal('✅ 放款完成，全链路结束', 'release')
            }
            // Fetch latest on-chain reputations after settlement
            setTimeout(async () => {
              await st.fetchReputation()
            }, 3000)
          } else {
            addTerminal(`📋 ${s}`, 'info')
          }

          // Also check for late-arriving parent_tx_hash
          if (s === 'settled' && data.parent_tx_hash && data.child_tx_hash) {
            addTerminal('✅ 放款完成，全链路结束', 'release')
          }
        }
        // Check for tx hashes on every settled poll (even if step already logged)
        if (data.step === 'settled' && data.parent_tx_hash && data.child_tx_hash) {
          const st = useStore.getState()
          if (!st.terminalEvents.some(e => e.text.includes('放款完成，全链路结束'))) {
            addTerminal('✅ 放款完成，全链路结束', 'release')
          }
        }
        // Update chain records with late-arriving tx hashes
        if (data.step === 'settled') {
          const st = useStore.getState()
          if (data.child_tx_hash && !st.repTxHashes.find(e => e.from === 'provider')?.txHash) {
            st.addRepTxHash({
              agent: 'sub_provider', from: 'provider', to: 'sub_provider', amount: '0.005',
              type: 'transfer', txHash: data.child_tx_hash,
              oldScore: '', newScore: '', delta: '',
            })
          }
          if (data.parent_tx_hash && !st.repTxHashes.find(e => e.from === 'buyer')?.txHash) {
            st.addRepTxHash({
              agent: 'provider', from: 'buyer', to: 'provider', amount: '',
              type: 'transfer', txHash: data.parent_tx_hash,
              oldScore: '', newScore: '', delta: '',
            })
          }
        }
        // Stop polling when both tx hashes arrive, or on failure
        if (data.step === 'settle_failed' || data.step?.startsWith('evaluated_slashed')) {
          clearInterval(interval)
        }
        if (data.step === 'settled' && data.child_tx_hash && data.parent_tx_hash) {
          clearInterval(interval)
        }
      } catch (e) {}
    }, 2000)
    return () => clearInterval(interval)
  }, [lastJobId, phase, pipelineStep, addTerminal])

  // Create bounty
  const createBounty = useCallback(async (params) => {
    pollingDone.current = false
    setTerminalEvents([]) // Clear terminal logs before new bounty
    loggedStepsRef.current = new Set() // Reset step tracking for new bounty
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
      useStore.setState({ lastPactId: data.pact_id })
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
    addTerminal,
  }

  return <AppContext.Provider value={value}>{children}</AppContext.Provider>
}

export function useApp() {
  const ctx = useContext(AppContext)
  if (!ctx) throw new Error('useApp must be used within AppProvider')
  return ctx
}
