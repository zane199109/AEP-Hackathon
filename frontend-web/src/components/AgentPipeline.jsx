import { Box, Typography, Chip, Collapse, IconButton, Button, Dialog, DialogTitle, DialogContent, DialogActions, Divider } from '@mui/material'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import ExpandLessIcon from '@mui/icons-material/ExpandLess'
import useStore from '../store/useStore'
import { useState } from 'react'

const AGENT_COLORS = { buyer: '#6366f1', provider: '#eab308', sub_provider: '#f472b6', evaluator: '#22c55e' }
const AGENT_LABELS = { buyer: 'Buyer', provider: 'Provider', sub_provider: 'Sub-Provider', evaluator: 'Evaluator' }

const STATUS_DISPLAY = {
  thinking: { icon: '💭', label: '分析中', color: '#a855f7' },
  decided: { icon: '🤔', label: '已决策', color: '#6366f1' },
  claimed: { icon: '✅', label: '已接单', color: '#22c55e' },
  claiming: { icon: '⏳', label: '接单中', color: '#eab308' },
  creating_sub_bounty: { icon: '📋', label: '发起子任务', color: '#f472b6' },
  generating_delivery: { icon: '✍️', label: '生成交付物', color: '#a855f7' },
  generating_sub_delivery: { icon: '✍️', label: '子交付物', color: '#a855f7' },
  submitting: { icon: '📤', label: '提交评估', color: '#eab308' },
  submitting_delivery: { icon: '📤', label: '提交评估', color: '#eab308' },
  merging: { icon: '🔄', label: '合并成果', color: '#f472b6' },
  submitted: { icon: '📦', label: '已提交', color: '#22c55e' },
  evaluating_sub: { icon: '🛡️', label: '评估子任务', color: '#a855f7' },
  sub_verified: { icon: '✅', label: '子任务通过', color: '#22c55e' },
  evaluating_final: { icon: '🛡️', label: '评估主任务', color: '#a855f7' },
  evaluated_verified: { icon: '✅', label: '评估通过', color: '#22c55e' },
  evaluated_slashed: { icon: '❌', label: '评估不通过', color: '#ef4444' },
  claim_failed: { icon: '❌', label: '接单失败', color: '#ef4444' },
}

function parseRuleBreakdown(breakdown) {
  if (!breakdown) return []
  return breakdown.split(', ').map(part => {
    const m = part.match(/(\w+):(PASS|FAIL)\(([\d.]+)\)/)
    if (m) return { name: m[1], passed: m[2] === 'PASS', score: parseFloat(m[3]) }
    return { name: part, passed: false, score: 0 }
  })
}

