import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'

interface TelemetryEvent {
  id: number; timestamp: string; event_name: string; event_category: string;
  pid: number; tid: number; process_name: string; process_exe: string;
  process_cmdline: string; parent_pid: number; parent_name: string; params: unknown;
}

interface ProcessNode {
  pid: number
  name: string
  exe: string
  cmdline: string
  timestamp: string
  children: ProcessNode[]
  eventCount: number
  events: TelemetryEvent[]
}

function TreeNode({ node, depth = 0, selected, onSelect }: {
  node: ProcessNode; depth?: number; selected: number | null;
  onSelect: (pid: number) => void
}) {
  const [expanded, setExpanded] = useState(depth < 2)
  const hasChildren = node.children.length > 0
  const isSelected = selected === node.pid

  return (
    <div>
      <div
        className={'flex items-center gap-1.5 py-1 px-2 rounded cursor-pointer text-sm group ' +
          (isSelected ? 'bg-fibratus-50 ring-1 ring-fibratus-300' : 'hover:bg-gray-50')}
        style={{ paddingLeft: depth * 20 + 8 }}
        onClick={() => onSelect(node.pid)}
      >
        {hasChildren ? (
          <button
            onClick={(e) => { e.stopPropagation(); setExpanded(!expanded) }}
            className="w-4 h-4 flex items-center justify-center text-gray-400 hover:text-gray-600 flex-shrink-0"
          >
            <svg className={'w-3 h-3 transition-transform ' + (expanded ? 'rotate-90' : '')} fill="currentColor" viewBox="0 0 20 20">
              <path fillRule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clipRule="evenodd" />
            </svg>
          </button>
        ) : (
          <span className="w-4 flex-shrink-0" />
        )}
        <span className="font-mono text-xs text-gray-400 w-12 flex-shrink-0 tabular-nums">{node.pid}</span>
        <span className={'font-medium truncate ' + (isSelected ? 'text-fibratus-700' : 'text-gray-800')}>{node.name}</span>
        {node.eventCount > 1 && (
          <span className="ml-auto text-xs text-gray-400 flex-shrink-0">{node.eventCount} events</span>
        )}
      </div>
      {expanded && hasChildren && (
        <div>
          {node.children.map((child, i) => (
            <TreeNode key={`${child.pid}-${i}`} node={child} depth={depth + 1} selected={selected} onSelect={onSelect} />
          ))}
        </div>
      )}
    </div>
  )
}

