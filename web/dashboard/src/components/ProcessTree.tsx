import { useState, useMemo, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
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

interface TelemetryEvent {
  id: number; timestamp: string; event_name: string; event_category: string;
  pid: number; tid: number; process_name: string; process_exe: string;
  process_cmdline: string; parent_pid: number; parent_name: string; params: unknown;
}

interface ProcessInfo {
  pid: number
  name: string
  exe: string
  cmdline: string
  timestamp: string
  eventCount: number
}

// Custom node component for process visualization
function ProcessNode({ data }: { data: ProcessInfo & { selected: boolean } }) {
  return (
    <div className={'rounded-lg border-2 px-3 py-2 bg-white shadow-sm min-w-[180px] ' +
      (data.selected ? 'border-fibratus-500 ring-2 ring-fibratus-200' : 'border-gray-200 hover:border-gray-300')}>
      <Handle type="target" position={Position.Top} className="!bg-gray-300 !w-2 !h-2" />
      <div className="flex items-center gap-2">
        <div className="flex h-6 w-6 items-center justify-center rounded bg-blue-100 text-blue-700 text-[10px] font-bold flex-shrink-0">
          {data.pid}
        </div>
        <span className="font-medium text-sm text-gray-900 truncate">{data.name}</span>
      </div>
      {data.exe && (
        <div className="mt-1 text-[10px] text-gray-400 font-mono truncate max-w-[200px]">{data.exe}</div>
      )}
      {data.eventCount > 1 && (
        <div className="mt-1 text-[10px] text-gray-400">{data.eventCount} events</div>
      )}
      <Handle type="source" position={Position.Bottom} className="!bg-gray-300 !w-2 !h-2" />
    </div>
  )
}

const nodeTypes = { process: ProcessNode }

export default function ProcessTree({ agentId }: { agentId: string }) {
  const [selectedPid, setSelectedPid] = useState<number | null>(null)

  const { data: res, isLoading, refetch } = useQuery({
    queryKey: ['agent-process-tree', agentId],
    queryFn: () => api.getAgentEvents(agentId, 5000),
  })

  const allEvents = (res?.data || []) as TelemetryEvent[]

  const { nodes, edges, selectedProcess } = useMemo(() => {
    const procEvents = allEvents.filter(e =>
      e.event_name === 'CreateProcess' || e.event_category === 'process'
    )

    // Build process map
    const byPid = new Map<number, ProcessInfo>()
    for (const evt of procEvents) {
      const existing = byPid.get(evt.pid)
      if (existing) {
        existing.eventCount++
      } else {
        byPid.set(evt.pid, {
          pid: evt.pid,
          name: evt.process_name,
          exe: evt.process_exe || '',
          cmdline: evt.process_cmdline || '',
          timestamp: evt.timestamp,
          eventCount: 1,
        })
      }
    }

    // Build parent-child relationships
    const children = new Map<number, Set<number>>()
    const hasParent = new Set<number>()
    for (const evt of procEvents) {
      if (evt.parent_pid && evt.parent_pid !== evt.pid && byPid.has(evt.pid)) {
        if (!children.has(evt.parent_pid)) children.set(evt.parent_pid, new Set())
        const set = children.get(evt.parent_pid)!
        if (!set.has(evt.pid)) {
          set.add(evt.pid)
          hasParent.add(evt.pid)
        }
      }
    }

    // Layout: simple tree layout with BFS
    const roots = Array.from(byPid.keys()).filter(pid => !hasParent.has(pid))
    const positions = new Map<number, { x: number; y: number }>()
    let xOffset = 0
    const LEVEL_HEIGHT = 100
    const NODE_WIDTH = 220

    function layoutTree(pid: number, depth: number, xStart: number): number {
      const kids = Array.from(children.get(pid) || [])
      if (kids.length === 0) {
        positions.set(pid, { x: xStart, y: depth * LEVEL_HEIGHT })
        return xStart + NODE_WIDTH
      }
      let x = xStart
      for (const kid of kids) {
        x = layoutTree(kid, depth + 1, x)
      }
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
          edges.push({
            id: key,
            source: String(parentPid),
            target: String(kidPid),
            type: 'smoothstep',
            style: { stroke: '#94a3b8', strokeWidth: 1.5 },
          })
        }
      }
    }

    const selectedProcess = selectedPid ? byPid.get(selectedPid) : null
    return { nodes, edges, selectedProcess }
  }, [allEvents, selectedPid])

  const onNodeClick = useCallback((_: unknown, node: Node) => {
    setSelectedPid(Number(node.id))
  }, [])

  if (isLoading) {
    return <div className="text-sm text-gray-400 text-center py-8">Loading process tree...</div>
  }

  if (nodes.length === 0) {
    return <div className="text-sm text-gray-400 text-center py-8">No process events found. Process tree requires CreateProcess telemetry events.</div>
  }

  return (
    <div className="flex gap-4" style={{ height: 'calc(100vh - 220px)' }}>
      {/* ReactFlow tree */}
      <div className="flex-1 border rounded-lg overflow-hidden bg-gray-50">
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
            nodeColor={(n) => n.id === String(selectedPid) ? '#7c3aed' : '#e5e7eb'}
            pannable
            zoomable
          />
        </ReactFlow>
      </div>

      {/* Detail pane */}
      <div className="w-72 border rounded-lg overflow-auto bg-white flex-shrink-0">
        <div className="sticky top-0 bg-white border-b px-3 py-2 z-10 flex items-center justify-between">
          <span className="text-xs font-medium text-gray-500">{nodes.length} processes</span>
          <button onClick={() => refetch()} className="text-xs text-fibratus-600 hover:underline">Refresh</button>
        </div>
        {selectedProcess ? (
          <div className="p-4 space-y-3">
            <div>
              <h4 className="font-medium text-gray-900">{selectedProcess.name}</h4>
              <p className="text-xs text-gray-500 mt-0.5">PID {selectedProcess.pid}</p>
            </div>
            <div className="space-y-2 text-sm">
              {selectedProcess.exe && (
                <div>
                  <span className="text-xs text-gray-500 block">Executable</span>
                  <span className="font-mono text-xs text-gray-700 break-all">{selectedProcess.exe}</span>
                </div>
              )}
              {selectedProcess.cmdline && (
                <div>
                  <span className="text-xs text-gray-500 block">Command Line</span>
                  <span className="font-mono text-xs text-gray-700 break-all">{selectedProcess.cmdline}</span>
                </div>
              )}
              <div>
                <span className="text-xs text-gray-500 block">First Seen</span>
                <span className="text-xs text-gray-700">{new Date(selectedProcess.timestamp).toLocaleString()}</span>
              </div>
              <div>
                <span className="text-xs text-gray-500 block">Events</span>
                <span className="text-xs text-gray-700">{selectedProcess.eventCount} event(s)</span>
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
  )
}
