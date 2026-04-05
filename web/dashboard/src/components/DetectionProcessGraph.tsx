import { useState, useMemo, useCallback, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type Detection } from '../lib/api'
import SeverityBadge from './SeverityBadge'
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  type Node,
  type Edge,
  Position,
  Handle,
  useReactFlow,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'

// ── Types ────────────────────────────────────────────────

interface TelemetryEvent {
  id: number; timestamp: string; event_name: string; event_category: string
  pid: number; tid: number; process_name: string; process_exe: string
  process_cmdline: string; parent_pid: number; parent_name: string
  params: Record<string, unknown>
}

interface ProcessInfo {
  pid: number
  name: string
  exe: string
  cmdline: string
  parentPid: number
  parentName: string
  firstSeen: string
  eventCount: number
  eventsByCategory: Record<string, number>
  childPids: number[]
  isFocus: boolean
  isOnFocusPath: boolean
}

interface Props {
  detection: Detection
  focusPid?: number
  focusProcessName?: string
}

// ── Colors / labels ──────────────────────────────────────

const catConfig: Record<string, { color: string; label: string }> = {
  process: { color: '#3b82f6', label: 'Process' },
  net:     { color: '#10b981', label: 'Network' },
  file:    { color: '#f59e0b', label: 'Files' },
  registry:{ color: '#8b5cf6', label: 'Registry' },
  image:   { color: '#6366f1', label: 'Modules' },
  dns:     { color: '#14b8a6', label: 'DNS' },
}

function catColor(cat: string) { return catConfig[cat]?.color || '#6b7280' }
function catLabel(cat: string) { return catConfig[cat]?.label || cat }

// ── Process node ─────────────────────────────────────────

function ProcessNode({ data }: { data: Record<string, unknown> }) {
  const d = data as unknown as ProcessInfo & {
    selected: boolean; expanded: boolean; hasChildren: boolean
    eventsExpanded: boolean
    onToggleChildren: (pid: number) => void
    onToggleEvents: (pid: number) => void
  }
  const border = d.isFocus ? '#ef4444' : d.isOnFocusPath ? '#3b82f6' : '#d1d5db'
  const bg = d.isFocus ? 'bg-red-50 dark:bg-red-950/40' : d.isOnFocusPath ? 'bg-blue-50/60 dark:bg-blue-950/40' : 'bg-white dark:bg-slate-800'

  return (
    <div className={`rounded-lg border-2 px-3 py-2 shadow-sm ${bg} ${d.selected ? 'ring-2 ring-fibratus-200' : ''}`}
      style={{ borderColor: border, width: 270 }}>
      <Handle type="target" position={Position.Top} className="!bg-gray-300 !w-2 !h-2" />
      <div className="flex items-center gap-2">
        <div className={'flex h-6 w-6 items-center justify-center rounded text-[10px] font-bold flex-shrink-0 ' +
          (d.isFocus ? 'bg-red-100 text-red-700' : 'bg-blue-100 text-blue-700')}>{d.pid}</div>
        <span className={'font-medium text-sm truncate flex-1 ' + (d.isFocus ? 'text-red-700 dark:text-red-400' : 'text-gray-900 dark:text-slate-100')}>{d.name}</span>
        {d.isFocus && <span className="rounded bg-red-100 text-red-600 text-[9px] px-1 py-0.5 font-bold flex-shrink-0">TRIGGER</span>}
      </div>
      {d.exe && <div className="mt-1 text-[10px] text-gray-400 dark:text-slate-500 font-mono truncate">{d.exe}</div>}

      <div className="mt-1.5 flex items-center gap-2">
        {/* Show events toggle */}
        {d.eventCount > 0 && (
          <button className="flex items-center gap-1 text-[10px] text-fibratus-600 hover:text-fibratus-800"
            onClick={(e) => { e.stopPropagation(); d.onToggleEvents(d.pid) }}>
            <svg className={'w-3 h-3 transition-transform ' + (d.eventsExpanded ? 'rotate-90' : '')}
              fill="currentColor" viewBox="0 0 20 20">
              <path fillRule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clipRule="evenodd" />
            </svg>
            {d.eventsExpanded ? 'Hide events' : `${d.eventCount} events`}
          </button>
        )}
        {/* Show children toggle */}
        {d.hasChildren && (
          <button className="flex items-center gap-1 text-[10px] text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300 ml-auto"
            onClick={(e) => { e.stopPropagation(); d.onToggleChildren(d.pid) }}>
            <svg className={'w-3 h-3 transition-transform ' + (d.expanded ? 'rotate-90' : '')}
              fill="currentColor" viewBox="0 0 20 20">
              <path fillRule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clipRule="evenodd" />
            </svg>
            {d.childPids.length} children
          </button>
        )}
      </div>
      <Handle type="source" position={Position.Bottom} className="!bg-gray-300 !w-2 !h-2" />
    </div>
  )
}

