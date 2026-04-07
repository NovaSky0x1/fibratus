import { useMemo } from 'react'
import { clsx } from 'clsx'
import { useAgentCommand } from '../../hooks/useAgentCommand'

interface LocalUser {
  Name: string
  Enabled: boolean
  LastLogon: string
  Description: string
}

interface AdminMember {
  Name: string
  ObjectClass: string
  PrincipalSource: string
}

interface UsersResponse {
  local_users: LocalUser[]
  admin_group: AdminMember[]
  sessions: string
}

interface ParsedSession {
  username: string
  sessionName: string
  id: string
  state: string
}

function parseSessions(raw: string): ParsedSession[] {
  if (!raw || !raw.trim()) return []
  const lines = raw.trim().split('\n')
  const sessions: ParsedSession[] = []

  for (const line of lines) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.toLowerCase().startsWith('username') || trimmed.startsWith('-')) continue

    // quser/qwinsta output varies, try to parse columns
    const parts = trimmed.split(/\s{2,}/).filter(Boolean)
    if (parts.length >= 3) {
      sessions.push({
        username: parts[0] || '-',
        sessionName: parts[1] || '-',
        id: parts[2] || '-',
        state: parts[3] || 'Active',
      })
    } else if (parts.length >= 1) {
      sessions.push({
        username: parts[0],
        sessionName: '-',
        id: '-',
        state: 'Active',
      })
    }
  }
  return sessions
}

