import React, { useState } from 'react'
import { Box, Card, CardContent, Typography, Button, TextField, Collapse, IconButton, Chip, Snackbar, Alert, FormControlLabel, Switch } from '@mui/material'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import ExpandLessIcon from '@mui/icons-material/ExpandLess'
import AutoAwesomeIcon from '@mui/icons-material/AutoAwesome'
import RocketLaunchIcon from '@mui/icons-material/RocketLaunch'
import { useApp } from '../context/AppContext'
import useStore from '../store/useStore'

const ETH_DECIMALS = 18

export default function PactEditor() {
  const { createBounty, loading, phase } = useApp()
  const pactForm = useStore(s => s.pactForm)
  const setPactForm = useStore(s => s.setPactForm)
  const addApproval = useStore(s => s.addApproval)
  const setDrawerOpen = useStore(s => s.setDrawerOpen)
  const bountiesStore = useStore(s => s.bounties)

  const [open, setOpen] = useState(true)
  const [nlInput, setNlInput] = useState(pactForm.nlInput || '')
  const [form, setForm] = useState(pactForm)
  const [snack, setSnack] = useState({ open: false, msg: '', severity: 'success' })
  const [demoSlash, setDemoSlash] = useState(false)

  const updateForm = (updates) => {
    const next = { ...form, ...updates }
    setForm(next)
    setPactForm({ ...pactForm, ...updates })
  }

  // Sync nlInput to store
  const handleNlChange = (e) => {
    const val = e.target.value
    setNlInput(val)
    updateForm({ nlInput: val })
  }

  const handleFieldChange = (field) => (e) => {
    const val = field === 'minReputation' ? parseInt(e.target.value) || 0 : e.target.value
    updateForm({ [field]: val })
  }

  const handleParse = async () => {
    const text = nlInput.trim()
    if (!text) return
    try {
      const res = await fetch('/api/parse-intent', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text })
      })
      const data = await res.json()
      const suggestions = {}
      if (data.suggested_amount_eth) suggestions.reward = data.suggested_amount_eth.toString()
      if (data.suggested_deadline_days) {
        const d = new Date()
        d.setDate(d.getDate() + data.suggested_deadline_days)
        // Use local time string, not UTC ISO string (which is 8 hours behind in UTC+8)
        const pad = (n) => String(n).padStart(2, '0')
        suggestions.deadline = `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
      }
      if (data.suggested_min_reputation) suggestions.minReputation = data.suggested_min_reputation
      updateForm({
        title: data.title || text,
        intent: data.intent || text,
        nlInput: text,
        ...suggestions,
      })
      setNlInput('')
      setSnack({ open: true, msg: `🧠 Intent parsed (${data.source === 'llm' ? 'LLM' : 'keyword'})`, severity: 'success' })
    } catch (e) {
      updateForm({ title: text, intent: text, nlInput: text })
      setNlInput('')
      setSnack({ open: true, msg: '✅ Intent parsed (fallback)', severity: 'success' })
    }
  }

  const handlePublish = async () => {
    const amountWei = (parseFloat(form.reward) * 10 ** ETH_DECIMALS).toFixed(0)
    // Format deadline: keep local time (no Z suffix)
    let deadlineVal = form.deadline
    if (!deadlineVal.includes('Z') && deadlineVal.length <= 16) {
      deadlineVal += ':00'
    }
    const result = await createBounty({
      title: form.title,
      amount: amountWei,
      deadline: deadlineVal,
      minReputation: form.minReputation,
      intent: form.intent || form.title,
      demoSlash,
    })
    if (result && !result.error) {
      setSnack({ open: true, msg: `✅ Bounty #${result.job_id} published!`, severity: 'success' })
      // Clear form on success
      const cleared = { title: '', reward: '0.001', deadline: '2026-06-10T00:00:00', minReputation: 60, intent: '', nlInput: '' }
      updateForm(cleared)
      setNlInput('')
      // Add to CAW pending approvals
      if (result.pact_id) {
        addApproval({ pactId: result.pact_id, jobId: result.job_id, amount: amountWei, type: 'lock', status: 'pending', timestamp: Date.now() })
      }
      // Close drawer
      setDrawerOpen(false)
    } else {
      setSnack({ open: true, msg: `❌ Failed: ${result?.error || 'unknown error'}`, severity: 'error' })
    }
  }

  return (
    <>
      <Card sx={{ mb: 1, border: '1px solid #1e293b', bgcolor: '#0f172a', transition: 'all 0.2s' }}>
        <CardContent sx={{ p: 1.5, '&:last-child': { pb: 1.5 } }}>
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <RocketLaunchIcon sx={{ color: '#6366f1', fontSize: '1.1rem' }} />
              <Typography variant="body2" fontWeight={700} sx={{ color: '#e2e8f0', letterSpacing: 1 }}>
                PACT EDITOR
              </Typography>
              {bountiesStore.length > 0 && (
                <Chip label={`${bountiesStore.length} open`} size="small"
                  sx={{ height: 18, fontSize: '0.55rem', bgcolor: '#6366f133', color: '#6366f1', fontWeight: 600 }} />
              )}
            </Box>
            <IconButton size="small" onClick={() => setOpen(!open)} sx={{ color: '#64748b' }}>
              {open ? <ExpandLessIcon /> : <ExpandMoreIcon />}
            </IconButton>
          </Box>

          <Collapse in={open}>
            {/* NL Input */}
            <TextField
              fullWidth size="small" multiline minRows={2} maxRows={4}
              placeholder="Describe your bounty... e.g. 'Analyze wallet 0xabc for suspicious transactions'"
              value={nlInput}
              onChange={handleNlChange}
              sx={{ mb: 1, '& .MuiOutlinedInput-root': { bgcolor: '#0a0e1a', fontSize: '0.8rem' } }}
            />

            <Button
              size="small" variant="outlined" fullWidth
              startIcon={<AutoAwesomeIcon />}
              onClick={handleParse}
              disabled={!nlInput.trim() || loading}
              sx={{ mb: 1.5, borderColor: '#6366f1', color: '#6366f1', '&:hover': { borderColor: '#818cf8', bgcolor: '#6366f111' } }}
            >
              AI Parse (auto-fills title)
            </Button>

            {/* Form fields — always visible */}
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1, mb: 1.5 }}>
              <TextField size="small" label="Title" value={form.title}
                onChange={handleFieldChange('title')}
                sx={{ '& .MuiOutlinedInput-root': { bgcolor: '#0a0e1a', fontSize: '0.8rem' } }} />
              <Box sx={{ display: 'flex', gap: 1 }}>
                <TextField size="small" label="Reward (ETH)" type="number" value={form.reward}
                  inputProps={{ step: '0.001', min: '0' }}
                  onChange={handleFieldChange('reward')}
                  sx={{ flex: 1, '& .MuiOutlinedInput-root': { bgcolor: '#0a0e1a', fontSize: '0.8rem' } }} />
                <TextField size="small" label="Min Reputation" type="number" value={form.minReputation}
                  inputProps={{ min: 0, max: 100 }}
                  onChange={handleFieldChange('minReputation')}
                  sx={{ flex: 1, '& .MuiOutlinedInput-root': { bgcolor: '#0a0e1a', fontSize: '0.8rem' } }} />
              </Box>
              <TextField size="small" label="Deadline" type="datetime-local" value={form.deadline}
                onChange={handleFieldChange('deadline')}
                sx={{ '& .MuiOutlinedInput-root': { bgcolor: '#0a0e1a', fontSize: '0.8rem' } }} />
            </Box>

            <FormControlLabel
              control={<Switch size="small" checked={demoSlash} onChange={(e) => setDemoSlash(e.target.checked)}
                sx={{ '& .MuiSwitch-thumb': { bgcolor: demoSlash ? '#ef4444' : '#64748b' } }} />}
              label={<Typography variant="caption" sx={{ color: demoSlash ? '#ef4444' : '#64748b', fontSize: '0.65rem' }}>
                🧪 演示仲裁不通过
              </Typography>}
              sx={{ mb: 1 }}
            />

            <Button
              fullWidth variant="contained" size="small"
              disabled={loading || !form.title}
              onClick={handlePublish}
              sx={{ bgcolor: '#6366f1', '&:hover': { bgcolor: '#4f46e5' }, textTransform: 'none' }}
            >
              {loading ? 'Publishing...' : '▶ Publish Bounty'}
            </Button>

            {/* Pact approval status */}
            {phase === 'pending_approval' && (
              <Box sx={{ mt: 1.5, p: 1, bgcolor: '#0a0e1a', borderRadius: 1, border: '1px solid #eab30844' }}>
                <Typography variant="caption" sx={{ color: '#eab308', display: 'flex', alignItems: 'center', gap: 0.5, fontSize: '0.6rem' }}>
                  ⏳ Waiting for CAW App approval...
                </Typography>
                <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.5rem', display: 'block', mt: 0.3 }}>
                  Open CAW App → Pact Management → Approve
                </Typography>
              </Box>
            )}

            {/* Wallet info */}
            <Typography variant="caption" sx={{ display: 'block', mt: 1, color: '#475569', fontSize: '0.5rem', textAlign: 'center' }}>
              Buyer: 0xa115...53fe
            </Typography>

            {/* Guidance after publish */}
            {bountiesStore.length > 0 && (
              <Typography variant="caption" sx={{ display: 'block', textAlign: 'center', mt: 1, color: '#22c55e', fontSize: '0.6rem' }}>
                👉 Bounty created! Go to the right sidebar and click <b>Claim</b>
              </Typography>
            )}
          </Collapse>
        </CardContent>
      </Card>

      <Snackbar open={snack.open} autoHideDuration={4000} onClose={() => setSnack({ ...snack, open: false })}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}>
        <Alert severity={snack.severity} variant="filled" sx={{ width: '100%' }} onClose={() => setSnack({ ...snack, open: false })}>
          {snack.msg}
        </Alert>
      </Snackbar>
    </>
  )
}
