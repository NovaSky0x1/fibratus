import { useState, useMemo, useCallback } from 'react'

interface TelemetryEvent {
  id: number; timestamp: string; event_name: string; event_category: string
  pid: number; tid: number; process_name: string; process_exe: string
  process_cmdline: string; parent_pid: number; parent_name: string
  params: Record<string, unknown>
}

interface TreeNode {
  pid: number
  name: string
  exe: string
  cmdline: string
  parentPid: number
  parentName: string
  eventName: string
  timestamp: string
  category: string
  children: TreeNode[]
  isTrigger: boolean
  isOnPath: boolean
  collapsed: boolean
  params: Record<string, unknown>
}

interface Props {
  events: TelemetryEvent[]
  focusPids: Record<number, boolean>
  onLoadContext?: (pid: number) => void
  loadingPid?: number | null
}

const catColors: Record<string, { border: string; badge: string; line: string }> = {
  process:  { border: 'border-l-blue-500',    badge: 'bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300',       line: 'bg-blue-300 dark:bg-blue-700' },
  file:     { border: 'border-l-amber-500',   badge: 'bg-amber-100 text-amber-700 dark:bg-amber-900/50 dark:text-amber-300',   line: 'bg-amber-300 dark:bg-amber-700' },
  registry: { border: 'border-l-purple-500',  badge: 'bg-purple-100 text-purple-700 dark:bg-purple-900/50 dark:text-purple-300', line: 'bg-purple-300 dark:bg-purple-700' },
  net:      { border: 'border-l-emerald-500', badge: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/50 dark:text-emerald-300', line: 'bg-emerald-300 dark:bg-emerald-700' },
  image:    { border: 'border-l-indigo-500',  badge: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/50 dark:text-indigo-300', line: 'bg-indigo-300 dark:bg-indigo-700' },
  dns:      { border: 'border-l-teal-500',    badge: 'bg-teal-100 text-teal-700 dark:bg-teal-900/50 dark:text-teal-300',       line: 'bg-teal-300 dark:bg-teal-700' },
  thread:   { border: 'border-l-pink-500',    badge: 'bg-pink-100 text-pink-700 dark:bg-pink-900/50 dark:text-pink-300',       line: 'bg-pink-300 dark:bg-pink-700' },
  mem:      { border: 'border-l-rose-500',    badge: 'bg-rose-100 text-rose-700 dark:bg-rose-900/50 dark:text-rose-300',       line: 'bg-rose-300 dark:bg-rose-700' },
  handle:   { border: 'border-l-gray-400',    badge: 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300',          line: 'bg-gray-300 dark:bg-gray-600' },
}

function getCat(cat: string) { return catColors[cat] || catColors.handle }

function eventDetail(evt: TelemetryEvent): string {
  const p = evt.params || {}
  switch (evt.event_category) {
    case 'file': return String(p.file_name || p.file_object || '')
    case 'registry': return String(p.key_name || p.key || '')
    case 'net': return [p.dip, p.dport].filter(Boolean).join(':') || ''
    case 'dns': return String(p.name || p.domain || '')
    case 'image': return String(p.file_name || p.image_name || '')
    default: return ''
  }
}

export default function ProcessChain({ events, focusPids, onLoadContext, loadingPid }: Props) {
  const [collapsedNodes, setCollapsedNodes] = useState<Set<string>>(new Set())
  const [selectedId, setSelectedId] = useState<string | null>(null)

  // Build tree: each event becomes a node, grouped by PID then branching by child PIDs
  const roots = useMemo(() => {
    if (!events.length) return []

    // Group events by PID
    const byPid = new Map<number, TelemetryEvent[]>()
    for (const evt of events) {
      if (evt.pid <= 0) continue
      const list = byPid.get(evt.pid) || []
      list.push(evt)
      byPid.set(evt.pid, list)
    }

    // Find parent-child relationships
    const childrenOf = new Map<number, Set<number>>()
    const parentOf = new Map<number, number>()
    for (const [pid, evts] of byPid) {
      const ppid = evts[0].parent_pid
      if (ppid > 0 && ppid !== pid && byPid.has(ppid)) {
        parentOf.set(pid, ppid)
        if (!childrenOf.has(ppid)) childrenOf.set(ppid, new Set())
        childrenOf.get(ppid)!.add(pid)
      }
    }

    // Find focus path
    const onPath = new Set<number>()
    for (const pid of Object.keys(focusPids).map(Number)) {
      onPath.add(pid)
      let cur = pid
      for (let i = 0; i < 20; i++) {
        const p = parentOf.get(cur)
        if (!p) break
        onPath.add(p)
        cur = p
      }
    }

    // Build tree nodes for each PID
    const buildPidNodes = (pid: number): TreeNode[] => {
      const evts = byPid.get(pid) || []
      const isTrigger = !!focusPids[pid]
      const isPath = onPath.has(pid)
      const kids = childrenOf.get(pid) || new Set()

      // Group by event category for non-process events
      const processEvts = evts.filter(e => e.event_category === 'process')
      const otherByCat = new Map<string, TelemetryEvent[]>()
      for (const e of evts) {
        if (e.event_category === 'process') continue
        const list = otherByCat.get(e.event_category) || []
        list.push(e)
        otherByCat.set(e.event_category, list)
      }

      // Root node for this PID = the process creation event or first event
      const mainEvt = processEvts.find(e => e.event_name === 'CreateProcess') || evts[0]
      const children: TreeNode[] = []

      // Add event category nodes as children (file, reg, net, dns, etc.)
      for (const [cat, catEvts] of otherByCat) {
        const catNode: TreeNode = {
          pid, name: `${cat.toUpperCase()} (${catEvts.length})`, exe: '',
          cmdline: '', parentPid: pid, parentName: mainEvt.process_name,
          eventName: catEvts[0].event_name, timestamp: catEvts[0].timestamp,
          category: cat, children: [], isTrigger: false, isOnPath: isPath,
          collapsed: !isTrigger, params: {},
        }
        // Each event in the category becomes a leaf node
        for (const e of catEvts.slice(0, 30)) {
          catNode.children.push({
            pid: e.pid, name: eventDetail(e) || e.event_name, exe: '',
            cmdline: '', parentPid: pid, parentName: '',
            eventName: e.event_name, timestamp: e.timestamp,
            category: e.event_category, children: [], isTrigger: false,
            isOnPath: false, collapsed: false, params: e.params || {},
          })
        }
        if (catEvts.length > 30) {
          catNode.children.push({
            pid: 0, name: `+${catEvts.length - 30} more`, exe: '',
            cmdline: '', parentPid: 0, parentName: '',
            eventName: '', timestamp: '', category: cat,
            children: [], isTrigger: false, isOnPath: false,
            collapsed: false, params: {},
          })
        }
        children.push(catNode)
      }

      // Add child process nodes
      for (const cpid of kids) {
        children.push(...buildPidNodes(cpid))
      }

      return [{
        pid, name: mainEvt.process_name, exe: mainEvt.process_exe || '',
        cmdline: mainEvt.process_cmdline || '', parentPid: mainEvt.parent_pid,
        parentName: mainEvt.parent_name || '',
        eventName: mainEvt.event_name || 'CreateProcess',
        timestamp: mainEvt.timestamp, category: 'process',
        children, isTrigger, isOnPath: isPath, collapsed: false,
        params: {},
      }]
    }

    // Find roots (PIDs with no parent in the data)
    const rootPids = [...byPid.keys()].filter(pid => !parentOf.has(pid))
    const tree: TreeNode[] = []
    for (const pid of rootPids) tree.push(...buildPidNodes(pid))
    return tree
  }, [events, focusPids])

  const toggleCollapse = useCallback((id: string) => {
    setCollapsedNodes(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id); else next.add(id)
      return next
    })
  }, [])

  if (roots.length === 0) {
    return <div className="flex items-center justify-center h-48 text-sm text-gray-400 dark:text-slate-500">No process events found for this detection.</div>
  }

  // Check if any root has a parent not in the tree (for "load parent" button)
  const topRoot = roots[0]
  const canLoadParent = topRoot && topRoot.parentPid > 0 && onLoadContext

  return (
    <div className="overflow-auto p-4 min-h-[300px]">
      {canLoadParent && (
        <button onClick={() => onLoadContext!(topRoot.pid)} disabled={loadingPid === topRoot.pid}
          className="mb-3 inline-flex items-center gap-1 rounded-lg border border-dashed border-gray-300 dark:border-slate-600 px-3 py-1.5 text-xs text-gray-500 dark:text-slate-400 hover:border-gray-400 hover:text-gray-700 dark:hover:text-slate-300 disabled:opacity-50">
          {loadingPid === topRoot.pid ? 'Loading...' : `Load parent (${topRoot.parentName || topRoot.parentPid})`}
        </button>
      )}
      <div className="flex items-start">
        {roots.map((node, i) => (
          <NodeRenderer key={`root-${i}`} node={node} depth={0}
            collapsedNodes={collapsedNodes} toggleCollapse={toggleCollapse}
            selectedId={selectedId} setSelectedId={setSelectedId}
            parentId="" index={i} />
        ))}
      </div>
    </div>
  )
}

interface NodeRendererProps {
  node: TreeNode
  depth: number
  collapsedNodes: Set<string>
  toggleCollapse: (id: string) => void
  selectedId: string | null
  setSelectedId: (id: string | null) => void
  parentId: string
  index: number
}

function NodeRenderer({ node, depth, collapsedNodes, toggleCollapse, selectedId, setSelectedId, parentId, index }: NodeRendererProps) {
  const id = `${parentId}-${node.pid}-${node.category}-${index}`
  const cat = getCat(node.category)
  const isCollapsed = node.collapsed ? !collapsedNodes.has(id) : collapsedNodes.has(id) // default collapsed state inverted by toggle
  const hasChildren = node.children.length > 0
  const isSelected = selectedId === id
  const isProcess = node.category === 'process' && node.eventName !== ''
  const isLeaf = !hasChildren && !isProcess

  return (
    <div className="flex items-start">
      {/* This node */}
      <div className="flex flex-col items-start">
        <div
          onClick={() => {
            if (hasChildren) toggleCollapse(id)
            setSelectedId(isSelected ? null : id)
          }}
          className={
            'rounded-lg border-l-4 border border-gray-200 dark:border-slate-700 px-3 py-2 cursor-pointer transition-all select-none ' +
            cat.border + ' ' +
            (node.isTrigger
              ? 'bg-red-50 dark:bg-red-950/30 border-red-200 dark:border-red-800 shadow-md shadow-red-100 dark:shadow-red-900/20 '
              : node.isOnPath
                ? 'bg-blue-50/50 dark:bg-blue-950/20 '
                : 'bg-white dark:bg-slate-800 hover:bg-gray-50 dark:hover:bg-slate-750 ') +
            (isSelected ? 'ring-2 ring-fibratus-400 ' : '') +
            (isLeaf ? 'min-w-[180px] max-w-[280px] ' : 'min-w-[240px] max-w-[320px] ')
          }
        >
          {/* Event badge + timestamp */}
          <div className="flex items-center gap-2 flex-wrap">
            <span className={`rounded px-1.5 py-0.5 text-[10px] font-bold ${cat.badge}`}>{node.eventName || node.name}</span>
            {node.timestamp && <span className="text-[10px] text-gray-400 dark:text-slate-500">{new Date(node.timestamp).toLocaleTimeString()}</span>}
            {node.isTrigger && <span className="rounded bg-red-500 text-white text-[9px] px-1.5 py-0.5 font-bold">TRIGGER</span>}
            {hasChildren && (
              <span className="text-[10px] text-gray-400 dark:text-slate-500 ml-auto">
                {isCollapsed ? `+${node.children.length}` : '−'}
              </span>
            )}
          </div>

          {/* Process info */}
          {isProcess && (
            <div className="mt-1 space-y-0.5">
              {node.exe && <div className="text-[11px] font-mono text-gray-700 dark:text-slate-300 truncate"><span className="text-gray-400 dark:text-slate-500">exe: </span>{node.exe}</div>}
              {node.pid > 0 && <div className="text-[11px] font-mono text-gray-500 dark:text-slate-400"><span className="text-gray-400 dark:text-slate-500">pid: </span><span className="text-red-600 dark:text-red-400 font-medium">{node.pid}</span>{node.parentPid > 0 && <span> <span className="text-gray-400 dark:text-slate-500">parent:</span> {node.parentPid}</span>}</div>}
            </div>
          )}

          {/* Leaf event detail */}
          {isLeaf && node.name && node.eventName && (
            <div className="mt-0.5 text-[10px] font-mono text-gray-500 dark:text-slate-400 truncate">{node.name}</div>
          )}

          {/* Params for leaf events */}
          {isLeaf && isSelected && node.params && Object.keys(node.params).length > 0 && (
            <div className="mt-1.5 pt-1.5 border-t border-gray-100 dark:border-slate-700 space-y-0.5">
              {Object.entries(node.params).slice(0, 8).map(([k, v]) => (
                <div key={k} className="text-[10px] font-mono flex gap-1">
                  <span className="text-gray-400 dark:text-slate-500 shrink-0">{k}:</span>
                  <span className="text-gray-700 dark:text-slate-300 truncate">{String(v)}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Children (branching to the right) */}
      {hasChildren && !isCollapsed && (
        <div className="flex items-start ml-0">
          {/* Horizontal connector line */}
          <div className="flex flex-col justify-center self-stretch shrink-0">
            <div className={`w-6 h-px ${cat.line} my-auto`} />
          </div>
          {/* Vertical branch with children */}
          <div className="flex flex-col gap-1.5 relative">
            {/* Vertical line connecting children */}
            {node.children.length > 1 && (
              <div className={`absolute left-0 top-3 bottom-3 w-px ${cat.line}`} />
            )}
            {node.children.map((child, ci) => (
              <div key={ci} className="flex items-start">
                {/* Horizontal tick from vertical line */}
                <div className={`w-4 h-px ${cat.line} mt-3 shrink-0`} />
                <NodeRenderer node={child} depth={depth + 1}
                  collapsedNodes={collapsedNodes} toggleCollapse={toggleCollapse}
                  selectedId={selectedId} setSelectedId={setSelectedId}
                  parentId={id} index={ci} />
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
