import { useEffect } from 'react'
import useStore from '../store/useStore'
export default function useSSE() {
  const addLog = useStore(s => s.addLog)
  const setPhase = useStore(s => s.setPhase)
  const setSseConnected = () => useStore.setState({ sseConnected: true })
  const setEvaluation = useStore(s => s.setEvaluation)
  const setPactStatus = useStore(s => s.setPactStatus)
  const addApproval = useStore(s => s.addApproval)
  const removeApproval = useStore(s => s.removeApproval)
  useEffect(() => {
    let es, timer
    function connect() {
      es = new EventSource('/api/events')
      es.addEventListener('connected', () => useStore.setState({ sseConnected: true }))
      es.onerror = () => {
        useStore.setState({ sseConnected: false })
        es.close()
        timer = setTimeout(connect, 3000)
      }
      es.addEventListener('bounty_posted', e => {
        try {
          const data = JSON.parse(e.data)
          const state = useStore.getState()
          useStore.setState(s => ({
            bounties: [...s.bounties, {
              id: data.job_id,
              title: `Bounty #${data.job_id}`,
              reward: '0.001',
              deadline: '2026-06-10',
              status: data.status,
              pactId: data.pact_id || state.lastPactId,
              pactStatus: state.lastPactId ? 'pending_approval' : 'active',
            }]
          }))
          addLog(`📌 Bounty Posted — Job #${data.job_id}`, 'lock')
        } catch (err) { /* ignore */ }
      })
      es.addEventListener('claimed', e => {
        try {
          const data = JSON.parse(e.data)
          addLog(`🤝 Bounty Claimed — Job #${data.job_id}`, 'claim')
    useStore.setState({ phase: 'claimed' })
        } catch (err) { /* ignore */ }
      })
      es.addEventListener('submitted', e => {
        try {
          const data = JSON.parse(e.data)
          addLog(`📦 Delivery Submitted — Job #${data.job_id} | Status: ${data.status}`, 'submit')
    useStore.setState({ phase: 'submitted' })
        } catch (err) { /* ignore */ }
      })
      // === New SSE Events ===
      es.addEventListener('evaluation_started', e => {
        try {
          const data = JSON.parse(e.data)
          addLog(`🛡️ AEP Evaluation Started — Job #${data.job_id}`, 'info')
    useStore.setState({ phase: 'evaluated' })
          useStore.setState({ isDualPanelOpen: true, currentEvaluation: { loading: true, jobId: data.job_id } })
        } catch (err) { /* ignore */ }
      })
      es.addEventListener('evaluation_result', e => {
        try {
          const data = JSON.parse(e.data)
          let extra = {}
          try { extra = JSON.parse(data.message) } catch (err) {}
          addLog(`🤖 AEP评估: ${data.status} — Job #${data.job_id}`, 'submit')
          if (data.status === 'slashed') {
            useStore.setState({
              phase: 'disputed',
              lastSlashedJob: data.job_id,
              evaluationResult: { passed: false, status: 'slashed', score: extra.score || 0, summary: extra.summary || '' },
            })
            addLog(`⚠️ 评估不通过，悬赏进入争议状态 — 需要仲裁`, 'slash')
          }
        } catch (err) { /* ignore */ }
      })
      es.addEventListener('awaiting_cobo_approval', e => {
        try {
          const data = JSON.parse(e.data)
          let extra = {}
          try { extra = JSON.parse(data.message) } catch (err) { /* ignore */ }
          useStore.setState(s => ({
            pendingApprovals: [...s.pendingApprovals, {
              pactId: extra.pact_id || data.job_id,
              jobId: data.job_id,
              amount: extra.amount || '0',
              type: extra.type || 'lock',
              status: 'pending',
              timestamp: Date.now(),
            }]
          }))
          addLog(`🔐 CAW Approval Needed — ${extra.type === 'lock' ? 'Lock' : 'Release'} ${(parseInt(extra.amount || '0') / 1e18).toFixed(4)} ETH`, 'lock')
        } catch (err) { /* ignore */ }
      })
      es.addEventListener('high_value_approval_required', e => {
        try {
          const data = JSON.parse(e.data)
          useStore.setState({ showApproval: true })
          addLog(`💰 High-value approval required — Job #${data.job_id}`, 'info')
        } catch (err) { /* ignore */ }
      })
      es.addEventListener('pact_approved', e => {
        try {
          const data = JSON.parse(e.data)
          useStore.setState(state => ({
            pactStatus: 'active',
            phase: 'published',
            lastPactId: null,
            pendingApprovals: state.pendingApprovals.filter(a => a.jobId !== data.job_id),
          }))
          addLog('✅ CAW pact approved — bounty published!', 'release')
          // Notify AppContext via custom event (phase sync)
          window.dispatchEvent(new CustomEvent('aep_pact_approved', { detail: data }))
        } catch (err) { /* ignore */ }
      })
      es.addEventListener('reputation_changed', e => {
        try {
          const data = JSON.parse(e.data)
          let extra = {}
          try { extra = JSON.parse(data.message) } catch (err) { /* ignore */ }
          if (extra.newScore !== undefined && extra.agent) {
            useStore.getState().updateReputationByAddr(extra.agent, extra.newScore)
          }
          addLog(`🏅 Reputation: ${extra.reason === 'arbitration_slashed' ? '−' : '+'}${Math.abs(extra.delta || 0)} (${extra.agent?.slice(0, 10)}...)`, 'reputation')
        } catch (err) { /* ignore */ }
      })
      es.addEventListener('reputation_updated', e => {
        try {
          const data = JSON.parse(e.data)
          let extra = {}
          try { extra = JSON.parse(data.message) } catch (err) { /* ignore */ }
          if (extra.agent && extra.txHash) {
            useStore.getState().addRepTxHash({ agent: extra.agent, oldScore: extra.oldScore, newScore: extra.newScore, delta: extra.delta, txHash: extra.txHash })
            useStore.getState().updateReputationByAddr(extra.agent, parseInt(extra.newScore))
          }
          addLog(`⛓️ 声誉更新上链: ${extra.agent?.slice(0, 10)}... ${extra.txHash?.slice(0, 18)}...`, 'reputation')
        } catch (err) { /* ignore */ }
      })
      es.addEventListener('agent_thinking', e => {
        try {
          const data = JSON.parse(e.data)
          let extra = {}
          try { extra = JSON.parse(data.message) } catch (err) {}
          const step = extra.step || 'Thinking...'
          useStore.getState().addAgentAction({
            agent: extra.agent || 'unknown',
            status: 'thinking',
            message: step,
            reasoning: extra.job_id ? `Job #${extra.job_id}` : '',
            type: 'thinking',
          })
          addLog(`💭 ${extra.agent?.toUpperCase()} 分析中: ${step.slice(0, 50)}`, 'info')
        } catch (err) {}
      })
      es.addEventListener('agent_decided', e => {
        try {
          const data = JSON.parse(e.data)
          let extra = {}
          try { extra = JSON.parse(data.message) } catch (err) {}
          const needsSub = extra.decision === 'true'
          useStore.getState().addAgentAction({
            agent: extra.agent || 'unknown',
            status: 'decided',
            message: needsSub ? '需要子任务协助' : '可独立完成',
            reasoning: extra.reasoning || '',
            type: 'decided',
          })
          addLog(`🤔 ${extra.agent?.toUpperCase()} 决策: ${extra.reasoning?.slice(0, 60)}`, 'info')
          addLog(`📋 ${extra.agent?.toUpperCase()} 决定: ${needsSub ? '需要子任务 ➔ 发起委托' : '自主完成'}`, 'decided')
        } catch (err) {}
      })
      es.addEventListener('agent_action', e => {
        try {
          const data = JSON.parse(e.data)
          let extra = {}
          try { extra = JSON.parse(data.message) } catch (err) {}
          useStore.getState().addAgentAction({
            agent: extra.agent || 'unknown',
            status: extra.action || 'busy',
            message: extra.action === 'claimed' ? '已接单 ✅' :
                     extra.action === 'claiming' ? '正在接单... ⏳' :
                     extra.action === 'creating_sub_bounty' ? '发起子任务...' :
                     extra.action === 'submitting' ? '提交交付物...' :
                     extra.action === 'submitted' ? '最终交付已提交 ✅' :
                     extra.action === 'claim_failed' ? '接单失败 ❌' :
                     extra.action === 'generating_delivery' ? '生成交付物...' :
                     extra.action === 'submitting_delivery' ? '提交到AEP评估...' :
                     extra.action === 'merging' ? '合并Sub-Provider成果...' :
                     extra.description || extra.action || 'Working...',
            reasoning: extra.action === 'creating_sub_bounty' ? extra.description || '' : '',
            type: extra.action || 'action',
          })
          if (extra.action === 'claiming') {
            addLog(`⏳ ${extra.agent?.toUpperCase()} 正在接单...`, 'info')
          } else if (extra.action === 'claimed') {
            addLog(`✅ ${extra.agent?.toUpperCase()} 接单成功 #${extra.job_id}`, 'claim')
          } else if (extra.action === 'creating_sub_bounty') {
            addLog(`📋 ${extra.agent?.toUpperCase()} 发起子任务: ${extra.description?.slice(0, 40)}`, 'info')
          } else if (extra.action === 'submitted') {
            addLog(`📦 ${extra.agent?.toUpperCase()} 最终交付已提交`, 'submit')
          } else if (extra.action === 'submitting_delivery') {
            addLog(`📤 ${extra.agent?.toUpperCase()} 交付物提交到AEP...`, 'info')
          } else if (extra.action === 'merging') {
            addLog(`🔄 ${extra.agent?.toUpperCase()} 合并Sub-Provider成果中...`, 'info')
          }
        } catch (err) {}
      })
      es.addEventListener('release_pending', e => {
        try {
          const data = JSON.parse(e.data)
          let extra = {}
          try { extra = JSON.parse(data.message) } catch (err) {}
          useStore.setState(s => ({
            pendingApprovals: [...s.pendingApprovals, {
              pactId: extra.pact_id || data.job_id,
              jobId: data.job_id,
              amount: extra.amount || '0',
              type: 'release',
              status: 'pending',
              timestamp: Date.now(),
            }]
          }))
          addLog('🔐 CAW 放款待审批 — 请在 CAW 钱包中确认放款', 'lock')
        } catch (err) { /* ignore */ }
      })
      es.addEventListener('settled', e => {
        try {
          const data = JSON.parse(e.data)
          useStore.setState(s => ({
            pendingApprovals: s.pendingApprovals.filter(a => a.jobId !== data.job_id),
            settled: true,
            phase: 'settled'
          }))
          addLog(`✅ Funds Settled — Job #${data.job_id}`, 'release')
          // Refresh on-chain reputations after settlement
          setTimeout(() => useStore.getState().fetchReputation(), 2000)
          useStore.getState().pollReputationUntilChange()

        } catch (err) { /* ignore */ }
      })
      es.addEventListener('slashed', e => {
        try {
          const data = JSON.parse(e.data)
          useStore.setState(s => ({
            pendingApprovals: s.pendingApprovals.filter(a => a.jobId !== data.job_id),
            phase: 'slashed'
          }))
          addLog(`⛓️ Slashed — Job #${data.job_id}`, 'slash')
        } catch (err) { /* ignore */ }
      })
    }
    connect()
    return () => { if (es) es.close(); clearTimeout(timer) }
  }, [addLog, setPhase, setEvaluation, setPactStatus, addApproval, removeApproval])
}
