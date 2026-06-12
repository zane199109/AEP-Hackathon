import { Box, IconButton } from '@mui/material'
import CloseIcon from '@mui/icons-material/Close'
import useStore from '../store/useStore'

export default function PactEditorDrawer({ children }) {
  const isOpen = useStore(s => s.isDrawerOpen)
  const setDrawerOpen = useStore(s => s.setDrawerOpen)

  return (
    <>
      {/* Overlay */}
      {isOpen && (
        <Box
          onClick={() => setDrawerOpen(false)}
          sx={{
            position: 'fixed', inset: 0, zIndex: 1200,
            bgcolor: 'rgba(0,0,0,0.5)', backdropFilter: 'blur(2px)',
          }}
        />
      )}

      {/* Drawer */}
      <Box sx={{
        position: 'fixed', top: 0, left: 0, bottom: 0, zIndex: 1300,
        width: '30%', minWidth: 320, maxWidth: 480,
        bgcolor: 'rgba(15,15,26,0.95)', backdropFilter: 'blur(12px)',
        borderRight: '1px solid #00f3ff44',
        transform: isOpen ? 'translateX(0)' : 'translateX(-100%)',
        transition: 'transform 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
        display: 'flex', flexDirection: 'column', overflow: 'auto',
      }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', px: 2, pt: 1.5, pb: 0.5 }}>
          <Box />
          <IconButton size="small" onClick={() => setDrawerOpen(false)} sx={{ color: '#64748b' }}>
            <CloseIcon />
          </IconButton>
        </Box>
        <Box sx={{ px: 2, pb: 2 }}>
          {children}
        </Box>
      </Box>
    </>
  )
}