// ── Event group node ─────────────────────────────────────

function EventGroupNode({ data }: { data: Record<string, unknown> }) {
  const d = data as unknown as {
    category: string; count: number; topItems: string[]
    selected: boolean; parentPid: number
  }
  const color = catColor(d.category)

  return (
    <div className={'rounded-md border px-2.5 py-1.5 shadow-sm bg-white dark:bg-slate-800 ' + (d.selected ? 'ring-2 ring-fibratus-200' : '')}
      style={{ borderColor: color + '60', width: 240 }}>
      <Handle type="target" position={Position.Top} style={{ background: color }} className="!w-2 !h-2" />
      <div className="flex items-center gap-2">
        <span className="rounded px-1.5 py-0.5 text-[10px] font-bold"
          style={{ backgroundColor: color + '20', color }}>{catLabel(d.category)}</span>
        <span className="text-[10px] text-gray-500 dark:text-slate-400 font-medium">{d.count} event{d.count > 1 ? 's' : ''}</span>
      </div>
      {d.topItems.length > 0 && (
        <div className="mt-1 space-y-0.5">
          {d.topItems.map((item, i) => (
            <div key={i} className="text-[10px] text-gray-600 dark:text-slate-400 font-mono truncate">{item}</div>
          ))}
          {d.count > d.topItems.length && (
            <div className="text-[9px] text-gray-400 dark:text-slate-500">+{d.count - d.topItems.length} more</div>
          )}
        </div>
      )}
    </div>
  )
}

const nodeTypes = { process: ProcessNode, eventGroup: EventGroupNode }

// ── Layout ───────────────────────────────────────────────

const PROC_W = 300
const PROC_H = 170
const EVT_W = 260
const EVT_H = 90

// ── Event summary ────────────────────────────────────────

function evtSummary(evt: TelemetryEvent): string {
  const p = evt.params || {}
  switch (evt.event_category) {
    case 'image': return (p.file_name || p.image_name || '') as string
    case 'net': return [p.dip, p.dport].filter(Boolean).join(':') || ''
    case 'dns': return (p.name || p.domain || '') as string
    case 'file': return (p.file_name || p.file_object || '') as string
    case 'registry': return (p.key_name || p.key || '') as string
    default: return ''
  }
}

// ── FitView helper ───────────────────────────────────────

function FitOnChange({ trigger }: { trigger: number }) {
  const { fitView } = useReactFlow()
  useEffect(() => { setTimeout(() => fitView({ padding: 0.3, duration: 300 }), 50) }, [trigger, fitView])
  return null
}

// ── Main component ───────────────────────────────────────

