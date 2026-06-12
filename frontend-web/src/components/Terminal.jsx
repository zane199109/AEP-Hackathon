import React, { useRef, useEffect } from 'react'
import { Box, Typography, Tooltip, IconButton } from '@mui/material'
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined'
import { useApp } from '../context/AppContext'

const COLORS = {
  lock: '#00f3ff',       // Cyan — funds locked
  release: '#00ff66',    // Green — CAW release
  claim: '#ffdd00',      // Yellow — claimed
  submit: '#ffdd00',     // Yellow — submitted
  reputation: '#22c55e', // Green — reputation
  slash: '#ff003c',      // Red — slashed
  info: '#94a3b8',       // Default gray
}

export default function Terminal() {
  const { terminalEvents } = useApp()
  const scrollRef = useRef(null)

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [terminalEvents])

  return (
    <Box sx={{
      height: 200, bgcolor: '#000', borderTop: '1px solid #1e293b',
      display: 'flex', flexDirection: 'column', position: 'relative',
      fontFamily: '"JetBrains Mono", "Fira Code", monospace',
    }}>
      {/* Terminal header */}
      <Box sx={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        px: 1.5, py: 0.3, bgcolor: '#0a0e1a', borderBottom: '1px solid #1e293b',
      }}>
        <Typography variant="caption" sx={{ color: '#e2e8f0', fontWeight: 600, letterSpacing: 1 }}>
          {'>'} COBO_TERMINAL
        </Typography>
        <Tooltip title={
          <Typography variant="caption" sx={{ fontSize: '0.65rem', lineHeight: 1.5 }}>
            Funds are custodied by Cobo Wallet-as-a-Service.
            Lock/Release operations execute via the Cobo Agentic Wallet API.
            In demo mode, CAW Guard confirmation is simulated.
          </Typography>
        } arrow placement="left">
          <IconButton size="small" sx={{ color: '#475569', p: 0.2 }}>
            <InfoOutlinedIcon sx={{ fontSize: '0.9rem' }} />
          </IconButton>
        </Tooltip>
      </Box>

      {/* Scrollable log area */}
      <Box ref={scrollRef} sx={{
        flex: 1, overflow: 'auto', px: 1.5, py: 0.5,
        '&::-webkit-scrollbar': { width: 4 },
        '&::-webkit-scrollbar-track': { background: '#000' },
        '&::-webkit-scrollbar-thumb': { background: '#1e293b', borderRadius: 2 },
      }}>
        {terminalEvents.length === 0 ? (
          <Typography variant="caption" sx={{ color: '#334155' }}>
            {'>'} Waiting for events... Connect via SSE and interact with the dashboard.
          </Typography>
        ) : terminalEvents.map((e, i) => (
          <Box key={i} sx={{
            display: 'flex', gap: 1, py: 0.15,
            opacity: i === 0 ? 1 : Math.max(0.4, 1 - i * 0.04), // Fade older entries
            fontFamily: 'monospace', fontSize: '0.7rem',
          }}>
            <Typography variant="caption" sx={{ color: '#475569', flexShrink: 0, fontSize: '0.6rem' }}>
              {e.time}
            </Typography>
            <Typography variant="caption" sx={{
              color: COLORS[e.type] || COLORS.info,
              whiteSpace: 'pre-wrap', wordBreak: 'break-all',
              fontWeight: i === 0 ? 600 : 400,
              fontSize: '0.65rem',
            }}>
              {'>'} {e.text}
            </Typography>
          </Box>
        ))}
      </Box>
    </Box>
  )
}
