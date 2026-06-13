import React, { useState, useEffect } from 'react'
import { ThemeProvider, createTheme } from '@mui/material/styles'
import CssBaseline from '@mui/material/CssBaseline'
import { Box, Snackbar, Alert, Typography } from '@mui/material'

import { AppProvider, useApp } from './context/AppContext'
import TopBar from './components/TopBar'
import SideNav from './components/SideNav'
import TopologyView from './components/TopologyView'
import CustodyPanel from './components/CustodyPanel'
import PactEditor from './components/PactEditor'
import Sidebar from './components/Sidebar'
import Terminal from './components/Terminal'
import ApprovalModal from './components/ApprovalModal'
import PactEditorDrawer from './components/PactEditorDrawer'
import AgentPipeline from './components/AgentPipeline'
import useSSE from './hooks/useSSE'
import useStore from './store/useStore'

const theme = createTheme({
  palette: {
    mode: 'dark',
    primary: { main: '#00f3ff' },
    secondary: { main: '#22c55e' },
    background: { default: '#0a0e1a', paper: '#111827' },
    divider: '#1e293b',
  },
  typography: {
    fontFamily: '"Inter", -apple-system, BlinkMacSystemFont, sans-serif',
    h6: { fontWeight: 600, fontSize: '0.95rem' },
    body2: { fontSize: '0.8rem' },
    caption: { fontFamily: '"JetBrains Mono", monospace', fontSize: '0.7rem' },
  },
  shape: { borderRadius: 10 },
  components: {
    MuiCard: { styleOverrides: { root: { border: '1px solid #1e293b', backgroundImage: 'none' } } },
    MuiButton: { styleOverrides: { root: { textTransform: 'none', fontWeight: 600 } } },
  },
})

function Dashboard() {
  const { sseConnected, terminalEvents, phase, evaluationResult, settled, addTerminal } = useApp()
  const viewMode = useStore(s => s.viewMode)
  const isDrawerOpen = useStore(s => s.isDrawerOpen)
  const [snack, setSnack] = useState({ open: false, msg: '', severity: 'success' })

  // Sync AppContext state with Zustand store
  useEffect(() => { useStore.setState({ sseConnected, settled }) }, [sseConnected, settled])

  // Initialize SSE
  useSSE({ addLog: addTerminal })

  // Fetch on-chain reputation on mount
  useEffect(() => {
    useStore.getState().fetchReputation()
  }, [])

  return (
    <Box sx={{ height: '100vh', display: 'flex', flexDirection: 'column', bgcolor: '#0a0e1a' }}>
      {/* Top Bar */}
      <TopBar />

      {/* Disclaimer */}
      <Box sx={{ bgcolor: '#1e293b', textAlign: 'center', py: 0.3 }}>
        <Typography variant="caption" sx={{ color: '#fbbf24', fontSize: '0.55rem' }}>
          ⚠️ Technical protocol demonstration using testnet assets only. No financial services.
        </Typography>
      </Box>

      {/* Main: SideNav + Content + Custody */}
      <Box sx={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
        {/* SideNav */}
        <SideNav />

        {/* Main Content */}
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', position: 'relative' }}>
          {/* View switch buttons */}
          <Box sx={{ display: 'flex', gap: 0.5, px: 1, pt: 0.5, pb: 0.5 }}>
            <Box
              onClick={() => useStore.setState({ viewMode: 'topology' })}
              sx={{ px: 1, py: 0.3, borderRadius: 1, cursor: 'pointer', fontSize: '0.65rem', fontWeight: 600,
                bgcolor: viewMode === 'topology' ? '#00f3ff22' : 'transparent',
                color: viewMode === 'topology' ? '#00f3ff' : '#475569',
                border: viewMode === 'topology' ? '1px solid #00f3ff44' : '1px solid transparent',
              }}
            >🔮 架构图</Box>
            <Box
              onClick={() => useStore.setState({ viewMode: 'dashboard' })}
              sx={{ px: 1, py: 0.3, borderRadius: 1, cursor: 'pointer', fontSize: '0.65rem', fontWeight: 600,
                bgcolor: viewMode === 'dashboard' ? '#6366f122' : 'transparent',
                color: viewMode === 'dashboard' ? '#6366f1' : '#475569',
                border: viewMode === 'dashboard' ? '1px solid #6366f144' : '1px solid transparent',
              }}
            >📋 控制台</Box>
          </Box>

          {/* View content */}
          {viewMode === 'topology' ? (
            <Box sx={{ flex: 1, display: 'flex', position: 'relative' }}>
              <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
                <TopologyView />
              </Box>
              {/* Right sidebar for topology mode */}
              <Sidebar />
            </Box>
          ) : (
            <Box sx={{ flex: 1, p: 2, color: '#64748b', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Typography variant="body2" sx={{ color: '#e2e8f0' }}>控制台视图开发中...</Typography>
            </Box>
          )}
        </Box>

        {/* Custody Panel */}
        <CustodyPanel />
      </Box>

      {/* Terminal */}
      <Terminal />

      {/* PactEditor Drawer */}
      <PactEditorDrawer>
        <PactEditor />
      </PactEditorDrawer>

      {/* Approval Modal */}
      <ApprovalModal />

      {/* Styles */}
      <style>{`
        @keyframes pulseNode { 0% { transform: scale(1); } 50% { transform: scale(1.1); } 100% { transform: scale(1); } }
        @keyframes pulseBtn { 0% { box-shadow: 0 0 0 0 rgba(34,197,94,0.4); } 70% { box-shadow: 0 0 0 8px rgba(34,197,94,0); } 100% { box-shadow: 0 0 0 0 rgba(34,197,94,0); } }
        ::-webkit-scrollbar { width: 4px; }
        ::-webkit-scrollbar-track { background: #0f172a; }
        ::-webkit-scrollbar-thumb { background: #334155; border-radius: 4px; }
      `}</style>

      <Snackbar open={snack.open} autoHideDuration={4000} onClose={() => setSnack({ ...snack, open: false })}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}>
        <Alert severity={snack.severity} variant="filled" sx={{ width: '100%' }}>
          {snack.msg}
        </Alert>
      </Snackbar>
    </Box>
  )
}

export default function App() {
  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <AppProvider>
        <Dashboard />
      </AppProvider>
    </ThemeProvider>
  )
}
