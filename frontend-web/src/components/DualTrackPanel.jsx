import React from 'react'
import { Box, Typography, LinearProgress, Paper, IconButton } from '@mui/material'
import CloseIcon from '@mui/icons-material/Close'
import { useApp } from '../context/AppContext'

export default function DualTrackPanel() {
  const { evaluationResult, ruleResults, setEvaluationResult } = useApp()
  if (!evaluationResult) return null

  const { passed, status, score, rule_score, llm_score, llm_reason, summary } = evaluationResult
  const llmDegraded = llm_score === undefined || llm_score === null
  const llmUnavailable = llm_score === 0 && !llm_reason && !llmDegraded
  const isSlashed = status === 'slashed'
  const isPendingReview = status === 'pending_review'
  const finalColor = passed ? '#22c55e' : isSlashed ? '#ef4444' : '#eab308'

  return (
    <Paper elevation={4} sx={{
      position: 'absolute', bottom: 12, left: 12, width: 260,
      bgcolor: 'rgba(15,23,42,0.92)', backdropFilter: 'blur(8px)',
      border: `1px solid ${finalColor}44`, borderRadius: 2, p: 1.5,
      zIndex: 10,
    }}>
      {/* Header */}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1, justifyContent: 'space-between' }}>
        <Typography variant="caption" fontWeight={700} sx={{ color: '#64748b', letterSpacing: 1 }}>
          🤖 DUAL-TRACK EVALUATION
        </Typography>
        {llmDegraded && passed && (
          <Box sx={{
            px: 0.5, py: 0.1, borderRadius: 0.5,
            bgcolor: '#eab30822', border: '1px solid #eab308',
          }}>
            <Typography variant="caption" sx={{ color: '#eab308', fontSize: '0.5rem', fontWeight: 600 }}>
              ⚠️ LLM DEGRADED
            </Typography>
          </Box>
        )}
        <IconButton size="small" onClick={() => setEvaluationResult(null)} sx={{ color: '#475569', p: 0.2 }}>
          <CloseIcon sx={{ fontSize: '0.9rem' }} />
        </IconButton>
        {llmUnavailable && (
          <Box sx={{
            px: 0.5, py: 0.1, borderRadius: 0.5,
            bgcolor: '#ef444422', border: '1px solid #ef4444',
          }}>
            <Typography variant="caption" sx={{ color: '#ef4444', fontSize: '0.5rem', fontWeight: 600 }}>
              🚫 LLM UNAVAILABLE
            </Typography>
          </Box>
        )}
      </Box>

      {/* Verdict badge */}
      <Box sx={{
        display: 'inline-flex', alignItems: 'center', gap: 0.5, px: 1, py: 0.3, borderRadius: 1, mb: 1,
        bgcolor: isSlashed ? '#ef444422' : passed ? '#22c55e22' : '#eab30822',
        border: `1px solid ${finalColor}`,
      }}>
        <Typography variant="caption" fontWeight={700} sx={{ color: finalColor }}>
          {passed ? '✅ PASS' : isSlashed ? '❌ SLASHED' : '⚠️ REVIEW'}
        </Typography>
        <Typography variant="caption" sx={{ color: '#64748b' }}>|</Typography>
        <Typography variant="caption" sx={{ color: '#94a3b8' }}>{status}</Typography>
      </Box>

      {/* Rule track */}
      <Box sx={{ mb: 0.8 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.2 }}>
          <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.55rem', fontWeight: 600 }}>
            RULE ENGINE
          </Typography>
          <Typography variant="caption" sx={{ color: rule_score >= 0.5 ? '#6366f1' : '#ef4444', fontSize: '0.6rem', fontWeight: 600 }}>
            {(rule_score || 0) >= 0.5 ? '✅ PASS' : '❌ FAIL'}
          </Typography>
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <Box sx={{ flex: 1 }}>
            <LinearProgress variant="determinate" value={(rule_score || 0) * 100}
              sx={{ height: 4, borderRadius: 2, bgcolor: '#1e293b', '& .MuiLinearProgress-bar': { bgcolor: '#6366f1' } }} />
          </Box>
          <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.55rem', minWidth: 24, textAlign: 'right' }}>
            {((rule_score || 0) * 100).toFixed(0)}%
          </Typography>
        </Box>
      </Box>

      {/* LLM track */}
      {!llmDegraded && (
        <Box sx={{ mb: 0.8 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.2 }}>
            <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.55rem', fontWeight: 600 }}>
              LLM JUDGE
            </Typography>
            <Typography variant="caption" sx={{ color: (llm_score || 0) >= 0.6 ? '#22c55e' : '#eab308', fontSize: '0.6rem', fontWeight: 600 }}>
              {(llm_score || 0) >= 0.6 ? '👍' : '⚠️'}
            </Typography>
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <Box sx={{ flex: 1 }}>
              <LinearProgress variant="determinate" value={(llm_score || 0) * 100}
                sx={{ height: 4, borderRadius: 2, bgcolor: '#1e293b', '& .MuiLinearProgress-bar': { bgcolor: (llm_score || 0) >= 0.6 ? '#22c55e' : '#eab308' } }} />
            </Box>
            <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.55rem', minWidth: 24, textAlign: 'right' }}>
              {((llm_score || 0) * 100).toFixed(0)}%
            </Typography>
          </Box>
        </Box>
      )}

      {/* LLM degraded fallback note */}
      {llmDegraded && passed && (
        <Box sx={{ bgcolor: '#eab30811', borderRadius: 1, p: 0.5, mb: 0.8 }}>
          <Typography variant="caption" sx={{ color: '#eab308', fontSize: '0.55rem', display: 'block' }}>
            LLM API timed out or unavailable. Fallback to rule-only evaluation.
          </Typography>
        </Box>
      )}

      {/* Final score */}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.5 }}>
        <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.55rem', fontWeight: 600 }}>
          FINAL
        </Typography>
        <Box sx={{ flex: 1 }}>
          <LinearProgress variant="determinate" value={(score || 0) * 100}
            sx={{ height: 6, borderRadius: 3, bgcolor: '#1e293b', '& .MuiLinearProgress-bar': { bgcolor: finalColor } }} />
        </Box>
        <Typography variant="caption" sx={{ color: finalColor, fontSize: '0.65rem', fontWeight: 700, minWidth: 32, textAlign: 'right' }}>
          {((score || 0) * 100).toFixed(0)}%
        </Typography>
      </Box>

      {/* LLM reasoning */}
      {llm_reason && (
        <Box sx={{ bgcolor: '#0a0e1a', borderRadius: 1, p: 0.5, mb: 0.3 }}>
          <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.5rem', display: 'block', mb: 0.2 }}>
            LLM REASONING
          </Typography>
          <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.55rem', lineHeight: 1.3, display: 'block' }}>
            {llm_reason}
          </Typography>
        </Box>
      )}

      {/* Rule results (template-based) */}
      {ruleResults && ruleResults.length > 0 && (
        <Box sx={{ mb: 1 }}>
          <Typography variant="caption" fontWeight={700} sx={{ color: '#64748b', fontSize: '0.55rem', display: 'block', mb: 0.3 }}>
            📋 RULE TEMPLATES
          </Typography>
          {ruleResults.map((r, i) => (
            <Box key={i} sx={{
              display: 'flex', alignItems: 'center', gap: 0.5, py: 0.2,
              px: 0.5, borderRadius: 0.5, mb: 0.2,
              bgcolor: r.pass ? '#22c55e11' : '#ef444411',
            }}>
              <Typography variant="caption" sx={{ fontSize: '0.6rem' }}>
                {r.pass ? '✅' : '❌'}
              </Typography>
              <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.55rem', fontWeight: 600, minWidth: 80 }}>
                {r.name}
              </Typography>
              <Typography variant="caption" sx={{ color: r.pass ? '#22c55e' : '#ef4444', fontSize: '0.5rem', flex: 1 }}>
                {r.detail}
              </Typography>
            </Box>
          ))}
        </Box>
      )}

      {/* Summary */}
      {summary && (
        <Typography variant="caption" sx={{ color: '#475569', fontSize: '0.5rem', display: 'block' }}>
          {summary}
        </Typography>
      )}
    </Paper>
  )
}
