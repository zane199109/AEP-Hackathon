import { create } from 'zustand'

const API = (path, options = {}) => {
  const opts = { headers: { 'Content-Type': 'application/json' }, ...options }
  return fetch(`/api${path}`, opts).then(r => r.json().catch(() => ({ status: r.status })))
}

const BUYER_ADDR = '0xa115523ac8f1391075c0f0d74418a4f159df53fe'
const PROVIDER_ADDR = '0x276e8c07f3c140d6f894ee5567df146d58db3c56'
const SUB_PROVIDER_ADDR = '0xe813c4298dc1263de7ec22293f1175ed2afa0623'

const calcRepReward = (rewardEth) => Math.max(1, Math.floor((rewardEth || 0) * 10))

const useStore = create((set, get) => ({
  // === State ===
  viewMode: 'topology',
  phase: 'idle',
  bounties: [],
  activeBounty: null,
  reputation: 50,
  providerReputation: 50,
  subProviderReputation: 50,
  repHistory: [],
  evaluationResult: null,
  ruleResults: null,
  showApproval: false,
  settled: false,
  terminalEvents: [],
  sseConnected: false,
  loading: false,
  lastPactId: sessionStorage.getItem('aep_lastPactId') || null,
  pactStatus: null,
  pendingApprovals: [],
  agentPipeline: [], // [{agent, status, message, reasoning, timestamp}]
  currentEvaluation: null,
  selectedPactId: null,
  isDrawerOpen: false,
  isDualPanelOpen: false,
  pactForm: JSON.parse(sessionStorage.getItem('aep_pactForm') || 'null') || {
    title: '', reward: '0.001', deadline: '2026-06-10T00:00:00', minReputation: 60, intent: '', nlInput: ''
  },
  lastSlashedJob: null,
  lastConfirmJob: null,
  pipelineData: null,
  repTxHashes: [],

  // === Actions ===
  addLog: (text, type = 'info') => set(state => ({
    terminalEvents: [{ time: new Date().toLocaleTimeString(), text, type }, ...state.terminalEvents].slice(0, 50)
  })),

  setPhase: (phase) => set({ phase }),

  fetchReputation: async () => {
    const [bp, sp] = await Promise.all([
      API(`/reputation/${PROVIDER_ADDR}`).catch(() => ({ score: 50 })),
      API(`/reputation/${SUB_PROVIDER_ADDR}`).catch(() => ({ score: 50 })),
    ])
    set({ providerReputation: bp.score, subProviderReputation: sp.score })
  },

  pollReputationUntilChange: () => {
    let attempts = 0
    const maxAttempts = 30  // ~2.5 minutes
    const oldProv = useStore.getState().providerReputation
    const oldSub = useStore.getState().subProviderReputation
    const timer = setInterval(async () => {
      attempts++
      const [bp, sp] = await Promise.all([
        API(`/reputation/${PROVIDER_ADDR}`).catch(() => null),
        API(`/reputation/${SUB_PROVIDER_ADDR}`).catch(() => null),
      ])
      if (bp && !bp.fallback_used && bp.score !== oldProv) {
        set({ providerReputation: bp.score })
        get().addLog(`⛓️ 链上声誉已更新: Provider ${oldProv} → ${bp.score}`, 'reputation')
      }
      if (sp && !sp.fallback_used && sp.score !== oldSub) {
        set({ subProviderReputation: sp.score })
        get().addLog(`⛓️ 链上声誉已更新: Sub-Provider ${oldSub} → ${sp.score}`, 'reputation')
      }
      if ((bp && !bp.fallback_used && bp.score !== oldProv) ||
          (sp && !sp.fallback_used && sp.score !== oldSub) ||
          attempts >= maxAttempts) {
        clearInterval(timer)
      }
    }, 5000)
  },

  addApproval: (approval) => set(state => {
    // Prevent duplicates - check if pactId or jobId already exists
    const exists = state.pendingApprovals.some(a => a.pactId === approval.pactId || a.jobId === approval.jobId)
    if (exists) return state
    return { pendingApprovals: [...state.pendingApprovals, approval] }
  }),

  removeApproval: (pactId) => set(state => ({
    pendingApprovals: state.pendingApprovals.filter(a => a.pactId !== pactId)
  })),

  setEvaluation: (evaluation) => set({
    evaluationResult: evaluation,
    isDualPanelOpen: !!evaluation
  }),

  addAgentAction: (action) => set(state => ({
    agentPipeline: [...state.agentPipeline, { ...action, timestamp: Date.now() }].slice(-20)
  })),
  clearAgentPipeline: () => set({ agentPipeline: [] }),

  setViewMode: (mode) => set({ viewMode: mode }),
  setSelectedPact: (id) => set({ selectedPactId: id }),
  setDrawerOpen: (open) => set({ isDrawerOpen: open }),
  setPactForm: (form) => {
    set({ pactForm: form })
    sessionStorage.setItem('aep_pactForm', JSON.stringify(form))
  },

  // === Bounty Operations ===
  createBounty: async (params) => {
    set({ loading: true, phase: 'pending_approval', pactStatus: 'pending_approval' })
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
    set({ loading: false })
    if (data.error) { set({ phase: 'idle' }); return { error: data.error } }
    if (data.pact_id) {
      set({ lastPactId: data.pact_id })
      sessionStorage.setItem('aep_lastPactId', data.pact_id)
    }
    if (data.job_id) set({ lastConfirmJob: data.job_id })
    get().addLog(`📌 Bounty #${data.job_id} posted`, 'lock')
    return data
  },

  claimBounty: async (bountyId) => {
    set({ loading: true })
    const data = await API(`/bounty/${bountyId}/claim`, {
      method: 'POST',
      body: JSON.stringify({ seller: PROVIDER_ADDR })
    })
    set({ loading: false })
    if (data.error) return { error: data.error }
    const state = get()
    const claimed = state.bounties.find(b => b.id === bountyId)
    set(state => ({
      bounties: state.bounties.filter(b => b.id !== bountyId),
      activeBounty: claimed ? { ...claimed, ...data } : { id: bountyId, ...data },
      phase: 'claimed'
    }))
    get().addLog(`🤝 Bounty Claimed — Job #${bountyId}`, 'claim')
    return data
  },

  submitDelivery: async (deliveryText) => {
    const state = get()
    if (!state.activeBounty) return { error: 'No active bounty' }
    set({ loading: true })
    const data = await API(`/bounty/${state.activeBounty.id}/submit`, {
      method: 'POST',
      body: JSON.stringify({ seller: PROVIDER_ADDR, data: deliveryText || '' })
    })
    set({ loading: false })
    if (data.error) return { error: data.error }
    set({
      phase: 'evaluated',
      evaluationResult: data.verdict,
      ruleResults: data.rule_results || null,
      showApproval: !!data.verdict,
      isDualPanelOpen: true
    })
    get().addLog(`📦 Delivery Submitted — Job #${state.activeBounty.id} | Score: ${((data.verdict?.score || 0) * 100).toFixed(0)}%`, 'submit')
    return data
  },

  confirmPayment: async () => {
    const state = get()
    if (!state.activeBounty) return { error: 'No active bounty' }
    set({ loading: true, phase: 'settling' })
    const data = await API(`/confirm/${state.activeBounty.id}`, { method: 'POST' })
    set({ loading: false })
    if (data.error) { set({ phase: 'evaluated' }); return { error: data.error } }
    if (data.settlement === 'settled') {
      const rewardEth = parseFloat(state.activeBounty.reward || '0.001')
      const delta = calcRepReward(rewardEth)
      set(s => ({
        reputation: s.reputation + delta,
        repHistory: [{ time: new Date().toLocaleTimeString(), delta: `+${delta}`, reason: `Job #${state.activeBounty.id} settlement` }, ...s.repHistory].slice(0, 10),
        settled: true,
        phase: 'settled'
      }))
      get().addLog(`🔐 CAW_Release | Job: ${state.activeBounty.id} | BuyerApproval: true`, 'release')
      get().addLog(`⛓️ On-chain Reputation +${delta} | Job: ${state.activeBounty.id}`, 'reputation')
      // Refresh on-chain scores
      setTimeout(() => get().fetchReputation(), 3000)
    }
    return data
  },

  confirmRelease: async () => {
    const state = get()
    if (!state.lastConfirmJob) return { error: 'No job to confirm' }
    set({ loading: true })
    const data = await API(`/confirm/${state.lastConfirmJob}`, { method: 'POST' })
    set({ loading: false })
    if (data.error) return { error: data.error }
    get().addLog(`🔐 CAW Release | Job: ${state.lastConfirmJob} | ✅ 放款完成`, 'release')
    return data
  },

  rejectBounty: async () => {
    const state = get()
    if (!state.activeBounty) return { error: 'No active bounty' }
    set({ loading: true })
    const data = await API(`/arbitrate/${state.activeBounty.id}`, { method: 'POST' })
    set({ loading: false })
    if (data.error) return { error: data.error }
    const penalty = 20
    set(s => ({
      reputation: Math.max(0, s.reputation - penalty),
      repHistory: [{ time: new Date().toLocaleTimeString(), delta: `-${penalty}`, reason: `Job #${state.activeBounty.id} arbitration` }, ...s.repHistory].slice(0, 10),
      settled: true,
      phase: 'slashed'
    }))
    get().addLog(`❌ Arbitration Slash | Job: ${state.activeBounty.id} | Penalty: -${penalty} rep`, 'slash')
    return data
  },

  updateReputationByAddr: (addr, newScore) => set(state => {
    const updates = {}
    if (addr === PROVIDER_ADDR) updates.providerReputation = newScore
    if (addr === SUB_PROVIDER_ADDR) updates.subProviderReputation = newScore
    if (addr === BUYER_ADDR) updates.reputation = newScore
    return updates
  }),

  addRepTxHash: (entry) => set(state => ({
    repTxHashes: [entry, ...state.repTxHashes].slice(0, 20)
  })),

  resetDemo: () => {
    set({
      phase: 'idle', bounties: [], activeBounty: null, evaluationResult: null,
      ruleResults: null, showApproval: false, settled: false, terminalEvents: [],
      reputation: 50, providerReputation: 50, subProviderReputation: 50,
      repHistory: [], pactStatus: null, lastPactId: null,
      pendingApprovals: [],
      agentPipeline: [], currentEvaluation: null, isDualPanelOpen: false,
      lastSlashedJob: null,
  lastConfirmJob: null,
      pipelineData: null,
      repTxHashes: [],
    })
    sessionStorage.removeItem('aep_lastPactId')
  },
}))

export { BUYER_ADDR, PROVIDER_ADDR, SUB_PROVIDER_ADDR }
export default useStore