export default function DetectionProcessGraph({ detection, focusPid }: Props) {
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  const [childrenExpanded, setChildrenExpanded] = useState<Set<number>>(new Set())
  const [eventsExpanded, setEventsExpanded] = useState<Set<number>>(new Set())
  const [initialized, setInitialized] = useState(false)
  const [detailTab, setDetailTab] = useState<string>('all')
  const [extraEvents, setExtraEvents] = useState<TelemetryEvent[]>([])
  const [loadingPid, setLoadingPid] = useState<number | null>(null)

  const { data: res, isLoading } = useQuery({
    queryKey: ['detection-process-tree', detection.id],
    queryFn: () => api.getDetectionProcessTree(detection.id),
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
  })

  const baseEvents = (res?.data?.events || []) as TelemetryEvent[]

  // Merge base + extra loaded events (deduplicated by event id)
  const treeEvents = useMemo(() => {
    if (extraEvents.length === 0) return baseEvents
    const seen = new Set<number>()
    const merged: TelemetryEvent[] = []
    for (const evt of [...baseEvents, ...extraEvents]) {
      if (!seen.has(evt.id)) { seen.add(evt.id); merged.push(evt) }
    }
    return merged
  }, [baseEvents, extraEvents])

  // Live-load context for a PID (parent + children)
  const loadContext = useCallback(async (pid: number) => {
    setLoadingPid(pid)
    try {
      const resp = await api.getDetectionProcessContext(detection.id, pid)
      if (resp.data?.events) {
        setExtraEvents(prev => [...prev, ...(resp.data!.events as TelemetryEvent[])])
        setChildrenExpanded(prev => { const n = new Set(prev); n.add(pid); return n })
      }
    } finally {
      setLoadingPid(null)
    }
  }, [detection.id])

  // Index events by PID
  const eventsByPid = useMemo(() => {
    const map = new Map<number, TelemetryEvent[]>()
    for (const evt of treeEvents) {
      const list = map.get(evt.pid) || []
      list.push(evt)
      map.set(evt.pid, list)
    }
    return map
  }, [treeEvents])

  // Build process map
  const { byPid, roots, childrenMap } = useMemo(() => {
    const byPid = new Map<number, ProcessInfo>()
    const childrenMap = new Map<number, number[]>()
    if (treeEvents.length === 0) return { byPid, roots: [] as number[], childrenMap }

    for (const evt of treeEvents) {
      if (evt.pid <= 0) continue
      let proc = byPid.get(evt.pid)
      if (!proc) {
        proc = {
          pid: evt.pid, name: evt.process_name, exe: evt.process_exe || '',
          cmdline: evt.process_cmdline || '', parentPid: evt.parent_pid,
          parentName: evt.parent_name || '', firstSeen: evt.timestamp,
          eventCount: 0, eventsByCategory: {}, childPids: [],
          isFocus: false, isOnFocusPath: false,
        }
        byPid.set(evt.pid, proc)
      }
      proc.eventCount++
      proc.eventsByCategory[evt.event_category] = (proc.eventsByCategory[evt.event_category] || 0) + 1
      if (!proc.cmdline && evt.process_cmdline) proc.cmdline = evt.process_cmdline
      if (!proc.exe && evt.process_exe) proc.exe = evt.process_exe
    }

    // Parent-child — only if parent exists
    const hasParent = new Set<number>()
    for (const proc of byPid.values()) {
      if (proc.parentPid > 0 && proc.parentPid !== proc.pid && byPid.has(proc.parentPid)) {
        const kids = childrenMap.get(proc.parentPid) || []
        if (!kids.includes(proc.pid)) kids.push(proc.pid)
        childrenMap.set(proc.parentPid, kids)
        hasParent.add(proc.pid)
      }
    }
    for (const proc of byPid.values()) proc.childPids = childrenMap.get(proc.pid) || []

    // Mark focus path
    let focusProc: ProcessInfo | undefined
    if (focusPid) {
      focusProc = byPid.get(focusPid)
      if (!focusProc) {
        for (const p of byPid.values()) { if (p.pid === focusPid) { focusProc = p; break } }
      }
    }
    if (focusProc) {
      focusProc.isFocus = true
      focusProc.isOnFocusPath = true
      let cur: ProcessInfo | undefined = focusProc
      for (let i = 0; i < 20 && cur; i++) {
        const parent = byPid.get(cur.parentPid)
        if (!parent || parent === cur) break
        parent.isOnFocusPath = true
        cur = parent
      }
      const markDown = (pid: number, depth: number) => {
        if (depth > 10) return
        for (const cpid of childrenMap.get(pid) || []) {
          const c = byPid.get(cpid)
          if (c) { c.isOnFocusPath = true; markDown(cpid, depth + 1) }
        }
      }
      markDown(focusProc.pid, 0)
    }

    const roots = Array.from(byPid.keys()).filter(pid => !hasParent.has(pid))
    return { byPid, roots, childrenMap }
  }, [treeEvents, focusPid])

  // Auto-expand focus path
  useEffect(() => {
    if (initialized || byPid.size === 0) return
    const ce = new Set<number>()
    const ee = new Set<number>()
    for (const [pid, proc] of byPid) {
      if (proc.isOnFocusPath && proc.childPids.length > 0) ce.add(pid)
      if (proc.isFocus) ee.add(pid) // auto-show events on trigger process
    }
    for (const r of roots) ce.add(r)
    setChildrenExpanded(ce)
    setEventsExpanded(ee)
    setInitialized(true)
  }, [byPid, roots, initialized])

  const toggleChildren = useCallback((pid: number) => {
    setChildrenExpanded(prev => { const n = new Set(prev); if (n.has(pid)) n.delete(pid); else n.add(pid); return n })
  }, [])

  const toggleEvents = useCallback((pid: number) => {
    setEventsExpanded(prev => { const n = new Set(prev); if (n.has(pid)) n.delete(pid); else n.add(pid); return n })
  }, [])

  // Build visible nodes and edges
  const { nodes, edges, visibleCount } = useMemo(() => {
    if (byPid.size === 0) return { nodes: [], edges: [], visibleCount: 0 }

    // Determine visible process PIDs
    const visible = new Set<number>()
    const addVisible = (pid: number) => {
      visible.add(pid)
      if (childrenExpanded.has(pid)) {
        for (const cpid of childrenMap.get(pid) || []) addVisible(cpid)
      }
    }
    for (const root of roots) addVisible(root)

    // Layout: two-pass — processes first, then event groups below them
    const positions = new Map<string, { x: number; y: number }>() // node id -> position
    let xOffset = 0

    // Count event group rows for a PID (adds vertical space below the process)
    const evtGroupHeight = (pid: number): number => {
      if (!eventsExpanded.has(pid)) return 0
      const proc = byPid.get(pid)
      if (!proc) return 0
      const cats = Object.keys(proc.eventsByCategory)
      if (cats.length === 0) return 0
      return EVT_H + 30 // space for one row of event groups
    }

    const layoutTree = (pid: number, depth: number, xStart: number, yBase: number): number => {
      if (!visible.has(pid)) return xStart
      const nodeId = `p-${pid}`
      const extraY = evtGroupHeight(pid)

      const kids = (childrenMap.get(pid) || []).filter(k => visible.has(k))
      if (kids.length === 0 || !childrenExpanded.has(pid)) {
        positions.set(nodeId, { x: xStart, y: yBase })
        // Layout event groups horizontally below this process
        if (eventsExpanded.has(pid)) layoutEventGroups(pid, xStart, yBase + PROC_H - 40)
        return xStart + PROC_W
      }
      const childY = yBase + PROC_H + extraY
      let x = xStart
      for (const kid of kids) x = layoutTree(kid, depth + 1, x, childY)
      const center = (xStart + x - PROC_W) / 2
      positions.set(nodeId, { x: center, y: yBase })
      if (eventsExpanded.has(pid)) layoutEventGroups(pid, center, yBase + PROC_H - 40)
      return x
    }

    const layoutEventGroups = (pid: number, xCenter: number, y: number) => {
      const proc = byPid.get(pid)
      if (!proc) return
      const cats = Object.keys(proc.eventsByCategory)
      const totalWidth = cats.length * EVT_W
      let x = xCenter + (PROC_W - totalWidth) / 2
      for (const cat of cats) {
        positions.set(`e-${pid}-${cat}`, { x, y })
        x += EVT_W
      }
    }

    for (const root of roots) {
      if (visible.has(root)) {
        xOffset = layoutTree(root, 0, xOffset, 0)
        xOffset += PROC_W / 3
      }
    }

    // Build nodes
    const allNodes: Node[] = []

    for (const pid of visible) {
      const info = byPid.get(pid)
      if (!info) continue
      const pos = positions.get(`p-${pid}`)
      allNodes.push({
        id: `p-${pid}`,
        type: 'process',
        position: pos || { x: 0, y: 0 },
        data: {
          ...info,
          selected: selectedNodeId === `p-${pid}`,
          expanded: childrenExpanded.has(pid),
          eventsExpanded: eventsExpanded.has(pid),
          hasChildren: info.childPids.length > 0,
          onToggleChildren: toggleChildren,
          onToggleEvents: toggleEvents,
        } as unknown as Record<string, unknown>,
      })

      // Event group nodes
      if (eventsExpanded.has(pid)) {
        const cats = Object.keys(info.eventsByCategory)
        for (const cat of cats) {
          const evts = (eventsByPid.get(pid) || []).filter(e => e.event_category === cat)
          const topItems = evts.slice(0, 3).map(e => evtSummary(e)).filter(Boolean)
          const pos = positions.get(`e-${pid}-${cat}`)
          allNodes.push({
            id: `e-${pid}-${cat}`,
            type: 'eventGroup',
            position: pos || { x: 0, y: 0 },
            data: {
              category: cat, count: info.eventsByCategory[cat],
              topItems, selected: selectedNodeId === `e-${pid}-${cat}`,
              parentPid: pid,
            } as unknown as Record<string, unknown>,
          })
        }
      }
    }

    // Build edges
    const allEdges: Edge[] = []

    // Process → child process edges (glowing blue for focus path)
    for (const pid of visible) {
      if (!childrenExpanded.has(pid)) continue
      for (const cpid of childrenMap.get(pid) || []) {
        if (!visible.has(cpid)) continue
        const pp = byPid.get(pid)
        const cp = byPid.get(cpid)
        const onFocus = !!(pp?.isOnFocusPath && cp?.isOnFocusPath)
        allEdges.push({
          id: `p-${pid}->p-${cpid}`,
          source: `p-${pid}`, target: `p-${cpid}`,
          type: 'smoothstep', animated: onFocus,
          style: {
            stroke: onFocus ? '#3b82f6' : '#94a3b8',
            strokeWidth: onFocus ? 3 : 1.5,
            filter: onFocus ? 'drop-shadow(0 0 6px rgba(59,130,246,0.7)) drop-shadow(0 0 12px rgba(59,130,246,0.4))' : undefined,
          },
        })
      }
    }

    // Process → event group edges
    for (const pid of visible) {
      if (!eventsExpanded.has(pid)) continue
      const info = byPid.get(pid)
      if (!info) continue
      for (const cat of Object.keys(info.eventsByCategory)) {
        allEdges.push({
          id: `p-${pid}->e-${pid}-${cat}`,
          source: `p-${pid}`, target: `e-${pid}-${cat}`,
          type: 'smoothstep',
          style: { stroke: catColor(cat) + '80', strokeWidth: 1.5, strokeDasharray: '4 3' },
        })
      }
    }

    return { nodes: allNodes, edges: allEdges, visibleCount: visible.size }
  }, [byPid, roots, childrenMap, childrenExpanded, eventsExpanded, selectedNodeId, toggleChildren, toggleEvents, eventsByPid])

  // Detail pane data
  const selectedProcess = selectedNodeId?.startsWith('p-') ? byPid.get(Number(selectedNodeId.slice(2))) : null
  const selectedEventGroup = selectedNodeId?.startsWith('e-') ? (() => {
    const parts = selectedNodeId.split('-')
    const pid = Number(parts[1])
    const cat = parts.slice(2).join('-')
    return { pid, cat, events: (eventsByPid.get(pid) || []).filter(e => e.event_category === cat) }
  })() : null

  const detailEvents = useMemo(() => {
    if (selectedEventGroup) return selectedEventGroup.events
    if (!selectedProcess) return []
    const all = eventsByPid.get(selectedProcess.pid) || []
    return detailTab === 'all' ? all : all.filter(e => e.event_category === detailTab)
  }, [selectedProcess, selectedEventGroup, eventsByPid, detailTab])

  const onNodeClick = useCallback((_: unknown, node: Node) => {
    setSelectedNodeId(node.id)
    setDetailTab('all')
  }, [])

  const expandAll = useCallback(() => {
    const ce = new Set<number>()
    for (const [pid, proc] of byPid) { if (proc.childPids.length > 0) ce.add(pid) }
    setChildrenExpanded(ce)
  }, [byPid])
  const collapseAll = useCallback(() => {
    setChildrenExpanded(new Set(roots))
    setEventsExpanded(new Set())
  }, [roots])

  if (isLoading) return <div className="flex items-center justify-center h-64 text-gray-400 dark:text-slate-500">Loading process tree...</div>
  if (byPid.size === 0) return <div className="flex items-center justify-center h-64 text-sm text-gray-400 dark:text-slate-500">No process events found.</div>

  return (
    <div className="flex flex-col" style={{ height: 'calc(100vh - 200px)' }}>
      {/* Context bar */}
      <div className="flex items-center gap-3 px-4 py-2.5 border-b border-gray-200 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50 flex-shrink-0">
        <SeverityBadge severity={detection.severity} />
        <span className="font-medium text-sm text-gray-900 dark:text-slate-100 truncate">{detection.title}</span>
        <span className="text-xs text-gray-400 dark:text-slate-500">{detection.agent_hostname}</span>
        {detection.labels?.['technique.id'] && (
          <span className="rounded bg-gray-100 dark:bg-slate-700 px-1.5 py-0.5 text-xs font-mono text-gray-600 dark:text-slate-400">{detection.labels['technique.id']}</span>
        )}
        <div className="ml-auto flex items-center gap-2">
          <span className="text-xs text-gray-400 dark:text-slate-500">{visibleCount}/{byPid.size} processes</span>
          <button onClick={expandAll} className="text-[10px] text-fibratus-600 hover:underline">Expand all</button>
          <button onClick={collapseAll} className="text-[10px] text-gray-500 dark:text-slate-400 hover:underline">Collapse</button>
        </div>
      </div>

      <div className="flex flex-1 min-h-0">
        <div className="flex-1 bg-gray-50 dark:bg-slate-900">
          <ReactFlow
            nodes={nodes} edges={edges} nodeTypes={nodeTypes}
            onNodeClick={onNodeClick}
            fitView fitViewOptions={{ padding: 0.3 }}
            minZoom={0.05} maxZoom={2}
          >
            <Background gap={20} size={1} />
            <Controls showInteractive={false} />
            <MiniMap nodeStrokeWidth={3} pannable zoomable
              nodeColor={(n) => {
                const d = n.data as unknown as ProcessInfo
                if (d?.isFocus) return '#ef4444'
                if (d?.isOnFocusPath) return '#3b82f6'
                return '#e5e7eb'
              }} />
            <FitOnChange trigger={nodes.length} />
          </ReactFlow>
        </div>

        {/* Detail pane */}
        <div className="w-80 border-l border-gray-200 dark:border-slate-700 overflow-auto bg-white dark:bg-slate-800 flex-shrink-0">
          {selectedProcess ? (
            <div className="flex flex-col h-full">
              <div className="p-4 border-b border-gray-200 dark:border-slate-700 space-y-2 flex-shrink-0">
                <div className="flex items-center gap-2">
                  <h4 className="font-medium text-gray-900 dark:text-slate-100">{selectedProcess.name}</h4>
                  {selectedProcess.isFocus && <span className="rounded bg-red-100 text-red-700 text-[10px] px-1.5 py-0.5 font-medium">Trigger</span>}
                </div>
                <p className="text-xs text-gray-500 dark:text-slate-400">PID {selectedProcess.pid}</p>
                {selectedProcess.exe && <p className="font-mono text-[11px] text-gray-600 dark:text-slate-400 break-all">{selectedProcess.exe}</p>}
                {selectedProcess.cmdline && (
                  <div className="rounded bg-gray-900 px-2 py-1.5 text-[11px] text-gray-100 font-mono break-all whitespace-pre-wrap max-h-20 overflow-auto">{selectedProcess.cmdline}</div>
                )}
                {/* Parent — navigate or load */}
                {selectedProcess.parentPid > 0 && (
                  byPid.has(selectedProcess.parentPid) ? (
                    <p className="text-xs text-gray-500 dark:text-slate-400">Parent: <button className="text-fibratus-600 hover:underline"
                      onClick={() => { setSelectedNodeId(`p-${selectedProcess.parentPid}`); setDetailTab('all') }}>
                      {selectedProcess.parentName} ({selectedProcess.parentPid})
                    </button></p>
                  ) : (
                    <button className="text-[11px] text-fibratus-600 hover:underline flex items-center gap-1"
                      disabled={loadingPid === selectedProcess.parentPid}
                      onClick={() => loadContext(selectedProcess.parentPid)}>
                      {loadingPid === selectedProcess.parentPid ? 'Loading...' : `Load parent: ${selectedProcess.parentName} (${selectedProcess.parentPid})`}
                    </button>
                  )
                )}
                {/* Children — navigate or load */}
                {selectedProcess.childPids.length > 0 ? (
                  <div className="flex flex-wrap gap-1">
                    <span className="text-[10px] text-gray-400 dark:text-slate-500">Children:</span>
                    {selectedProcess.childPids.map(cpid => {
                      const child = byPid.get(cpid)
                      return child ? (
                        <button key={cpid} className="rounded bg-gray-100 dark:bg-slate-700 px-1.5 py-0.5 text-[10px] text-gray-700 dark:text-slate-300 hover:bg-gray-200 dark:hover:bg-slate-600"
                          onClick={() => { setSelectedNodeId(`p-${cpid}`); setDetailTab('all'); if (!childrenExpanded.has(selectedProcess.pid)) toggleChildren(selectedProcess.pid) }}>
                          {child.name}
                        </button>
                      ) : null
                    })}
                  </div>
                ) : (
                  <button className="text-[11px] text-fibratus-600 hover:underline flex items-center gap-1"
                    disabled={loadingPid === selectedProcess.pid}
                    onClick={() => loadContext(selectedProcess.pid)}>
                    {loadingPid === selectedProcess.pid ? 'Loading...' : 'Load children'}
                  </button>
                )}
              </div>
              {/* Event tabs */}
              <div className="px-3 py-2 border-b border-gray-200 dark:border-slate-700 flex items-center gap-1 flex-shrink-0 flex-wrap">
                <button onClick={() => setDetailTab('all')}
                  className={'rounded-full px-2 py-0.5 text-[10px] font-medium border ' +
                    (detailTab === 'all' ? 'bg-gray-800 text-white border-gray-800' : 'bg-white dark:bg-slate-700 text-gray-500 dark:text-slate-400 border-gray-200 dark:border-slate-600')}>
                  All ({eventsByPid.get(selectedProcess.pid)?.length || 0})
                </button>
                {Object.entries(selectedProcess.eventsByCategory).map(([cat, count]) => (
                  <button key={cat} onClick={() => setDetailTab(detailTab === cat ? 'all' : cat)}
                    className={'rounded-full px-2 py-0.5 text-[10px] font-medium border ' +
                      (detailTab === cat ? 'bg-gray-800 text-white border-gray-800' : 'bg-white dark:bg-slate-700 text-gray-500 dark:text-slate-400 border-gray-200 dark:border-slate-600')}>
                    {catLabel(cat)} ({count})
                  </button>
                ))}
              </div>
              <div className="flex-1 overflow-auto">
                {detailEvents.map((evt, i) => <EventRow key={`${evt.id}-${i}`} evt={evt} />)}
                {detailEvents.length === 0 && <div className="text-xs text-gray-400 dark:text-slate-500 text-center py-8">No events</div>}
              </div>
            </div>
          ) : selectedEventGroup ? (
            <div className="flex flex-col h-full">
              <div className="p-4 border-b border-gray-200 dark:border-slate-700 flex-shrink-0">
                <div className="flex items-center gap-2">
                  <span className="rounded px-2 py-1 text-xs font-bold"
                    style={{ backgroundColor: catColor(selectedEventGroup.cat) + '20', color: catColor(selectedEventGroup.cat) }}>
                    {catLabel(selectedEventGroup.cat)}
                  </span>
                  <span className="text-xs text-gray-500 dark:text-slate-400">{selectedEventGroup.events.length} events</span>
                </div>
                <p className="text-xs text-gray-400 dark:text-slate-500 mt-1">
                  Process: <button className="text-fibratus-600 hover:underline"
                    onClick={() => setSelectedNodeId(`p-${selectedEventGroup.pid}`)}>
                    {byPid.get(selectedEventGroup.pid)?.name} ({selectedEventGroup.pid})
                  </button>
                </p>
              </div>
              <div className="flex-1 overflow-auto">
                {detailEvents.map((evt, i) => <EventRow key={`${evt.id}-${i}`} evt={evt} />)}
              </div>
            </div>
          ) : (
            <div className="p-4 text-sm text-gray-400 dark:text-slate-500 text-center mt-20">Click a node to view details</div>
          )}
        </div>
      </div>
    </div>
  )
}

