import React, { useState, useEffect } from 'react'
import { ReactFlow, Background, Controls, MarkerType } from 'reactflow'
import 'reactflow/dist/style.css'
import { Box } from '@mui/material'
import useStore from '../store/useStore'
import DualTrackPanel from '../components/DualTrackPanel'
import { BuyerNode, EngineNode, VaultNode, ProviderNode, EvaluatorNode, SettlementNode, SubProviderNode } from '../nodes/CustomNodes'

const nodeTypes = {
  buyer: BuyerNode, engine: EngineNode, vault: VaultNode,
  provider: ProviderNode, evaluator: EvaluatorNode, settlement: SettlementNode,
  sub_provider: SubProviderNode,
}

const ALL_NODE_DEFS = {
  buyer: { id: 'buyer', type: 'buyer', position: { x: 20, y: 200 }, data: { label: 'Buyer', sub: 'Funds Provider', color: '#6366f1' } },
  engine: { id: 'engine', type: 'engine', position: { x: 270, y: 50 }, data: { label: 'AEP Engine', sub: 'Orchestrator', color: '#6366f1' } },
  vault: { id: 'vault', type: 'vault', position: { x: 520, y: 50 }, data: { label: 'CAW Vault', sub: 'Fund Lock', color: '#6366f1' } },
  provider: { id: 'provider', type: 'provider', position: { x: 270, y: 350 }, data: { label: 'Provider', sub: 'Worker', color: '#eab308' } },
  sub_provider: { id: 'sub_provider', type: 'sub_provider', position: { x: 270, y: 500 }, data: { label: 'Sub-Provider', sub: 'Sub Worker', color: '#f472b6' } },
  evaluator: { id: 'evaluator', type: 'evaluator', position: { x: 520, y: 200 }, data: { label: 'Evaluator', sub: 'Rule + LLM', color: '#eab308' } },
  settlement: { id: 'settlement', type: 'settlement', position: { x: 520, y: 350 }, data: { label: 'Settlement', sub: 'CAW Release', color: '#22c55e' } },
}

const PHASE_NODES = {
  idle: ['buyer'], pending_approval: ['buyer', 'engine', 'vault'],
  published: ['buyer', 'engine', 'vault'], claimed: ['buyer', 'engine', 'vault', 'provider'],
  creating_sub_bounty: ['buyer', 'engine', 'vault', 'provider', 'sub_provider'],
  sub_claimed: ['buyer', 'engine', 'vault', 'provider', 'sub_provider'],
  sub_submitted: ['buyer', 'engine', 'vault', 'provider', 'sub_provider', 'evaluator'],
  submitted: ['buyer', 'engine', 'vault', 'provider', 'sub_provider', 'evaluator'],
  evaluated: ['buyer', 'engine', 'vault', 'provider', 'sub_provider', 'evaluator'],
  verified: ['buyer', 'engine', 'vault', 'provider', 'sub_provider', 'evaluator'],
  settling: ['buyer', 'engine', 'vault', 'provider', 'sub_provider', 'evaluator', 'settlement'],
  slashed: ['buyer', 'engine', 'vault', 'provider', 'sub_provider', 'evaluator'],
  settled: ['buyer', 'engine', 'vault', 'provider', 'sub_provider', 'evaluator', 'settlement'],
}

const PHASE_ORDER = ['idle', 'pending_approval', 'published', 'claimed', 'creating_sub_bounty', 'sub_claimed', 'sub_submitted', 'submitted', 'evaluated', 'verified', 'settling', 'slashed', 'settled']

