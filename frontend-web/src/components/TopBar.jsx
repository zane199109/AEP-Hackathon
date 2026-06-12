import { Box, Typography, Chip, Button } from '@mui/material'
import AccountBalanceWalletIcon from '@mui/icons-material/AccountBalanceWallet'
import useStore from '../store/useStore'

const API = (path, options = {}) => {
  const opts = { headers: { 'Content-Type': 'application/json' }, ...options }
  return fetch(`/api${path}`, opts).then(r => r.json().catch(() => ({ status: r.status })))
}

export default function TopBar() {
  const phase = useStore(s => s.phase)
  const sseConnected = useStore(s => s.sseConnected)
  const evaluationResult = useStore(s => s.evaluationResult)
  const lastSlashedJob = useStore(s => s.lastSlashedJob)
  const resetDemo = useStore(s => s.resetDemo)
  const addLog = useStore(s => s.addLog)
  const setPhase = useStore(s => s.setPhase)

  const phaseColor = phase === 'pending_approval' ? '#eab308' : phase === 'settled' || phase === 'slashed' ? '#22c55e' : phase === 'disputed' ? '#ef4444' : '#6366f1'
  const phaseBg = phase === 'pending_approval' ? '#eab30822' : phase === 'settled' || phase === 'slashed' ? '#22c55e22' : phase === 'disputed' ? '#ef444422' : '#6366f122'
  const phaseBorder = phase === 'pending_approval' ? '#eab30844' : phase === 'settled' || phase === 'slashed' ? '#22c55e44' : phase === 'disputed' ? '#ef444444' : '#6366f144'

  const isDone = phase === 'settled' || phase === 'slashed'

  const handleNewRound = () => {
    resetDemo()
    addLog('🔄 新一轮测试已就绪，创建新的 Pact 开始', 'info')
  }

  const handleArbitrate = async () => {
    if (!lastSlashedJob) {
      addLog('❌ 仲裁失败: 找不到争议悬赏ID', 'slash')
      return
    }
    addLog(`⚖️ 仲裁执行中 — Job #${lastSlashedJob}...`, 'info')
    const data = await API(`/arbitrate/${lastSlashedJob}`, { method: 'POST' }).catch(() => null)
    if (data && !data.error) {
      setPhase('slashed')
      addLog(`❌ 仲裁完成，Provider 声誉已扣除`, 'slash')
    } else {
      addLog(`❌ 仲裁失败: ${data?.error || '未知错误'}`, 'slash')
    }
  }

  return (
    <Box sx={{
      height: 56, display: 'flex', alignItems: 'center', px: 2, gap: 2,
      bgcolor: '#0f0f1a', borderBottom: '1px solid #00f3ff',
    }}>
      <AccountBalanceWalletIcon sx={{ color: '#00f3ff', fontSize: '1.3rem' }} />
      <Typography variant="body1" fontWeight={700} sx={{ color: '#e2e8f0', letterSpacing: 1 }}>
        🏛️ AEP Console
      </Typography>
      <Chip size="small" label="Base Sepolia" sx={{ height: 20, fontSize: '0.55rem', bgcolor: '#22c55e22', color: '#22c55e', fontWeight: 600 }} />
      <Box sx={{ flex: 1 }} />
      {phase === 'disputed' && (
        <Button size="small" variant="outlined"
          onClick={handleArbitrate}
          sx={{ fontSize: '0.6rem', borderColor: '#ef4444', color: '#ef4444', py: 0.1, '&:hover': { borderColor: '#ef4444', bgcolor: '#ef444411' } }}
        >
          ⚖️ 执行仲裁
        </Button>
      )}
      {isDone && (
        <Button size="small" variant="outlined"
          onClick={handleNewRound}
          sx={{ fontSize: '0.6rem', borderColor: '#6366f144', color: '#6366f1', py: 0.1, '&:hover': { borderColor: '#6366f1', bgcolor: '#6366f111' } }}
        >
          🔄 新一轮测试
        </Button>
      )}
      {phase !== 'idle' && (
        <Chip size="small" label={`📌 ${phase.toUpperCase()}`}
          sx={{ fontWeight: 600, fontSize: '0.6rem', height: 22, bgcolor: phaseBg, color: phaseColor, border: '1px solid', borderColor: phaseBorder }} />
      )}
      <Chip size="small" label={sseConnected ? '🟢 Live' : '🔴 Off'} color={sseConnected ? 'success' : 'error'} variant="outlined" />
      <Typography variant="caption" sx={{ color: '#64748b', fontSize: '0.6rem' }}>0x3f8e...a1b2</Typography>
    </Box>
  )
}