// ── Event row ────────────────────────────────────────────

function EventRow({ evt }: { evt: TelemetryEvent }) {
  const [open, setOpen] = useState(false)
  const summary = evtSummary(evt)
  const color = catColor(evt.event_category)

  return (
    <div className="border-b border-gray-100 dark:border-slate-700">
      <div className="flex items-center gap-2 px-3 py-1.5 cursor-pointer hover:bg-gray-50/50 dark:hover:bg-slate-700/50 text-[12px]"
        onClick={() => setOpen(!open)}>
        <span className="text-gray-400 dark:text-slate-500 tabular-nums font-mono text-[10px] w-16 flex-shrink-0">
          {new Date(evt.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
        </span>
        <span className="rounded px-1.5 py-0.5 text-[10px] font-medium flex-shrink-0"
          style={{ backgroundColor: color + '20', color }}>{evt.event_name}</span>
        <span className="text-gray-600 dark:text-slate-400 truncate text-[11px] font-mono">{summary}</span>
      </div>
      {open && evt.params && Object.keys(evt.params).length > 0 && (
        <div className="px-3 pb-2 pt-0.5 bg-gray-50/50 dark:bg-slate-800/50 text-[11px]">
          {Object.entries(evt.params).map(([k, v]) => (
            <div key={k} className="flex gap-1.5 min-w-0">
              <span className="text-gray-400 dark:text-slate-500 flex-shrink-0">{k}:</span>
              <span className="font-mono text-gray-700 dark:text-slate-300 break-all">{typeof v === 'object' ? JSON.stringify(v) : String(v)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
