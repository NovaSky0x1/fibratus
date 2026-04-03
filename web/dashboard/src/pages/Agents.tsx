import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Agent, type Command } from '../lib/api'
import StatusBadge from '../components/StatusBadge'
import SlidePanel from '../components/SlidePanel'
import ConfirmDialog from '../components/ConfirmDialog'
import RemoteShell from '../components/RemoteShell'
import FileBrowser from '../components/FileBrowser'

function AgentEventsTab({ agentId }: { agentId: string }) {
  const { data: res, isLoading } = useQuery({
    queryKey: ['agent-events', agentId],
    queryFn: () => api.getAgentEvents(agentId, 200),
    refetchInterval: 5000,
  })

  const events = (res?.data || []) as Array<{
    id: number; timestamp: string; event_name: string; event_category: string;
    pid: number; tid: number; process_name: string; process_exe: string;
    process_cmdline: string; parent_name: string; params: unknown;
  }>

  const [expanded, setExpanded] = useState<number | null>(null)

  if (isLoading) return <div className="text-gray-400 py-8 text-center text-sm">Loading events...</div>

  return (
    <div className="space-y-2">
      <div className="text-xs text-gray-400 mb-2">{events.length} events (auto-refreshing)</div>
      <div className="rounded-xl border border-gray-200 bg-white shadow-sm overflow-hidden">
        <table className="w-full text-left text-xs">
          <thead className="border-b border-gray-100 bg-gray-50/50">
            <tr>
              <th className="px-3 py-2 font-medium text-gray-500 w-40">Timestamp</th>
              <th className="px-3 py-2 font-medium text-gray-500 w-32">Event</th>
              <th className="px-3 py-2 font-medium text-gray-500 w-16">PID</th>
              <th className="px-3 py-2 font-medium text-gray-500">Process</th>
              <th className="px-3 py-2 font-medium text-gray-500">Details</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-50">
            {events.map(evt => (
              <>
                <tr key={evt.id} className="hover:bg-gray-50/50 cursor-pointer" onClick={() => setExpanded(expanded === evt.id ? null : evt.id)}>
                  <td className="px-3 py-1.5 text-gray-500 tabular-nums whitespace-nowrap font-mono">
                    {new Date(evt.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', fractionalSecondDigits: 3 } as Intl.DateTimeFormatOptions)}
                  </td>
                  <td className="px-3 py-1.5">
                    <span className={'inline-block px-1.5 py-0.5 rounded text-xs font-medium ' + (
                      evt.event_category === 'process' ? 'bg-blue-100 text-blue-700' :
                      evt.event_category === 'net' ? 'bg-green-100 text-green-700' :
                      evt.event_category === 'file' ? 'bg-yellow-100 text-yellow-700' :
                      evt.event_category === 'registry' ? 'bg-purple-100 text-purple-700' :
                      evt.event_category === 'image' ? 'bg-indigo-100 text-indigo-700' :
                      'bg-gray-100 text-gray-700'
                    )}>{evt.event_name}</span>
                  </td>
                  <td className="px-3 py-1.5 text-gray-500 tabular-nums font-mono">{evt.pid}</td>
                  <td className="px-3 py-1.5 font-mono text-gray-700 truncate max-w-[200px]">{evt.process_name}</td>
                  <td className="px-3 py-1.5 text-gray-500 truncate max-w-[300px] font-mono">{evt.process_cmdline || evt.process_exe || '-'}</td>
                </tr>
                {expanded === evt.id && (
                  <tr key={`${evt.id}-detail`}>
                    <td colSpan={5} className="px-4 py-3 bg-gray-50">
                      <pre className="text-xs font-mono whitespace-pre-wrap break-all max-h-48 overflow-auto">
                        {JSON.stringify(evt.params, null, 2)}
                      </pre>
                    </td>
                  </tr>
                )}
              </>
            ))}
            {events.length === 0 && (
              <tr><td colSpan={5} className="px-3 py-8 text-center text-gray-400">No events yet</td></tr>
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
  const [deleteTarget, setDeleteTarget] = useState<Agent | null>(null)
  const [activeTab, setActiveTab] = useState<'details' | 'response' | 'terminal' | 'files' | 'events' | 'history'>('details')
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
          <h1 className="text-2xl font-bold text-gray-900">Agents</h1>
          <p className="mt-1 text-sm text-gray-500">{total} agent(s) registered</p>
        </div>
      </div>

      {/* Filters */}
      <div className="mt-6 flex gap-4">
        <input
          type="text"
          placeholder="Search by hostname..."
          value={search}
          onChange={(e) => { setSearch(e.target.value); setPage(1) }}
          className="rounded-lg border border-gray-300 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
        />
        <select
          value={statusFilter}
          onChange={(e) => { setStatusFilter(e.target.value); setPage(1) }}
          className="rounded-lg border border-gray-300 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
        >
          <option value="">All statuses</option>
          <option value="online">Online</option>
          <option value="offline">Offline</option>
          <option value="stale">Stale</option>
        </select>
      </div>

      {/* Agent table */}
      <div className="mt-6 rounded-xl border border-gray-200 bg-white shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 bg-gray-50/50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-500">Hostname</th>
                <th className="px-6 py-3 font-medium text-gray-500">Status</th>
                <th className="px-6 py-3 font-medium text-gray-500">OS</th>
                <th className="px-6 py-3 font-medium text-gray-500">Engine</th>
                <th className="px-6 py-3 font-medium text-gray-500">Last Heartbeat</th>
                <th className="px-6 py-3 font-medium text-gray-500">Registered</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading && (
                <tr><td colSpan={6} className="px-6 py-12 text-center text-gray-400">Loading...</td></tr>
              )}
              {!isLoading && agents.map((agent) => (
                <tr key={agent.id} className="cursor-pointer hover:bg-gray-50/50" onClick={() => { setSelectedAgent(agent); setActiveTab('details') }}>
                  <td className="px-6 py-3">
                    <span className="font-medium text-gray-900">{agent.hostname}</span>
                    <span className="ml-2 text-xs text-gray-400">{agent.id.slice(0, 8)}</span>
                  </td>
                  <td className="px-6 py-3"><StatusBadge status={agent.status} /></td>
                  <td className="px-6 py-3 text-gray-600">{agent.os_version}</td>
                  <td className="px-6 py-3 text-gray-600">{agent.engine_version}</td>
                  <td className="px-6 py-3 text-gray-500">{agent.last_heartbeat ? timeAgo(new Date(agent.last_heartbeat)) : 'Never'}</td>
                  <td className="px-6 py-3 text-gray-500">{new Date(agent.registered_at).toLocaleDateString()}</td>
                </tr>
              ))}
              {!isLoading && agents.length === 0 && (
                <tr><td colSpan={6} className="px-6 py-12 text-center text-gray-400">No agents enrolled. Create an enrollment token in Settings.</td></tr>
              )}
            </tbody>
          </table>
        </div>
        {total > perPage && (
          <div className="flex items-center justify-between border-t border-gray-100 px-6 py-3">
            <span className="text-sm text-gray-500">Page {page} of {totalPages}</span>
            <div className="flex gap-2">
              <button onClick={() => setPage(Math.max(1, page - 1))} disabled={page <= 1} className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm disabled:opacity-50">Previous</button>
              <button onClick={() => setPage(page + 1)} disabled={page >= totalPages} className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm disabled:opacity-50">Next</button>
            </div>
          </div>
        )}
      </div>

      {/* Agent detail + response slide-out */}
      <SlidePanel open={!!selectedAgent} title={selectedAgent?.hostname || 'Agent'} onClose={() => setSelectedAgent(null)} wide>
        {selectedAgent && (
          <div>
            {/* Tabs */}
            <div className="flex gap-1 border-b border-gray-200 mb-6">
              {(['details', 'response', 'terminal', 'files', 'events', 'history'] as const).map(tab => (
                <button
                  key={tab}
                  onClick={() => setActiveTab(tab)}
                  className={'px-4 py-2 text-sm font-medium border-b-2 -mb-px capitalize ' +
                    (activeTab === tab ? 'border-fibratus-600 text-fibratus-600' : 'border-transparent text-gray-500 hover:text-gray-700')}
                >
                  {tab === 'response' ? 'Active Response' : tab === 'history' ? 'Command History' : tab === 'terminal' ? 'Terminal' : tab === 'files' ? 'File Browser' : tab === 'events' ? 'Events' : tab}
                </button>
              ))}
            </div>

            {/* Details Tab */}
            {activeTab === 'details' && (
              <div className="space-y-4">
                <div className="flex items-center gap-3">
                  <StatusBadge status={selectedAgent.status} />
                  <span className="text-sm text-gray-500 font-mono">{selectedAgent.id}</span>
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
                    <div key={label} className="rounded-lg bg-gray-50 px-3 py-2">
                      <span className="text-xs text-gray-500">{label}</span>
                      <p className="text-sm font-medium text-gray-900">{value}</p>
                    </div>
                  ))}
                </div>
                <div className="pt-4 border-t border-gray-200">
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
                  <h4 className="text-sm font-medium text-gray-700 mb-3">Quick Actions</h4>
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
                    <button onClick={() => setDeleteTarget(selectedAgent)} className="rounded-lg border border-gray-300 bg-gray-50 px-3 py-2.5 text-sm font-medium text-gray-700 hover:bg-gray-100">
                      Uninstall Agent
                    </button>
                  </div>
                </div>

                {/* Command input */}
                <div className="border-t border-gray-200 pt-4">
                  <h4 className="text-sm font-medium text-gray-700 mb-3">Send Command</h4>
                  <select
                    value={cmdType}
                    onChange={(e) => setCmdType(e.target.value)}
                    className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm mb-2"
                  >
                    <option value="">Select command type...</option>
                    <option value="kill_process">Kill Process</option>
                    <option value="list_directory">Browse Directory</option>
                    <option value="get_file">Pull File</option>
                    <option value="run_command">Run Shell Command</option>
                  </select>

                  {cmdType === 'kill_process' && (
                    <input type="number" placeholder="Process ID (PID)" value={cmdInput} onChange={(e) => setCmdInput(e.target.value)}
                      className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm mb-2" />
                  )}
                  {cmdType === 'list_directory' && (
                    <input type="text" placeholder="Directory path (e.g., C:\Users)" value={cmdInput} onChange={(e) => setCmdInput(e.target.value)}
                      className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm mb-2" />
                  )}
                  {cmdType === 'get_file' && (
                    <input type="text" placeholder="File path (e.g., C:\Windows\System32\drivers\etc\hosts)" value={cmdInput} onChange={(e) => setCmdInput(e.target.value)}
                      className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm mb-2" />
                  )}
                  {cmdType === 'run_command' && (
                    <input type="text" placeholder="Command (e.g., whoami /all)" value={cmdInput} onChange={(e) => setCmdInput(e.target.value)}
                      className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm font-mono mb-2" />
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
                  <span className="text-sm text-gray-500">Shell:</span>
                  <button
                    onClick={() => setShellType('powershell')}
                    className={'px-3 py-1 text-xs rounded-full font-medium ' + (shellType === 'powershell' ? 'bg-blue-100 text-blue-800' : 'bg-gray-100 text-gray-600')}
                  >
                    PowerShell
                  </button>
                  <button
                    onClick={() => setShellType('cmd')}
                    className={'px-3 py-1 text-xs rounded-full font-medium ' + (shellType === 'cmd' ? 'bg-blue-100 text-blue-800' : 'bg-gray-100 text-gray-600')}
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

            {/* Command History Tab */}
            {activeTab === 'history' && (
              <div className="space-y-3">
                {commands.length === 0 && (
                  <p className="text-sm text-gray-400 text-center py-8">No commands sent to this agent yet.</p>
                )}
                {commands.map((cmd) => (
                  <div key={cmd.id} className="rounded-lg border border-gray-200 p-3">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-gray-900 capitalize">{cmd.type.replace('_', ' ')}</span>
                        <span className={'inline-flex rounded-full px-2 py-0.5 text-xs font-medium ' + statusColor(cmd.status)}>
                          {cmd.status}
                        </span>
                      </div>
                      <span className="text-xs text-gray-500">{new Date(cmd.created_at).toLocaleString()}</span>
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