export default function AgentUsers({ agentId }: { agentId: string }) {
  const { data, isLoading, error, execute, lastUpdated } = useAgentCommand<UsersResponse>(
    agentId,
    'get_users',
  )

  const _lu: any = data?.local_users; const localUsers = Array.isArray(_lu) ? _lu : _lu ? [_lu] : []
  const _ag: any = data?.admin_group; const adminGroup = Array.isArray(_ag) ? _ag : _ag ? [_ag] : []
  const sessions = useMemo(() => parseSessions(data?.sessions || ''), [data?.sessions])

  return (
    <div className="flex flex-col gap-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-slate-100">Users & Sessions</h3>
        <div className="flex items-center gap-3">
          {lastUpdated && (
            <span className="text-xs text-gray-400 dark:text-slate-500">
              Updated {lastUpdated.toLocaleTimeString()}
            </span>
          )}
          <button
            onClick={execute}
            disabled={isLoading}
            className="rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 p-1.5 text-gray-500 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700 disabled:opacity-50"
            title="Refresh"
          >
            <svg className={clsx('h-4 w-4', isLoading && 'animate-spin')} fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
          </button>
        </div>
      </div>

      {/* Error */}
      {error && (
        <div className="rounded-lg border border-red-300 dark:border-red-700 bg-red-50 dark:bg-red-900/20 p-4">
          <p className="text-sm text-red-700 dark:text-red-400">{error}</p>
          <button
            onClick={execute}
            className="mt-2 rounded-lg bg-red-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-700"
          >
            Retry
          </button>
        </div>
      )}

      {/* Loading skeleton */}
      {isLoading && !data && (
        <div className="space-y-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="h-32 rounded-xl bg-gray-200 dark:bg-slate-700 animate-pulse" />
          ))}
        </div>
      )}

      {data && (
        <div className="space-y-6">
          {/* Active Sessions */}
          <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
            <div className="px-6 py-3 border-b border-gray-100 dark:border-slate-700">
              <h4 className="text-sm font-semibold text-gray-900 dark:text-slate-100">
                Active Sessions
                <span className="ml-2 rounded-full bg-gray-100 dark:bg-slate-700 px-2 py-0.5 text-xs font-medium text-gray-600 dark:text-slate-400">
                  {sessions.length}
                </span>
              </h4>
            </div>
            {sessions.length === 0 ? (
              <div className="px-6 py-4">
                {data.sessions && data.sessions.trim() ? (
                  <pre className="font-mono text-xs text-gray-600 dark:text-slate-400 whitespace-pre-wrap">{data.sessions}</pre>
                ) : (
                  <p className="text-sm text-gray-400 dark:text-slate-500">No active sessions</p>
                )}
              </div>
            ) : (
              <table className="w-full text-left text-sm">
                <thead className="bg-gray-50/50 dark:bg-slate-800/50">
                  <tr>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Username</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Session</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">ID</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">State</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
                  {sessions.map((s, idx) => (
                    <tr key={idx} className="even:bg-gray-50 dark:even:bg-slate-800/50">
                      <td className="px-6 py-2 font-medium text-gray-900 dark:text-slate-100 text-xs">{s.username}</td>
                      <td className="px-6 py-2 font-mono text-xs text-gray-600 dark:text-slate-400">{s.sessionName}</td>
                      <td className="px-6 py-2 font-mono text-xs text-gray-600 dark:text-slate-400 tabular-nums">{s.id}</td>
                      <td className="px-6 py-2">
                        <span className="inline-flex rounded-full bg-emerald-100 dark:bg-emerald-900/30 px-2.5 py-0.5 text-xs font-medium text-emerald-700 dark:text-emerald-400">
                          {s.state}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          {/* Local Accounts */}
          <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
            <div className="px-6 py-3 border-b border-gray-100 dark:border-slate-700">
              <h4 className="text-sm font-semibold text-gray-900 dark:text-slate-100">
                Local Accounts
                <span className="ml-2 rounded-full bg-gray-100 dark:bg-slate-700 px-2 py-0.5 text-xs font-medium text-gray-600 dark:text-slate-400">
                  {localUsers.length}
                </span>
              </h4>
            </div>
            {localUsers.length === 0 ? (
              <p className="px-6 py-4 text-sm text-gray-400 dark:text-slate-500">No local accounts returned</p>
            ) : (
              <table className="w-full text-left text-sm">
                <thead className="bg-gray-50/50 dark:bg-slate-800/50">
                  <tr>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Name</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Enabled</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Last Logon</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Description</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
                  {localUsers.map((u) => (
                    <tr key={u.Name} className="even:bg-gray-50 dark:even:bg-slate-800/50">
                      <td className="px-6 py-2 font-medium text-gray-900 dark:text-slate-100 text-xs">{u.Name}</td>
                      <td className="px-6 py-2">
                        <span
                          className={clsx(
                            'inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium',
                            u.Enabled
                              ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
                              : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400',
                          )}
                        >
                          {u.Enabled ? 'Yes' : 'No'}
                        </span>
                      </td>
                      <td className="px-6 py-2 text-gray-600 dark:text-slate-400 text-xs tabular-nums">
                        {u.LastLogon || '-'}
                      </td>
                      <td className="px-6 py-2 text-gray-500 dark:text-slate-400 text-xs">{u.Description || '-'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          {/* Admin Group Members */}
          <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
            <div className="px-6 py-3 border-b border-gray-100 dark:border-slate-700">
              <h4 className="text-sm font-semibold text-gray-900 dark:text-slate-100">
                Admin Group Members
                <span className="ml-2 rounded-full bg-gray-100 dark:bg-slate-700 px-2 py-0.5 text-xs font-medium text-gray-600 dark:text-slate-400">
                  {adminGroup.length}
                </span>
              </h4>
            </div>
            {adminGroup.length === 0 ? (
              <p className="px-6 py-4 text-sm text-gray-400 dark:text-slate-500">No admin group members returned</p>
            ) : (
              <table className="w-full text-left text-sm">
                <thead className="bg-gray-50/50 dark:bg-slate-800/50">
                  <tr>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Name</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Object Class</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Source</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
                  {adminGroup.map((m) => (
                    <tr key={m.Name} className="even:bg-gray-50 dark:even:bg-slate-800/50">
                      <td className="px-6 py-2 font-medium text-gray-900 dark:text-slate-100 text-xs">{m.Name}</td>
                      <td className="px-6 py-2 text-gray-600 dark:text-slate-400 text-xs">{m.ObjectClass}</td>
                      <td className="px-6 py-2 text-gray-600 dark:text-slate-400 text-xs">{m.PrincipalSource}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
