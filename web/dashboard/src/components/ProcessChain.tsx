import { useState, useMemo } from 'react'

interface TelemetryEvent {
  id: number; timestamp: string; event_name: string; event_category: string
  pid: number; tid: number; process_name: string; process_exe: string
  process_cmdline: string; parent_pid: number; parent_name: string
  params: Record<string, unknown>
}

interface ProcessNode {
  pid: number
  name: string
  exe: string
  cmdline: string
  parentPid: number
  parentName: string
  events: TelemetryEvent[]
  eventsByCategory: Record<string, TelemetryEvent[]>
  childPids: number[]
  isTrigger: boolean
}

interface Props {
  events: TelemetryEvent[]
  focusPids: Record<number, boolean>
  onLoadContext?: (pid: number) => void
  loadingPid?: number | null
}

const catStyle: Record<string, { bg: string; text: string; border: string }> = {
  process:  { bg: 'bg-blue-50 dark:bg-blue-950/40',    text: 'text-blue-700 dark:text-blue-400',       border: 'border-blue-200 dark:border-blue-800' },
  file:     { bg: 'bg-amber-50 dark:bg-amber-950/40',   text: 'text-amber-700 dark:text-amber-400',     border: 'border-amber-200 dark:border-amber-800' },
  registry: { bg: 'bg-purple-50 dark:bg-purple-950/40',  text: 'text-purple-700 dark:text-purple-400',   border: 'border-purple-200 dark:border-purple-800' },
  net:      { bg: 'bg-emerald-50 dark:bg-emerald-950/40',text: 'text-emerald-700 dark:text-emerald-400', border: 'border-emerald-200 dark:border-emerald-800' },
  image:    { bg: 'bg-indigo-50 dark:bg-indigo-950/40',  text: 'text-indigo-700 dark:text-indigo-400',   border: 'border-indigo-200 dark:border-indigo-800' },
  dns:      { bg: 'bg-teal-50 dark:bg-teal-950/40',     text: 'text-teal-700 dark:text-teal-400',       border: 'border-teal-200 dark:border-teal-800' },
  thread:   { bg: 'bg-pink-50 dark:bg-pink-950/40',     text: 'text-pink-700 dark:text-pink-400',       border: 'border-pink-200 dark:border-pink-800' },
  mem:      { bg: 'bg-rose-50 dark:bg-rose-950/40',     text: 'text-rose-700 dark:text-rose-400',       border: 'border-rose-200 dark:border-rose-800' },
  handle:   { bg: 'bg-gray-50 dark:bg-gray-900/40',     text: 'text-gray-700 dark:text-gray-400',       border: 'border-gray-200 dark:border-gray-700' },
}

function getCatStyle(cat: string) {
  return catStyle[cat] || catStyle.handle
}

function evtDetail(evt: TelemetryEvent): string {
  const p = evt.params || {}
  switch (evt.event_category) {
    case 'file': return String(p.file_name || p.file_object || p.path || '')
    case 'registry': return String(p.key_name || p.key || p.path || '')
    case 'net': return [p.dip, p.dport].filter(Boolean).join(':') || String(p.sip || '')
    case 'dns': return String(p.name || p.domain || '')
    case 'image': return String(p.file_name || p.image_name || '')
    case 'process': return String(p.exe || p.cmdline || '')
    default: return ''
  }
}

