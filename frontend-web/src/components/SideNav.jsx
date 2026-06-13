import React from 'react'
import { Box, Typography, Button } from '@mui/material'
import DashboardIcon from '@mui/icons-material/Dashboard'
import DescriptionIcon from '@mui/icons-material/Description'
import GavelIcon from '@mui/icons-material/Gavel'
import SmartToyIcon from '@mui/icons-material/SmartToy'
import SettingsIcon from '@mui/icons-material/Settings'
import useStore from '../store/useStore'

const menuItems = [
  { key: 'dashboard', label: 'Dashboard', icon: <DashboardIcon />, active: true },
  { key: 'pacts', label: 'Pacts', icon: <DescriptionIcon />, active: false },
  { key: 'arbitrations', label: 'Arbitrations', icon: <GavelIcon />, active: false },
  { key: 'agents', label: 'Agents', icon: <SmartToyIcon />, active: false },
  { key: 'settings', label: 'Settings', icon: <SettingsIcon />, active: false },
]

export default function SideNav() {
  const setDrawerOpen = useStore(s => s.setDrawerOpen)

  return (
    <Box sx={{
      width: 220, bgcolor: '#0f0f1a', borderRight: '1px solid #1e1e2e',
      display: 'flex', flexDirection: 'column', justifyContent: 'space-between', py: 1,
    }}>
      <Box>
        {menuItems.map(item => (
          <Box key={item.key} sx={{
            display: 'flex', alignItems: 'center', gap: 1.5, px: 2, py: 1.2, cursor: item.active ? 'pointer' : 'default',
            color: item.active ? '#00f3ff' : '#d7e0ecff', opacity: item.active ? 1 : 0.7,
            borderLeft: item.active ? '2px solid #00f3ff' : '2px solid transparent',
            bgcolor: item.active ? '#00f3ff08' : 'transparent',
            '&:hover': item.active ? { bgcolor: '#00f3ff11' } : {},
          }}>
            {React.cloneElement(item.icon, { sx: { fontSize: '1.1rem' } })}
            <Typography variant="body2" fontWeight={item.active ? 600 : 400} sx={{ fontSize: '0.8rem' }}>
              {item.label}
            </Typography>
          </Box>
        ))}
      </Box>

      <Box sx={{ px: 2, pt: 2, borderTop: '1px solid #1e1e2e', mt: 2 }}>
        <Button
          fullWidth size="small" variant="outlined"
          startIcon={<Box component="span" sx={{ fontSize: '1rem' }}>＋</Box>}
          onClick={() => setDrawerOpen(true)}
          sx={{ borderColor: '#00f3ff', color: '#00f3ff', fontSize: '0.7rem', '&:hover': { borderColor: '#00f3ff', bgcolor: '#00f3ff11' } }}
        >
          新建 Pact
        </Button>
        <Typography variant="caption" sx={{ display: 'block', textAlign: 'center', mt: 0.5, color: '#64748b', fontSize: '0.5rem' }}>
          N
        </Typography>
      </Box>
    </Box>
  )
}
