import { Box, Typography, Chip } from '@mui/material'
import useStore from '../store/useStore'
import { useState, useEffect, useRef } from 'react'

export default function CustodyPanel() {
  const pendingApprovals = useStore(s => s.pendingApprovals)
  const settled = useStore(s => s.settled)
  const phase = useStore(s => s.phase)
  const repTxHashes = useStore(s => s.repTxHashes)
  const [pactStatuses, setPactStatuses] = useState({})
  const [history, setHistory] = useState([])
  const [splitPos, setSplitPos] = useState(55)
  const panelRef = useRef(null)

  // Drag handle
  const handleDragStart = (e) => {
    e.preventDefault()
    const startY = e.clientY
    const startH = splitPos
    const onMove = (me) => {
      if (!panelRef.current) return
      const rect = panelRef.current.getBoundingClientRect()
      const pct = ((me.clientY - rect.top) / rect.height) * 100
      setSplitPos(Math.max(25, Math.min(75, pct)))
    }
    const onUp = () => {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
  }

  // Query real CAW pact status for each pending approval every 3s
  useEffect(() => {
    if (pendingApprovals.length === 0) return
    const poll = async () => {
      const updates = {}
      for (const item of pendingApprovals) {
        try {
          const res = await fetch('/api/pact/' + item.pactId)
          const data = await res.json()
          if (data && data.status) {
            updates[item.pactId] = data.status
          }
        } catch (e) {}
      }
      if (Object.keys(updates).length > 0) {
        setPactStatuses(prev => ({ ...prev, ...updates }))
        for (const item of pendingApprovals) {
          if (updates[item.pactId] === 'active') {
            setHistory(prev => {
              if (prev.some(h => h.pactId === item.pactId)) return prev
              return [{ ...item, approvedAt: new Date().toLocaleTimeString() }, ...prev]
            })
          }
        }
      }
    }
    poll()
    const interval = setInterval(poll, 3000)
    return () => clearInterval(interval)
  }, [pendingApprovals])

  const formatAmount = (wei) => {
    const eth = parseInt(wei || '0') / 1e18
    return eth > 0 ? eth.toFixed(4) : '0'
  }

  return (
    <Box ref={panelRef} sx={{
      width: 280, bgcolor: '#111122', borderLeft: '1px solid #2a2a3a',
      display: 'flex', flexDirection: 'column', overflow: 'hidden', height: '100%',
    }}>
      {/* Header */}
      <Box sx={{ p: 1.5, borderBottom: '1px solid #2a2a3a', flexShrink: 0 }}>
        <Typography variant="caption" fontWeight={700} sx={{ color: '#eab308', letterSpacing: 1 }}>
          🔐 CAW Pact 队列
        </Typography>
      </Box>

      {/* Pending list - scrollable */}
      <Box sx={{
        flex: `0 0 ${splitPos}%`, overflow: 'auto', p: 1,
        display: 'flex', flexDirection: 'column', gap: 1,
        '&::-webkit-scrollbar': { width: 4 },
        '&::-webkit-scrollbar-track': { background: '#0a0e1a' },
        '&::-webkit-scrollbar-thumb': { background: '#2a2a3a', borderRadius: 2 },
      }}>
        {pendingApprovals.length === 0 && !settled && phase !== 'pending_approval' && (
          <Typography variant="caption" sx={{ color: '#475569', textAlign: 'center', mt: 2 }}>
            暂无待审批项
          </Typography>
        )}

        {pendingApprovals.map((item, i) => {
          const realStatus = pactStatuses[item.pactId]
          const isActive = realStatus === 'active'
          const isPending = !realStatus || realStatus === 'pending_approval'

          return (
            <Box key={item.pactId || i} sx={{
              p: 1, borderRadius: 1, bgcolor: '#0a0a1a', flexShrink: 0,
              border: isActive ? '1px solid #22c55e44' : '1px solid #eab30844',
              opacity: isActive ? 0.5 : 1,
              transition: 'all 0.3s',
            }}>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="caption" fontWeight={600} sx={{ color: '#e2e8f0', fontSize: '0.65rem' }}>
                  Pact #{item.jobId}
                </Typography>
                <Chip label={isActive ? '✅ 已审批' : '⏳ 待审批'} size="small"
                  sx={{ height: 16, fontSize: '0.5rem',
                    bgcolor: isActive ? '#22c55e22' : '#eab30822',
                    color: isActive ? '#22c55e' : '#eab308'
                  }} />
              </Box>
              <Typography variant="caption" sx={{ color: '#22c55e', fontSize: '0.6rem', display: 'block' }}>
                {item.type === 'lock' ? '🔒 Lock' : '💸 Release'} {formatAmount(item.amount)} ETH
                {item.type === 'release' && (
                  <Typography variant="caption" sx={{ color: '#eab308', fontSize: '0.5rem', display: 'block' }}>
                    放款待审批
                  </Typography>
                )}
              </Typography>
              <Typography variant="caption" sx={{ color: '#475569', fontSize: '0.5rem', display: 'block', mt: 0.2 }}>
                CAW: {item.pactId?.slice(0, 8)}... | {realStatus || 'pending_approval'}
              </Typography>
              {isPending && (
                <Typography variant="caption" sx={{ color: '#eab308', fontSize: '0.55rem', display: 'block', mt: 0.3 }}>
                  📱 请在 Cobo 钱包中审批
                </Typography>
              )}
              {isActive && (
                <Typography variant="caption" sx={{ color: '#22c55e', fontSize: '0.55rem', display: 'block', mt: 0.3 }}>
                  ✅ 已在 Cobo 中确认
                </Typography>
              )}
            </Box>
          )
        })}
      </Box>

      {/* Drag handle */}
      <Box
        onMouseDown={handleDragStart}
        sx={{
          height: 6, cursor: 'row-resize', flexShrink: 0,
          bgcolor: '#1e293b', display: 'flex', alignItems: 'center', justifyContent: 'center',
          '&:hover': { bgcolor: '#2a2a3a' },
          '&::after': { content: '""', width: 20, height: 2, borderRadius: 1, bgcolor: '#475569' },
        }}
      />

      {/* On-chain Records — replaced "历史审批" */}
      <Box sx={{
        flex: `1 1 ${100 - splitPos}%`, overflow: 'auto', p: 1.5,
        borderTop: 'none',
        '&::-webkit-scrollbar': { width: 4 },
        '&::-webkit-scrollbar-track': { background: '#0a0e1a' },
        '&::-webkit-scrollbar-thumb': { background: '#2a2a3a', borderRadius: 2 },
      }}>
        <Typography variant="caption" fontWeight={700} sx={{ color: '#eab308', letterSpacing: 1, mb: 1, display: 'block' }}>
          ⛓️ 链上记录
        </Typography>
        {repTxHashes.length === 0 ? (
          <Typography variant="caption" sx={{ color: '#334155', fontSize: '0.6rem', textAlign: 'center', display: 'block', mt: 2 }}>
            暂无链上交易记录
          </Typography>
        ) : repTxHashes.map((entry, i) => (
          <Box key={i} sx={{
            p: 1, borderRadius: 1, bgcolor: '#0a0a1a', mb: 0.5,
            border: '1px solid #1e293b', flexShrink: 0,
          }}>
              {entry.type === 'transfer' ? (
                <>
                  <Typography variant="caption" fontWeight={600} sx={{ color: '#e2e8f0', fontSize: '0.65rem', display: 'block' }}>
                    💸 {entry.from === 'buyer' ? 'Buyer → Provider' : 'Provider → SubProvider'}
                  </Typography>
                  <Typography variant="caption" sx={{ color: '#22c55e', fontSize: '0.6rem', display: 'block' }}>
                    {entry.amount} ETH
                  </Typography>
                  <Typography variant="caption" sx={{ color: '#475569', fontSize: '0.5rem', display: 'block', mt: 0.2 }}>
                    <a href={`https://sepolia.etherscan.io/tx/${entry.txHash}`} target="_blank" rel="noopener noreferrer"
                      style={{ color: '#6366f1' }}>
                      {entry.txHash?.slice(0, 20)}...
                    </a>
                  </Typography>
                </>
              ) : (
                <>
                  <Typography variant="caption" fontWeight={600} sx={{ color: '#e2e8f0', fontSize: '0.65rem', display: 'block' }}>
                    🏅 {entry.agent?.slice(0, 10)}... {entry.delta > 0 ? '+' : ''}{entry.delta}分
                  </Typography>
                  <Typography variant="caption" sx={{ color: '#94a3b8', fontSize: '0.6rem', display: 'block' }}>
                    {entry.oldScore} → {entry.newScore}分
                  </Typography>
                  <Typography variant="caption" sx={{ color: '#475569', fontSize: '0.5rem', display: 'block', mt: 0.2 }}>
                    <a href={`https://sepolia.etherscan.io/tx/${entry.txHash}`} target="_blank" rel="noopener noreferrer"
                      style={{ color: '#6366f1' }}>
                      {entry.txHash?.slice(0, 20)}...
                    </a>
                  </Typography>
                </>
              )}
          </Box>
        ))}
      </Box>
    </Box>
  )
}
