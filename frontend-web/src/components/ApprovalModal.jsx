import React from 'react'
import { Dialog, DialogTitle, DialogContent, DialogActions, Button, Typography, Box, Chip, LinearProgress, Divider } from '@mui/material'
import { useApp } from '../context/AppContext'

export default function ApprovalModal() {
  const { showApproval, setShowApproval, evaluationResult, activeBounty, confirmPayment, rejectBounty, loading } = useApp()

  const handleAccept = async () => {
    await confirmPayment()
    setShowApproval(false)
  }

  const handleReject = async () => {
    await rejectBounty()
    setShowApproval(false)
  }

  const handleClose = () => {
    setShowApproval(false)
  }

  if (!showApproval || !evaluationResult) return null

  const { passed, status, score, rule_score, llm_score, llm_reason, summary } = evaluationResult
  const isPendingReview = status === 'pending_review'
  const rewardEth = activeBounty?.reward || '0.001'
  const repDelta = Math.max(1, Math.floor(parseFloat(rewardEth) * 10))
  const finalColor = passed ? '#22c55e' : '#eab308'
  const isSlashed = status === 'slashed'

  return (
    <Dialog open={showApproval} onClose={handleClose} maxWidth="sm" fullWidth
      PaperProps={{
        sx: {
          bgcolor: '#111827', border: `1px solid ${finalColor}44`, borderRadius: 2,
          backgroundImage: 'none', color: '#e2e8f0',
        }
      }}
    >
      <DialogTitle sx={{
        display: 'flex', alignItems: 'center', gap: 1, pb: 1, borderBottom: '1px solid #1e293b',
      }}>
        <Box sx={{
          width: 32, height: 32, borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center',
          bgcolor: isPendingReview ? '#eab30822' : '#22c55e22',
          border: `2px solid ${isPendingReview ? '#eab308' : '#22c55e'}`,
        }}>
          <Typography variant="body2" sx={{ color: isPendingReview ? '#eab308' : '#22c55e' }}>
            {isPendingReview ? '⚠️' : '✅'}
          </Typography>
        </Box>
        <Box>
          <Typography variant="body2" fontWeight={700}>
            {isPendingReview ? 'Manual Review Required' : 'Buyer Confirmation'}
          </Typography>
          <Typography variant="caption" sx={{ color: '#64748b' }}>
            {isPendingReview
              ? 'AI is uncertain — please review and decide'
              : 'Delivery passed evaluation. Confirm to release funds.'}
          </Typography>
        </Box>
      </DialogTitle>

      <DialogContent sx={{ pt: 2, pb: 1 }}>
        {/* Evaluation summary */}
        <Box sx={{ bgcolor: '#0a0e1a', borderRadius: 1, p: 1.5, mb: 1.5, border: '1px solid #1e293b' }}>
          <Typography variant="caption" fontWeight={700} sx={{ color: '#64748b', display: 'block', mb: 1 }}>
            🤖 EVALUATION RESULT — {status.toUpperCase()}
          </Typography>

          <Box sx={{ display: 'flex', gap: 1.5, mb: 1 }}>
            <Box sx={{ flex: 1, textAlign: 'center' }}>
              <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.85rem' }}>RULE</Typography>
              <Box sx={{ mt: 0.3 }}>
                <LinearProgress variant="determinate" value={(rule_score || 0) * 100}
                  sx={{ height: 6, borderRadius: 3, bgcolor: '#1e293b', '& .MuiLinearProgress-bar': { bgcolor: '#6366f1' } }} />
                <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.85rem' }}>
                  {((rule_score || 0) * 100).toFixed(0)}%
                </Typography>
              </Box>
            </Box>
            {llm_score !== undefined && llm_score !== null && (
              <Box sx={{ flex: 1, textAlign: 'center' }}>
                <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.85rem' }}>LLM</Typography>
                <Box sx={{ mt: 0.3 }}>
                  <LinearProgress variant="determinate" value={(llm_score || 0) * 100}
                    sx={{ height: 6, borderRadius: 3, bgcolor: '#1e293b', '& .MuiLinearProgress-bar': { bgcolor: llm_score >= 0.6 ? '#22c55e' : '#eab308' } }} />
                  <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.85rem' }}>
                    {((llm_score || 0) * 100).toFixed(0)}%
                  </Typography>
                </Box>
              </Box>
            )}
            <Box sx={{ flex: 1, textAlign: 'center' }}>
              <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.85rem' }}>FINAL</Typography>
              <Box sx={{ mt: 0.3 }}>
                <LinearProgress variant="determinate" value={(score || 0) * 100}
                  sx={{ height: 8, borderRadius: 3, bgcolor: '#1e293b', '& .MuiLinearProgress-bar': { bgcolor: finalColor } }} />
                <Typography variant="body2" fontWeight={700} sx={{ color: finalColor }}>
                  {((score || 0) * 100).toFixed(0)}%
                </Typography>
              </Box>
            </Box>
          </Box>

          {llm_reason && (
            <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.75rem', display: 'block', mb: 0.5 }}>
              <span style={{ color: '#64748b' }}>LLM: </span>{llm_reason}
            </Typography>
          )}
          {summary && (
            <Typography variant="caption" sx={{ color: '#475569', fontSize: '0.85rem', display: 'block' }}>
              {summary}
            </Typography>
          )}
        </Box>

        {/* Decision consequences */}
        <Box sx={{ bgcolor: '#0a0e1a', borderRadius: 1, p: 1.5, border: '1px solid #1e293b' }}>
          <Typography variant="caption" fontWeight={700} sx={{ color: '#64748b', display: 'block', mb: 1 }}>
            💰 DECISION CONSEQUENCES
          </Typography>

          <Box sx={{ display: 'flex', gap: 1 }}>
            {/* Accept column */}
            <Box sx={{
              flex: 1, p: 1, borderRadius: 1, border: '1px solid #22c55e44',
              bgcolor: '#22c55e11',
            }}>
              <Typography variant="caption" fontWeight={700} sx={{ color: '#22c55e', display: 'block', mb: 0.5 }}>
                ✅ ACCEPT
              </Typography>
              <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.85rem', display: 'block' }}>
                Release {rewardEth} ETH
              </Typography>
              <Typography variant="caption" sx={{ color: '#22c55e', fontSize: '0.75rem', fontWeight: 600, display: 'block' }}>
                Reputation +{repDelta}
              </Typography>
              <Typography variant="caption" sx={{ color: '#475569', fontSize: '0.8rem', display: 'block', mt: 0.3 }}>
                CAW Release → settled
              </Typography>
            </Box>

            {/* Reject column */}
            <Box sx={{
              flex: 1, p: 1, borderRadius: 1, border: '1px solid #ef444444',
              bgcolor: '#ef444411',
            }}>
              <Typography variant="caption" fontWeight={700} sx={{ color: '#ef4444', display: 'block', mb: 0.5 }}>
                ❌ REJECT
              </Typography>
              <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.85rem', display: 'block' }}>
                Funds returned to buyer
              </Typography>
              <Typography variant="caption" sx={{ color: '#ef4444', fontSize: '0.75rem', fontWeight: 600, display: 'block' }}>
                Reputation -20
              </Typography>
              <Typography variant="caption" sx={{ color: '#475569', fontSize: '0.8rem', display: 'block', mt: 0.3 }}>
                ⛓️ On-chain slash event
              </Typography>
            </Box>
          </Box>
        </Box>
      </DialogContent>

      <DialogActions sx={{ px: 2, pb: 2, borderTop: '1px solid #1e293b', pt: 1.5, gap: 1 }}>
        <Button onClick={handleClose} size="small" sx={{ color: '#64748b', fontSize: '0.75rem', minWidth: 80 }}>
          Later
        </Button>
        <Button onClick={handleReject} variant="outlined" size="small" disabled={loading}
          sx={{
            borderColor: '#ef4444', color: '#ef4444', fontWeight: 600, fontSize: '0.75rem', minWidth: 100,
            '&:hover': { borderColor: '#dc2626', bgcolor: '#ef444411' },
          }}
        >
          {loading ? '...' : isSlashed ? '⛓️ Confirm Arbitration' : '❌ Reject'}
        </Button>
        {!isSlashed && (
          <Button onClick={handleAccept} variant="contained" size="small" disabled={loading}
            sx={{
              bgcolor: '#22c55e', color: '#000', fontWeight: 700, fontSize: '0.75rem', minWidth: 120,
              '&:hover': { bgcolor: '#16a34a' },
              animation: 'pulseBtn 1.5s ease infinite',
            }}
          >
            {loading ? 'Processing...' : '✅ Accept'}
          </Button>
        )}
      </DialogActions>
    </Dialog>
  )
}