const EDGE_COLORS = {
  idle: { stroke: '#334155', 'buyer->engine': '#6366f1' },
  pending_approval: { stroke: '#334155', 'buyer->engine': '#eab308', 'engine->vault': '#eab308' },
  published: { stroke: '#334155', 'buyer->engine': '#00f3ff', 'engine->vault': '#00f3ff' },
  claimed: { stroke: '#334155', 'buyer->engine': '#00f3ff', 'engine->vault': '#00f3ff', 'provider->engine': '#ffdd00' },
  creating_sub_bounty: { stroke: '#334155', 'buyer->engine': '#00f3ff', 'engine->vault': '#00f3ff', 'provider->engine': '#ffdd00', 'provider->sub_provider': '#f472b6' },
  sub_claimed: { stroke: '#334155', 'buyer->engine': '#00f3ff', 'engine->vault': '#00f3ff', 'provider->engine': '#ffdd00', 'provider->sub_provider': '#f472b6' },
  sub_submitted: { stroke: '#334155', 'buyer->engine': '#00f3ff', 'engine->vault': '#00f3ff', 'provider->engine': '#ffdd00', 'engine->evaluator': '#ffdd00', 'sub_provider->provider': '#f472b6' },
  submitted: { stroke: '#334155', 'buyer->engine': '#00f3ff', 'engine->vault': '#00f3ff', 'provider->engine': '#ffdd00', 'engine->evaluator': '#ffdd00' },
  evaluated: { stroke: '#334155', 'buyer->engine': '#00f3ff', 'engine->vault': '#00f3ff', 'provider->engine': '#ffdd00', 'engine->evaluator': '#ffdd00', 'engine->settlement': '#00ff66' },
  slashed: { stroke: '#334155', 'buyer->engine': '#00f3ff', 'engine->vault': '#ff003c', 'provider->engine': '#ff003c' },
  settling: { stroke: '#334155', 'buyer->engine': '#00f3ff', 'engine->vault': '#00f3ff', 'provider->engine': '#ffdd00', 'engine->evaluator': '#ffdd00', 'buyer->engine-confirm': '#818cf8', 'engine->settlement': '#818cf8' },
  settled: { stroke: '#334155', 'buyer->engine': '#00f3ff', 'engine->vault': '#00f3ff', 'provider->engine': '#ffdd00', 'engine->evaluator': '#ffdd00', 'buyer->engine-confirm': '#00ff66', 'engine->settlement': '#00ff66' },
}

function buildEdges(phase) {
  const colors = EDGE_COLORS[phase] || EDGE_COLORS.idle
  return [
    { id: 'e1', source: 'buyer', target: 'engine', label: 'Post', animated: phase !== 'idle', markerEnd: { type: MarkerType.ArrowClosed }, style: { stroke: colors['buyer->engine'] || colors.stroke, strokeWidth: 2 } },
    { id: 'e2', source: 'engine', target: 'vault', label: 'Create Pact', animated: phase !== 'idle', markerEnd: { type: MarkerType.ArrowClosed }, style: { stroke: colors['engine->vault'] || colors.stroke, strokeWidth: 2 } },
    { id: 'e3', source: 'provider', target: 'engine', label: 'Claim/Submit', animated: phase === 'claimed' || phase === 'submitted' || phase === 'evaluated' || phase.startsWith('sub_'), markerEnd: { type: MarkerType.ArrowClosed }, style: { stroke: colors['provider->engine'] || colors.stroke, strokeWidth: 2 } },
    { id: 'e4', source: 'engine', target: 'evaluator', label: 'Dual-Track', animated: phase === 'submitted' || phase === 'evaluated' || phase === 'sub_submitted', markerEnd: { type: MarkerType.ArrowClosed }, style: { stroke: colors['engine->evaluator'] || colors.stroke, strokeWidth: 2 } },
    { id: 'e5', source: 'buyer', target: 'engine', label: 'Confirm', animated: phase === 'evaluated' || phase === 'settled', markerEnd: { type: MarkerType.ArrowClosed }, style: { stroke: colors['buyer->engine-confirm'] || colors.stroke, strokeWidth: 2 } },
    { id: 'e6', source: 'engine', target: 'settlement', label: 'Release', animated: phase === 'settling' || phase === 'settled', markerEnd: { type: MarkerType.ArrowClosed }, style: { stroke: colors['engine->settlement'] || colors.stroke, strokeWidth: 2 } },
    { id: 'e7', source: 'provider', target: 'sub_provider', label: 'Delegate', animated: phase === 'creating_sub_bounty' || phase === 'sub_claimed', markerEnd: { type: MarkerType.ArrowClosed }, style: { stroke: colors['provider->sub_provider'] || colors.stroke, strokeWidth: 2 } },
    { id: 'e8', source: 'sub_provider', target: 'provider', label: 'Deliver', animated: phase === 'sub_submitted', markerEnd: { type: MarkerType.ArrowClosed }, style: { stroke: colors['sub_provider->provider'] || colors.stroke, strokeWidth: 2 } },
  ]
}

