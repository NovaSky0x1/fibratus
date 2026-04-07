import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ReactFlow, Background, Controls, MiniMap,
  type Node, type Edge, Position, Handle,
  ReactFlowProvider, useReactFlow,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import Dagre from '@dagrejs/dagre'

// ═══════════════════════════════════════════════════
// Types
// ═══════════════════════════════════════════════════

interface TelemetryEvent {
  id: number; timestamp: string; event_name: string; event_category: string
  pid: number; tid: number; process_name: string; process_exe: string
  process_cmdline: string; parent_pid: number; parent_name: string
  params: Record<string, unknown>; raw_event: unknown
}

interface Props {
  events: TelemetryEvent[]
  focusPids: Record<number, boolean>
  onLoadContext?: (pid: number) => void
  loadingPid?: number | null
}

// ═══════════════════════════════════════════════════
// Colors per event category
// ═══════════════════════════════════════════════════

const catColors: Record<string, { accent: string; bg: string; badge: string; edge: string }> = {
  process:  { accent: '#3b82f6', bg: 'bg-white dark:bg-slate-800',   badge: 'bg-blue-500/10 text-blue-700 dark:text-blue-300 border border-blue-200 dark:border-blue-800',       edge: '#93c5fd' },
  file:     { accent: '#f59e0b', bg: 'bg-white dark:bg-slate-800',   badge: 'bg-amber-500/10 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-800',   edge: '#fcd34d' },
  registry: { accent: '#8b5cf6', bg: 'bg-white dark:bg-slate-800',   badge: 'bg-purple-500/10 text-purple-700 dark:text-purple-300 border border-purple-200 dark:border-purple-800', edge: '#c4b5fd' },
  net:      { accent: '#10b981', bg: 'bg-white dark:bg-slate-800',   badge: 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800', edge: '#6ee7b7' },
  image:    { accent: '#6366f1', bg: 'bg-white dark:bg-slate-800',   badge: 'bg-indigo-500/10 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800', edge: '#a5b4fc' },
  dns:      { accent: '#14b8a6', bg: 'bg-white dark:bg-slate-800',   badge: 'bg-teal-500/10 text-teal-700 dark:text-teal-300 border border-teal-200 dark:border-teal-800',       edge: '#5eead4' },
  thread:   { accent: '#ec4899', bg: 'bg-white dark:bg-slate-800',   badge: 'bg-pink-500/10 text-pink-700 dark:text-pink-300 border border-pink-200 dark:border-pink-800',       edge: '#f9a8d4' },
  mem:      { accent: '#f43f5e', bg: 'bg-white dark:bg-slate-800',   badge: 'bg-rose-500/10 text-rose-700 dark:text-rose-300 border border-rose-200 dark:border-rose-800',       edge: '#fda4af' },
  handle:   { accent: '#6b7280', bg: 'bg-white dark:bg-slate-800',   badge: 'bg-gray-500/10 text-gray-700 dark:text-gray-300 border border-gray-200 dark:border-gray-700',       edge: '#d1d5db' },
}
function cc(cat: string) { return catColors[cat] || catColors.handle }

function eventDetail(evt: TelemetryEvent): string {
  const p = evt.params || {}
  switch (evt.event_category) {
    case 'file': return String(p.file_name || p.file_object || p.path || '')
    case 'registry': return String(p.key_name || p.key || p.path || '')
    case 'net': return [p.dip || p.sip, p.dport || p.sport].filter(Boolean).join(':')
    case 'dns': return String(p.name || p.domain || '')
    case 'image': return String(p.file_name || p.image_name || '')
    default: return ''
  }
}

// ═══════════════════════════════════════════════════
// Event Node — used for ALL events (process + file + net + etc)
// ═══════════════════════════════════════════════════

function EventBlock({ data }: { data: Record<string, unknown> }) {
  const d = data as {
    eventName: string; category: string; timestamp: string
    processName: string; exe: string; cmdline: string
    pid: number; ppid: number; parentName: string
    detail: string; isTrigger: boolean; isOnPath: boolean
    canLoadParent: boolean; loading: boolean
    onLoadParent?: () => void
    params: Record<string, unknown>
  }
  const c = cc(d.category)
  const isProcess = d.category === 'process'

  return (
    <div
      className={
        'rounded-xl shadow-sm transition-shadow hover:shadow-lg border ' + c.bg + ' ' +
        (d.isTrigger
          ? 'border-red-400 dark:border-red-500 ring-2 ring-red-200 dark:ring-red-800/50 '
          : d.isOnPath
            ? 'border-blue-300 dark:border-blue-700 '
            : 'border-gray-200 dark:border-slate-700 ')
      }
      style={{ width: isProcess ? 300 : 260, borderLeftWidth: 4, borderLeftColor: c.accent }}
    >
      <Handle type="target" position={Position.Left} className="!bg-transparent !border-0 !w-3 !h-3" />

      <div className="px-3 py-2.5">
        {/* Header row */}
        <div className="flex items-center gap-1.5 flex-wrap">
          <span className={`rounded-md px-1.5 py-0.5 text-[10px] font-bold ${c.badge}`}>{d.eventName}</span>
          {d.timestamp && <span className="text-[9px] text-gray-400 dark:text-slate-500 font-mono">{new Date(d.timestamp).toLocaleString()}</span>}
          {d.isTrigger && <span className="rounded-md bg-red-500 text-white text-[9px] px-1.5 py-0.5 font-bold tracking-wide">TRIGGER</span>}
        </div>

        {/* Process info */}
        {isProcess && (
          <div className="mt-2 space-y-1">
            {d.exe && (
              <div className="text-[11px] leading-snug">
                <span className="text-gray-400 dark:text-slate-500 font-mono text-[10px]">exe </span>
                <span className="text-gray-800 dark:text-slate-200 font-mono break-all">{d.exe}</span>
              </div>
            )}
            {d.cmdline && d.cmdline !== d.exe && (
              <div className="text-[10px] leading-snug">
                <span className="text-gray-400 dark:text-slate-500 font-mono">cmd </span>
                <span className="text-gray-600 dark:text-slate-300 font-mono break-all">{d.cmdline}</span>
              </div>
            )}
            <div className="flex items-center gap-3 text-[10px] font-mono pt-0.5">
              <span><span className="text-gray-400 dark:text-slate-500">pid </span><span className="text-red-600 dark:text-red-400 font-semibold">{d.pid}</span></span>
              {d.ppid > 0 && <span><span className="text-gray-400 dark:text-slate-500">parent </span><span className="text-blue-600 dark:text-blue-400">{d.ppid}</span></span>}
            </div>
          </div>
        )}

        {/* Non-process event detail */}
        {!isProcess && d.detail && (
          <div className="mt-1.5 text-[10px] font-mono text-gray-700 dark:text-slate-300 break-all leading-snug">{d.detail}</div>
        )}

        {/* Key params */}
        {!isProcess && d.params && Object.keys(d.params).length > 0 && (
          <div className="mt-1.5 space-y-0.5">
            {Object.entries(d.params).slice(0, 4).map(([k, v]) => (
              <div key={k} className="text-[9px] font-mono leading-tight">
                <span className="text-gray-400 dark:text-slate-500">{k} </span>
                <span className="text-gray-600 dark:text-slate-400 break-all">{String(v)}</span>
              </div>
            ))}
          </div>
        )}

        {/* Load parent */}
        {d.canLoadParent && (
          <button onClick={(e) => { e.stopPropagation(); d.onLoadParent?.() }}
            disabled={d.loading}
            className="mt-2 w-full rounded-md border border-dashed border-gray-300 dark:border-slate-600 px-2 py-1 text-[10px] text-gray-500 dark:text-slate-400 hover:text-fibratus-600 hover:border-fibratus-400 transition-colors disabled:opacity-50">
            {d.loading ? 'Loading ancestors...' : `Load ancestors (parent: ${d.parentName || d.ppid})`}
          </button>
        )}
      </div>

      <Handle type="source" position={Position.Right} className="!bg-transparent !border-0 !w-3 !h-3" />
    </div>
  )
}

const nodeTypes = { eventBlock: EventBlock }

// ═══════════════════════════════════════════════════
// Dagre Layout
// ═══════════════════════════════════════════════════

function layout(nodes: Node[], edges: Edge[]) {
  const g = new Dagre.graphlib.Graph().setDefaultEdgeLabel(() => ({}))
  g.setGraph({ rankdir: 'LR', nodesep: 16, ranksep: 50, marginx: 30, marginy: 30 })

  for (const n of nodes) {
    const isProc = (n.data as Record<string, unknown>).category === 'process'
    g.setNode(n.id, { width: isProc ? 300 : 260, height: isProc ? 130 : 70 })
  }
  for (const e of edges) g.setEdge(e.source, e.target)
  Dagre.layout(g)

  return {
    nodes: nodes.map(n => {
      const p = g.node(n.id)
      const isProc = (n.data as Record<string, unknown>).category === 'process'
      return { ...n, position: { x: p.x - (isProc ? 150 : 130), y: p.y - (isProc ? 65 : 35) } }
    }),
    edges,
  }
}

// ═══════════════════════════════════════════════════
// Fit view on load
// ═══════════════════════════════════════════════════

function FitOnLoad({ trigger }: { trigger: number }) {
  const { fitView } = useReactFlow()
  useEffect(() => { setTimeout(() => fitView({ padding: 0.15, duration: 400 }), 150) }, [fitView, trigger])
  return null
}

// ═══════════════════════════════════════════════════
// Main
// ═══════════════════════════════════════════════════

function Inner({ events, focusPids, onLoadContext, loadingPid }: Props) {
  const [expandedPids, setExpandedPids] = useState<Set<number>>(new Set())
  const [layoutTrigger, setLayoutTrigger] = useState(0)

  // Auto-expand trigger PIDs
  useEffect(() => {
    const auto = new Set<number>()
    for (const pid of Object.keys(focusPids).map(Number)) auto.add(pid)
    if (auto.size > 0) setExpandedPids(auto)
  }, [focusPids])

  const toggleExpand = useCallback((pid: number) => {
    setExpandedPids(prev => {
      const n = new Set(prev)
      if (n.has(pid)) n.delete(pid); else n.add(pid)
      return n
    })
    setLayoutTrigger(t => t + 1)
  }, [])

  const { nodes, edges } = useMemo(() => {
    if (!events.length) return { nodes: [] as Node[], edges: [] as Edge[] }

    const allNodes: Node[] = []
    const allEdges: Edge[] = []

    // Group by PID
    const byPid = new Map<number, TelemetryEvent[]>()
    for (const e of events) {
      if (e.pid <= 0) continue
      const list = byPid.get(e.pid) || []
      list.push(e)
      byPid.set(e.pid, list)
    }

    // Parent-child
    const parentOf = new Map<number, number>()
    const childrenOf = new Map<number, Set<number>>()
    for (const [pid, evts] of byPid) {
      const ppid = evts[0].parent_pid
      if (ppid > 0 && ppid !== pid && byPid.has(ppid)) {
        parentOf.set(pid, ppid)
        if (!childrenOf.has(ppid)) childrenOf.set(ppid, new Set())
        childrenOf.get(ppid)!.add(pid)
      }
    }

    // Focus path
    const onPath = new Set<number>()
    for (const pid of Object.keys(focusPids).map(Number)) {
      onPath.add(pid)
      let cur = pid
      for (let i = 0; i < 30; i++) { const p = parentOf.get(cur); if (!p) break; onPath.add(p); cur = p }
    }

    // Build nodes for each PID
    for (const [pid, evts] of byPid) {
      const mainEvt = evts.find(e => e.event_name === 'CreateProcess') || evts[0]
      const isTrigger = !!focusPids[pid]
      const procId = `p-${pid}`
      const canLoadParent = mainEvt.parent_pid > 0 && !byPid.has(mainEvt.parent_pid) && !!onLoadContext

      // Process node
      allNodes.push({
        id: procId, type: 'eventBlock', position: { x: 0, y: 0 },
        data: {
          eventName: mainEvt.event_name || 'NEW_PROCESS', category: 'process',
          timestamp: mainEvt.timestamp, processName: mainEvt.process_name,
          exe: mainEvt.process_exe, cmdline: mainEvt.process_cmdline,
          pid, ppid: mainEvt.parent_pid, parentName: mainEvt.parent_name,
          detail: '', isTrigger, isOnPath: onPath.has(pid),
          canLoadParent, loading: loadingPid === pid,
          onLoadParent: canLoadParent ? () => onLoadContext!(pid) : undefined,
          params: {},
        },
      })

      // Edge to parent
      if (parentOf.has(pid)) {
        const isPathEdge = onPath.has(pid)
        allEdges.push({
          id: `e-${parentOf.get(pid)}-${pid}`, source: `p-${parentOf.get(pid)}`, target: procId,
          type: 'smoothstep',
          style: { stroke: isPathEdge ? '#ef4444' : '#d1d5db', strokeWidth: isPathEdge ? 2.5 : 1.5 },
          animated: isTrigger,
        })
      }

      // Event nodes (when PID is expanded)
      if (expandedPids.has(pid)) {
        const nonProcess = evts.filter(e => e.event_category !== 'process')
        for (const evt of nonProcess.slice(0, 50)) {
          const evtId = `evt-${evt.id}`
          const c = cc(evt.event_category)
          allNodes.push({
            id: evtId, type: 'eventBlock', position: { x: 0, y: 0 },
            data: {
              eventName: evt.event_name, category: evt.event_category,
              timestamp: evt.timestamp, processName: '', exe: '', cmdline: '',
              pid: 0, ppid: 0, parentName: '',
              detail: eventDetail(evt), isTrigger: false, isOnPath: false,
              canLoadParent: false, loading: false, params: evt.params || {},
            },
          })
          allEdges.push({
            id: `e-${procId}-${evtId}`, source: procId, target: evtId,
            type: 'smoothstep',
            style: { stroke: c.edge, strokeWidth: 1 },
          })
        }
      }
    }

    return layout(allNodes, allEdges)
  }, [events, focusPids, expandedPids, onLoadContext, loadingPid])

  const onNodeClick = useCallback((_: unknown, node: Node) => {
    const d = node.data as Record<string, unknown>
    if (d.category === 'process' && (d.pid as number) > 0) {
      toggleExpand(d.pid as number)
    }
  }, [toggleExpand])

  if (nodes.length === 0) {
    return <div className="flex items-center justify-center h-64 text-sm text-gray-400 dark:text-slate-500">No process events found for this detection.</div>
  }

  return (
    <div className="h-full w-full" style={{ minHeight: 500 }}>
      <ReactFlow
        nodes={nodes} edges={edges}
        nodeTypes={nodeTypes}
        onNodeClick={onNodeClick}
        fitView
        minZoom={0.05} maxZoom={2.5}
        proOptions={{ hideAttribution: true }}
        defaultEdgeOptions={{ type: 'smoothstep' }}
      >
        <Background color="#f0f0f0" gap={24} size={1} />
        <Controls position="bottom-left" showInteractive={false} />
        <MiniMap pannable zoomable position="bottom-right"
          style={{ border: '1px solid #e5e7eb', borderRadius: 8 }}
          nodeColor={(n) => {
            const d = n.data as Record<string, unknown>
            if (d.isTrigger) return '#ef4444'
            if (d.isOnPath) return '#3b82f6'
            return cc(d.category as string).accent
          }}
        />
        <FitOnLoad trigger={layoutTrigger} />
      </ReactFlow>
    </div>
  )
}

export default function ProcessChain(props: Props) {
  return (
    <ReactFlowProvider>
      <Inner {...props} />
    </ReactFlowProvider>
  )
}
