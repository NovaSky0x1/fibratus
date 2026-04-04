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
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'

// ── Types ─────────────────────────────────��──────────────

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

interface NodeData extends ProcessInfo {
  selected: boolean
  expanded: boolean
  hasChildren: boolean
  onToggle: (pid: number) => void
}

interface Props {
  detection: Detection
  focusPid?: number
  focusProcessName?: string
}

// ── Category colors ──────────────────────────────────────

const catColors: Record<string, string> = {
  process: '#3b82f6',
  net: '#10b981',
  file: '#f59e0b',
  registry: '#8b5cf6',
  image: '#6366f1',
  dns: '#14b8a6',
}

// ── Custom node ──────────────────────────��───────────────

function ProcessNode({ data }: { data: NodeData }) {
  const borderColor = data.isFocus ? '#ef4444' : data.isOnFocusPath ? '#3b82f6' : '#d1d5db'
  const ringClass = data.selected ? 'ring-2 ring-fibratus-200' : ''
  const bgColor = data.isFocus ? 'bg-red-50' : data.isOnFocusPath ? 'bg-blue-50/60' : 'bg-white'

  return (
    <div className={`rounded-lg border-2 px-3 py-2 shadow-sm ${bgColor} ${ringClass}`}
      style={{ borderColor, width: 260 }}>
      <Handle type="target" position={Position.Top} className="!bg-gray-300 !w-2 !h-2" />

      <div className="flex items-center gap-2">
        <div className={'flex h-6 w-6 items-center justify-center rounded text-[10px] font-bold flex-shrink-0 ' +
          (data.isFocus ? 'bg-red-100 text-red-700' : 'bg-blue-100 text-blue-700')}>
          {data.pid}
        </div>
        <span className={'font-medium text-sm truncate flex-1 ' + (data.isFocus ? 'text-red-700' : 'text-gray-900')}>
          {data.name}
        </span>
        {data.isFocus && (
          <span className="rounded bg-red-100 text-red-600 text-[9px] px-1 py-0.5 font-bold flex-shrink-0">TRIGGER</span>
        )}
      </div>

      {data.exe && (
        <div className="mt-1 text-[10px] text-gray-400 font-mono truncate">{data.exe}</div>
      )}

      {data.eventCount > 0 && (
        <div className="mt-1.5 flex items-center gap-1 flex-wrap">
          {Object.entries(data.eventsByCategory).map(([cat, count]) => (
            <span key={cat} className="rounded px-1 py-0.5 text-[9px] font-medium"
              style={{ backgroundColor: (catColors[cat] || '#6b7280') + '20', color: catColors[cat] || '#6b7280' }}>
              {cat} {count}
            </span>
          ))}
        </div>
      )}

      {/* Expand/collapse toggle */}
      {data.hasChildren && (
        <button
          className="mt-1.5 flex items-center gap-1 text-[10px] text-gray-500 hover:text-gray-700 w-full"
          onClick={(e) => { e.stopPropagation(); data.onToggle(data.pid) }}
        >
          <svg className={'w-3 h-3 transition-transform ' + (data.expanded ? 'rotate-90' : '')}
            fill="currentColor" viewBox="0 0 20 20">
            <path fillRule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clipRule="evenodd" />
          </svg>
          {data.expanded ? 'Collapse' : `Expand (${data.childPids.length})`}
        </button>
      )}

      <Handle type="source" position={Position.Bottom} className="!bg-gray-300 !w-2 !h-2" />
    </div>
  )
}

const nodeTypes = { process: ProcessNode }

// ── Layout constants ─────────────────────────��───────────

const NODE_W = 290     // width including margin
const LEVEL_H = 160    // vertical spacing between levels

// ── Main component ─────────────────────────────────���─────

