import { useState, useMemo, useCallback } from 'react'
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
  isFocus: boolean
  isOnFocusPath: boolean
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

// ── Custom node ──────────────────────────────────────────

function ProcessNode({ data }: { data: ProcessInfo & { selected: boolean } }) {
  const borderColor = data.isFocus ? '#ef4444' : data.isOnFocusPath ? '#3b82f6' : '#e5e7eb'
  const ringClass = data.selected ? 'ring-2 ring-fibratus-200' : ''
  const bgColor = data.isFocus ? 'bg-red-50' : data.isOnFocusPath ? 'bg-blue-50' : 'bg-white'

  return (
    <div className={`rounded-lg border-2 px-3 py-2 shadow-sm min-w-[200px] max-w-[280px] ${bgColor} ${ringClass}`}
      style={{ borderColor }}>
      <Handle type="target" position={Position.Top} className="!bg-gray-300 !w-2 !h-2" />
      <div className="flex items-center gap-2">
        <div className={'flex h-6 w-6 items-center justify-center rounded text-[10px] font-bold flex-shrink-0 ' +
          (data.isFocus ? 'bg-red-100 text-red-700' : 'bg-blue-100 text-blue-700')}>
          {data.pid}
        </div>
        <span className={'font-medium text-sm truncate ' + (data.isFocus ? 'text-red-700' : 'text-gray-900')}>
          {data.name}
        </span>
        {data.isFocus && (
          <span className="rounded bg-red-100 text-red-600 text-[9px] px-1 py-0.5 font-bold flex-shrink-0">TRIGGER</span>
        )}
      </div>
      {data.exe && (
        <div className="mt-1 text-[10px] text-gray-400 font-mono truncate">{data.exe}</div>
      )}
      {/* Event category breakdown */}
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
      <Handle type="source" position={Position.Bottom} className="!bg-gray-300 !w-2 !h-2" />
    </div>
  )
}

const nodeTypes = { process: ProcessNode }

// ── Main component ───────────────────────────────────────