const PULSE_MAP = {
  bounty_posted: ['buyer', 'engine', 'vault'],
  claimed: ['provider', 'engine'],
  submitted: ['provider', 'engine', 'evaluator'],
  settled: ['buyer', 'engine', 'settlement'],
}

export default function TopologyView() {
  const phase = useStore(s => s.phase)
  const terminalEvents = useStore(s => s.terminalEvents)
  const [nodes, setNodes] = useState([])
  const [edges, setEdges] = useState([])
  const [maxPhaseIdx, setMaxPhaseIdx] = useState(0)

  // Accumulative node visibility: once a node appears it never disappears
  useEffect(() => {
    const currentIdx = PHASE_ORDER.indexOf(phase)
    if (currentIdx < 0) return
    if (currentIdx > maxPhaseIdx) setMaxPhaseIdx(currentIdx)
    // Collect all nodes from idle up to the max phase reached
    const maxIdx = Math.max(currentIdx, maxPhaseIdx)
    const accumulatedIds = []
    for (let i = 0; i <= maxIdx; i++) {
      (PHASE_NODES[PHASE_ORDER[i]] || []).forEach(id => {
        if (!accumulatedIds.includes(id)) accumulatedIds.push(id)
      })
    }
    setNodes(accumulatedIds.map(id => ({ ...ALL_NODE_DEFS[id], data: { ...ALL_NODE_DEFS[id].data, pulse: false } })))
    setEdges(buildEdges(phase))
  }, [phase, maxPhaseIdx])

  useEffect(() => {
    if (terminalEvents.length === 0) return
    const last = terminalEvents[0]
    const typeMap = { '📌': 'bounty_posted', '🤝': 'claimed', '📦': 'submitted', '✅': 'settled', '🔐': 'settled' }
    const prefix = last.text.slice(0, 2)
    const eventType = typeMap[prefix]
    if (!eventType || !PULSE_MAP[eventType]) return
    const colorMap = { bounty_posted: '#6366f1', claimed: '#eab308', submitted: '#eab308', settled: '#22c55e' }
    const pulseIds = PULSE_MAP[eventType]
    const color = colorMap[eventType]
    setNodes(prev => prev.map(n => ({ ...n, data: { ...n.data, color: pulseIds.includes(n.id) ? color : n.data.color, pulse: pulseIds.includes(n.id) } })))
    setTimeout(() => setNodes(prev => prev.map(n => ({ ...n, data: { ...n.data, pulse: false } }))), 600)
  }, [terminalEvents])

  return (
    <Box sx={{ flex: 1, position: 'relative' }}>
      <ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} fitView
        attributionPosition="bottom-left"
        defaultEdgeOptions={{ style: { stroke: '#334155', strokeWidth: 2 }, labelStyle: { fill: '#64748b', fontSize: 10 } }}
      >
        <Background color="#1e293b" gap={20} />
        <Controls style={{ background: '#111827', color: '#e2e8f0' }} />
      </ReactFlow>
      <DualTrackPanel />
    </Box>
  )
}