export default function ProcessTree({ agentId }: { agentId: string }) {
  const [selected, setSelected] = useState<number | null>(null)

  const { data: res, isLoading, refetch } = useQuery({
    queryKey: ['agent-process-tree', agentId],
    queryFn: () => api.getAgentEvents(agentId, 5000),
  })

  const allEvents = (res?.data || []) as TelemetryEvent[]

  const { roots, processMap } = useMemo(() => {
    // Filter to process creation events
    const procEvents = allEvents.filter(e =>
      e.event_name === 'CreateProcess' || e.event_category === 'process'
    )

    // Build nodes by PID — use the latest event for each PID
    const byPid = new Map<number, { events: TelemetryEvent[]; node: ProcessNode }>()

    for (const evt of procEvents) {
      const existing = byPid.get(evt.pid)
      if (existing) {
        existing.events.push(evt)
        existing.node.eventCount++
      } else {
        byPid.set(evt.pid, {
          events: [evt],
          node: {
            pid: evt.pid,
            name: evt.process_name,
            exe: evt.process_exe || '',
            cmdline: evt.process_cmdline || '',
            timestamp: evt.timestamp,
            children: [],
            eventCount: 1,
            events: [evt],
          },
        })
      }
    }

    // Update events reference and build parent-child links
    for (const entry of byPid.values()) {
      entry.node.events = entry.events
    }

    const childPids = new Set<number>()
    for (const evt of procEvents) {
      if (evt.parent_pid && evt.parent_pid !== evt.pid) {
        const parent = byPid.get(evt.parent_pid)
        const child = byPid.get(evt.pid)
        if (parent && child && !parent.node.children.some(c => c.pid === child.node.pid)) {
          parent.node.children.push(child.node)
          childPids.add(evt.pid)
        }
      }
    }

    // Sort children by timestamp
    for (const entry of byPid.values()) {
      entry.node.children.sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime())
    }

    // Roots are processes that aren't children of any other process
    const roots = Array.from(byPid.values())
      .filter(e => !childPids.has(e.node.pid))
      .map(e => e.node)
      .sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime())

    return { roots, processMap: byPid }
  }, [allEvents])

  const selectedNode = selected ? processMap.get(selected)?.node : null

  if (isLoading) {
    return <div className="text-sm text-gray-400 text-center py-8">Loading process tree...</div>
  }

  if (roots.length === 0) {
    return <div className="text-sm text-gray-400 text-center py-8">No process events found. Process tree requires CreateProcess telemetry events.</div>
  }

  return (
    <div className="flex gap-4" style={{ height: 'calc(100vh - 220px)' }}>
      {/* Tree view */}
      <div className="flex-1 border rounded-lg overflow-auto bg-white">
        <div className="sticky top-0 bg-white border-b px-3 py-2 flex items-center justify-between z-10">
          <span className="text-xs font-medium text-gray-500">{processMap.size} processes</span>
          <button onClick={() => refetch()} className="text-xs text-fibratus-600 hover:underline">Refresh</button>
        </div>
        <div className="py-1">
          {roots.map((root, i) => (
            <TreeNode key={`${root.pid}-${i}`} node={root} selected={selected} onSelect={setSelected} />
          ))}
        </div>
      </div>

      {/* Detail pane */}
      <div className="w-80 border rounded-lg overflow-auto bg-white flex-shrink-0">
        {selectedNode ? (
          <div className="p-4 space-y-3">
            <div>
              <h4 className="font-medium text-gray-900">{selectedNode.name}</h4>
              <p className="text-xs text-gray-500 mt-0.5">PID {selectedNode.pid}</p>
            </div>
            <div className="space-y-2 text-sm">
              {selectedNode.exe && (
                <div>
                  <span className="text-xs text-gray-500 block">Executable</span>
                  <span className="font-mono text-xs text-gray-700 break-all">{selectedNode.exe}</span>
                </div>
              )}
              {selectedNode.cmdline && (
                <div>
                  <span className="text-xs text-gray-500 block">Command Line</span>
                  <span className="font-mono text-xs text-gray-700 break-all">{selectedNode.cmdline}</span>
                </div>
              )}
              <div>
                <span className="text-xs text-gray-500 block">First Seen</span>
                <span className="text-xs text-gray-700">{new Date(selectedNode.timestamp).toLocaleString()}</span>
              </div>
              <div>
                <span className="text-xs text-gray-500 block">Children</span>
                <span className="text-xs text-gray-700">{selectedNode.children.length} child process(es)</span>
              </div>
            </div>
            {selectedNode.events.length > 0 && (
              <div className="border-t pt-3 mt-3">
                <span className="text-xs font-medium text-gray-500">Events ({selectedNode.events.length})</span>
                <div className="mt-2 space-y-1.5 max-h-60 overflow-auto">
                  {selectedNode.events.map((evt, i) => (
                    <div key={i} className="rounded bg-gray-50 px-2 py-1.5">
                      <div className="flex items-center justify-between">
                        <span className="text-xs font-medium text-gray-700">{evt.event_name}</span>
                        <span className="text-xs text-gray-400">{new Date(evt.timestamp).toLocaleTimeString()}</span>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        ) : (
          <div className="p-4 text-sm text-gray-400 text-center mt-20">
            Select a process to view details
          </div>
        )}
      </div>
    </div>
  )
}
