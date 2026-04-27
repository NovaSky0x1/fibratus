import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, Command } from '../../lib/api'
import { RefreshCw, ChevronDown, ChevronRight, Clock } from 'lucide-react'

function Skeleton({ className = '' }: { className?: string }) {
  return <div className={`animate-pulse bg-gray-200 dark:bg-slate-700 rounded ${className}`} />
}

const TYPE_BADGE_COLORS: Record<string, string> = {
  isolate: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300',
  unisolate: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  kill_process: 'bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300',
  run_command: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
  list_directory: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300',
  get_file: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300',
  collect_info: 'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/40 dark:text-cyan-300',
  uninstall: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300',
  start_capture: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300',
  stop_capture: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300',
  yara_scan: 'bg-pink-100 text-pink-700 dark:bg-pink-900/40 dark:text-pink-300',
}

const STATUS_BADGE: Record<string, string> = {
  pending: 'bg-gray-100 text-gray-600 dark:bg-slate-700 dark:text-slate-400',
  running: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300 animate-pulse',
  completed: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  failed: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300',
}

function formatDuration(createdAt: string, completedAt: string | null): string {
  if (!completedAt) return '-'
  const ms = new Date(completedAt).getTime() - new Date(createdAt).getTime()
  if (ms < 1000) return ms + 'ms'
  if (ms < 60000) return (ms / 1000).toFixed(1) + 's'
  return Math.floor(ms / 60000) + 'm ' + Math.round((ms % 60000) / 1000) + 's'
}