export default function AgentPipeline() {
  const agentPipeline = useStore(s => s.agentPipeline)
  const pipelineData = useStore(s => s.pipelineData)
  const [expanded, setExpanded] = useState(true)
  const [deliveryOpen, setDeliveryOpen] = useState(false)
  const [deliveryContent, setDeliveryContent] = useState('')
  const [evalOpen, setEvalOpen] = useState(false)

  if (agentPipeline.length === 0 && !pipelineData) return null

  const latestByAgent = {}
  agentPipeline.forEach(item => { latestByAgent[item.agent] = item })

  const viewDelivery = (content) => {
    setDeliveryContent(content)
    setDeliveryOpen(true)
  }

  const ruleItems = parseRuleBreakdown(pipelineData?.eval_rule_breakdown)

  return (
    <>
      <Box sx={{
        position: 'absolute', bottom: 12, right: 12, width: 320, zIndex: 20,
        bgcolor: 'rgba(15,23,42,0.95)', backdropFilter: 'blur(8px)',
        border: '1px solid #6366f144', borderRadius: 2, overflow: 'hidden',
        boxShadow: '0 4px 20px rgba(0,0,0,0.5)',
      }}>
        <Box onClick={() => setExpanded(!expanded)}
          sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', p: 1.5, cursor: 'pointer',
            borderBottom: expanded ? '1px solid #1e293b' : 'none' }}
        >
          <Typography variant="caption" fontWeight={700} sx={{ color: '#6366f1', letterSpacing: 1, display: 'flex', alignItems: 'center', gap: 0.5 }}>
            🤖 AGENT PIPELINE
            <Chip label={agentPipeline.length} size="small" sx={{ height: 16, fontSize: '0.5rem', bgcolor: '#6366f133', color: '#6366f1' }} />
          </Typography>
          <IconButton size="small" sx={{ color: '#475569', p: 0.2 }}>
            {expanded ? <ExpandLessIcon sx={{ fontSize: '0.9rem' }} /> : <ExpandMoreIcon sx={{ fontSize: '0.9rem' }} />}
          </IconButton>
        </Box>

        <Collapse in={expanded} sx={{ maxHeight: 500, overflow: 'auto',
          '&::-webkit-scrollbar': { width: 4 },
          '&::-webkit-scrollbar-track': { background: '#0a0e1a' },
          '&::-webkit-scrollbar-thumb': { background: '#2a2a3a', borderRadius: 2 },
        }}>
          <Box sx={{ p: 1 }}>
            {Object.keys(latestByAgent).length > 0 && (
              <Box sx={{ display: 'flex', gap: 0.5, mb: 1, flexWrap: 'wrap' }}>
                {Object.entries(latestByAgent).map(([agent, item]) => {
                  const color = AGENT_COLORS[agent] || '#94a3b8'
                  const sd = STATUS_DISPLAY[item.status] || { icon: '•', label: item.status, color: '#94a3b8' }
                  return (
                    <Chip key={agent}
                      label={`${sd.icon} ${AGENT_LABELS[agent] || agent}: ${sd.label}`}
                      size="small"
                      sx={{ height: 20, fontSize: '0.55rem', bgcolor: `${color}22`, color, border: `1px solid ${color}44`, fontWeight: 600 }}
                    />
                  )
                })}
              </Box>
            )}

            {agentPipeline.map((item, i) => {
              const color = AGENT_COLORS[item.agent] || '#94a3b8'
              const sd = STATUS_DISPLAY[item.status] || { icon: '•', label: item.status || 'unknown', color: '#94a3b8' }
              const isLatest = i === agentPipeline.length - 1
              return (
                <Box key={i} sx={{ display: 'flex', gap: 1, py: 0.4, px: 0.5,
                  borderLeft: `2px solid ${color}44`, ml: 0.5, mb: 0.3,
                  bgcolor: isLatest ? `${color}11` : 'transparent', borderRadius: '0 4px 4px 0',
                }}>
                  <Typography variant="caption" sx={{ fontSize: '0.65rem', flexShrink: 0, mt: 0.1, color: sd.color }}>{sd.icon}</Typography>
                  <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                      <Typography variant="caption" fontWeight={600} sx={{ color, fontSize: '0.6rem' }}>{AGENT_LABELS[item.agent] || item.agent?.toUpperCase()}</Typography>
                      <Typography variant="caption" sx={{ color: sd.color, fontSize: '0.55rem', fontWeight: 600 }}>{sd.label}</Typography>
                    </Box>
                    {item.reasoning && (
                      <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.5rem', display: 'block', mt: 0.2, fontStyle: 'italic', borderLeft: '1px solid #334155', pl: 0.5 }}>
                        {item.reasoning}
                      </Typography>
                    )}
                  </Box>
                </Box>
              )
            })}

            {pipelineData && pipelineData.step && (
              <Box sx={{ mt: 1, pt: 1, borderTop: '1px solid #1e293b' }}>
                {pipelineData.reasoning && (
                  <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.55rem', display: 'block', mb: 0.5 }}>
                    💡 {pipelineData.reasoning}
                  </Typography>
                )}
                {pipelineData.eval_status && (
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.5 }}>
                    <Typography variant="caption" sx={{ color: pipelineData.eval_status === 'verified' ? '#22c55e' : '#ef4444', fontSize: '0.55rem' }}>
                      📊 评估: {pipelineData.eval_status} | {(pipelineData.eval_score * 100).toFixed(0)}分
                    </Typography>
                    <Button size="small" variant="outlined"
                      onClick={() => setEvalOpen(true)}
                      sx={{ fontSize: '0.45rem', borderColor: '#6366f144', color: '#94a3b8', py: 0, minWidth: 0, height: 16, lineHeight: 1 }}>
                      详情
                    </Button>
                  </Box>
                )}
                <Box sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap' }}>
                  {pipelineData.sub_delivery && (
                    <Button size="small" variant="outlined"
                      onClick={() => viewDelivery(pipelineData.sub_delivery)}
                      sx={{ fontSize: '0.5rem', borderColor: '#6366f144', color: '#f472b6', py: 0.1, minWidth: 0 }}>
                      查看子交付物
                    </Button>
                  )}
                  {pipelineData.final_delivery && (
                    <Button size="small" variant="outlined"
                      onClick={() => viewDelivery(pipelineData.final_delivery)}
                      sx={{ fontSize: '0.5rem', borderColor: '#6366f144', color: '#22c55e', py: 0.1, minWidth: 0 }}>
                      查看最终交付物
                    </Button>
                  )}
                </Box>
              </Box>
            )}
          </Box>
        </Collapse>
      </Box>

      {/* Delivery dialog */}
      <Dialog open={deliveryOpen} onClose={() => setDeliveryOpen(false)}
        PaperProps={{ sx: { bgcolor: '#111827', border: '1px solid #1e293b', color: '#e2e8f0', maxWidth: 600 } }}>
        <DialogTitle sx={{ fontSize: '0.85rem', fontWeight: 700, borderBottom: '1px solid #1e293b' }}>📄 交付物内容</DialogTitle>
        <DialogContent>
          <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.7rem', whiteSpace: 'pre-wrap', display: 'block', mt: 1, fontFamily: 'monospace', lineHeight: 1.5 }}>
            {deliveryContent}
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeliveryOpen(false)} size="small" sx={{ color: '#64748b' }}>关闭</Button>
        </DialogActions>
      </Dialog>

      {/* Evaluation detail dialog */}
      <Dialog open={evalOpen} onClose={() => setEvalOpen(false)}
        PaperProps={{ sx: { bgcolor: '#111827', border: '1px solid #1e293b', color: '#e2e8f0', maxWidth: 500, minWidth: 350 } }}>
        <DialogTitle sx={{ fontSize: '0.85rem', fontWeight: 700, borderBottom: '1px solid #1e293b' }}>
          📊 评估详情
        </DialogTitle>
        <DialogContent sx={{ pt: 2 }}>
          {/* Verdict */}
          <Typography variant="caption" fontWeight={700} sx={{ color: '#94a3b8', fontSize: '0.6rem', display: 'block', mb: 1 }}>
            总体裁决: <span style={{ color: pipelineData?.eval_status === 'verified' ? '#22c55e' : '#ef4444' }}>
              {pipelineData?.eval_status === 'verified' ? '✅ PASS' : '❌ FAIL'}
            </span>
            {' | '}综合分数: <span style={{ color: '#e2e8f0' }}>{(pipelineData?.eval_score * 100 || 0).toFixed(0)}分</span>
          </Typography>

          <Divider sx={{ my: 1, borderColor: '#1e293b' }} />

          {/* Rule breakdown */}
          <Typography variant="caption" fontWeight={700} sx={{ color: '#6366f1', fontSize: '0.6rem', display: 'block', mb: 1 }}>
            🔍 规则引擎评估
          </Typography>
          {ruleItems.map((r, i) => (
            <Box key={i} sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', py: 0.3, px: 1, bgcolor: '#0a0e1a', borderRadius: 1, mb: 0.5 }}>
              <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.55rem' }}>{r.name}</Typography>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Typography variant="caption" sx={{ color: r.passed ? '#22c55e' : '#ef4444', fontSize: '0.5rem', fontWeight: 600 }}>
                  {r.passed ? '✅ PASS' : '❌ FAIL'}
                </Typography>
                <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.5rem' }}>
                  {(r.score * 100).toFixed(0)}分
                </Typography>
              </Box>
            </Box>
          ))}
          <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.5rem', display: 'block', mt: 0.5, mb: 1 }}>
            规则得分: {(pipelineData?.eval_score * 100 || 0).toFixed(0)}分
          </Typography>

          <Divider sx={{ my: 1, borderColor: '#1e293b' }} />

          {/* LLM evaluation */}
          <Typography variant="caption" fontWeight={700} sx={{ color: '#a855f7', fontSize: '0.6rem', display: 'block', mb: 1 }}>
            🤖 LLM 评估
          </Typography>
          <Box sx={{ bgcolor: '#0a0e1a', borderRadius: 1, p: 1 }}>
            <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.55rem', display: 'block', mb: 0.5 }}>
              LLM 评分: {(pipelineData?.eval_llm_score * 100 || 0).toFixed(0)}分
            </Typography>
            <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.5rem', display: 'block', fontStyle: 'italic', lineHeight: 1.4 }}>
              {pipelineData?.eval_llm_reason || pipelineData?.eval_summary || '无'}
            </Typography>
          </Box>

          {pipelineData?.eval_summary && (
            <>
              <Divider sx={{ my: 1, borderColor: '#1e293b' }} />
              <Typography variant="caption" fontWeight={700} sx={{ color: '#94a3b8', fontSize: '0.55rem', display: 'block', mb: 0.5 }}>
                摘要
              </Typography>
              <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.5rem', display: 'block' }}>
                {pipelineData.eval_summary}
              </Typography>
            </>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEvalOpen(false)} size="small" sx={{ color: '#64748b' }}>关闭</Button>
        </DialogActions>
      </Dialog>
    </>
  )
}
