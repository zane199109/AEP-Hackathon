import React from 'react'
import { Box, Typography } from '@mui/material'

export default function BaseNode({ data, children }) {
  return (
    <Box sx={{
      px: 2, py: 1.5, borderRadius: 3, border: 2, borderColor: data.color,
      bgcolor: data.pulse ? `${data.color}22` : '#111827',
      boxShadow: data.pulse ? `0 0 20px ${data.color}44` : 'none',
      textAlign: 'center', transition: 'all 0.3s',
      animation: data.pulse ? 'pulseNode 0.6s ease' : 'none',
      minWidth: 100, position: 'relative',
    }}>
      {children}
      <Typography variant="body2" fontWeight={700} sx={{ color: data.color }}>
        {data.label}
      </Typography>
      <Typography variant="caption" sx={{ color: '#64748b' }}>
        {data.sub}
      </Typography>
    </Box>
  )
}

export function BuyerNode({ data }) {
  return (
    <BaseNode data={data}>
      <Box sx={{ fontSize: '1.2rem', mb: 0.3 }}>👤</Box>
    </BaseNode>
  )
}

export function EngineNode({ data }) {
  return (
    <Box sx={{
      px: 2, py: 1.5, borderRadius: 2, border: 2, borderColor: data.color,
      bgcolor: data.pulse ? `${data.color}22` : '#111827',
      boxShadow: data.pulse ? `0 0 20px ${data.color}44` : 'none',
      textAlign: 'center', transition: 'all 0.3s',
      animation: data.pulse ? 'pulseNode 0.6s ease' : 'none',
      minWidth: 100, position: 'relative',
      clipPath: 'polygon(25% 0%, 75% 0%, 100% 25%, 100% 75%, 75% 100%, 25% 100%, 0% 75%, 0% 25%)',
    }}>
      <Box sx={{ fontSize: '1.2rem', mb: 0.3 }}>⚙️</Box>
      <Typography variant="body2" fontWeight={700} sx={{ color: data.color }}>
        {data.label}
      </Typography>
      <Typography variant="caption" sx={{ color: '#64748b' }}>
        {data.sub}
      </Typography>
    </Box>
  )
}

export function VaultNode({ data }) {
  return (
    <BaseNode data={data}>
      <Box sx={{ fontSize: '1.2rem', mb: 0.3 }}>🔒</Box>
    </BaseNode>
  )
}

export function ProviderNode({ data }) {
  return (
    <BaseNode data={data}>
      <Box sx={{ fontSize: '1.2rem', mb: 0.3 }}>🔧</Box>
    </BaseNode>
  )
}

export function EvaluatorNode({ data }) {
  return (
    <BaseNode data={data}>
      <Box sx={{ fontSize: '1.2rem', mb: 0.3 }}>🧠</Box>
    </BaseNode>
  )
}

export function SettlementNode({ data }) {
  return (
    <BaseNode data={data}>
      <Box sx={{ fontSize: '1.2rem', mb: 0.3 }}>💳</Box>
    </BaseNode>
  )
}