function truncateResult(result: unknown, maxLen = 80): string {
  if (result == null) return '-'
  const s = typeof result === 'string' ? result : JSON.stringify(result)
  return s.length > maxLen ? s.slice(0, maxLen) + '...' : s
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

export default function AgentCommandHistory({ agentId }: { agentId: string }) {
  const [expandedId, setExpandedId] = useState<string | null>(null)

  const { data: res, isLoading, refetch } = useQuery({
    queryKey: ['agent-commands', agentId],
    queryFn: () => api.getAgentCommands(agentId),
    refetchInterval: 5000,
  })

  const commands = (res?.data || []) as Command[]

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Clock className="w-4 h-4 text-gray-500 dark:text-slate-400" />
          <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Command History</h3>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs text-gray-400 dark:text-slate-500 tabular-nums">{commands.length} command(s)</span>
          <button
            onClick={() => refetch()}
            className="p-1.5 rounded text-gray-400 dark:text-slate-500 hover:text-gray-600 dark:hover:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-700 transition-colors"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Table */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50 overflow-hidden">
        <div className="overflow-auto" style={{ maxHeight: 'calc(100vh - 260px)' }}>
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-slate-900 sticky top-0 z-10">
              <tr>
                <th className="px-4 py-2.5 w-5" />
                <th className="px-4 py-2.5 font-medium text-gray-500 dark:text-slate-400">Type</th>
                <th className="px-4 py-2.5 font-medium text-gray-500 dark:text-slate-400">Status</th>
                <th className="px-4 py-2.5 font-medium text-gray-500 dark:text-slate-400">Created By</th>
                <th className="px-4 py-2.5 font-medium text-gray-500 dark:text-slate-400">Created</th>
                <th className="px-4 py-2.5 font-medium text-gray-500 dark:text-slate-400">Duration</th>
                <th className="px-4 py-2.5 font-medium text-gray-500 dark:text-slate-400">Result</th>
              </tr>
            </thead>
            <tbody>
              {isLoading && (
                <tr><td colSpan={7} className="p-6">
                  <div className="space-y-2">
                    {[...Array(5)].map((_, i) => <Skeleton key={i} className="h-7 w-full" />)}
                  </div>
                </td></tr>
              )}
              {!isLoading && commands.map(cmd => {
                const isExpanded = expandedId === cmd.id
                const createdByEmail = (cmd as unknown as { created_by_email?: string }).created_by_email

                return (
                  <tr
                    key={cmd.id}
                    className={`border-t border-gray-100 dark:border-slate-700 cursor-pointer transition-colors ${
                      isExpanded ? 'bg-gray-50 dark:bg-slate-700/50' : 'hover:bg-gray-50/50 dark:hover:bg-slate-700/50'
                    }`}
                    onClick={() => setExpandedId(isExpanded ? null : cmd.id)}
                  >
                    <td className="px-4 py-2.5 align-top text-gray-400 dark:text-slate-500">
                      {isExpanded ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
                    </td>
                    <td className="px-4 py-2.5 align-top">
                      <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium capitalize ${
                        TYPE_BADGE_COLORS[cmd.type] || 'bg-gray-100 dark:bg-slate-700 text-gray-700 dark:text-slate-300'
                      }`}>
                        {cmd.type.replace(/_/g, ' ')}
                      </span>
                      {isExpanded && (
                        <div className="mt-3 space-y-2" onClick={e => e.stopPropagation()}>
                          {cmd.payload != null && (
                            <div>
                              <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Payload</span>
                              <pre className="mt-0.5 text-xs font-mono text-gray-600 dark:text-slate-400 bg-gray-50 dark:bg-slate-900 rounded p-2 whitespace-pre-wrap break-all">
{JSON.stringify(cmd.payload, null, 2)}
                              </pre>
                            </div>
                          )}
                          {cmd.result != null && (
                            <div>
                              <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Full Result</span>
                              <pre className="mt-0.5 text-xs font-mono bg-gray-900 dark:bg-gray-950 text-gray-100 rounded-lg p-3 max-h-64 overflow-auto whitespace-pre-wrap break-all">
{JSON.stringify(cmd.result, null, 2)}
                              </pre>
                            </div>
                          )}
                          {cmd.error_message && (
                            <div>
                              <span className="text-[10px] uppercase tracking-wide text-red-400 dark:text-red-500">Error</span>
                              <p className="mt-0.5 text-xs text-red-600 dark:text-red-400 font-mono">{cmd.error_message}</p>
                            </div>
                          )}
                          <div className="text-[10px] text-gray-400 dark:text-slate-500 font-mono">
                            ID: {cmd.id}
                          </div>
                        </div>
                      )}
                    </td>
                    <td className="px-4 py-2.5 align-top">
                      <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${STATUS_BADGE[cmd.status] || STATUS_BADGE.pending}`}>
                        {cmd.status}
                      </span>
                    </td>
                    <td className="px-4 py-2.5 align-top text-gray-600 dark:text-slate-400">
                      {createdByEmail || cmd.created_by || '-'}
                    </td>
                    <td className="px-4 py-2.5 align-top text-gray-500 dark:text-slate-400 tabular-nums whitespace-nowrap">
                      <div>{timeAgo(new Date(cmd.created_at))}</div>
                      <div className="text-[10px] text-gray-400 dark:text-slate-500 mt-0.5">
                        {new Date(cmd.created_at).toLocaleString()}
                      </div>
                    </td>
                    <td className="px-4 py-2.5 align-top text-gray-500 dark:text-slate-400 tabular-nums font-mono">
                      {formatDuration(cmd.created_at, cmd.completed_at)}
                    </td>
                    <td className="px-4 py-2.5 align-top text-gray-500 dark:text-slate-400 font-mono max-w-[200px]">
                      {cmd.status === 'failed' ? (
                        <span className="text-red-500 dark:text-red-400">{cmd.error_message || 'Failed'}</span>
                      ) : cmd.status === 'completed' ? (
                        <span className="break-all line-clamp-1">{truncateResult(cmd.result)}</span>
                      ) : cmd.status === 'running' ? (
                        <span className="text-blue-500 dark:text-blue-400 animate-pulse">Running...</span>
                      ) : (
                        <span className="text-gray-400 dark:text-slate-500">Pending</span>
                      )}
                    </td>
                  </tr>
                )
              })}
              {!isLoading && commands.length === 0 && (
                <tr>
                  <td colSpan={7} className="px-4 py-12 text-center text-gray-400 dark:text-slate-500">
                    No commands sent to this agent yet
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
