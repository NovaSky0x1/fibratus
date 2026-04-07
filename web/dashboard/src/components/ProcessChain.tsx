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
  params: Record<string, unknown>; raw_event?: unknown
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

const catColors: Record<string, { accent: string; badge: string; edge: string }> = {
  process:  { accent: '#3b82f6', badge: 'bg-blue-500/10 text-blue-700 dark:text-blue-300 border border-blue-200 dark:border-blue-800',       edge: '#93c5fd' },
  file:     { accent: '#f59e0b', badge: 'bg-amber-500/10 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-800',   edge: '#fcd34d' },
  registry: { accent: '#8b5cf6', badge: 'bg-purple-500/10 text-purple-700 dark:text-purple-300 border border-purple-200 dark:border-purple-800', edge: '#c4b5fd' },
  net:      { accent: '#10b981', badge: 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800', edge: '#6ee7b7' },
  image:    { accent: '#6366f1', badge: 'bg-indigo-500/10 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800', edge: '#a5b4fc' },
  dns:      { accent: '#14b8a6', badge: 'bg-teal-500/10 text-teal-700 dark:text-teal-300 border border-teal-200 dark:border-teal-800',       edge: '#5eead4' },
  thread:   { accent: '#ec4899', badge: 'bg-pink-500/10 text-pink-700 dark:text-pink-300 border border-pink-200 dark:border-pink-800',       edge: '#f9a8d4' },
  mem:      { accent: '#f43f5e', badge: 'bg-rose-500/10 text-rose-700 dark:text-rose-300 border border-rose-200 dark:border-rose-800',       edge: '#fda4af' },
  handle:   { accent: '#6b7280', badge: 'bg-gray-500/10 text-gray-700 dark:text-gray-300 border border-gray-200 dark:border-gray-700',       edge: '#d1d5db' },
}
function cc(cat: string) { return catColors[cat] || catColors.handle }

function evtDetail(evt: TelemetryEvent): string {
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
// Process Node — main process in the chain
// ═══════════════════════════════════════════════════

function ProcessBlock({ data }: { data: Record<string, unknown> }) {
  const d = data as {
    eventName: string; exe: string; cmdline: string
    pid: number; ppid: number; parentName: string; timestamp: string
    isTrigger: boolean; isOnPath: boolean
    canLoadParent: boolean; loading: boolean; onLoadParent?: () => void
    catSummary: string; expanded: boolean
  }

  return (
    <div className={
      'rounded-xl shadow-sm border bg-white dark:bg-slate-800 ' +
      (d.isTrigger
        ? 'border-red-400 dark:border-red-500 ring-2 ring-red-200/60 dark:ring-red-800/40 '
        : d.isOnPath
          ? 'border-blue-300 dark:border-blue-700 '
          : 'border-gray-200 dark:border-slate-700 ')
    } style={{ width: 300, borderLeftWidth: 4, borderLeftColor: cc('process').accent }}>
      <Handle type="target" position={Position.Left} className="!bg-transparent !border-0 !w-3 !h-3" />
      <div className="px-3 py-2.5">
        <div className="flex items-center gap-1.5 flex-wrap">
          <span className={`rounded-md px-1.5 py-0.5 text-[10px] font-bold ${cc('process').badge}`}>{d.eventName}</span>
          <span className="text-[9px] text-gray-400 dark:text-slate-500 font-mono">{d.timestamp && new Date(d.timestamp).toLocaleString()}</span>
          {d.isTrigger && <span className="rounded-md bg-red-500 text-white text-[9px] px-1.5 py-0.5 font-bold">TRIGGER</span>}
        </div>
        {d.exe && <div className="mt-1.5 text-[11px] font-mono text-gray-800 dark:text-slate-200 break-all leading-snug"><span className="text-gray-400 dark:text-slate-500 text-[10px]">exe </span>{d.exe}</div>}
        {d.cmdline && d.cmdline !== d.exe && <div className="mt-0.5 text-[10px] font-mono text-gray-500 dark:text-slate-400 break-all leading-snug">{d.cmdline}</div>}
        <div className="mt-1.5 flex items-center gap-3 text-[10px] font-mono">
          <span><span className="text-gray-400 dark:text-slate-500">pid </span><span className="text-red-600 dark:text-red-400 font-semibold">{d.pid}</span></span>
          {d.ppid > 0 && <span><span className="text-gray-400 dark:text-slate-500">parent </span><span className="text-blue-600 dark:text-blue-400">{d.ppid}</span></span>}
        </div>
        {d.catSummary && <div className="mt-1.5 text-[9px] text-gray-400 dark:text-slate-500">{d.expanded ? 'Click to collapse' : d.catSummary + ' — click to expand'}</div>}
        {d.canLoadParent && (
          <button onClick={(e) => { e.stopPropagation(); d.onLoadParent?.() }}
            disabled={d.loading}
            className="mt-1.5 w-full rounded-md border border-dashed border-gray-300 dark:border-slate-600 px-2 py-1 text-[10px] text-gray-500 hover:text-fibratus-600 hover:border-fibratus-400 disabled:opacity-50">
            {d.loading ? 'Loading ancestors...' : `Load ancestors (${d.parentName || d.ppid})`}
          </button>
        )}
      </div>
      <Handle type="source" position={Position.Right} className="!bg-transparent !border-0 !w-3 !h-3" />
    </div>
  )
}

// ═══════════════════════════════════════════════════
// Category Node — FILE (24), REGISTRY (8), etc.
// ═══════════════════════════════════════════════════

function CategoryBlock({ data }: { data: Record<string, unknown> }) {
  const d = data as { category: string; count: number; expanded: boolean }
  const c = cc(d.category)
  return (
    <div className="rounded-lg shadow-sm border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 cursor-pointer hover:shadow-md transition-shadow"
      style={{ width: 180, borderLeftWidth: 4, borderLeftColor: c.accent }}>
      <Handle type="target" position={Position.Left} className="!bg-transparent !border-0 !w-2 !h-2" />
      <div className="px-3 py-2">
        <div className="flex items-center gap-2">
          <span className={`rounded-md px-1.5 py-0.5 text-[10px] font-bold ${c.badge}`}>{d.category.toUpperCase()}</span>
          <span className="text-[10px] text-gray-500 dark:text-slate-400">{d.count}</span>
        </div>
        <div className="mt-1 text-[9px] text-gray-400">{d.expanded ? 'Click to collapse' : 'Click to expand'}</div>
      </div>
      <Handle type="source" position={Position.Right} className="!bg-transparent !border-0 !w-2 !h-2" />
    </div>
  )
}

// ═══════════════════════════════════════════════════
// Event Node — individual event detail
// ═══════════════════════════════════════════════════

function EventBlock({ data }: { data: Record<string, unknown> }) {
  const d = data as { eventName: string; category: string; timestamp: string; detail: string; params: Record<string, unknown> }
  const c = cc(d.category)
  return (
    <div className="rounded-lg shadow-sm border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800"
      style={{ width: 240, borderLeftWidth: 3, borderLeftColor: c.accent }}>
      <Handle type="target" position={Position.Left} className="!bg-transparent !border-0 !w-1.5 !h-1.5" />
      <div className="px-2.5 py-1.5">
        <div className="flex items-center gap-1.5">
          <span className={`rounded px-1 py-0.5 text-[9px] font-bold ${c.badge}`}>{d.eventName}</span>
          <span className="text-[8px] text-gray-400 font-mono">{d.timestamp && new Date(d.timestamp).toLocaleTimeString()}</span>
        </div>
        {d.detail && <div className="mt-1 text-[10px] font-mono text-gray-600 dark:text-slate-300 break-all leading-snug">{d.detail}</div>}
      </div>
      <Handle type="source" position={Position.Right} className="!bg-transparent !border-0 !w-1.5 !h-1.5" />
    </div>
  )
}

const nodeTypes = { processBlock: ProcessBlock, categoryBlock: CategoryBlock, eventBlock: EventBlock }

// ═══════════════════════════════════════════════════
// Dagre Layout — proper heights per node type
// ═══════════════════════════════════════════════════

function doLayout(nodes: Node[], edges: Edge[]) {
  const g = new Dagre.graphlib.Graph().setDefaultEdgeLabel(() => ({}))
  g.setGraph({ rankdir: 'LR', nodesep: 24, ranksep: 60, marginx: 40, marginy: 40 })

  for (const n of nodes) {
    let w = 240, h = 50
    if (n.type === 'processBlock') { w = 300; h = 160 }
    else if (n.type === 'categoryBlock') { w = 180; h = 55 }
    else { w = 240; h = 45 }
    g.setNode(n.id, { width: w, height: h })
  }
  for (const e of edges) g.setEdge(e.source, e.target)
  Dagre.layout(g)

  return {
    nodes: nodes.map(n => {
      const p = g.node(n.id)
      return { ...n, position: { x: p.x - p.width / 2, y: p.y - p.height / 2 } }
    }),
    edges,
  }
}

function FitOnLoad({ trigger }: { trigger: number }) {
  const { fitView } = useReactFlow()
  useEffect(() => { setTimeout(() => fitView({ padding: 0.12, duration: 400 }), 150) }, [fitView, trigger])
  return null
}

// ═══════════════════════════════════════════════════
// Main
// ═══════════════════════════════════════════════════

function Inner({ events, focusPids, onLoadContext, loadingPid }: Props) {
  // Two-level expansion: level 1 = show categories for a PID, level 2 = show events for a category
  const [expandedPids, setExpandedPids] = useState<Set<number>>(new Set())
  const [expandedCats, setExpandedCats] = useState<Set<string>>(new Set())
  const [layoutTrigger, setLayoutTrigger] = useState(0)

  const togglePid = useCallback((pid: number) => {
    setExpandedPids(prev => {
      const n = new Set(prev)
      if (n.has(pid)) { n.delete(pid) } else { n.add(pid) }
      return n
    })
    setLayoutTrigger(t => t + 1)
  }, [])

  const toggleCat = useCallback((catId: string) => {
    setExpandedCats(prev => {
      const n = new Set(prev)
      if (n.has(catId)) { n.delete(catId) } else { n.add(catId) }
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

    for (const [pid, evts] of byPid) {
      const mainEvt = evts.find(e => e.event_name === 'CreateProcess') || evts[0]
      const isTrigger = !!focusPids[pid]
      const procId = `p-${pid}`
      const canLoadParent = mainEvt.parent_pid > 0 && !byPid.has(mainEvt.parent_pid) && !!onLoadContext
      const pidExpanded = expandedPids.has(pid)

      // Count events by category for summary
      const catCounts = new Map<string, number>()
      for (const e of evts) {
        if (e.event_category === 'process') continue
        catCounts.set(e.event_category, (catCounts.get(e.event_category) || 0) + 1)
      }
      const catSummary = Array.from(catCounts.entries()).map(([c, n]) => `${c}(${n})`).join(' ')

      // Process node
      allNodes.push({
        id: procId, type: 'processBlock', position: { x: 0, y: 0 },
        data: {
          eventName: mainEvt.event_name || 'NEW_PROCESS',
          exe: mainEvt.process_exe, cmdline: mainEvt.process_cmdline,
          pid, ppid: mainEvt.parent_pid, parentName: mainEvt.parent_name,
          timestamp: mainEvt.timestamp,
          isTrigger, isOnPath: onPath.has(pid),
          canLoadParent, loading: loadingPid === pid,
          onLoadParent: canLoadParent ? () => onLoadContext!(pid) : undefined,
          catSummary, expanded: pidExpanded,
        },
      })

      // Edge to parent process
      if (parentOf.has(pid)) {
        const isPathEdge = onPath.has(pid)
        allEdges.push({
          id: `e-${parentOf.get(pid)}-${pid}`, source: `p-${parentOf.get(pid)}`, target: procId,
          type: 'smoothstep', style: { stroke: isPathEdge ? '#ef4444' : '#d1d5db', strokeWidth: isPathEdge ? 2.5 : 1.5 },
          animated: isTrigger,
        })
      }

      // Level 1: Category nodes (when process is expanded)
      if (pidExpanded) {
        for (const [cat, count] of catCounts) {
          const catId = `cat-${pid}-${cat}`
          const catExpanded = expandedCats.has(catId)
          const c = cc(cat)

          allNodes.push({
            id: catId, type: 'categoryBlock', position: { x: 0, y: 0 },
            data: { category: cat, count, expanded: catExpanded },
          })
          allEdges.push({
            id: `e-${procId}-${catId}`, source: procId, target: catId,
            type: 'smoothstep', style: { stroke: c.edge, strokeWidth: 1.5 },
          })

          // Level 2: Individual event nodes (when category is expanded)
          if (catExpanded) {
            const catEvts = evts.filter(e => e.event_category === cat)
            for (const evt of catEvts.slice(0, 25)) {
              const evtId = `evt-${evt.id}`
              allNodes.push({
                id: evtId, type: 'eventBlock', position: { x: 0, y: 0 },
                data: {
                  eventName: evt.event_name, category: cat,
                  timestamp: evt.timestamp, detail: evtDetail(evt),
                  params: evt.params || {},
                },
              })
              allEdges.push({
                id: `e-${catId}-${evtId}`, source: catId, target: evtId,
                type: 'smoothstep', style: { stroke: c.edge, strokeWidth: 1 },
              })
            }
          }
        }
      }
    }

    return doLayout(allNodes, allEdges)
  }, [events, focusPids, expandedPids, expandedCats, onLoadContext, loadingPid])

  const onNodeClick = useCallback((_: unknown, node: Node) => {
    const d = node.data as Record<string, unknown>
    if (node.type === 'processBlock' && (d.pid as number) > 0) {
      togglePid(d.pid as number)
    } else if (node.type === 'categoryBlock') {
      toggleCat(node.id)
    }
  }, [togglePid, toggleCat])

  if (nodes.length === 0) {
    return <div className="flex items-center justify-center h-64 text-sm text-gray-400 dark:text-slate-500">No process events found for this detection.</div>
  }

  return (
    <div className="h-full w-full" style={{ minHeight: 500 }}>
      <ReactFlow
        nodes={nodes} edges={edges}
        nodeTypes={nodeTypes}
        onNodeClick={onNodeClick}
        fitView minZoom={0.05} maxZoom={2.5}
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
            return cc((d.category as string) || 'handle').accent
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