export default function ProcessChain({ events, focusPids, onLoadContext, loadingPid }: Props) {
  const [expandedCats, setExpandedCats] = useState<Set<string>>(new Set())
  const [selectedNode, setSelectedNode] = useState<number | null>(null)

  // Build process nodes from events
  const { nodes, chain } = useMemo(() => {
    const byPid = new Map<number, ProcessNode>()

    for (const evt of events) {
      if (evt.pid <= 0) continue
      let node = byPid.get(evt.pid)
      if (!node) {
        node = {
          pid: evt.pid, name: evt.process_name, exe: evt.process_exe || '',
          cmdline: evt.process_cmdline || '', parentPid: evt.parent_pid,
          parentName: evt.parent_name || '', events: [], eventsByCategory: {},
          childPids: [], isTrigger: !!focusPids[evt.pid],
        }
        byPid.set(evt.pid, node)
      }
      node.events.push(evt)
      if (!node.eventsByCategory[evt.event_category]) node.eventsByCategory[evt.event_category] = []
      node.eventsByCategory[evt.event_category].push(evt)
      if (!node.cmdline && evt.process_cmdline) node.cmdline = evt.process_cmdline
      if (!node.exe && evt.process_exe) node.exe = evt.process_exe
    }

    // Build child links
    for (const node of byPid.values()) {
      if (node.parentPid > 0 && byPid.has(node.parentPid) && node.parentPid !== node.pid) {
        const parent = byPid.get(node.parentPid)!
        if (!parent.childPids.includes(node.pid)) parent.childPids.push(node.pid)
      }
    }

    // Build the chain: walk up from trigger to root, then down from trigger
    const triggerPid = Object.keys(focusPids).map(Number).find(p => byPid.has(p))
    const chain: number[] = []

    if (triggerPid) {
      // Walk up
      const ancestors: number[] = []
      let cur = byPid.get(triggerPid)
      for (let i = 0; i < 20 && cur; i++) {
        if (cur.parentPid > 0 && byPid.has(cur.parentPid) && cur.parentPid !== cur.pid) {
          ancestors.unshift(cur.parentPid)
          cur = byPid.get(cur.parentPid)
        } else break
      }
      chain.push(...ancestors, triggerPid)

      // Walk down from trigger (first level children only for now)
      for (const cpid of byPid.get(triggerPid)?.childPids || []) {
        chain.push(cpid)
      }
    } else {
      // No trigger found — show all
      chain.push(...byPid.keys())
    }

    return { nodes: byPid, chain }
  }, [events, focusPids])

  // Auto-expand trigger's event categories
  useMemo(() => {
    const auto = new Set<string>()
    for (const pid of Object.keys(focusPids).map(Number)) {
      const node = nodes.get(pid)
      if (node) {
        for (const cat of Object.keys(node.eventsByCategory)) {
          auto.add(`${pid}-${cat}`)
        }
      }
    }
    if (auto.size > 0 && expandedCats.size === 0) setExpandedCats(auto)
  }, [nodes, focusPids]) // eslint-disable-line

  const toggleCat = (key: string) => {
    setExpandedCats(prev => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key); else next.add(key)
      return next
    })
  }

  if (chain.length === 0) {
    return <div className="flex items-center justify-center h-64 text-sm text-gray-400">No process events found for this detection.</div>
  }

  return (
    <div className="overflow-auto p-4">
      {/* Horizontal process chain */}
      <div className="flex items-start gap-0 min-w-max">
        {chain.map((pid, idx) => {
          const node = nodes.get(pid)
          if (!node) return null
          const isSelected = selectedNode === pid

          return (
            <div key={pid} className="flex items-start">
              {/* Connector arrow */}
              {idx > 0 && (
                <div className="flex items-center self-center shrink-0 mt-8">
                  <div className="w-8 h-px bg-gray-300 dark:bg-slate-600" />
                  <div className="w-0 h-0 border-t-[5px] border-t-transparent border-b-[5px] border-b-transparent border-l-[8px] border-l-gray-300 dark:border-l-slate-600 -ml-px" />
                </div>
              )}

              {/* Process node */}
              <div className="flex flex-col items-center shrink-0">
                <div
                  onClick={() => setSelectedNode(isSelected ? null : pid)}
                  className={
                    'rounded-xl border-2 p-3 cursor-pointer transition-all w-64 ' +
                    (node.isTrigger
                      ? 'border-red-400 dark:border-red-600 bg-red-50 dark:bg-red-950/30 shadow-lg shadow-red-100 dark:shadow-red-900/20'
                      : isSelected
                        ? 'border-fibratus-400 bg-fibratus-50/50 dark:bg-fibratus-950/20 shadow-md'
                        : 'border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 hover:border-gray-300 dark:hover:border-slate-600 hover:shadow-md')
                  }
                >
                  <div className="flex items-center gap-2">
                    <span className={'rounded px-1.5 py-0.5 text-[10px] font-bold ' +
                      (node.isTrigger ? 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-400' : 'bg-gray-100 text-gray-600 dark:bg-slate-700 dark:text-slate-400')}>
                      {node.pid}
                    </span>
                    <span className={'font-semibold text-sm truncate ' +
                      (node.isTrigger ? 'text-red-700 dark:text-red-400' : 'text-gray-900 dark:text-slate-100')}>
                      {node.name}
                    </span>
                    {node.isTrigger && <span className="rounded bg-red-500 text-white text-[9px] px-1.5 py-0.5 font-bold shrink-0">TRIGGER</span>}
                  </div>
                  {node.exe && <div className="mt-1.5 text-[10px] font-mono text-gray-400 dark:text-slate-500 truncate">{node.exe}</div>}

                  {/* Event category summary chips */}
                  <div className="mt-2 flex flex-wrap gap-1">
                    {Object.entries(node.eventsByCategory).map(([cat, evts]) => {
                      const s = getCatStyle(cat)
                      const key = `${pid}-${cat}`
                      const isExp = expandedCats.has(key)
                      return (
                        <button key={cat} onClick={(e) => { e.stopPropagation(); toggleCat(key) }}
                          className={`rounded-md px-1.5 py-0.5 text-[10px] font-medium border transition-colors ${s.bg} ${s.text} ${s.border} ${isExp ? 'ring-1 ring-offset-1' : 'opacity-80 hover:opacity-100'}`}>
                          {cat} ({evts.length})
                        </button>
                      )
                    })}
                  </div>

                  {/* Walk up / children indicators */}
                  <div className="mt-2 flex items-center gap-2 text-[10px]">
                    {node.parentPid > 0 && !nodes.has(node.parentPid) && onLoadContext && (
                      <button onClick={(e) => { e.stopPropagation(); onLoadContext(node.pid) }}
                        disabled={loadingPid === node.pid}
                        className="text-fibratus-600 hover:underline disabled:opacity-50">
                        {loadingPid === node.pid ? 'Loading...' : 'Load parent'}
                      </button>
                    )}
                    {node.childPids.length > 0 && (
                      <span className="text-gray-400 dark:text-slate-500">{node.childPids.length} children</span>
                    )}
                  </div>
                </div>

                {/* Expanded event categories */}
                {Object.entries(node.eventsByCategory).map(([cat, evts]) => {
                  const key = `${pid}-${cat}`
                  if (!expandedCats.has(key)) return null
                  const s = getCatStyle(cat)
                  return (
                    <div key={cat} className={`mt-2 w-64 rounded-lg border ${s.border} ${s.bg} p-2 max-h-48 overflow-auto`}>
                      <div className={`text-[10px] font-bold ${s.text} mb-1`}>{cat.toUpperCase()} — {evts.length} events</div>
                      <div className="space-y-0.5">
                        {evts.slice(0, 20).map(evt => (
                          <div key={evt.id} className="text-[10px] font-mono text-gray-600 dark:text-slate-400 truncate flex gap-1">
                            <span className={`shrink-0 ${s.text}`}>{evt.event_name}</span>
                            <span className="text-gray-400 dark:text-slate-500 truncate">{evtDetail(evt)}</span>
                          </div>
                        ))}
                        {evts.length > 20 && <div className="text-[9px] text-gray-400">+{evts.length - 20} more</div>}
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
