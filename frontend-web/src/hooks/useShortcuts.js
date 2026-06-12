import { useEffect } from 'react'
import useStore from '../store/useStore'

const isDev = typeof process !== 'undefined' && process.env?.NODE_ENV === 'development'

export default function useShortcuts() {
  const setDrawerOpen = useStore(s => s.setDrawerOpen)
  const showApproval = useStore(s => s.showApproval)
  const isDualPanelOpen = useStore(s => s.isDualPanelOpen)
  const confirmPayment = useStore(s => s.confirmPayment)
  const submitDelivery = useStore(s => s.submitDelivery)
  const resetDemo = useStore(s => s.resetDemo)

  useEffect(() => {
    function handleKey(e) {
      // Global: N = open drawer, ESC = close things
      if (e.key === 'n' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.target?.tagName?.match(/INPUT|TEXTAREA/)) {
        e.preventDefault()
        setDrawerOpen(true)
        return
      }

      if (e.key === 'Escape') {
        if (isDualPanelOpen) {
          useStore.setState({ isDualPanelOpen: false })
        }
        return
      }

      // Dev-only shortcuts (Shift combos)
      if (!isDev) return
      if (!e.shiftKey) return

      switch (e.key) {
        case 'A': // Force approve current pending
          useStore.setState(s => ({ pendingApprovals: s.pendingApprovals.slice(1) }))
          break
        case 'S': // Force submit delivery
          submitDelivery('Auto-submitted delivery (Shift+S)')
          break
        case 'D': // Force close DualTrackPanel
          useStore.setState({ isDualPanelOpen: false, evaluationResult: null })
          break
        case 'R': // Reset demo
          resetDemo()
          break
      }
    }

    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  }, [setDrawerOpen, showApproval, isDualPanelOpen, confirmPayment, submitDelivery, resetDemo])
}
