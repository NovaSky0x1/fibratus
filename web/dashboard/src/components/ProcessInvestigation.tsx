import { useState, useMemo, useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import SeverityBadge from './SeverityBadge'

// ── Types ────────────────────────────────────────────────

interface TelemetryEvent {
  id: number; timestamp: string; event_name: string; event_category: string
  pid: number; tid: number; process_name: string; process_exe: string
  process_cmdline: string; parent_pid: number; parent_name: string
  params: Record<string, unknown>; raw_event: unknown
}

interface ProcessNode {
  id: string
  pid: number
  name: string
  exe: string
  cmdline: string
  parentPid: number
  parentName: string
  firstSeen: string
  lastSeen: string
  eventCount: number
  eventsByCategory: Record<string, number>
  children: ProcessNode[]
  isOnFocusPath: boolean
  isFocus: boolean
  depth: number
}

interface Props {
  agentId: string
  focusPid?: number
  focusProcessName?: string
  focusEventId?: number
  detection?: {
    title: string; severity: string; hostname: string; timestamp: string
    ruleName: string; labels: Record<string, string>
  }
}

// ── Category colors ──────────────────────────────────────

const catColor: Record<string, string> = {
  process: 'bg-blue-100 text-blue-700',
  net: 'bg-green-100 text-green-700',
  file: 'bg-yellow-100 text-yellow-700',
  registry: 'bg-purple-100 text-purple-700',
  image: 'bg-indigo-100 text-indigo-700',
  dns: 'bg-teal-100 text-teal-700',
  thread: 'bg-orange-100 text-orange-700',
  mem: 'bg-pink-100 text-pink-700',
}

// ── Tree builder ─────────────────────────────────────────

function buildTree(events: TelemetryEvent[], focusPid?: number, focusName?: string) {
  const nodeMap = new Map<string, ProcessNode>()

  // First pass: build nodes
  for (const evt of events) {
    const key = `${evt.pid}:${evt.process_name}`
    let node = nodeMap.get(key)
    if (!node) {
      node = {
        id: key, pid: evt.pid, name: evt.process_name, exe: evt.process_exe || '',
        cmdline: evt.process_cmdline || '', parentPid: evt.parent_pid,
        parentName: evt.parent_name || '', firstSeen: evt.timestamp,
        lastSeen: evt.timestamp, eventCount: 0, eventsByCategory: {},
        children: [], isOnFocusPath: false, isFocus: false, depth: 0,
      }
      nodeMap.set(key, node)
    }
    node.eventCount++
    node.eventsByCategory[evt.event_category] = (node.eventsByCategory[evt.event_category] || 0) + 1
    if (evt.timestamp < node.firstSeen) node.firstSeen = evt.timestamp
    if (evt.timestamp > node.lastSeen) node.lastSeen = evt.timestamp
    if (!node.cmdline && evt.process_cmdline) node.cmdline = evt.process_cmdline
    if (!node.exe && evt.process_exe) node.exe = evt.process_exe
  }

  // Second pass: link children to parents
  const childKeys = new Set<string>()
  for (const node of nodeMap.values()) {
    if (node.parentPid <= 0) continue
    // Try exact match first
    let parentKey = `${node.parentPid}:${node.parentName}`
    let parent = nodeMap.get(parentKey)
    if (!parent) {
      // Fallback: find any node with matching PID
      for (const [k, n] of nodeMap) {
        if (n.pid === node.parentPid && k !== node.id) { parent = n; break }
      }
    }
    if (parent && !parent.children.some(c => c.id === node.id)) {
      parent.children.push(node)
      childKeys.add(node.id)
    }
  }

  // Sort children by first seen
  for (const node of nodeMap.values()) {
    node.children.sort((a, b) => a.firstSeen.localeCompare(b.firstSeen))
  }

  // Roots
  const roots = Array.from(nodeMap.values())
    .filter(n => !childKeys.has(n.id))
    .sort((a, b) => a.firstSeen.localeCompare(b.firstSeen))

  // Mark focus path
  const focusKey = focusPid ? `${focusPid}:${focusName || ''}` : null
  let focusNode: ProcessNode | null = null
  if (focusKey) {
    focusNode = nodeMap.get(focusKey) || null
    if (!focusNode && focusPid) {
      for (const n of nodeMap.values()) {
        if (n.pid === focusPid) { focusNode = n; break }
      }
    }
  }
  if (focusNode) {
    focusNode.isFocus = true
    focusNode.isOnFocusPath = true
    // Mark descendants
    const markDown = (n: ProcessNode) => { n.isOnFocusPath = true; n.children.forEach(markDown) }
    markDown(focusNode)
    // Mark ancestors
    const markUp = (pid: number, name: string) => {
      const key = `${pid}:${name}`
      let parent = nodeMap.get(key)
      if (!parent) {
        for (const n of nodeMap.values()) {
          if (n.pid === pid) { parent = n; break }
        }
      }
      if (parent && !parent.isOnFocusPath) {
        parent.isOnFocusPath = true
        if (parent.parentPid > 0) markUp(parent.parentPid, parent.parentName)
      }
    }
    if (focusNode.parentPid > 0) markUp(focusNode.parentPid, focusNode.parentName)
  }

  // Set depths
  const setDepth = (nodes: ProcessNode[], d: number) => {
    for (const n of nodes) { n.depth = d; setDepth(n.children, d + 1) }
  }
  setDepth(roots, 0)

  return { roots, nodeMap, focusNode }
}

// ── Tree Node ────────────────────────────────────────────

function TreeNode({ node, selected, onSelect, expanded, onToggle }: {
  node: ProcessNode; selected: string | null
  onSelect: (id: string) => void; expanded: Set<string>
  onToggle: (id: string) => void
}) {
  const isSelected = selected === node.id
  const isExpanded = expanded.has(node.id)
  const hasKids = node.children.length > 0

  return (
    <div>
      <div
        data-node-id={node.id}
        className={'flex items-center gap-1 py-1 px-2 cursor-pointer text-[13px] group ' +
          (isSelected ? 'bg-fibratus-50 ring-1 ring-fibratus-300 rounded' :
           node.isFocus ? 'bg-blue-50 border-l-2 border-blue-500' :
           node.isOnFocusPath ? 'bg-blue-50/40' : 'hover:bg-gray-50')}
        style={{ paddingLeft: node.depth * 16 + 4 }}
        onClick={() => onSelect(node.id)}
      >
        {hasKids ? (
          <button onClick={e => { e.stopPropagation(); onToggle(node.id) }}
            className="w-4 h-4 flex items-center justify-center text-gray-400 hover:text-gray-600 flex-shrink-0">
            <svg className={'w-3 h-3 transition-transform ' + (isExpanded ? 'rotate-90' : '')}
              fill="currentColor" viewBox="0 0 20 20">
              <path fillRule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clipRule="evenodd" />
            </svg>
          </button>
        ) : <span className="w-4 flex-shrink-0" />}

        <span className={'w-2 h-2 rounded-full flex-shrink-0 ' +
          (node.isFocus ? 'bg-red-500' : node.isOnFocusPath ? 'bg-blue-400' : 'bg-gray-300')} />

        <span className={'font-medium truncate ' + (node.isFocus ? 'text-red-700' : isSelected ? 'text-fibratus-700' : 'text-gray-800')}>
          {node.name}
        </span>
        <span className="text-[10px] text-gray-400 font-mono flex-shrink-0">{node.pid}</span>
        <span className="ml-auto text-[10px] text-gray-400 flex-shrink-0">{node.eventCount}</span>
      </div>
      {isExpanded && hasKids && node.children.map(child => (
        <TreeNode key={child.id} node={child} selected={selected}
          onSelect={onSelect} expanded={expanded} onToggle={onToggle} />
      ))}
    </div>
  )
}

// ── Event row ────────────────────────────────────────────

function EventRow({ evt, isFocus, isOpen, onToggle }: {
  evt: TelemetryEvent; isFocus: boolean; isOpen: boolean; onToggle: () => void
}) {
  return (
    <div className={'border-b border-gray-100 ' + (isFocus ? 'bg-red-50 border-l-2 border-red-400' : '')}>
      <div className="flex items-center gap-2 px-3 py-1.5 cursor-pointer hover:bg-gray-50/50 text-[13px]"
        onClick={onToggle}>
        <span className="text-gray-400 tabular-nums font-mono text-xs w-20 flex-shrink-0">
          {new Date(evt.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', fractionalSecondDigits: 3 } as Intl.DateTimeFormatOptions)}
        </span>
        <span className={'rounded px-1.5 py-0.5 text-[11px] font-medium flex-shrink-0 ' +
          (catColor[evt.event_category] || 'bg-gray-100 text-gray-700')}>
          {evt.event_name}
        </span>
        <span className="text-gray-600 truncate text-xs">
          {summarizeEvent(evt)}
        </span>
      </div>
      {isOpen && (
        <div className="px-3 pb-3 pt-1 bg-gray-50/50 text-xs space-y-2" onClick={e => e.stopPropagation()}>
          <div className="grid grid-cols-2 gap-x-4 gap-y-1">
            <div><span className="text-gray-400">Process</span> <span className="font-mono text-gray-700">{evt.process_name}</span></div>
            <div><span className="text-gray-400">PID</span> <span className="font-mono text-gray-700">{evt.pid}</span></div>
            {evt.process_exe && <div className="col-span-2"><span className="text-gray-400">Exe</span> <span className="font-mono text-gray-700 break-all">{evt.process_exe}</span></div>}
            {evt.parent_name && <div><span className="text-gray-400">Parent</span> <span className="font-mono text-gray-700">{evt.parent_name} ({evt.parent_pid})</span></div>}
          </div>
          {evt.process_cmdline && (
            <div>
              <span className="text-gray-400 text-[11px]">Command Line</span>
              <div className="mt-0.5 rounded bg-gray-900 px-2 py-1.5 text-gray-100 font-mono break-all whitespace-pre-wrap text-[11px]">{evt.process_cmdline}</div>
            </div>
          )}
          {(() => { const pp = parseParams(evt.params); return Object.keys(pp).length > 0 ? pp : null })() && (
            <div>
              <span className="text-gray-400 text-[11px]">Parameters</span>
              <div className="mt-1 grid grid-cols-2 gap-x-4 gap-y-0.5">
                {Object.entries(parseParams(evt.params)).map(([k, v]) => (
                  <div key={k} className="min-w-0">
                    <span className="text-gray-400">{k}:</span>{' '}
                    <span className="font-mono text-gray-700 break-all">{typeof v === 'object' ? JSON.stringify(v) : String(v)}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function parseParams(raw: unknown): Record<string, unknown> {
  if (!raw) return {}
  if (typeof raw === 'string') {
    try { return JSON.parse(raw) } catch { return {} }
  }
  if (typeof raw === 'object' && raw !== null) return raw as Record<string, unknown>
  return {}
}

function summarizeEvent(evt: TelemetryEvent): string {
  const p = parseParams(evt.params)
  const name = evt.event_name || ''

  // Also try to extract from raw_event if params is empty
  if (Object.keys(p).length === 0 && evt.raw_event) {
    const raw = parseParams(evt.raw_event)
    const innerParams = parseParams(raw.params)
    if (innerParams.name) return String(innerParams.name)
  }

  // DNS events (categorized as 'net' by ETW)
  if (name === 'QueryDns' || name === 'ReplyDns') {
    return String(p.name || p.domain || '')
  }

  switch (evt.event_category) {
    case 'net': return [p.dip, p.dport].filter(Boolean).join(':') || String(p.name || '')
    case 'dns': return String(p.name || p.domain || '')
    case 'file': return String(p.file_name || p.file_object || '')
    case 'registry': return String(p.key_name || p.key || '')
    case 'image': return String(p.file_name || p.image_name || '')
    default: return evt.process_cmdline ? evt.process_cmdline.slice(0, 80) : ''
  }
}

// ── Main component ───────────────────────────────────────

export default function ProcessInvestigation({ agentId, focusPid, focusProcessName, focusEventId, detection }: Props) {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [openEvent, setOpenEvent] = useState<number | null>(null)
  const [catFilter, setCatFilter] = useState<string>('all')
  const [treeSearch, setTreeSearch] = useState('')
  const focusRef = useRef<HTMLDivElement>(null)

  const { data: res, isLoading, refetch } = useQuery({
    queryKey: ['investigation-events', agentId],
    queryFn: () => api.getAgentEvents(agentId, 5000),
  })

  const allEvents = (res?.data || []) as TelemetryEvent[]

  const { roots, nodeMap, focusNode } = useMemo(() => {
    return buildTree(allEvents, focusPid, focusProcessName)
  }, [allEvents, focusPid, focusProcessName])

  // Auto-expand focus path on first load
  useEffect(() => {
    if (roots.length === 0) return
    const toExpand = new Set<string>()
    const expandPath = (nodes: ProcessNode[]) => {
      for (const n of nodes) {
        if (n.isOnFocusPath || n.isFocus) {
          toExpand.add(n.id)
          expandPath(n.children)
        }
      }
    }
    expandPath(roots)
    // Also expand roots
    roots.forEach(r => toExpand.add(r.id))
    setExpanded(toExpand)
    if (focusNode) setSelectedId(focusNode.id)
  }, [roots, focusNode])

  // Auto-scroll to focus
  useEffect(() => {
    if (focusNode) {
      setTimeout(() => {
        const el = document.querySelector(`[data-node-id="${focusNode.id}"]`)
        el?.scrollIntoView({ behavior: 'smooth', block: 'center' })
      }, 100)
    }
  }, [focusNode])

  const toggleExpand = (id: string) => {
    setExpanded(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id); else next.add(id)
      return next
    })
  }

  // Events for selected process
  const selectedNode = selectedId ? nodeMap.get(selectedId) : null
  const selectedEvents = useMemo(() => {
    if (!selectedNode) return []
    return allEvents
      .filter(e => e.pid === selectedNode.pid && e.process_name === selectedNode.name)
      .sort((a, b) => a.timestamp.localeCompare(b.timestamp))
  }, [allEvents, selectedNode])

  const filteredEvents = catFilter === 'all' ? selectedEvents :
    selectedEvents.filter(e => e.event_category === catFilter)

  // Filter tree by search
  const filteredRoots = useMemo(() => {
    if (!treeSearch) return roots
    const term = treeSearch.toLowerCase()
    const matches = new Set<string>()
    for (const n of nodeMap.values()) {
      if (n.name.toLowerCase().includes(term) || String(n.pid).includes(term)) {
        matches.add(n.id)
      }
    }
    // Keep nodes that match or have a descendant that matches
    const keep = (node: ProcessNode): boolean => {
      if (matches.has(node.id)) return true
      return node.children.some(keep)
    }
    return roots.filter(keep)
  }, [roots, nodeMap, treeSearch])

  const categories = useMemo(() => {
    const cats = new Map<string, number>()
    for (const e of selectedEvents) {
      cats.set(e.event_category, (cats.get(e.event_category) || 0) + 1)
    }
    return cats
  }, [selectedEvents])

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-gray-400">Loading process data...</div>
  }

  return (
    <div className="flex flex-col" style={{ height: 'calc(100vh - 200px)' }}>
      {/* Context bar */}
      {detection && (
        <div className="flex items-center gap-3 px-4 py-2.5 border-b bg-gray-50/50 flex-shrink-0">
          <SeverityBadge severity={detection.severity} />
          <span className="font-medium text-sm text-gray-900">{detection.title}</span>
          <span className="text-xs text-gray-400">{detection.hostname}</span>
          <span className="text-xs text-gray-400">{new Date(detection.timestamp).toLocaleString()}</span>
          {detection.labels?.['technique.id'] && (
            <span className="ml-auto rounded bg-gray-100 px-1.5 py-0.5 text-xs font-mono text-gray-600">
              {detection.labels['technique.id']} — {detection.labels['technique.name'] || ''}
            </span>
          )}
        </div>
      )}

      <div className="flex flex-1 min-h-0">
        {/* Left: Process tree */}
        <div className="w-80 border-r flex flex-col min-h-0 flex-shrink-0">
          <div className="px-3 py-2 border-b bg-white flex items-center gap-2 flex-shrink-0">
            <input value={treeSearch} onChange={e => setTreeSearch(e.target.value)}
              placeholder="Search processes..."
              className="flex-1 rounded border border-gray-200 px-2 py-1 text-xs focus:ring-1 focus:ring-fibratus-500 focus:outline-none" />
            <span className="text-[10px] text-gray-400">{nodeMap.size}</span>
            <button onClick={() => refetch()} className="text-[10px] text-fibratus-600 hover:underline">Refresh</button>
          </div>
          <div className="flex-1 overflow-auto py-1" ref={focusRef}>
            {filteredRoots.length === 0 ? (
              <div className="text-xs text-gray-400 text-center py-8">No process events found</div>
            ) : filteredRoots.map(root => (
              <TreeNode key={root.id} node={root} selected={selectedId}
                onSelect={setSelectedId} expanded={expanded} onToggle={toggleExpand} />
            ))}
          </div>
        </div>

        {/* Right: Timeline */}
        <div className="flex-1 flex flex-col min-h-0">
          {selectedNode ? (
            <>
              {/* Process detail header */}
              <div className="px-4 py-3 border-b bg-white flex-shrink-0 space-y-1">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-sm text-gray-900">{selectedNode.name}</span>
                  <span className="text-xs font-mono text-gray-400">PID {selectedNode.pid}</span>
                  {selectedNode.isFocus && <span className="rounded bg-red-100 text-red-700 text-[10px] px-1.5 py-0.5 font-medium">Trigger</span>}
                </div>
                {selectedNode.exe && <div className="text-xs text-gray-500 font-mono truncate">{selectedNode.exe}</div>}
                {selectedNode.cmdline && (
                  <div className="rounded bg-gray-900 px-2 py-1.5 text-[11px] text-gray-100 font-mono break-all whitespace-pre-wrap max-h-16 overflow-auto">
                    {selectedNode.cmdline}
                  </div>
                )}
              </div>

              {/* Category filter tabs */}
              <div className="px-4 py-2 border-b bg-white flex items-center gap-1.5 flex-shrink-0 flex-wrap">
                <button onClick={() => setCatFilter('all')}
                  className={'rounded-full px-2 py-0.5 text-[11px] font-medium border ' +
                    (catFilter === 'all' ? 'bg-gray-800 text-white border-gray-800' : 'bg-white text-gray-500 border-gray-200 hover:bg-gray-50')}>
                  All ({selectedEvents.length})
                </button>
                {Array.from(categories.entries()).map(([cat, count]) => (
                  <button key={cat} onClick={() => setCatFilter(catFilter === cat ? 'all' : cat)}
                    className={'rounded-full px-2 py-0.5 text-[11px] font-medium border ' +
                      (catFilter === cat ? 'bg-gray-800 text-white border-gray-800' : 'bg-white text-gray-500 border-gray-200 hover:bg-gray-50')}>
                    {cat} ({count})
                  </button>
                ))}
              </div>

              {/* Events list */}
              <div className="flex-1 overflow-auto">
                {filteredEvents.length === 0 ? (
                  <div className="text-xs text-gray-400 text-center py-8">No events match filter</div>
                ) : filteredEvents.map((evt, idx) => (
                  <EventRow key={`${evt.id}-${idx}`} evt={evt}
                    isFocus={focusEventId === evt.id}
                    isOpen={openEvent === idx}
                    onToggle={() => setOpenEvent(openEvent === idx ? null : idx)} />
                ))}
              </div>
            </>
          ) : (
            <div className="flex-1 flex items-center justify-center text-sm text-gray-400">
              Select a process from the tree to view its timeline
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