export default function DetectionProcessGraph({ detection, focusPid, focusProcessName }: Props) {
  const [selectedPid, setSelectedPid] = useState<number | null>(null)
  const [expanded, setExpanded] = useState<Set<number>>(new Set())
  const [initialized, setInitialized] = useState(false)

  const { data: res, isLoading } = useQuery({
    queryKey: ['detection-process-tree', detection.id],
    queryFn: () => api.getDetectionProcessTree(detection.id),
  })

  const treeEvents = (res?.data?.events || []) as TelemetryEvent[]

  // Build the full process map (stable across expand/collapse changes)
  const { byPid, roots, childrenMap } = useMemo(() => {
    const byPid = new Map<number, ProcessInfo>()
    const childrenMap = new Map<number, number[]>()

    if (treeEvents.length === 0) return { byPid, roots: [] as number[], childrenMap }

    // First pass: build process nodes from events
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

    // Build parent-child map
    const hasParent = new Set<number>()
    for (const proc of byPid.values()) {
      if (proc.parentPid > 0 && proc.parentPid !== proc.pid && byPid.has(proc.parentPid)) {
        const kids = childrenMap.get(proc.parentPid) || []
        if (!kids.includes(proc.pid)) kids.push(proc.pid)
        childrenMap.set(proc.parentPid, kids)
        hasParent.add(proc.pid)
      }
    }

    // Set childPids on each node
    for (const proc of byPid.values()) {
      proc.childPids = childrenMap.get(proc.pid) || []
    }

    // Mark focus path
    let focusProc: ProcessInfo | undefined
    if (focusPid) {
      focusProc = byPid.get(focusPid)
      if (!focusProc) {
        for (const p of byPid.values()) {
          if (p.name === focusProcessName && p.pid === focusPid) { focusProc = p; break }
        }
      }
    }
    if (focusProc) {
      focusProc.isFocus = true
      focusProc.isOnFocusPath = true
      // Walk up ancestors
      let cur: ProcessInfo | undefined = focusProc
      for (let i = 0; i < 20 && cur; i++) {
        const parent = byPid.get(cur.parentPid)
        if (!parent || parent === cur) break
        parent.isOnFocusPath = true
        cur = parent
      }
      // Walk down descendants
      const markDown = (pid: number, depth: number) => {
        if (depth > 10) return
        for (const childPid of childrenMap.get(pid) || []) {
          const child = byPid.get(childPid)
          if (child) { child.isOnFocusPath = true; markDown(childPid, depth + 1) }
        }
      }
      markDown(focusProc.pid, 0)
    }

    const roots = Array.from(byPid.keys()).filter(pid => !hasParent.has(pid))
    return { byPid, roots, childrenMap }
  }, [treeEvents, focusPid, focusProcessName])

  // Auto-expand focus path on first load
  useEffect(() => {
    if (initialized || byPid.size === 0) return
    const toExpand = new Set<number>()
    for (const [pid, proc] of byPid) {
      if (proc.isOnFocusPath && proc.childPids.length > 0) {
        toExpand.add(pid)
      }
    }
    // Also expand roots
    for (const r of roots) toExpand.add(r)
    setExpanded(toExpand)
    setInitialized(true)
  }, [byPid, roots, initialized])

  const toggleExpand = useCallback((pid: number) => {
    setExpanded(prev => {
      const next = new Set(prev)
      if (next.has(pid)) next.delete(pid); else next.add(pid)
      return next
    })
  }, [])

  // Compute visible nodes/edges based on expand state
  const { nodes, edges, visibleCount } = useMemo(() => {
    if (byPid.size === 0) return { nodes: [], edges: [], visibleCount: 0 }

    // Determine visible PIDs: a PID is visible if all ancestors up to root are expanded
    const visible = new Set<number>()

    const addVisible = (pid: number) => {
      visible.add(pid)
      if (expanded.has(pid)) {
        for (const childPid of childrenMap.get(pid) || []) {
          addVisible(childPid)
        }
      }
    }
    for (const root of roots) addVisible(root)

    // Layout visible nodes
    const positions = new Map<number, { x: number; y: number }>()
    let xOffset = 0

    const layoutTree = (pid: number, depth: number, xStart: number): number => {
      if (!visible.has(pid)) return xStart

      const kids = (childrenMap.get(pid) || []).filter(k => visible.has(k))
      if (kids.length === 0 || !expanded.has(pid)) {
        positions.set(pid, { x: xStart, y: depth * LEVEL_H })
        return xStart + NODE_W
      }
      let x = xStart
      for (const kid of kids) { x = layoutTree(kid, depth + 1, x) }
      const center = (xStart + x - NODE_W) / 2
      positions.set(pid, { x: center, y: depth * LEVEL_H })
      return x
    }

    for (const root of roots) {
      if (visible.has(root)) {
        xOffset = layoutTree(root, 0, xOffset)
        xOffset += NODE_W / 2
      }
    }

    // Build ReactFlow nodes
    const nodes: Node[] = []
    for (const pid of visible) {
      const info = byPid.get(pid)
      if (!info) continue
      nodes.push({
        id: String(pid),
        type: 'process',
        position: positions.get(pid) || { x: 0, y: 0 },
        data: {
          ...info,
          selected: pid === selectedPid,
          expanded: expanded.has(pid),
          hasChildren: info.childPids.length > 0,
          onToggle: toggleExpand,
        },
      })
    }

    // Build edges (only between visible nodes)
    const edges: Edge[] = []
    for (const pid of visible) {
      if (!expanded.has(pid)) continue
      for (const childPid of childrenMap.get(pid) || []) {
        if (!visible.has(childPid)) continue
        const parentProc = byPid.get(pid)
        const childProc = byPid.get(childPid)
        const onFocus = !!(parentProc?.isOnFocusPath && childProc?.isOnFocusPath)
        edges.push({
          id: `${pid}-${childPid}`,
          source: String(pid),
          target: String(childPid),
          type: 'smoothstep',
          animated: onFocus,
          style: {
            stroke: onFocus ? '#3b82f6' : '#94a3b8',
            strokeWidth: onFocus ? 2.5 : 1.5,
          },
        })
      }
    }

    return { nodes, edges, visibleCount: visible.size }
  }, [byPid, roots, childrenMap, expanded, selectedPid, toggleExpand])

  const selectedProcess = selectedPid ? byPid.get(selectedPid) : null

  const onNodeClick = useCallback((_: unknown, node: Node) => {
    setSelectedPid(Number(node.id))
  }, [])

  const expandAll = useCallback(() => {
    const all = new Set<number>()
    for (const [pid, proc] of byPid) {
      if (proc.childPids.length > 0) all.add(pid)
    }
    setExpanded(all)
  }, [byPid])

  const collapseAll = useCallback(() => {
    // Keep only roots expanded
    setExpanded(new Set(roots))
  }, [roots])

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-gray-400">Loading process tree...</div>
  }

  if (byPid.size === 0) {
    return (
      <div className="flex items-center justify-center h-64 text-sm text-gray-400">
        No process events found within the detection time window.
      </div>
    )
  }

  return (
    <div className="flex flex-col" style={{ height: 'calc(100vh - 200px)' }}>
      {/* Context bar */}
      <div className="flex items-center gap-3 px-4 py-2.5 border-b bg-gray-50/50 flex-shrink-0">
        <SeverityBadge severity={detection.severity} />
        <span className="font-medium text-sm text-gray-900 truncate">{detection.title}</span>
        <span className="text-xs text-gray-400">{detection.agent_hostname}</span>
        <span className="text-xs text-gray-400">{new Date(detection.timestamp).toLocaleString()}</span>
        {detection.labels?.['technique.id'] && (
          <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-mono text-gray-600">
            {detection.labels['technique.id']}
          </span>
        )}
        <div className="ml-auto flex items-center gap-2">
          <span className="text-xs text-gray-400">{visibleCount}/{byPid.size} processes</span>
          <button onClick={expandAll} className="text-[10px] text-fibratus-600 hover:underline">Expand all</button>
          <button onClick={collapseAll} className="text-[10px] text-gray-500 hover:underline">Collapse</button>
        </div>
      </div>

      <div className="flex flex-1 min-h-0">
        {/* ReactFlow graph */}
        <div className="flex-1 bg-gray-50">
          <ReactFlow
            key={`${expanded.size}-${visibleCount}`}
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            onNodeClick={onNodeClick}
            fitView
            fitViewOptions={{ padding: 0.4 }}
            minZoom={0.05}
            maxZoom={2}
            defaultEdgeOptions={{ animated: false }}
          >
            <Background gap={20} size={1} />
            <Controls showInteractive={false} />
            <MiniMap
              nodeStrokeWidth={3}
              nodeColor={(n) => {
                const d = n.data as unknown as ProcessInfo
                if (d?.isFocus) return '#ef4444'
                if (d?.isOnFocusPath) return '#3b82f6'
                return '#e5e7eb'
              }}
              pannable
              zoomable
            />
          </ReactFlow>
        </div>

        {/* Detail pane */}
        <div className="w-72 border-l overflow-auto bg-white flex-shrink-0">
          {selectedProcess ? (
            <div className="p-4 space-y-3">
              <div>
                <div className="flex items-center gap-2">
                  <h4 className="font-medium text-gray-900">{selectedProcess.name}</h4>
                  {selectedProcess.isFocus && <span className="rounded bg-red-100 text-red-700 text-[10px] px-1.5 py-0.5 font-medium">Trigger</span>}
                </div>
                <p className="text-xs text-gray-500 mt-0.5">PID {selectedProcess.pid}</p>
              </div>
              {selectedProcess.exe && (
                <div>
                  <span className="text-xs text-gray-500 block">Executable</span>
                  <span className="font-mono text-xs text-gray-700 break-all">{selectedProcess.exe}</span>
                </div>
              )}
              {selectedProcess.cmdline && (
                <div>
                  <span className="text-xs text-gray-500 block">Command Line</span>
                  <div className="rounded bg-gray-900 px-2 py-1.5 text-[11px] text-gray-100 font-mono break-all whitespace-pre-wrap max-h-32 overflow-auto">
                    {selectedProcess.cmdline}
                  </div>
                </div>
              )}
              {selectedProcess.parentName && (
                <div>
                  <span className="text-xs text-gray-500 block">Parent</span>
                  <span className="text-xs text-gray-700">{selectedProcess.parentName} (PID {selectedProcess.parentPid})</span>
                </div>
              )}
              <div>
                <span className="text-xs text-gray-500 block">Events ({selectedProcess.eventCount})</span>
                <div className="mt-1 flex flex-wrap gap-1">
                  {Object.entries(selectedProcess.eventsByCategory).map(([cat, count]) => (
                    <span key={cat} className="rounded px-1.5 py-0.5 text-[11px] font-medium"
                      style={{ backgroundColor: (catColors[cat] || '#6b7280') + '20', color: catColors[cat] || '#6b7280' }}>
                      {cat}: {count}
                    </span>
                  ))}
                </div>
              </div>
              {selectedProcess.childPids.length > 0 && (
                <div>
                  <span className="text-xs text-gray-500 block">Children ({selectedProcess.childPids.length})</span>
                  <div className="mt-1 flex flex-wrap gap-1">
                    {selectedProcess.childPids.map(cpid => {
                      const child = byPid.get(cpid)
                      return child ? (
                        <button key={cpid} onClick={() => { setSelectedPid(cpid); if (!expanded.has(selectedProcess.pid)) toggleExpand(selectedProcess.pid) }}
                          className="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-700 hover:bg-gray-200">
                          {child.name} ({cpid})
                        </button>
                      ) : null
                    })}
                  </div>
                </div>
              )}
            </div>
          ) : (
            <div className="p-4 text-sm text-gray-400 text-center mt-20">
              Click a process node to view details
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
