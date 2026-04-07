import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Agent, type Command } from '../lib/api'
import StatusBadge from '../components/StatusBadge'
import SlidePanel from '../components/SlidePanel'
import ConfirmDialog from '../components/ConfirmDialog'
import RemoteShell from '../components/RemoteShell'
import FileBrowser from '../components/FileBrowser'
import ProcessInvestigation from '../components/ProcessInvestigation'
import { useTableSort } from '../hooks/useTableSort'
import SortableHeader from '../components/SortableHeader'

interface TelemetryEvent {
  id: number; timestamp: string; event_name: string; event_category: string;
  pid: number; tid: number; process_name: string; process_exe: string;
  process_cmdline: string; parent_pid: number; parent_name: string; params: unknown;
  raw_event: unknown;
}

const categoryColors: Record<string, string> = {
  process: 'bg-blue-100 text-blue-700',
  net: 'bg-green-100 text-green-700',
  file: 'bg-yellow-100 text-yellow-700',
  registry: 'bg-purple-100 text-purple-700',
  image: 'bg-indigo-100 text-indigo-700',
  dns: 'bg-teal-100 text-teal-700',
}

function AgentEventsTab({ agentId }: { agentId: string }) {
  const [limit, setLimit] = useState(500)
  const [live, setLive] = useState(false)
  const [filter, setFilter] = useState('')

  const { data: res, refetch } = useQuery({
    queryKey: ['agent-events', agentId, limit],
    queryFn: () => api.getAgentEvents(agentId, limit),
    refetchInterval: live ? 5000 : false,
  })

  const allEvents = (res?.data || []) as TelemetryEvent[]
  const events = filter
    ? allEvents.filter(e =>
        e.event_name.toLowerCase().includes(filter.toLowerCase()) ||
        e.process_name.toLowerCase().includes(filter.toLowerCase()) ||
        (e.process_cmdline || '').toLowerCase().includes(filter.toLowerCase()))
    : allEvents

  const [expandedIdx, setExpandedIdx] = useState<number | null>(null)

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-3">
        <input value={filter} onChange={e => setFilter(e.target.value)} placeholder="Filter events..."
          className="flex-1 rounded border px-3 py-1.5 text-sm focus:ring-2 focus:ring-fibratus-500 focus:outline-none" />
        <label className="flex items-center gap-1.5 text-xs text-gray-500 dark:text-slate-400 cursor-pointer select-none">
          <input type="checkbox" checked={live} onChange={e => setLive(e.target.checked)} className="rounded" />
          Live
        </label>
        <button onClick={() => refetch()} className="text-xs text-fibratus-600 hover:underline">Refresh</button>
        <select value={limit} onChange={e => setLimit(Number(e.target.value))} className="text-xs border rounded px-2 py-1">
          <option value={100}>100</option>
          <option value={500}>500</option>
          <option value={1000}>1000</option>
          <option value={5000}>5000</option>
        </select>
        <span className="text-xs text-gray-400 dark:text-slate-500">{events.length} events</span>
      </div>

      <div className="border rounded overflow-auto" style={{ maxHeight: 'calc(100vh - 220px)' }}>
        <table className="w-full text-left text-xs">
          <thead className="bg-gray-50 dark:bg-slate-900 sticky top-0 z-10">
            <tr>
              <th className="px-3 py-2 font-medium text-gray-500 dark:text-slate-400 w-28">Time</th>
              <th className="px-3 py-2 font-medium text-gray-500 dark:text-slate-400 w-28">Event</th>
              <th className="px-3 py-2 font-medium text-gray-500 dark:text-slate-400 w-14">PID</th>
              <th className="px-3 py-2 font-medium text-gray-500 dark:text-slate-400 w-40">Process</th>
              <th className="px-3 py-2 font-medium text-gray-500 dark:text-slate-400">Command Line / Details</th>
            </tr>
          </thead>
          <tbody>
            {events.map((evt, idx) => {
              const isOpen = expandedIdx === idx
              return (
                <tr key={`${evt.id}-${idx}`} className={'border-t border-gray-100 dark:border-slate-700 cursor-pointer ' + (isOpen ? 'bg-gray-50 dark:bg-slate-700' : 'hover:bg-gray-50/50 dark:hover:bg-slate-700/30')}
                  onClick={() => setExpandedIdx(isOpen ? null : idx)}>
                  <td className="px-3 py-1.5 text-gray-500 dark:text-slate-400 tabular-nums whitespace-nowrap font-mono align-top">
                    {new Date(evt.timestamp).toLocaleTimeString()}
                  </td>
                  <td className="px-3 py-1.5 align-top">
                    <span className={'inline-block px-1.5 py-0.5 rounded text-xs font-medium ' +
                      (categoryColors[evt.event_category] || 'bg-gray-100 text-gray-700')}>{evt.event_name}</span>
                  </td>
                  <td className="px-3 py-1.5 text-gray-500 dark:text-slate-400 tabular-nums font-mono align-top">{evt.pid}</td>
                  <td className="px-3 py-1.5 font-mono text-gray-700 dark:text-slate-300 align-top">{evt.process_name}</td>
                  <td className="px-3 py-1.5 text-gray-500 dark:text-slate-400 font-mono align-top">
                    {!isOpen ? (
                      <span className="break-all block">{evt.process_cmdline || evt.process_exe || '-'}</span>
                    ) : (
                      <div className="space-y-2" onClick={e => e.stopPropagation()}>
                        <div className="text-gray-700 dark:text-slate-300 break-all whitespace-pre-wrap">{evt.process_cmdline || evt.process_exe || '-'}</div>
                        {evt.parent_name && <div className="text-gray-400 dark:text-slate-500">Parent: {evt.parent_name} (PID {evt.parent_pid})</div>}
                        <pre className="text-xs bg-gray-900 text-gray-100 rounded p-2 max-h-48 overflow-auto whitespace-pre-wrap break-all">
{JSON.stringify(evt.params, null, 2)}
                        </pre>
                      </div>
                    )}
                  </td>
                </tr>
              )
            })}
            {events.length === 0 && (
              <tr><td colSpan={5} className="px-3 py-8 text-center text-gray-400 dark:text-slate-500">No events</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

export default function Agents() {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [page, setPage] = useState(1)
  const [selectedAgent, setSelectedAgent] = useState<Agent | null>(null)
  const [agentToDelete, setAgentToDelete] = useState<Agent | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Agent | null>(null)
  const [activeTab, setActiveTab] = useState<'details' | 'response' | 'terminal' | 'files' | 'events' | 'processes' | 'history'>('details')
  const [shellType, setShellType] = useState<'cmd' | 'powershell'>('powershell')

  // Command input state
  const [cmdType, setCmdType] = useState('')
  const [cmdInput, setCmdInput] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['agents', page, statusFilter, search],
    queryFn: () => api.getAgents({ page: String(page), status: statusFilter, search }),
  })

  const { data: cmdData, refetch: refetchCmds } = useQuery({
    queryKey: ['agent-commands', selectedAgent?.id],
    queryFn: () => selectedAgent ? api.getAgentCommands(selectedAgent.id) : Promise.resolve({ data: [] }),
    enabled: !!selectedAgent,
    refetchInterval: 5000,
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteAgent(id),
    onSuccess: () => {
      setDeleteTarget(null)
      setAgentToDelete(null)
      setSelectedAgent(null)
      queryClient.invalidateQueries({ queryKey: ['agents'] })
    },
  })

  const commandMutation = useMutation({
    mutationFn: (args: { agentId: string; type: string; payload?: Record<string, unknown> }) =>
      api.createCommand(args.agentId, args.type, args.payload),
    onSuccess: () => {
      setCmdType('')
      setCmdInput('')
      refetchCmds()
    },
  })

  const agents = (data?.data || []) as Agent[]
  const { sorted: sortedAgents, sort, toggleSort } = useTableSort(agents, 'hostname', 'asc')
  const total = data?.meta?.total ?? 0
  const perPage = data?.meta?.per_page ?? 50
  const totalPages = Math.ceil(total / perPage)
  const commands = (cmdData?.data || []) as Command[]

  const sendCommand = (type: string, payload?: Record<string, unknown>) => {
    if (!selectedAgent) return
    commandMutation.mutate({ agentId: selectedAgent.id, type, payload })
  }

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-slate-100">Agents</h1>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">{total} agent(s) registered</p>
        </div>
      </div>

      {/* Filters */}
      <div className="mt-6 flex gap-4">
        <input
          type="text"
          placeholder="Search by hostname..."
          value={search}
          onChange={(e) => { setSearch(e.target.value); setPage(1) }}
          className="rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-4 py-2 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
        />
        <select
          value={statusFilter}
          onChange={(e) => { setStatusFilter(e.target.value); setPage(1) }}
          className="rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-4 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
        >
          <option value="">All statuses</option>
          <option value="online">Online</option>
          <option value="offline">Offline</option>
          <option value="stale">Stale</option>
        </select>
      </div>

      {/* Agent table */}
      <div className="mt-6 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
              <tr>
                <SortableHeader label="Hostname" sortKey="hostname" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Status" sortKey="status" sort={sort} onSort={toggleSort} />
                <SortableHeader label="OS" sortKey="os_version" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Engine" sortKey="engine_version" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Last Heartbeat" sortKey="last_heartbeat" sort={sort} onSort={toggleSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Registered</th>
                <th className="px-6 py-3 w-16"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {isLoading && (
                <tr><td colSpan={7} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">Loading...</td></tr>
              )}
              {!isLoading && sortedAgents.map((agent) => (
                <tr key={agent.id} className="cursor-pointer hover:bg-gray-50/50 dark:hover:bg-slate-700/30" onClick={() => window.location.href = `/agents/${agent.id}`}>
                  <td className="px-6 py-3">
                    <span className="font-medium text-gray-900 dark:text-slate-100">{agent.hostname}</span>
                    <span className="ml-2 text-xs text-gray-400 dark:text-slate-500">{agent.id.slice(0, 8)}</span>
                  </td>
                  <td className="px-6 py-3"><StatusBadge status={agent.status} /></td>
                  <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{agent.os_version}</td>
                  <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{agent.engine_version}</td>
                  <td className="px-6 py-3 text-gray-500 dark:text-slate-400">{agent.last_heartbeat ? timeAgo(new Date(agent.last_heartbeat)) : 'Never'}</td>
                  <td className="px-6 py-3 text-gray-500 dark:text-slate-400">{new Date(agent.registered_at).toLocaleDateString()}</td>
                  <td className="px-6 py-3">
                    <button
                      onClick={(e) => { e.stopPropagation(); setAgentToDelete(agent) }}
                      className="rounded p-1 text-gray-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-950/30 transition-colors"
                      title="Delete agent"
                    >
                      <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" /></svg>
                    </button>
                  </td>
                </tr>
              ))}
              {!isLoading && sortedAgents.length === 0 && (
                <tr><td colSpan={7} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">No agents enrolled. Create an enrollment token in Settings.</td></tr>
              )}
            </tbody>
          </table>
        </div>
        {total > perPage && (
          <div className="flex items-center justify-between border-t border-gray-100 dark:border-slate-700 px-6 py-3">
            <span className="text-sm text-gray-500 dark:text-slate-400">Page {page} of {totalPages}</span>
            <div className="flex gap-2">
              <button onClick={() => setPage(Math.max(1, page - 1))} disabled={page <= 1} className="rounded-lg border border-gray-300 dark:border-slate-600 px-3 py-1.5 text-sm text-gray-700 dark:text-slate-300 disabled:opacity-50">Previous</button>
              <button onClick={() => setPage(page + 1)} disabled={page >= totalPages} className="rounded-lg border border-gray-300 dark:border-slate-600 px-3 py-1.5 text-sm text-gray-700 dark:text-slate-300 disabled:opacity-50">Next</button>
            </div>
          </div>
        )}
      </div>

      {/* Delete confirmation */}
      <ConfirmDialog
        open={!!agentToDelete}
        title={`Delete ${agentToDelete?.hostname || 'agent'}?`}
        message={`This will remove the agent from the fleet. If the agent is still running, it will be automatically uninstalled when it next connects to the server.`}
        confirmLabel="Delete Agent"
        onConfirm={() => agentToDelete && deleteMutation.mutate(agentToDelete.id)}
        onCancel={() => setAgentToDelete(null)}
      />

      {/* Agent detail + response slide-out */}
      <SlidePanel open={!!selectedAgent} title={selectedAgent?.hostname || 'Agent'} onClose={() => setSelectedAgent(null)} wide>
        {selectedAgent && (
          <div>
            {/* Tabs */}
            <div className="flex gap-1 border-b border-gray-200 dark:border-slate-700 mb-6">
              {(['details', 'response', 'terminal', 'files', 'events', 'processes', 'history'] as const).map(tab => (
                <button
                  key={tab}
                  onClick={() => setActiveTab(tab)}
                  className={'px-4 py-2 text-sm font-medium border-b-2 -mb-px capitalize ' +
                    (activeTab === tab ? 'border-fibratus-600 text-fibratus-600' : 'border-transparent text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300')}
                >
                  {tab === 'response' ? 'Active Response' : tab === 'history' ? 'Command History' : tab === 'terminal' ? 'Terminal' : tab === 'files' ? 'File Browser' : tab === 'events' ? 'Events' : tab === 'processes' ? 'Process Tree' : tab}
                </button>
              ))}
            </div>

            {/* Details Tab */}
            {activeTab === 'details' && (
              <div className="space-y-4">
                <div className="flex items-center gap-3">
                  <StatusBadge status={selectedAgent.status} />
                  <span className="text-sm text-gray-500 dark:text-slate-400 font-mono">{selectedAgent.id}</span>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  {[
                    ['OS Version', selectedAgent.os_version],
                    ['Engine Version', selectedAgent.engine_version],
                    ['Group', selectedAgent.group_name || selectedAgent.group_id || 'None'],
                    ['Last Heartbeat', selectedAgent.last_heartbeat ? timeAgo(new Date(selectedAgent.last_heartbeat)) : 'Never'],
                    ['Registered', new Date(selectedAgent.registered_at).toLocaleString()],
                    ['Organization', selectedAgent.org_id?.slice(0, 12) + '...'],
                  ].map(([label, value]) => (
                    <div key={label} className="rounded-lg bg-gray-50 dark:bg-slate-900 px-3 py-2">
                      <span className="text-xs text-gray-500 dark:text-slate-400">{label}</span>
                      <p className="text-sm font-medium text-gray-900 dark:text-slate-100">{value}</p>
                    </div>
                  ))}
                </div>
                <div className="pt-4 border-t border-gray-200 dark:border-slate-700">
                  <button
                    onClick={() => setDeleteTarget(selectedAgent)}
                    className="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700"
                  >
                    Remove Agent
                  </button>
                </div>
              </div>
            )}

            {/* Active Response Tab */}
            {activeTab === 'response' && (
              <div className="space-y-4">
                {/* Quick actions */}
                <div>
                  <h4 className="text-sm font-medium text-gray-700 dark:text-slate-300 mb-3">Quick Actions</h4>
                  <div className="grid grid-cols-2 gap-2">
                    <button onClick={() => sendCommand('isolate')} className="rounded-lg border border-red-300 bg-red-50 px-3 py-2.5 text-sm font-medium text-red-700 hover:bg-red-100">
                      Isolate Host
                    </button>
                    <button onClick={() => sendCommand('unisolate')} className="rounded-lg border border-emerald-300 bg-emerald-50 px-3 py-2.5 text-sm font-medium text-emerald-700 hover:bg-emerald-100">
                      Unisolate Host
                    </button>
                    <button onClick={() => sendCommand('collect_info')} className="rounded-lg border border-blue-300 bg-blue-50 px-3 py-2.5 text-sm font-medium text-blue-700 hover:bg-blue-100">
                      Collect System Info
                    </button>
                    <button onClick={() => setDeleteTarget(selectedAgent)} className="rounded-lg border border-gray-300 dark:border-slate-600 bg-gray-50 dark:bg-slate-700 px-3 py-2.5 text-sm font-medium text-gray-700 dark:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-600">
                      Uninstall Agent
                    </button>
                  </div>
                </div>

                {/* Command input */}
                <div className="border-t border-gray-200 dark:border-slate-700 pt-4">
                  <h4 className="text-sm font-medium text-gray-700 dark:text-slate-300 mb-3">Send Command</h4>
                  <select
                    value={cmdType}
                    onChange={(e) => setCmdType(e.target.value)}
                    className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 mb-2"
                  >
                    <option value="">Select command type...</option>
                    <option value="kill_process">Kill Process</option>
                    <option value="list_directory">Browse Directory</option>
                    <option value="get_file">Pull File</option>
                    <option value="run_command">Run Shell Command</option>
                  </select>

                  {cmdType === 'kill_process' && (
                    <input type="number" placeholder="Process ID (PID)" value={cmdInput} onChange={(e) => setCmdInput(e.target.value)}
                      className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 mb-2" />
                  )}
                  {cmdType === 'list_directory' && (
                    <input type="text" placeholder="Directory path (e.g., C:\Users)" value={cmdInput} onChange={(e) => setCmdInput(e.target.value)}
                      className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 mb-2" />
                  )}
                  {cmdType === 'get_file' && (
                    <input type="text" placeholder="File path (e.g., C:\Windows\System32\drivers\etc\hosts)" value={cmdInput} onChange={(e) => setCmdInput(e.target.value)}
                      className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 mb-2" />
                  )}
                  {cmdType === 'run_command' && (
                    <input type="text" placeholder="Command (e.g., whoami /all)" value={cmdInput} onChange={(e) => setCmdInput(e.target.value)}
                      className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 font-mono mb-2" />
                  )}

                  {cmdType && (
                    <button
                      onClick={() => {
                        const payload: Record<string, unknown> = {}
                        if (cmdType === 'kill_process') payload.pid = parseInt(cmdInput)
                        if (cmdType === 'list_directory') payload.path = cmdInput || 'C:\\'
                        if (cmdType === 'get_file') payload.path = cmdInput
                        if (cmdType === 'run_command') payload.command = cmdInput
                        sendCommand(cmdType, payload)
                      }}
                      disabled={commandMutation.isPending}
                      className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
                    >
                      {commandMutation.isPending ? 'Sending...' : 'Execute'}
                    </button>
                  )}
                </div>
              </div>
            )}

            {/* Terminal Tab */}
            {activeTab === 'terminal' && (
              <div className="space-y-3">
                <div className="flex items-center gap-2">
                  <span className="text-sm text-gray-500 dark:text-slate-400">Shell:</span>
                  <button
                    onClick={() => setShellType('powershell')}
                    className={'px-3 py-1 text-xs rounded-full font-medium ' + (shellType === 'powershell' ? 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400' : 'bg-gray-100 dark:bg-slate-700 text-gray-600 dark:text-slate-400')}
                  >
                    PowerShell
                  </button>
                  <button
                    onClick={() => setShellType('cmd')}
                    className={'px-3 py-1 text-xs rounded-full font-medium ' + (shellType === 'cmd' ? 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400' : 'bg-gray-100 dark:bg-slate-700 text-gray-600 dark:text-slate-400')}
                  >
                    CMD
                  </button>
                </div>
                <RemoteShell
                  agentId={selectedAgent.id}
                  hostname={selectedAgent.hostname}
                  shellType={shellType}
                />
              </div>
            )}

            {/* File Browser Tab */}
            {activeTab === 'files' && (
              <FileBrowser agentId={selectedAgent.id} />
            )}

            {/* Agent Events Tab */}
            {activeTab === 'events' && (
              <AgentEventsTab agentId={selectedAgent.id} />
            )}

            {/* Process Tree Tab */}
            {activeTab === 'processes' && (
              <ProcessInvestigation agentId={selectedAgent.id} />
            )}

            {/* Command History Tab */}
            {activeTab === 'history' && (
              <div className="space-y-3">
                {commands.length === 0 && (
                  <p className="text-sm text-gray-400 dark:text-slate-500 text-center py-8">No commands sent to this agent yet.</p>
                )}
                {commands.map((cmd) => (
                  <div key={cmd.id} className="rounded-lg border border-gray-200 dark:border-slate-700 p-3">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-gray-900 dark:text-slate-100 capitalize">{cmd.type.replace('_', ' ')}</span>
                        <span className={'inline-flex rounded-full px-2 py-0.5 text-xs font-medium ' + statusColor(cmd.status)}>
                          {cmd.status}
                        </span>
                        {(cmd as unknown as { created_by_email?: string }).created_by_email && (
                          <span className="text-xs text-gray-400 dark:text-slate-500">by {(cmd as unknown as { created_by_email: string }).created_by_email}</span>
                        )}
                      </div>
                      <span className="text-xs text-gray-500 dark:text-slate-400">{new Date(cmd.created_at).toLocaleString()}</span>
                    </div>
                    {cmd.error_message && (
                      <p className="mt-1 text-xs text-red-600">{cmd.error_message}</p>
                    )}
                    {cmd.result != null && cmd.status === 'completed' && (
                      <details className="mt-2">
                        <summary className="text-xs text-fibratus-600 cursor-pointer hover:text-fibratus-800">View result</summary>
                        <pre className="mt-1 max-h-60 overflow-auto rounded-lg bg-gray-900 p-3 text-xs text-gray-100 font-mono">
                          {JSON.stringify(cmd.result, null, 2)}
                        </pre>
                      </details>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </SlidePanel>

      {/* Delete confirmation */}
      <ConfirmDialog
        open={!!deleteTarget}
        title="Remove Agent"
        message={'Are you sure you want to remove ' + (deleteTarget?.hostname || '') + '? This cannot be undone.'}
        confirmLabel="Remove"
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}

function timeAgo(date: Date): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
  if (seconds < 60) return seconds + 's ago'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return minutes + 'm ago'
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return hours + 'h ago'
  return Math.floor(hours / 24) + 'd ago'
}

function statusColor(status: string): string {
  switch (status) {
    case 'pending': return 'bg-yellow-100 text-yellow-800'
    case 'running': return 'bg-blue-100 text-blue-800'
    case 'completed': return 'bg-emerald-100 text-emerald-800'
    case 'failed': return 'bg-red-100 text-red-800'
    default: return 'bg-gray-100 text-gray-800'
  }
}
