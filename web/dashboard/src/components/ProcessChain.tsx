import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ReactFlow, Background, Controls, MiniMap,
  type Node, type Edge, Position, Handle,
  ReactFlowProvider, useReactFlow,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import ELK from 'elkjs/lib/elk.bundled.js'

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
  // Raw detection events JSON — used to enrich trigger process nodes with hashes, certs, etc.
  detectionEvents?: Record<string, unknown>[]
}

// ═══════════════════════════════════════════════════
// Colors
// ═══════════════════════════════════════════════════

const cat: Record<string, { accent: string; badge: string; edge: string }> = {
  process:  { accent: '#3b82f6', badge: 'bg-blue-100 text-blue-800 dark:bg-blue-900/60 dark:text-blue-300 border border-blue-200 dark:border-blue-700', edge: '#93c5fd' },
  file:     { accent: '#f59e0b', badge: 'bg-amber-100 text-amber-800 dark:bg-amber-900/60 dark:text-amber-300 border border-amber-200 dark:border-amber-700', edge: '#fcd34d' },
  registry: { accent: '#8b5cf6', badge: 'bg-purple-100 text-purple-800 dark:bg-purple-900/60 dark:text-purple-300 border border-purple-200 dark:border-purple-700', edge: '#c4b5fd' },
  net:      { accent: '#10b981', badge: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/60 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-700', edge: '#6ee7b7' },
  image:    { accent: '#6366f1', badge: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-900/60 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-700', edge: '#a5b4fc' },
  dns:      { accent: '#14b8a6', badge: 'bg-teal-100 text-teal-800 dark:bg-teal-900/60 dark:text-teal-300 border border-teal-200 dark:border-teal-700', edge: '#5eead4' },
  thread:   { accent: '#ec4899', badge: 'bg-pink-100 text-pink-800 dark:bg-pink-900/60 dark:text-pink-300 border border-pink-200 dark:border-pink-700', edge: '#f9a8d4' },
  mem:      { accent: '#f43f5e', badge: 'bg-rose-100 text-rose-800 dark:bg-rose-900/60 dark:text-rose-300 border border-rose-200 dark:border-rose-700', edge: '#fda4af' },
  handle:   { accent: '#6b7280', badge: 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300 border border-gray-200 dark:border-gray-600', edge: '#d1d5db' },
}
function cc(c: string) { return cat[c] || cat.handle }

function parseParams(raw: unknown): Record<string, unknown> {
  if (!raw) return {}
  if (typeof raw === 'object' && !Array.isArray(raw)) return raw as Record<string, unknown>
  if (typeof raw === 'string') { try { return JSON.parse(raw) } catch { return {} } }
  return {}
}

function evtPreview(evt: TelemetryEvent): string {
  const p = parseParams(evt.params)
  switch (evt.event_category) {
    case 'file': return String(p.file_path || '')
    case 'registry': return String(p.key_path || '')
    case 'net': return [p.dip, p.dport].filter(Boolean).join(':')
    case 'dns': return String(p.name || '')
    case 'image': return String(p.file_path || '')
    case 'handle': return String(p.handle_name || '')
    default: return ''
  }
}

// ═══════════════════════════════════════════════════
// Process Node
// ═══════════════════════════════════════════════════

function ProcessNode({ data }: { data: Record<string, unknown> }) {
  const d = data as {
    exe: string; cmdline: string; pid: number; ppid: number; parentName: string
    timestamp: string; eventName: string; isTrigger: boolean; isOnPath: boolean
    canLoadParent: boolean; loading: boolean; onLoadParent?: () => void
    catSummary: string; expanded: boolean
    md5: string; sha256: string; isSigned: boolean; certSubject: string; sid: string; username: string
  }
  return (
    <div className={
      'rounded-xl shadow-sm border bg-white dark:bg-slate-800 ' +
      (d.isTrigger ? 'border-red-400 dark:border-red-500 ring-2 ring-red-200/50 dark:ring-red-800/30 '
        : d.isOnPath ? 'border-blue-300 dark:border-blue-700 ' : 'border-gray-200 dark:border-slate-700 ')
    } style={{ width: 300, borderLeftWidth: 4, borderLeftColor: cc('process').accent }}>
      <Handle type="target" position={Position.Left} className="!bg-transparent !border-0 !w-3 !h-3" />
      <div className="px-3 py-2.5">
        <div className="flex items-center gap-1.5 flex-wrap">
          <span className={`rounded-md px-1.5 py-0.5 text-[10px] font-bold ${cc('process').badge}`}>{d.eventName}</span>
          {d.timestamp && <span className="text-[9px] text-gray-400 dark:text-slate-500 font-mono">{new Date(d.timestamp).toLocaleString()}</span>}
          {d.isTrigger && <span className="rounded-md bg-red-500 text-white text-[9px] px-1.5 py-0.5 font-bold">TRIGGER</span>}
        </div>
        {d.exe && <div className="mt-1.5 text-[11px] font-mono text-gray-800 dark:text-slate-200 break-all leading-snug"><span className="text-gray-400 text-[10px]">exe </span>{d.exe}</div>}
        {d.cmdline && d.cmdline !== d.exe && <div className="mt-0.5 text-[10px] font-mono text-gray-500 dark:text-slate-400 break-all leading-snug">{d.cmdline}</div>}
        <div className="mt-1.5 flex gap-3 text-[10px] font-mono">
          <span><span className="text-gray-400">pid </span><span className="text-red-600 dark:text-red-400 font-semibold">{d.pid}</span></span>
          {d.ppid > 0 && <span><span className="text-gray-400">parent </span><span className="text-blue-600 dark:text-blue-400">{d.ppid}</span></span>}
        </div>
        {(d.username || d.sid) && <div className="mt-1 text-[10px] font-mono text-gray-500 dark:text-slate-400">{d.username && <span>{d.username} </span>}{d.sid && <span className="text-gray-400 text-[9px]">{d.sid}</span>}</div>}
        {d.sha256 && <div className="mt-0.5 text-[9px] font-mono text-gray-400 dark:text-slate-500 break-all">sha256: {d.sha256}</div>}
        {d.md5 && <div className="text-[9px] font-mono text-gray-400 dark:text-slate-500">md5: {d.md5}</div>}
        {d.isSigned !== undefined && <div className="text-[9px] font-mono text-gray-400">{d.isSigned ? '✓ signed' : '✗ unsigned'}{d.certSubject ? ` — ${d.certSubject}` : ''}</div>}
        {d.catSummary && <div className="mt-1.5 text-[9px] text-gray-400">{d.expanded ? '▼ click to collapse' : '▶ ' + d.catSummary}</div>}
        {d.canLoadParent && (
          <button onClick={(e) => { e.stopPropagation(); d.onLoadParent?.() }} disabled={d.loading}
            className="mt-1.5 w-full rounded-md border border-dashed border-gray-300 dark:border-slate-600 px-2 py-1 text-[10px] text-gray-500 hover:text-fibratus-600 hover:border-fibratus-400 disabled:opacity-50">
            {d.loading ? 'Loading...' : `▲ Load ancestors (${d.parentName || d.ppid})`}
          </button>
        )}
      </div>
      <Handle type="source" position={Position.Right} className="!bg-transparent !border-0 !w-3 !h-3" />
    </div>
  )
}

// ═══════════════════════════════════════════════════
// Category Node
// ═══════════════════════════════════════════════════

function CategoryNode({ data }: { data: Record<string, unknown> }) {
  const d = data as { category: string; count: number; expanded: boolean; preview: string[]; events: { eventName: string; detail: string; timestamp: string; idx: number }[]; onSelectEvent?: (catId: string, idx: number) => void; nodeId: string }
  const c = cc(d.category)
  return (
    <div className="rounded-lg shadow-sm border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 cursor-pointer hover:shadow-md"
      style={{ width: d.expanded ? 320 : 220, borderLeftWidth: 4, borderLeftColor: c.accent }}>
      <Handle type="target" position={Position.Left} className="!bg-transparent !border-0 !w-2 !h-2" />
      <div className="px-3 py-2">
        <div className="flex items-center gap-2">
          <span className={`rounded-md px-1.5 py-0.5 text-[10px] font-bold ${c.badge}`}>{d.category.toUpperCase()}</span>
          <span className="text-[10px] text-gray-500 dark:text-slate-400">{d.count} events</span>
          <span className="ml-auto text-[9px] text-gray-400">{d.expanded ? '▼' : '▶'}</span>
        </div>
        {!d.expanded && d.preview.length > 0 && (
          <div className="mt-1.5 space-y-0.5">
            {d.preview.map((p, i) => (
              <div key={i} className="text-[9px] font-mono text-gray-500 dark:text-slate-400 truncate">{p}</div>
            ))}
            {d.count > d.preview.length && <div className="text-[8px] text-gray-400">+{d.count - d.preview.length} more</div>}
          </div>
        )}
        {d.expanded && d.events && (
          <div className="mt-2 max-h-64 overflow-y-auto space-y-1 nowheel" onClick={e => e.stopPropagation()}>
            {d.events.map((evt, i) => (
              <div key={i}
                onClick={() => d.onSelectEvent?.(d.nodeId, evt.idx)}
                className="rounded border border-gray-100 dark:border-slate-600 px-2 py-1 cursor-pointer hover:bg-gray-50 dark:hover:bg-slate-700 transition-colors">
                <div className="flex items-center gap-1.5">
                  <span className={`rounded px-1 py-0.5 text-[8px] font-bold ${c.badge}`}>{evt.eventName}</span>
                  <span className="text-[8px] text-gray-400 font-mono">{evt.timestamp && new Date(evt.timestamp).toLocaleTimeString()}</span>
                </div>
                {evt.detail && <div className="mt-0.5 text-[9px] font-mono text-gray-600 dark:text-slate-300 break-all leading-tight">{evt.detail}</div>}
              </div>
            ))}
          </div>
        )}
      </div>
      <Handle type="source" position={Position.Right} className="!bg-transparent !border-0 !w-2 !h-2" />
    </div>
  )
}

// ═══════════════════════════════════════════════════
// Event Node — shows more detail per category
const nodeTypes = { processNode: ProcessNode, categoryNode: CategoryNode }

// ═══════════════════════════════════════════════════
// ELK Layout
// ═══════════════════════════════════════════════════

const elk = new ELK()

async function doLayout(nodes: Node[], edges: Edge[]): Promise<{ nodes: Node[]; edges: Edge[] }> {
  const elkNodes = nodes.map(n => {
    let w = 260, h = 55
    if (n.type === 'processNode') { w = 300; h = 160 }
    else if (n.type === 'categoryNode') {
      const expanded = (n.data as Record<string, unknown>).expanded
      w = expanded ? 320 : 220
      h = expanded ? 300 : 90
    }
    else { h = 50 + ((n.data as Record<string, unknown>).infoLines as string[] || []).length * 14 }
    return { id: n.id, width: w, height: Math.max(h, 50) }
  })

  const elkEdges = edges.map(e => ({ id: e.id, sources: [e.source], targets: [e.target] }))

  const graph = await elk.layout({
    id: 'root',
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': 'RIGHT',
      'elk.spacing.nodeNode': '30',
      'elk.layered.spacing.nodeNodeBetweenLayers': '80',
      'elk.spacing.edgeNode': '20',
      'elk.spacing.edgeEdge': '15',
      'elk.edgeRouting': 'SPLINES',
      'elk.layered.nodePlacement.strategy': 'NETWORK_SIMPLEX',
    },
    children: elkNodes,
    edges: elkEdges,
  })

  const posMap = new Map<string, { x: number; y: number }>()
  for (const child of graph.children || []) {
    posMap.set(child.id, { x: child.x || 0, y: child.y || 0 })
  }

  return {
    nodes: nodes.map(n => ({ ...n, position: posMap.get(n.id) || { x: 0, y: 0 } })),
    edges,
  }
}

// ═══════════════════════════════════════════════════
// Detail Panel (right sidebar)
// ═══════════════════════════════════════════════════

function DetailPanel({ event, onClose }: { event: TelemetryEvent | null; onClose: () => void }) {
  if (!event) return null
  const params = parseParams(event.params)
  return (
    <div className="w-80 h-full bg-white dark:bg-slate-800 border-l border-gray-200 dark:border-slate-700 overflow-y-auto shrink-0">
      <div className="px-4 py-3 border-b border-gray-200 dark:border-slate-700 flex items-center justify-between sticky top-0 bg-white dark:bg-slate-800">
        <span className={`rounded-md px-2 py-0.5 text-[10px] font-bold ${cc(event.event_category).badge}`}>{event.event_name}</span>
        <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-sm">✕</button>
      </div>
      <div className="p-4 space-y-3">
        <div>
          <div className="text-[10px] text-gray-400 uppercase tracking-wider">Process</div>
          <div className="text-xs font-mono text-gray-800 dark:text-slate-200 break-all">{event.process_name} (PID {event.pid})</div>
          {event.process_exe && <div className="text-[10px] font-mono text-gray-500 dark:text-slate-400 break-all mt-0.5">{event.process_exe}</div>}
        </div>
        <div>
          <div className="text-[10px] text-gray-400 uppercase tracking-wider">Timestamp</div>
          <div className="text-xs font-mono text-gray-800 dark:text-slate-200">{new Date(event.timestamp).toLocaleString()}</div>
        </div>
        {Object.keys(params).length > 0 && (
          <div>
            <div className="text-[10px] text-gray-400 uppercase tracking-wider mb-1">Parameters</div>
            <div className="space-y-1">
              {Object.entries(params).map(([k, v]) => (
                <div key={k} className="text-[10px] font-mono">
                  <span className="text-gray-400">{k}</span>
                  <div className="text-gray-700 dark:text-slate-300 break-all ml-2">{String(v)}</div>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// ═══════════════════════════════════════════════════
// Main
// ═══════════════════════════════════════════════════

function FitOnLoad({ trigger }: { trigger: number }) {
  const { fitView } = useReactFlow()
  useEffect(() => { setTimeout(() => fitView({ padding: 0.12, duration: 400 }), 200) }, [fitView, trigger])
  return null
}

function Inner({ events, focusPids, onLoadContext, loadingPid, detectionEvents }: Props) {
  const [expandedPids, setExpandedPids] = useState<Set<number>>(new Set())
  const [expandedCats, setExpandedCats] = useState<Set<string>>(new Set())
  const [loadedAncestors, setLoadedAncestors] = useState<Set<number>>(new Set())
  const [layoutTrigger, setLayoutTrigger] = useState(0)
  const [laidOut, setLaidOut] = useState<{ nodes: Node[]; edges: Edge[] }>({ nodes: [], edges: [] })
  const [selectedEvt, setSelectedEvt] = useState<TelemetryEvent | null>(null)

  const togglePid = useCallback((pid: number) => {
    setExpandedPids(prev => { const n = new Set(prev); if (n.has(pid)) n.delete(pid); else n.add(pid); return n })
    setLayoutTrigger(t => t + 1)
  }, [])

  const toggleCat = useCallback((catId: string) => {
    setExpandedCats(prev => { const n = new Set(prev); if (n.has(catId)) n.delete(catId); else n.add(catId); return n })
    setLayoutTrigger(t => t + 1)
  }, [])

  // Build raw nodes/edges (unpositioned)
  const catEvtStore = useMemo(() => new Map<string, TelemetryEvent[]>(), [])

  const { rawNodes, rawEdges, evtMap } = useMemo(() => {
    catEvtStore.clear()
    const allNodes: Node[] = []
    const allEdges: Edge[] = []
    const evtMap = new Map<string, TelemetryEvent>()
    if (!events.length) return { rawNodes: allNodes, rawEdges: allEdges, evtMap }

    const byPid = new Map<number, TelemetryEvent[]>()
    for (const e of events) { if (e.pid > 0) { const l = byPid.get(e.pid) || []; l.push(e); byPid.set(e.pid, l) } }

    const parentOf = new Map<number, number>()
    for (const [pid, evts] of byPid) {
      const ppid = evts[0].parent_pid
      if (ppid > 0 && ppid !== pid && byPid.has(ppid)) parentOf.set(pid, ppid)
    }

    const onPath = new Set<number>()
    for (const pid of Object.keys(focusPids).map(Number)) {
      onPath.add(pid); let cur = pid
      for (let i = 0; i < 30; i++) { const p = parentOf.get(cur); if (!p) break; onPath.add(p); cur = p }
    }

    for (const [pid, evts] of byPid) {
      const main = evts.find(e => e.event_name === 'CreateProcess') || evts[0]
      const isTrigger = !!focusPids[pid]
      const procId = `p-${pid}`
      // Look up enrichment from detection events for this PID
      const detEvt = (detectionEvents || []).find(e => (e.proc as Record<string, unknown>)?.pid === pid)?.proc as Record<string, unknown> | undefined
      const canLP = main.parent_pid > 0 && !byPid.has(main.parent_pid) && !!onLoadContext && !loadedAncestors.has(pid)
      const pidExp = expandedPids.has(pid)

      const catCounts = new Map<string, number>()
      for (const e of evts) { if (e.event_category !== 'process') catCounts.set(e.event_category, (catCounts.get(e.event_category) || 0) + 1) }
      const catSummary = Array.from(catCounts.entries()).map(([c, n]) => `${c}(${n})`).join(' ')

      allNodes.push({ id: procId, type: 'processNode', position: { x: 0, y: 0 }, data: {
        eventName: main.event_name || 'NEW_PROCESS', exe: main.process_exe, cmdline: main.process_cmdline,
        pid, ppid: main.parent_pid, parentName: main.parent_name, timestamp: main.timestamp,
        isTrigger, isOnPath: onPath.has(pid), canLoadParent: canLP, loading: loadingPid === pid,
        onLoadParent: canLP ? () => { setLoadedAncestors(prev => new Set(prev).add(pid)); onLoadContext!(pid) } : undefined, catSummary, expanded: pidExp,
        // Enrichment from detection events
        md5: detEvt?.md5 || '', sha256: detEvt?.sha256 || '',
        isSigned: detEvt?.is_signed, certSubject: detEvt?.cert_subject || '',
        sid: detEvt?.sid || '', username: detEvt?.username || '',
      }})
      evtMap.set(procId, main)

      if (parentOf.has(pid)) {
        const isP = onPath.has(pid)
        allEdges.push({ id: `e-${parentOf.get(pid)}-${pid}`, source: `p-${parentOf.get(pid)}`, target: procId,
          type: 'smoothstep', style: { stroke: isP ? '#ef4444' : '#d1d5db', strokeWidth: isP ? 2.5 : 1.5 }, animated: isTrigger })
      }

      if (pidExp) {
        for (const [c, count] of catCounts) {
          const catId = `cat-${pid}-${c}`
          const catExp = expandedCats.has(catId)
          const catEvts = evts.filter(e => e.event_category === c)
          const preview = catEvts.slice(0, 3).map(e => evtPreview(e) || e.event_name).filter(Boolean)
          // Embed event data in the category node — no separate event graph nodes
          const slicedEvts = catEvts.slice(0, 200)
          const embeddedEvents = catExp ? slicedEvts.map((e, i) => ({
            eventName: e.event_name, detail: evtPreview(e), timestamp: e.timestamp, idx: i,
          })) : []
          // Store sliced events for detail panel lookup
          catEvtStore.set(catId, slicedEvts)
          allNodes.push({ id: catId, type: 'categoryNode', position: { x: 0, y: 0 },
            data: { category: c, count, expanded: catExp, preview, events: embeddedEvents, nodeId: catId,
              onSelectEvent: (nid: string, idx: number) => {
                const evts = catEvtStore.get(nid)
                if (evts?.[idx]) setSelectedEvt(evts[idx])
              },
            } })
          allEdges.push({ id: `e-${procId}-${catId}`, source: procId, target: catId,
            type: 'smoothstep', style: { stroke: cc(c).edge, strokeWidth: 1.5 } })
          // Map all events for detail panel
          for (const evt of catEvts) evtMap.set(`${catId}-${evt.id}`, evt)
        }
      }
    }
    return { rawNodes: allNodes, rawEdges: allEdges, evtMap }
  }, [events, focusPids, expandedPids, expandedCats, onLoadContext, loadingPid, detectionEvents, loadedAncestors, catEvtStore])

  // Run ELK layout async
  useEffect(() => {
    if (rawNodes.length === 0) { setLaidOut({ nodes: [], edges: [] }); return }
    doLayout(rawNodes, rawEdges).then(result => setLaidOut(result))
  }, [rawNodes, rawEdges])

  const onNodeClick = useCallback((_: unknown, node: Node) => {
    // Always show detail panel for any node that has an event in the map
    const evt = evtMap.get(node.id)
    if (evt) setSelectedEvt(prev => prev?.id === evt.id ? null : evt)

    // Also handle expand/collapse
    if (node.type === 'processNode') togglePid((node.data as Record<string, unknown>).pid as number)
    else if (node.type === 'categoryNode') toggleCat(node.id)
  }, [togglePid, toggleCat, evtMap])

  if (laidOut.nodes.length === 0 && events.length === 0) {
    return <div className="flex items-center justify-center h-64 text-sm text-gray-400">No process events found.</div>
  }

  return (
    <div className="h-full w-full flex" style={{ minHeight: 500 }}>
      <div className="flex-1 min-w-0">
        <ReactFlow
          nodes={laidOut.nodes} edges={laidOut.edges}
          nodeTypes={nodeTypes} onNodeClick={onNodeClick}
          fitView minZoom={0.05} maxZoom={2.5}
          proOptions={{ hideAttribution: true }}
          defaultEdgeOptions={{ type: 'smoothstep' }}
        >
          <Background color="transparent" gap={0} size={0} />
          <Controls position="bottom-left" showInteractive={false} />
          <MiniMap pannable zoomable position="bottom-right"
            style={{ border: '1px solid #e5e7eb', borderRadius: 8 }}
            nodeColor={(n) => {
              const d = n.data as Record<string, unknown>
              if (d.isTrigger) return '#ef4444'
              if (d.isOnPath) return '#3b82f6'
              return cc((d.category as string) || 'handle').accent
            }} />
          <FitOnLoad trigger={layoutTrigger} />
        </ReactFlow>
      </div>
      {selectedEvt && <DetailPanel event={selectedEvt} onClose={() => setSelectedEvt(null)} />}
    </div>
  )
}

export default function ProcessChain(props: Props) {
  return <ReactFlowProvider><Inner {...props} /></ReactFlowProvider>
}