export default function DetectionProcessGraph({ detection, focusPid, focusProcessName }: Props) {
  const [selectedPid, setSelectedPid] = useState<number | null>(null)

  const { data: res, isLoading } = useQuery({
    queryKey: ['detection-process-tree', detection.id],
    queryFn: () => api.getDetectionProcessTree(detection.id),
  })

  const treeEvents = (res?.data?.events || []) as TelemetryEvent[]

  const { nodes, edges, selectedProcess, processCount } = useMemo(() => {
    if (treeEvents.length === 0) return { nodes: [], edges: [], selectedProcess: null, processCount: 0 }

    // Build process map from events
    const byKey = new Map<string, ProcessInfo>()

    for (const evt of treeEvents) {
      const key = `${evt.pid}:${evt.process_name}`
      let proc = byKey.get(key)
      if (!proc) {
        proc = {
          pid: evt.pid, name: evt.process_name, exe: evt.process_exe || '',
          cmdline: evt.process_cmdline || '', parentPid: evt.parent_pid,
          parentName: evt.parent_name || '', firstSeen: evt.timestamp,
          eventCount: 0, eventsByCategory: {},
          isFocus: false, isOnFocusPath: false,
        }
        byKey.set(key, proc)
      }
      proc.eventCount++
      proc.eventsByCategory[evt.event_category] = (proc.eventsByCategory[evt.event_category] || 0) + 1
      if (!proc.cmdline && evt.process_cmdline) proc.cmdline = evt.process_cmdline
      if (!proc.exe && evt.process_exe) proc.exe = evt.process_exe
    }

    // Mark focus process
    let focusKey: string | null = null
    if (focusPid) {
      focusKey = `${focusPid}:${focusProcessName || ''}`
      let focusProc = byKey.get(focusKey)
      if (!focusProc) {
        // Fallback: match by PID alone
        for (const [k, p] of byKey) {
          if (p.pid === focusPid) { focusProc = p; focusKey = k; break }
        }
      }
      if (focusProc) {
        focusProc.isFocus = true
        focusProc.isOnFocusPath = true

        // Mark ancestors
        const markUp = (pid: number, name: string) => {
          const key = `${pid}:${name}`
          let parent = byKey.get(key)
          if (!parent) {
            for (const p of byKey.values()) {
              if (p.pid === pid) { parent = p; break }
            }
          }
          if (parent && !parent.isOnFocusPath) {
            parent.isOnFocusPath = true
            if (parent.parentPid > 0) markUp(parent.parentPid, parent.parentName)
          }
        }
        if (focusProc.parentPid > 0) markUp(focusProc.parentPid, focusProc.parentName)

        // Mark descendants
        const childrenOf = new Map<number, ProcessInfo[]>()
        for (const p of byKey.values()) {
          if (p.parentPid > 0) {
            const kids = childrenOf.get(p.parentPid) || []
            kids.push(p)
            childrenOf.set(p.parentPid, kids)
          }
        }
        const markDown = (pid: number) => {
          const kids = childrenOf.get(pid) || []
          for (const k of kids) { k.isOnFocusPath = true; markDown(k.pid) }
        }
        markDown(focusProc.pid)
      }
    }

    // Build parent-child relationships (deduplicated by PID)
    const byPid = new Map<number, ProcessInfo>()
    for (const p of byKey.values()) {
      const existing = byPid.get(p.pid)
      if (!existing || p.isFocus || (p.isOnFocusPath && !existing.isOnFocusPath)) {
        byPid.set(p.pid, p)
      }
    }

    const children = new Map<number, Set<number>>()
    const hasParent = new Set<number>()
    for (const proc of byPid.values()) {
      if (proc.parentPid > 0 && proc.parentPid !== proc.pid && byPid.has(proc.parentPid)) {
        if (!children.has(proc.parentPid)) children.set(proc.parentPid, new Set())
        children.get(proc.parentPid)!.add(proc.pid)
        hasParent.add(proc.pid)
      }
    }

    // Layout
    const roots = Array.from(byPid.keys()).filter(pid => !hasParent.has(pid))
    const positions = new Map<number, { x: number; y: number }>()
    const LEVEL_HEIGHT = 120
    const NODE_WIDTH = 240
    let xOffset = 0

    function layoutTree(pid: number, depth: number, xStart: number): number {
      const kids = Array.from(children.get(pid) || [])
      if (kids.length === 0) {
        positions.set(pid, { x: xStart, y: depth * LEVEL_HEIGHT })
        return xStart + NODE_WIDTH
      }
      let x = xStart
      for (const kid of kids) { x = layoutTree(kid, depth + 1, x) }
      const center = (xStart + x - NODE_WIDTH) / 2
      positions.set(pid, { x: center, y: depth * LEVEL_HEIGHT })
      return x
    }

    for (const root of roots) {
      xOffset = layoutTree(root, 0, xOffset)
      xOffset += NODE_WIDTH / 2
    }

    // Build ReactFlow nodes and edges
    const nodes: Node[] = Array.from(byPid.entries()).map(([pid, info]) => ({
      id: String(pid),
      type: 'process',
      position: positions.get(pid) || { x: 0, y: 0 },
      data: { ...info, selected: pid === selectedPid },
    }))

    const edgeSet = new Set<string>()
    const edges: Edge[] = []
    for (const [parentPid, kidSet] of children) {
      for (const kidPid of kidSet) {
        const key = `${parentPid}-${kidPid}`
        if (!edgeSet.has(key)) {
          edgeSet.add(key)
          const parentProc = byPid.get(parentPid)
          const kidProc = byPid.get(kidPid)
          const onFocus = parentProc?.isOnFocusPath && kidProc?.isOnFocusPath
          edges.push({
            id: key,
            source: String(parentPid),
            target: String(kidPid),
            type: 'smoothstep',
            animated: onFocus,
            style: {
              stroke: onFocus ? '#3b82f6' : '#94a3b8',
              strokeWidth: onFocus ? 2 : 1.5,
            },
          })
        }
      }
    }

    const selectedProcess = selectedPid ? byPid.get(selectedPid) : null
    return { nodes, edges, selectedProcess, processCount: byPid.size }
  }, [treeEvents, focusPid, focusProcessName, selectedPid])

  const onNodeClick = useCallback((_: unknown, node: Node) => {
    setSelectedPid(Number(node.id))
  }, [])

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-gray-400">Loading process tree...</div>
  }

  if (nodes.length === 0) {
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
        <span className="font-medium text-sm text-gray-900">{detection.title}</span>
        <span className="text-xs text-gray-400">{detection.agent_hostname}</span>
        <span className="text-xs text-gray-400">{new Date(detection.timestamp).toLocaleString()}</span>
        {detection.labels?.['technique.id'] && (
          <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-mono text-gray-600">
            {detection.labels['technique.id']}
          </span>
        )}
        <span className="ml-auto text-xs text-gray-400">{processCount} processes &middot; {treeEvents.length} events</span>
      </div>

      <div className="flex flex-1 min-h-0">
        {/* ReactFlow graph */}
        <div className="flex-1 bg-gray-50">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            onNodeClick={onNodeClick}
            fitView
            fitViewOptions={{ padding: 0.3 }}
            minZoom={0.1}
            maxZoom={2}
            defaultEdgeOptions={{ animated: false }}
          >
            <Background gap={20} size={1} />
            <Controls showInteractive={false} />
            <MiniMap
              nodeStrokeWidth={3}
              nodeColor={(n) => {
                const d = n.data as ProcessInfo
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
                <span className="text-xs text-gray-500 block">Events</span>
                <div className="mt-1 flex flex-wrap gap-1">
                  {Object.entries(selectedProcess.eventsByCategory).map(([cat, count]) => (
                    <span key={cat} className="rounded px-1.5 py-0.5 text-[11px] font-medium"
                      style={{ backgroundColor: (catColors[cat] || '#6b7280') + '20', color: catColors[cat] || '#6b7280' }}>
                      {cat}: {count}
                    </span>
                  ))}
                </div>
              </div>
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
