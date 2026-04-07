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
  params: Record<string, unknown>
}

interface Props {
  events: TelemetryEvent[]
  focusPids: Record<number, boolean>
  onLoadContext?: (pid: number) => void
  loadingPid?: number | null
}

// ═══════════════════════════════════════════════════
// Colors
// ═══════════════════════════════════════════════════

const catStyle: Record<string, { bg: string; border: string; badge: string; text: string }> = {
  process:  { bg: 'bg-white dark:bg-slate-800',          border: 'border-blue-400',    badge: 'bg-blue-100 text-blue-800 dark:bg-blue-900/60 dark:text-blue-300',       text: 'text-blue-600 dark:text-blue-400' },
  file:     { bg: 'bg-white dark:bg-slate-800',          border: 'border-amber-400',   badge: 'bg-amber-100 text-amber-800 dark:bg-amber-900/60 dark:text-amber-300',   text: 'text-amber-600 dark:text-amber-400' },
  registry: { bg: 'bg-white dark:bg-slate-800',          border: 'border-purple-400',  badge: 'bg-purple-100 text-purple-800 dark:bg-purple-900/60 dark:text-purple-300', text: 'text-purple-600 dark:text-purple-400' },
  net:      { bg: 'bg-white dark:bg-slate-800',          border: 'border-emerald-400', badge: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/60 dark:text-emerald-300', text: 'text-emerald-600 dark:text-emerald-400' },
  image:    { bg: 'bg-white dark:bg-slate-800',          border: 'border-indigo-400',  badge: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-900/60 dark:text-indigo-300', text: 'text-indigo-600 dark:text-indigo-400' },
  dns:      { bg: 'bg-white dark:bg-slate-800',          border: 'border-teal-400',    badge: 'bg-teal-100 text-teal-800 dark:bg-teal-900/60 dark:text-teal-300',       text: 'text-teal-600 dark:text-teal-400' },
  thread:   { bg: 'bg-white dark:bg-slate-800',          border: 'border-pink-400',    badge: 'bg-pink-100 text-pink-800 dark:bg-pink-900/60 dark:text-pink-300',       text: 'text-pink-600 dark:text-pink-400' },
  mem:      { bg: 'bg-white dark:bg-slate-800',          border: 'border-rose-400',    badge: 'bg-rose-100 text-rose-800 dark:bg-rose-900/60 dark:text-rose-300',       text: 'text-rose-600 dark:text-rose-400' },
  handle:   { bg: 'bg-white dark:bg-slate-800',          border: 'border-gray-300',    badge: 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300',          text: 'text-gray-500 dark:text-gray-400' },
}
function cs(cat: string) { return catStyle[cat] || catStyle.handle }

function evtSummary(evt: TelemetryEvent): string {
  const p = evt.params || {}
  switch (evt.event_category) {
    case 'file': return String(p.file_name || p.file_object || p.path || '')
    case 'registry': return String(p.key_name || p.key || p.path || '')
    case 'net': return [p.dip, p.dport].filter(Boolean).join(':') || ''
    case 'dns': return String(p.name || p.domain || '')
    case 'image': return String(p.file_name || p.image_name || '')
    default: return ''
  }
}

// ═══════════════════════════════════════════════════
// Custom Nodes
// ═══════════════════════════════════════════════════

function ProcessNodeComponent({ data }: { data: Record<string, unknown> }) {
  const d = data as {
    label: string; exe: string; cmdline: string; pid: number; ppid: number
    parentName: string; timestamp: string; eventName: string
    isTrigger: boolean; isOnPath: boolean; category: string
    onLoadParent?: () => void; canLoadParent?: boolean; loading?: boolean
  }
  const s = cs(d.category)

  return (
    <div className={
      'rounded-xl border-l-4 border shadow-sm transition-shadow hover:shadow-md ' +
      s.border + ' ' + s.bg + ' ' +
      (d.isTrigger ? 'border-red-400 dark:border-red-600 bg-red-50 dark:bg-red-950/30 shadow-red-100 dark:shadow-red-900/20 ' : 'border-gray-200 dark:border-slate-700 ') +
      (d.isOnPath && !d.isTrigger ? 'bg-blue-50/40 dark:bg-blue-950/20 ' : '')
    } style={{ width: 280, padding: '10px 14px' }}>
      <Handle type="target" position={Position.Left} className="!bg-gray-300 dark:!bg-slate-600 !w-2 !h-2 !border-0" />

      <div className="flex items-center gap-2 flex-wrap">
        <span className={`rounded px-1.5 py-0.5 text-[10px] font-bold ${s.badge}`}>{d.eventName}</span>
        <span className="text-[10px] text-gray-400 dark:text-slate-500">{d.timestamp && new Date(d.timestamp).toLocaleTimeString()}</span>
        {d.isTrigger && <span className="rounded bg-red-500 text-white text-[9px] px-1.5 py-0.5 font-bold">TRIGGER</span>}
      </div>

      {d.exe && <div className="mt-1.5 text-[11px] font-mono text-gray-700 dark:text-slate-200 break-all leading-tight"><span className="text-gray-400 dark:text-slate-500">exe </span>{d.exe}</div>}
      {d.cmdline && d.cmdline !== d.exe && <div className="mt-0.5 text-[10px] font-mono text-gray-500 dark:text-slate-400 break-all leading-tight">{d.cmdline}</div>}

      <div className="mt-1.5 flex items-center gap-3 text-[10px] font-mono">
        {d.pid > 0 && <span><span className="text-gray-400 dark:text-slate-500">pid </span><span className="text-red-600 dark:text-red-400 font-semibold">{d.pid}</span></span>}
        {d.ppid > 0 && <span><span className="text-gray-400 dark:text-slate-500">parent </span>{d.ppid}</span>}
      </div>

      {d.canLoadParent && (
        <button onClick={(e) => { e.stopPropagation(); d.onLoadParent?.() }}
          disabled={d.loading}
          className="mt-1.5 text-[10px] text-fibratus-600 dark:text-fibratus-400 hover:underline disabled:opacity-50">
          {d.loading ? 'Loading...' : `Load parent (${d.parentName || d.ppid})`}
        </button>
      )}

      <Handle type="source" position={Position.Right} className="!bg-gray-300 dark:!bg-slate-600 !w-2 !h-2 !border-0" />
    </div>
  )
}

function EventNodeComponent({ data }: { data: Record<string, unknown> }) {
  const d = data as {
    label: string; eventName: string; detail: string; timestamp: string
    category: string; isTrigger: boolean; params: Record<string, unknown>
  }
  const s = cs(d.category)

  return (
    <div className={
      'rounded-lg border-l-4 border shadow-sm ' + s.border + ' ' + s.bg + ' border-gray-200 dark:border-slate-700'
    } style={{ width: 240, padding: '8px 12px' }}>
      <Handle type="target" position={Position.Left} className="!bg-gray-300 dark:!bg-slate-600 !w-1.5 !h-1.5 !border-0" />

      <div className="flex items-center gap-2">
        <span className={`rounded px-1.5 py-0.5 text-[9px] font-bold ${s.badge}`}>{d.eventName}</span>
        <span className="text-[9px] text-gray-400 dark:text-slate-500">{d.timestamp && new Date(d.timestamp).toLocaleTimeString()}</span>
      </div>
      {d.detail && <div className="mt-1 text-[10px] font-mono text-gray-600 dark:text-slate-300 break-all leading-tight">{d.detail}</div>}

      <Handle type="source" position={Position.Right} className="!bg-gray-300 dark:!bg-slate-600 !w-1.5 !h-1.5 !border-0" />
    </div>
  )
}

function CategoryNodeComponent({ data }: { data: Record<string, unknown> }) {
  const d = data as { label: string; count: number; category: string }
  const s = cs(d.category)

  return (
    <div className={
      'rounded-lg border-l-4 border shadow-sm cursor-pointer hover:shadow-md transition-shadow ' +
      s.border + ' ' + s.bg + ' border-gray-200 dark:border-slate-700'
    } style={{ width: 200, padding: '8px 12px' }}>
      <Handle type="target" position={Position.Left} className="!bg-gray-300 dark:!bg-slate-600 !w-2 !h-2 !border-0" />

      <div className="flex items-center gap-2">
        <span className={`rounded px-1.5 py-0.5 text-[10px] font-bold ${s.badge}`}>{d.category.toUpperCase()}</span>
        <span className="text-[10px] text-gray-500 dark:text-slate-400 font-medium">{d.count} events</span>
      </div>

      <Handle type="source" position={Position.Right} className="!bg-gray-300 dark:!bg-slate-600 !w-2 !h-2 !border-0" />
    </div>
  )
}

const nodeTypes = {
  process: ProcessNodeComponent,
  event: EventNodeComponent,
  category: CategoryNodeComponent,
}

// ═══════════════════════════════════════════════════
// Layout with Dagre
// ═══════════════════════════════════════════════════

function layoutGraph(nodes: Node[], edges: Edge[]): { nodes: Node[]; edges: Edge[] } {
  const g = new Dagre.graphlib.Graph().setDefaultEdgeLabel(() => ({}))
  g.setGraph({ rankdir: 'LR', nodesep: 20, ranksep: 60, marginx: 20, marginy: 20 })

  for (const node of nodes) {
    const w = node.type === 'process' ? 280 : node.type === 'category' ? 200 : 240
    const h = node.type === 'process' ? 120 : 50
    g.setNode(node.id, { width: w, height: h })
  }
  for (const edge of edges) {
    g.setEdge(edge.source, edge.target)
  }

  Dagre.layout(g)

  return {
    nodes: nodes.map(node => {
      const pos = g.node(node.id)
      const w = node.type === 'process' ? 280 : node.type === 'category' ? 200 : 240
      const h = node.type === 'process' ? 120 : 50
      return { ...node, position: { x: pos.x - w / 2, y: pos.y - h / 2 } }
    }),
    edges,
  }
}

// ═══════════════════════════════════════════════════
// Auto fit
// ═══════════════════════════════════════════════════

function FitOnLoad() {
  const { fitView } = useReactFlow()
  useEffect(() => { setTimeout(() => fitView({ padding: 0.2, duration: 300 }), 100) }, [fitView])
  return null
}

// ═══════════════════════════════════════════════════
// Main Component
// ═══════════════════════════════════════════════════

function ProcessChainInner({ events, focusPids, onLoadContext, loadingPid }: Props) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set())

  const toggleExpand = useCallback((catId: string) => {
    setExpanded(prev => { const n = new Set(prev); if (n.has(catId)) n.delete(catId); else n.add(catId); return n })
  }, [])

  const { nodes, edges } = useMemo(() => {
    if (!events.length) return { nodes: [], edges: [] }

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
    for (const [pid, evts] of byPid) {
      const ppid = evts[0].parent_pid
      if (ppid > 0 && ppid !== pid && byPid.has(ppid)) parentOf.set(pid, ppid)
    }

    // Focus path
    const onPath = new Set<number>()
    for (const pid of Object.keys(focusPids).map(Number)) {
      onPath.add(pid)
      let cur = pid
      for (let i = 0; i < 20; i++) { const p = parentOf.get(cur); if (!p) break; onPath.add(p); cur = p }
    }

    // Build nodes per PID
    for (const [pid, evts] of byPid) {
      const mainEvt = evts.find(e => e.event_name === 'CreateProcess') || evts[0]
      const isTrigger = !!focusPids[pid]
      const procNodeId = `p-${pid}`
      const canLoadParent = mainEvt.parent_pid > 0 && !byPid.has(mainEvt.parent_pid) && !!onLoadContext

      allNodes.push({
        id: procNodeId, type: 'process',
        position: { x: 0, y: 0 },
        data: {
          label: mainEvt.process_name, exe: mainEvt.process_exe, cmdline: mainEvt.process_cmdline,
          pid, ppid: mainEvt.parent_pid, parentName: mainEvt.parent_name,
          timestamp: mainEvt.timestamp, eventName: mainEvt.event_name || 'NEW_PROCESS',
          isTrigger, isOnPath: onPath.has(pid), category: 'process',
          canLoadParent, loading: loadingPid === pid,
          onLoadParent: canLoadParent ? () => onLoadContext!(pid) : undefined,
        },
      })

      // Edge to parent
      if (parentOf.has(pid)) {
        allEdges.push({
          id: `e-${parentOf.get(pid)}-${pid}`,
          source: `p-${parentOf.get(pid)}`, target: procNodeId,
          style: { stroke: isTrigger || onPath.has(pid) ? '#ef4444' : '#d1d5db', strokeWidth: onPath.has(pid) ? 2 : 1 },
          animated: isTrigger,
        })
      }

      // Group non-process events by category
      const catGroups = new Map<string, TelemetryEvent[]>()
      for (const e of evts) {
        if (e.event_category === 'process') continue
        const list = catGroups.get(e.event_category) || []
        list.push(e)
        catGroups.set(e.event_category, list)
      }

      for (const [cat, catEvts] of catGroups) {
        const catNodeId = `cat-${pid}-${cat}`

        allNodes.push({
          id: catNodeId, type: 'category',
          position: { x: 0, y: 0 },
          data: { label: cat, count: catEvts.length, category: cat },
        })
        allEdges.push({
          id: `e-${procNodeId}-${catNodeId}`,
          source: procNodeId, target: catNodeId,
          style: { stroke: '#d1d5db', strokeWidth: 1 },
        })

        // If expanded, show individual events
        if (expanded.has(catNodeId)) {
          for (const evt of catEvts.slice(0, 15)) {
            const evtNodeId = `evt-${evt.id}`
            allNodes.push({
              id: evtNodeId, type: 'event',
              position: { x: 0, y: 0 },
              data: {
                label: evt.event_name, eventName: evt.event_name,
                detail: evtSummary(evt), timestamp: evt.timestamp,
                category: cat, isTrigger: false, params: evt.params,
              },
            })
            allEdges.push({
              id: `e-${catNodeId}-${evtNodeId}`,
              source: catNodeId, target: evtNodeId,
              style: { stroke: '#e5e7eb', strokeWidth: 1 },
            })
          }
        }
      }
    }

    return layoutGraph(allNodes, allEdges)
  }, [events, focusPids, expanded, onLoadContext, loadingPid])

  const onNodeClick = useCallback((_: unknown, node: Node) => {
    if (node.type === 'category') toggleExpand(node.id)
  }, [toggleExpand])

  if (nodes.length === 0) {
    return <div className="flex items-center justify-center h-64 text-sm text-gray-400 dark:text-slate-500">No process events found.</div>
  }

  return (
    <div className="h-full w-full" style={{ minHeight: 400 }}>
      <ReactFlow
        nodes={nodes} edges={edges}
        nodeTypes={nodeTypes}
        onNodeClick={onNodeClick}
        fitView
        minZoom={0.1} maxZoom={2}
        defaultEdgeOptions={{ type: 'smoothstep' }}
        proOptions={{ hideAttribution: true }}
      >
        <Background color="#e5e7eb" gap={20} size={1} />
        <Controls position="bottom-left" showInteractive={false} />
        <MiniMap pannable zoomable position="bottom-right"
          nodeColor={(n) => n.data?.isTrigger ? '#ef4444' : n.data?.isOnPath ? '#3b82f6' : '#e5e7eb'} />
        <FitOnLoad />
      </ReactFlow>
    </div>
  )
}

export default function ProcessChain(props: Props) {
  return (
    <ReactFlowProvider>
      <ProcessChainInner {...props} />
    </ReactFlowProvider>
  )
}
