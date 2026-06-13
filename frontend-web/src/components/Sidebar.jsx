import React, { useState } from 'react'
import { Box, Card, CardContent, Typography, Button, Chip, Divider, TextField, Dialog, DialogTitle, DialogContent, DialogActions, LinearProgress, Tooltip } from '@mui/material'
import { useApp } from '../context/AppContext'
import useStore, { BUYER_ADDR, PROVIDER_ADDR, SUB_PROVIDER_ADDR } from '../store/useStore'

export default function Sidebar() {
  const { bounties, activeBounty, reputation, repHistory, evaluationResult, loading, claimBounty, submitDelivery, setEvaluationResult, phase, pactStatus, showApproval, setShowApproval } = useApp()
  const providerReputation = useStore(s => s.providerReputation)
  const subProviderReputation = useStore(s => s.subProviderReputation)
  const agentPipeline = useStore(s => s.agentPipeline)
  const pipelineData = useStore(s => s.pipelineData)
  const repTxHashes = useStore(s => s.repTxHashes)
  const [submitOpen, setSubmitOpen] = useState(false)
  const [deliveryText, setDeliveryText] = useState('')
  const [evalOpen, setEvalOpen] = useState(false)
  const [deliveryOpen, setDeliveryOpen] = useState(false)
  const [deliveryContent, setDeliveryContent] = useState('')

  // Get latest state for each agent
  const getAgentState = (agent) => {
    const items = agentPipeline.filter(a => a.agent === agent)
    if (items.length === 0) return null
    return items[items.length - 1]
  }

  const providerState = getAgentState('provider')
  const subProviderState = getAgentState('sub_provider')

  const agentStatusText = (state) => {
    if (!state) return ''
    const labels = {
      thinking: '🤔 分析中', decided: '✅ 已决策', claimed: '✅ 已接单',
      claiming: '⏳ 接单中', creating_sub_bounty: '📋 发子任务',
      generating_delivery: '✍️ 生成中', submitting_delivery: '📤 评估中',
      merging: '🔄 合并中', submitted: '📦 已提交', claim_failed: '❌ 失败',
      auto_chain_failed: '❌ 流程中断',
    }
    return labels[state.status] || state.message || '进行中'
  }

  const viewDelivery = (content) => {
    setDeliveryContent(content)
    setDeliveryOpen(true)
  }

  const noRep = (val) => typeof val !== 'number' || val === 0 ? '—' : val

  const repColor = (val) => {
    if (typeof val !== 'number' || val === 0) return '#64748b'
    return val >= 70 ? '#22c55e' : val >= 40 ? '#eab308' : '#ef4444'
  }

  const repValue = (val) => {
    if (typeof val !== 'number' || val === 0) return 0
    return val
  }

  const handleClaim = async (bountyId) => {
    const result = await claimBounty(bountyId)
    if (result.error) {
      // Could show error toast
    }
  }

  const handleSubmit = async () => {
    const result = await submitDelivery(deliveryText)
    if (result && !result.error) {
      setSubmitOpen(false)
      setDeliveryText('')
    }
  }

  const handleConfirm = async () => {
    await confirmPayment()
  }

  return (
    <Box sx={{ width: 320, overflow: 'auto', borderLeft: '1px solid #1e293b', bgcolor: '#0f172a', p: 1.5, display: 'flex', flexDirection: 'column', gap: 1.5 }}>
      {/* Section 1: Open Bounties */}
      <Card sx={{ border: '1px solid #1e293b', bgcolor: '#111827' }}>
        <CardContent sx={{ p: 1.5, '&:last-child': { pb: 1.5 } }}>
          <Typography variant="caption" fontWeight={700} sx={{ color: '#64748b', mb: 1, display: 'flex', alignItems: 'center', gap: 0.5 }}>
            📋 OPEN BOUNTIES <Chip label={bounties.length} size="small" sx={{ height: 18, fontSize: '0.85rem', bgcolor: '#6366f133', color: '#6366f1' }} />
          </Typography>
          {bounties.length === 0 ? (
            <Typography variant="caption" sx={{ color: '#475569' }}>No open bounties. Create one with Pact Editor.</Typography>
          ) : bounties.map((b, i) => (
            <Box key={b.id} sx={{ p: 1, mb: 0.5, bgcolor: '#0a0e1a', borderRadius: 1, border: '1px solid #1e293b' }}>
              <Typography variant="caption" fontWeight={600} sx={{ color: '#e2e8f0', display: 'block' }}>{b.title}</Typography>
              <Box sx={{ display: 'flex', gap: 1, mt: 0.3 }}>
                <Typography variant="caption" sx={{ color: '#22c55e', fontSize: '0.75rem' }}>{b.reward} ETH</Typography>
                <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.75rem' }}>Due: {b.deadline}</Typography>
                {b.pactStatus === 'pending_approval' && (
                  <Chip label="Pending" size="small" sx={{ height: 16, fontSize: '0.8rem', bgcolor: '#eab30833', color: '#eab308' }} />
                )}
                {(!b.pactStatus || b.pactStatus === 'active') && (
                  <Chip label="Ready" size="small" sx={{ height: 16, fontSize: '0.8rem', bgcolor: '#22c55e33', color: '#22c55e' }} />
                )}
              </Box>
            </Box>
          ))}
        </CardContent>
      </Card>

      {/* Section 2: Agent Information */}
      <Card sx={{ border: '1px solid #1e293b', bgcolor: '#111827' }}>
        <CardContent sx={{ p: 1.5, '&:last-child': { pb: 1.5 } }}>
          <Typography variant="caption" fontWeight={700} sx={{ color: '#64748b', mb: 1, display: 'flex', alignItems: 'center', gap: 0.5 }}>
            🏆 AGENT INFORMATION
          </Typography>

          {/* Buyer Agent */}
          <Box sx={{ p: 1, bgcolor: '#0a0e1a', borderRadius: 1, border: '1px solid #1e293b', mb: 1 }}>
            <Typography variant="caption" fontWeight={600} sx={{ color: '#6366f1', display: 'block', fontSize: '0.75rem' }}>
              🟣 Buyer Agent
            </Typography>
            <Typography variant="caption" sx={{ color: '#475569', fontSize: '0.8rem', display: 'block', mb: 0.3 }}>
              ID: {BUYER_ADDR ? `${BUYER_ADDR.slice(0, 10)}...${BUYER_ADDR.slice(-4)}` : '加载中...'}
            </Typography>
            <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.8rem', display: 'block', mb: 0.5 }}>
              发布悬赏任务，通过 CAW Pact 锁定资金
            </Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <Typography variant="caption" fontWeight={700} sx={{ color: reputation >= 70 ? '#22c55e' : reputation >= 40 ? '#eab308' : '#ef4444', fontSize: '0.85rem' }}>
                {reputation}
              </Typography>
              <Box sx={{ flex: 1 }}>
                <LinearProgress variant="determinate" value={reputation}
                  sx={{ height: 4, borderRadius: 2, bgcolor: '#1e293b', '& .MuiLinearProgress-bar': { bgcolor: reputation >= 70 ? '#22c55e' : reputation >= 40 ? '#eab308' : '#ef4444' } }} />
              </Box>
              <Typography variant="caption" sx={{ color: '#475569', fontSize: '0.8rem' }}>{reputation}/100</Typography>
            </Box>
          </Box>

          {/* Provider Agent */}
          <Box sx={{ p: 1, bgcolor: '#0a0e1a', borderRadius: 1, border: '1px solid #1e293b', mb: 1 }}>
            <Typography variant="caption" fontWeight={600} sx={{ color: '#eab308', display: 'block', fontSize: '0.75rem' }}>
              🟡 Provider Agent
            </Typography>
            <Typography variant="caption" sx={{ color: '#475569', fontSize: '0.8rem', display: 'block', mb: 0.3 }}>
              ID: {PROVIDER_ADDR ? `${PROVIDER_ADDR.slice(0, 10)}...${PROVIDER_ADDR.slice(-4)}` : '加载中...'}
            </Typography>
            <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.8rem', display: 'block', mb: 0.5 }}>
              自动接单 → 分析任务(LLM) → 发布子任务 → 合并提交
            </Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.5 }}>
              <Typography variant="caption" sx={{ color: '#eab308', fontSize: '0.85rem', fontWeight: 600 }}>
                {agentStatusText(providerState)}
              </Typography>
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <Typography variant="caption" fontWeight={700} sx={{ color: repColor(providerReputation), fontSize: '0.85rem' }}>
                {noRep(providerReputation)}
              </Typography>
              <Box sx={{ flex: 1 }}>
                <LinearProgress variant="determinate" value={repValue(providerReputation)}
                  sx={{ height: 4, borderRadius: 2, bgcolor: '#1e293b', '& .MuiLinearProgress-bar': { bgcolor: repColor(providerReputation) } }} />
              </Box>
              <Typography variant="caption" sx={{ color: '#475569', fontSize: '0.8rem' }}>{noRep(providerReputation)}/100</Typography>
            </Box>
          </Box>

          {/* Sub-Provider Agent */}
          <Box sx={{ p: 1, bgcolor: '#0a0e1a', borderRadius: 1, border: '1px solid #1e293b' }}>
            <Typography variant="caption" fontWeight={600} sx={{ color: '#f472b6', display: 'block', fontSize: '0.75rem' }}>
              🟢 Sub-Provider Agent
            </Typography>
            <Typography variant="caption" sx={{ color: '#475569', fontSize: '0.8rem', display: 'block', mb: 0.3 }}>
              ID: {SUB_PROVIDER_ADDR ? `${SUB_PROVIDER_ADDR.slice(0, 10)}...${SUB_PROVIDER_ADDR.slice(-4)}` : '加载中...'}
            </Typography>
            <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.8rem', display: 'block', mb: 0.5 }}>
              自动接子单 → LLM生成交付物 → 提交 → AEP评估
            </Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.5 }}>
              <Typography variant="caption" sx={{ color: '#f472b6', fontSize: '0.85rem', fontWeight: 600 }}>
                {agentStatusText(subProviderState)}
              </Typography>
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <Typography variant="caption" fontWeight={700} sx={{ color: repColor(subProviderReputation), fontSize: '0.85rem' }}>
                {noRep(subProviderReputation)}
              </Typography>
              <Box sx={{ flex: 1 }}>
                <LinearProgress variant="determinate" value={repValue(subProviderReputation)}
                  sx={{ height: 4, borderRadius: 2, bgcolor: '#1e293b', '& .MuiLinearProgress-bar': { bgcolor: repColor(subProviderReputation) } }} />
              </Box>
              <Typography variant="caption" sx={{ color: '#475569', fontSize: '0.8rem' }}>{noRep(subProviderReputation)}/100</Typography>
            </Box>
          </Box>
        </CardContent>
      </Card>

      {/* Section 3: Provider Workflow Pipeline */}
      <Card sx={{ border: pipelineData?.step ? '1px solid #eab308' : '1px solid #1e293b', bgcolor: '#111827', maxHeight: 300, overflow: 'auto' }}>
        <CardContent sx={{ p: 1.5, '&:last-child': { pb: 1.5 } }}>
          <Typography variant="caption" fontWeight={700} sx={{ color: '#eab308', mb: 1, display: 'flex', alignItems: 'center', gap: 0.5 }}>
            ⚡ PROVIDER WORKFLOW
            {pipelineData?.step && <Chip label={pipelineData.step} size="small" sx={{ height: 16, fontSize: '0.8rem', bgcolor: '#eab30833', color: '#eab308' }} />}
          </Typography>

          {!pipelineData?.step ? (
            <Typography variant="caption" sx={{ color: '#475569' }}>等待自动流程启动...</Typography>
          ) : (
            <>
              {/* Step indicator */}
              <Box sx={{ display: 'flex', gap: 0.3, mb: 1, flexWrap: 'wrap' }}>
                {['claiming','analyzing','decided','creating_sub_bounty','sub_claimed','generating_sub_delivery','submitted','evaluating_final','evaluated_verified','settled'].map(s => {
                  const steps = {claiming:'接单',analyzing:'分析',decided:'决策',creating_sub_bounty:'子任务',sub_claimed:'子接单',generating_sub_delivery:'子交付',submitted:'提交',evaluating_final:'评估',evaluated_verified:'通过',settled:'放款'}
                  const done = ['claiming','analyzing','decided','creating_sub_bounty','sub_claimed','generating_sub_delivery','submitted','evaluating_final','evaluated_verified','settled'].indexOf(s) <= ['claiming','analyzing','decided','creating_sub_bounty','sub_claimed','generating_sub_delivery','submitted','evaluating_final','evaluated_verified','settled'].indexOf(pipelineData.step)
                  return <Chip key={s} label={steps[s]||s} size="small" sx={{ height: 14, fontSize: '0.75rem', bgcolor: done ? '#22c55e33' : '#1e293b', color: done ? '#22c55e' : '#475569' }} />
                })}
              </Box>

              {/* Reasoning */}
              {pipelineData.reasoning && (
                <Box sx={{ p: 0.8, bgcolor: '#0a0e1a', borderRadius: 1, border: '1px solid #1e293b', mb: 0.8 }}>
                  <Typography variant="caption" fontWeight={600} sx={{ color: '#a855f7', fontSize: '0.85rem', display: 'block', mb: 0.3 }}>
                    💭 LLM 推理
                  </Typography>
                  <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.8rem', display: 'block', lineHeight: 1.3 }}>
                    {pipelineData.reasoning}
                  </Typography>
                </Box>
              )}

              {/* Agent Action Log */}
              {agentPipeline.length > 0 && (
                <Box sx={{ maxHeight: 120, overflow: 'auto', mb: 0.8, '&::-webkit-scrollbar': { width: 3 } }}>
                  {agentPipeline.slice(-5).map((item, i) => {
                    const sd = {thinking:{icon:'💭'},decided:{icon:'🤔'},claimed:{icon:'✅'},claiming:{icon:'⏳'},creating_sub_bounty:{icon:'📋'},generating_delivery:{icon:'✍️'},submitting:{icon:'📤'},submitted:{icon:'📦'}}
                    const icon = sd[item.status]?.icon || '•'
                    return (
                      <Typography key={i} variant="caption" sx={{ color: '#64748b', fontSize: '0.63rem', display: 'block', py: 0.1 }}>
                        {icon} {item.agent === 'provider' ? 'Provider' : 'Sub-Provider'} — {item.message}
                      </Typography>
                    )
                  })}
                </Box>
              )}

              {/* Delivery buttons */}
              <Box sx={{ display: 'flex', gap: 0.5, mb: 0.8, flexWrap: 'wrap' }}>
                {pipelineData.sub_delivery && (
                  <Button size="small" variant="outlined" onClick={() => viewDelivery(pipelineData.sub_delivery)}
                    sx={{ fontSize: '0.8rem', borderColor: '#f472b644', color: '#f472b6', py: 0.1, minWidth: 0 }}>
                    📄 子交付物
                  </Button>
                )}
                {pipelineData.final_delivery && (
                  <Button size="small" variant="outlined" onClick={() => viewDelivery(pipelineData.final_delivery)}
                    sx={{ fontSize: '0.8rem', borderColor: '#22c55e44', color: '#22c55e', py: 0.1, minWidth: 0 }}>
                    📄 最终交付物
                  </Button>
                )}
              </Box>

              {/* Evaluation result */}
              {pipelineData.eval_status && (
                <Box sx={{ p: 0.8, bgcolor: '#0a0e1a', borderRadius: 1, border: `1px solid ${pipelineData.eval_status === 'verified' ? '#22c55e44' : '#ef444444'}`, mb: 0.8 }}>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <Typography variant="caption" fontWeight={600} sx={{ color: pipelineData.eval_status === 'verified' ? '#22c55e' : '#ef4444', fontSize: '0.85rem' }}>
                      {pipelineData.eval_status === 'verified' ? '✅ 评估通过' : '❌ 评估不通过'}
                    </Typography>
                    <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.8rem' }}>
                      {(pipelineData.eval_score * 100).toFixed(0)}分
                    </Typography>
                  </Box>
                  <Box sx={{ display: 'flex', gap: 0.5, mt: 0.5 }}>
                    <Button size="small" variant="outlined" onClick={() => setEvalOpen(true)}
                      sx={{ fontSize: '0.75rem', borderColor: '#6366f144', color: '#6366f1', py: 0, minWidth: 0, height: 16, lineHeight: 1 }}>
                      📊 评估详情
                    </Button>
                  </Box>
                </Box>
              )}

              {/* Waiting for confirmation — show confirm button */}
              {pipelineData.step === 'awaiting_confirmation' && (
                <Box>
                  <Chip label="⏳ 评估通过，等待 Buyer 确认放款" size="small"
                    sx={{ width: '100%', fontWeight: 600, bgcolor: '#22c55e22', color: '#22c55e', border: '1px solid #22c55e44', fontSize: '0.85rem', mb: 0.5 }} />
                  <Button size="small" fullWidth variant="contained"
                    onClick={async () => {
                      await useStore.getState().confirmRelease()
                    }}
                    sx={{ fontSize: '0.75rem', bgcolor: '#22c55e', color: '#000', '&:hover': { bgcolor: '#16a34a' }, py: 0.3 }}>
                    🔐 确认放款 (CAW Release)
                  </Button>
                </Box>
              )}
              {pipelineData.step === 'settled' && (
                <Chip label="✅ 放款完成，全链路结束" size="small"
                  sx={{ width: '100%', fontWeight: 600, bgcolor: '#22c55e22', color: '#22c55e', border: '1px solid #22c55e44', fontSize: '0.85rem' }} />
              )}

              {/* Slashed - show arbitration */}
              {pipelineData.step === 'evaluated_slashed' && (
                <Chip label="❌ 评估不通过 — 需要仲裁" size="small" color="error"
                  sx={{ width: '100%', fontWeight: 700, fontSize: '0.85rem' }} />
              )}
              {/* Auto-chain failed */}
              {pipelineData.step === 'auto_chain_failed' && (
                <Chip label="❌ 自动流程中断 — 请刷新页面重试" size="small"
                  sx={{ width: '100%', fontWeight: 600, bgcolor: '#ef444422', color: '#ef4444', border: '1px solid #ef444444', fontSize: '0.75rem' }} />
              )}
            </>
          )}
        </CardContent>
      </Card>

      {/* Submit Dialog */}
      <Dialog open={submitOpen} onClose={() => setSubmitOpen(false)} PaperProps={{ sx: { bgcolor: '#111827', border: '1px solid #1e293b', color: '#e2e8f0' } }}>
        <DialogTitle sx={{ fontSize: '0.9rem', fontWeight: 700 }}>Submit Deliverable</DialogTitle>
        <DialogContent>
          <TextField autoFocus multiline minRows={4} fullWidth size="small"
            placeholder="Describe your deliverable..."
            value={deliveryText} onChange={e => setDeliveryText(e.target.value)}
            sx={{ mt: 1, '& .MuiOutlinedInput-root': { bgcolor: '#0a0e1a', fontSize: '0.8rem' } }} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setSubmitOpen(false)} size="small" sx={{ color: '#64748b' }}>Cancel</Button>
          <Button onClick={handleSubmit} variant="contained" size="small" disabled={loading || !deliveryText.trim()}
            sx={{ bgcolor: '#eab308', color: '#000', '&:hover': { bgcolor: '#facc15' } }}>
            Submit & Evaluate
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delivery dialog */}
      <Dialog open={deliveryOpen} onClose={() => setDeliveryOpen(false)}
        PaperProps={{ sx: { bgcolor: '#111827', border: '1px solid #1e293b', color: '#e2e8f0', maxWidth: '90vw', width: 800, height: '80vh' } }}>
        <DialogTitle sx={{ fontSize: '0.85rem', fontWeight: 700, borderBottom: '1px solid #1e293b' }}>📄 交付物内容</DialogTitle>
        <DialogContent sx={{ overflow: 'auto' }}>
          <Typography variant="body2" sx={{ color: '#94a3b8', fontSize: '0.75rem', whiteSpace: 'pre-wrap', fontFamily: 'monospace', lineHeight: 1.6 }}>
            {deliveryContent}
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeliveryOpen(false)} size="small" sx={{ color: '#64748b' }}>关闭</Button>
        </DialogActions>
      </Dialog>

      {/* Evaluation detail dialog */}
      <Dialog open={evalOpen} onClose={() => setEvalOpen(false)}
        PaperProps={{ sx: { bgcolor: '#111827', border: '1px solid #1e293b', color: '#e2e8f0', maxWidth: 600, minWidth: 400 } }}>
        <DialogTitle sx={{ fontSize: '0.85rem', fontWeight: 700, borderBottom: '1px solid #1e293b' }}>📊 评估详情</DialogTitle>
        <DialogContent sx={{ pt: 2, overflow: 'auto' }}>
          {/* ---- Sub-Task Evaluation ---- */}
          {pipelineData?.sub_eval_status && (
            <>
              <Typography variant="caption" fontWeight={700} sx={{ color: '#f472b6', fontSize: '0.8rem', display: 'block', mb: 0.5 }}>
                🟢 子任务评估
              </Typography>
              <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.85rem', display: 'block', mb: 1 }}>
                裁决: <span style={{ color: pipelineData.sub_eval_status === 'verified' ? '#22c55e' : '#ef4444' }}>
                  {pipelineData.sub_eval_status === 'verified' ? '✅ PASS' : '❌ FAIL'}
                </span> | 分数: {(pipelineData.sub_eval_score * 100 || 0).toFixed(0)}分
              </Typography>
              <Typography variant="caption" fontWeight={700} sx={{ color: '#6366f1', fontSize: '0.85rem', display: 'block', mb: 0.3 }}>🔍 规则引擎</Typography>
              {pipelineData.sub_eval_rule_breakdown?.split(', ').map((part, i) => {
                const m = part.match(/(\w+):(PASS|FAIL)\(([\d.]+)\)/)
                if (!m) return null
                return (
                  <Box key={i} sx={{ display: 'flex', justifyContent: 'space-between', py: 0.2, px: 1, bgcolor: '#0a0e1a', borderRadius: 1, mb: 0.3 }}>
                    <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.8rem' }}>{m[1]}</Typography>
                    <Typography variant="caption" sx={{ color: m[2] === 'PASS' ? '#22c55e' : '#ef4444', fontSize: '0.8rem', fontWeight: 600 }}>
                      {m[2] === 'PASS' ? '✅' : '❌'} {(parseFloat(m[3]) * 100).toFixed(0)}分
                    </Typography>
                  </Box>
                )
              })}
              <Box sx={{ bgcolor: '#0a0e1a', borderRadius: 1, p: 0.8, mb: 1.5 }}>
                <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.8rem', display: 'block', mb: 0.2 }}>🤖 LLM评分: {(pipelineData.sub_eval_llm_score * 100 || 0).toFixed(0)}分</Typography>
                <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.8rem', fontStyle: 'italic', lineHeight: 1.3 }}>{pipelineData.sub_eval_llm_reason || pipelineData.sub_eval_summary || '无'}</Typography>
              </Box>
              <Divider sx={{ my: 1.5, borderColor: '#1e293b' }} />
            </>
          )}

          {/* ---- Main Task Evaluation ---- */}
          <Typography variant="caption" fontWeight={700} sx={{ color: '#eab308', fontSize: '0.8rem', display: 'block', mb: 0.5 }}>
            🟡 主任务评估
          </Typography>
          <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.85rem', display: 'block', mb: 1 }}>
            总体裁决: <span style={{ color: pipelineData?.eval_status === 'verified' ? '#22c55e' : '#ef4444' }}>
              {pipelineData?.eval_status === 'verified' ? '✅ PASS' : '❌ FAIL'}
            </span> | 综合分数: <span style={{ color: '#e2e8f0' }}>{(pipelineData?.eval_score * 100 || 0).toFixed(0)}分</span>
          </Typography>
          <Typography variant="caption" fontWeight={700} sx={{ color: '#6366f1', fontSize: '0.85rem', display: 'block', mb: 0.3 }}>🔍 规则引擎评估</Typography>
          {pipelineData?.eval_rule_breakdown?.split(', ').map((part, i) => {
            const m = part.match(/(\w+):(PASS|FAIL)\(([\d.]+)\)/)
            if (!m) return null
            return (
              <Box key={i} sx={{ display: 'flex', justifyContent: 'space-between', py: 0.2, px: 1, bgcolor: '#0a0e1a', borderRadius: 1, mb: 0.3 }}>
                <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.8rem' }}>{m[1]}</Typography>
                <Typography variant="caption" sx={{ color: m[2] === 'PASS' ? '#22c55e' : '#ef4444', fontSize: '0.8rem', fontWeight: 600 }}>
                  {m[2] === 'PASS' ? '✅' : '❌'} {(parseFloat(m[3]) * 100).toFixed(0)}分
                </Typography>
              </Box>
            )
          })}
          <Box sx={{ bgcolor: '#0a0e1a', borderRadius: 1, p: 0.8, mb: 1 }}>
            <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.8rem', display: 'block', mb: 0.2 }}>🤖 LLM 评分: {(pipelineData?.eval_llm_score * 100 || 0).toFixed(0)}分</Typography>
            <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.8rem', fontStyle: 'italic', lineHeight: 1.3 }}>{pipelineData?.eval_llm_reason || pipelineData?.eval_summary || '无'}</Typography>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEvalOpen(false)} size="small" sx={{ color: '#64748b' }}>关闭</Button>
        </DialogActions>
      </Dialog>

    </Box>
  )
}
