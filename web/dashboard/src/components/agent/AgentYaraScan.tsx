import { useState, useMemo } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { api, type Agent, type YaraRule } from '../../lib/api'
import {
  ScanSearch, Loader2, CheckCircle2, AlertTriangle, Info,
} from 'lucide-react'

type YaraMatchString = {
  Name?: string
  Offset?: number
  Data?: string | number[]
}

type YaraMatch = {
  Rule?: string
  Namespace?: string
  Tags?: string[]
  Metas?: { Identifier: string; Value: unknown }[]
  Strings?: YaraMatchString[]
}

type YaraResult = {
  pid?: number
  path?: string
  matches?: YaraMatch[]
  match_count?: number
  scanned_at?: string
}

async function waitForCommand(cmdId: string): Promise<Record<string, unknown>> {
  // Matches the 60s window used by FileBrowser / useAgentCommand.
  for (let i = 0; i < 150; i++) {
    await new Promise(r => setTimeout(r, 400))
    const res = await api.getCommand(cmdId)
    const cmd = res?.data as { id: string; status: string; result: unknown; error_message: string } | undefined
    if (!cmd) continue
    if (cmd.status === 'completed') {
      const r = cmd.result
      if (typeof r === 'string') { try { return JSON.parse(r) } catch { return { raw: r } } }
      return (r || {}) as Record<string, unknown>
    }
    if (cmd.status === 'failed') throw new Error(cmd.error_message || 'Command failed')
  }
  throw new Error('Timed out waiting for YARA scan result')
}

function decodeMatchData(d: YaraMatchString['Data']): string {
  if (d == null) return ''
  if (typeof d === 'string') return d
  try {
    // Byte array → printable string fallback to hex.
    return d.map(b => (b >= 0x20 && b < 0x7f ? String.fromCharCode(b) : `\\x${b.toString(16).padStart(2, '0')}`)).join('')
  } catch {
    return ''
  }
}

export default function AgentYaraScan({ agentId, agent: _agent }: { agentId: string; agent: Agent }) {
  void _agent
  const [targetKind, setTargetKind] = useState<'pid' | 'path'>('pid')
  const [pid, setPid] = useState('')
  const [path, setPath] = useState('')
  const [error, setError] = useState('')
  const [result, setResult] = useState<YaraResult | null>(null)
  // Selected rule IDs — empty set means "use all enabled rules"
  const [selectedRules, setSelectedRules] = useState<Set<string>>(new Set())
  const [ruleSearch, setRuleSearch] = useState('')

  const rulesQuery = useQuery({
    queryKey: ['yara-rules'],
    queryFn: () => api.listYaraRules(),
    staleTime: 30_000,
  })
  const allRules: YaraRule[] = useMemo(
    () => ((rulesQuery.data?.data as YaraRule[] | undefined) ?? []).filter(r => r.enabled && r.validation_status === 'valid'),
    [rulesQuery.data]
  )
  const filteredRules = useMemo(() => {
    const q = ruleSearch.toLowerCase().trim()
    if (!q) return allRules
    return allRules.filter(r => r.name.toLowerCase().includes(q) || (r.source || '').toLowerCase().includes(q))
  }, [allRules, ruleSearch])

  const scan = useMutation({
    mutationFn: async () => {
      setError('')
      setResult(null)
      const payload: Record<string, string | number | string[]> = {}
      if (targetKind === 'pid') {
        const p = Number(pid)
        if (!p || p < 1) throw new Error('Enter a valid numeric PID')
        payload.pid = p
      } else {
        if (!path.trim()) throw new Error('Enter a file or directory path')
        payload.path = path.trim()
      }
      if (selectedRules.size > 0) {
        payload.rule_ids = Array.from(selectedRules)
      }
      const res = await api.createCommand(agentId, 'yara_scan', payload)
      const cmdId = (res?.data as { id?: string })?.id
      if (!cmdId) throw new Error('Failed to queue yara_scan command')
      const r = await waitForCommand(cmdId)
      return r as unknown as YaraResult
    },
    onSuccess: (data) => setResult(data),
    onError: (e: Error) => setError(e.message),
  })

  const toggleRule = (id: string) => {
    setSelectedRules(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id); else next.add(id)
      return next
    })
  }
  const selectAll = () => setSelectedRules(new Set(filteredRules.map(r => r.id)))
  const clearAll = () => setSelectedRules(new Set())

  const busy = scan.isPending

  return (
    <div className="p-6 space-y-5">
      <header className="space-y-2">
        <div className="flex items-center gap-2">
          <ScanSearch className="w-5 h-5 text-fibratus-600" />
          <h1 className="text-xl font-semibold text-gray-900 dark:text-slate-100">YARA Scan</h1>
        </div>
        <p className="text-sm text-gray-500 dark:text-slate-400">
          Run an on-demand YARA scan against a process (by PID) or a file/directory on disk. Rules come from
          the agent's configured YARA rule sources (<code className="font-mono text-xs">yara.rule.paths</code>
          {' '}and <code className="font-mono text-xs">yara.rule.strings</code> in <code className="font-mono text-xs">fibratus.yml</code>).
          Requires <code className="font-mono text-xs">yara.enabled: true</code> on the agent.
        </p>
      </header>

      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-5 space-y-4">
        <div className="flex gap-2 text-sm">
          <button
            onClick={() => setTargetKind('pid')}
            className={`rounded-md px-3 py-1.5 ${targetKind === 'pid' ? 'bg-fibratus-600 text-white' : 'bg-gray-100 dark:bg-slate-700 text-gray-700 dark:text-slate-300'}`}
          >Process (PID)</button>
          <button
            onClick={() => setTargetKind('path')}
            className={`rounded-md px-3 py-1.5 ${targetKind === 'path' ? 'bg-fibratus-600 text-white' : 'bg-gray-100 dark:bg-slate-700 text-gray-700 dark:text-slate-300'}`}
          >File / Directory</button>
        </div>

        {targetKind === 'pid' ? (
          <div>
            <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Process ID</label>
            <input
              type="number"
              value={pid}
              onChange={e => setPid(e.target.value)}
              placeholder="1234"
              disabled={busy}
              className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none disabled:opacity-50"
            />
          </div>
        ) : (
          <div>
            <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Path</label>
            <input
              type="text"
              value={path}
              onChange={e => setPath(e.target.value)}
              placeholder="C:\\Users\\Public\\sample.exe"
              disabled={busy}
              className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm font-mono text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none disabled:opacity-50"
            />
            <p className="mt-1 text-xs text-gray-500 dark:text-slate-500">
              File scans read the target on disk. Directory paths are scanned recursively by the agent's libyara build.
            </p>
          </div>
        )}

        {/* Rule selector — account-wide rule set, narrow with checkboxes */}
        <div className="pt-2 border-t border-gray-200 dark:border-slate-700">
          <div className="flex items-center justify-between mb-2">
            <label className="text-xs font-medium text-gray-500 dark:text-slate-400">
              Rules ({selectedRules.size === 0 ? `all ${allRules.length} enabled` : `${selectedRules.size} of ${allRules.length} selected`})
            </label>
            <div className="flex gap-3 text-xs">
              <button type="button" onClick={selectAll} disabled={busy} className="text-fibratus-600 hover:underline disabled:opacity-50">Select all</button>
              <button type="button" onClick={clearAll} disabled={busy} className="text-fibratus-600 hover:underline disabled:opacity-50">Clear</button>
            </div>
          </div>
          <input
            type="text"
            value={ruleSearch}
            onChange={e => setRuleSearch(e.target.value)}
            placeholder="Filter by name or source…"
            disabled={busy}
            className="w-full mb-2 rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-1.5 text-xs text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none disabled:opacity-50"
          />
          <div className="max-h-40 overflow-y-auto rounded border border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-900/40">
            {rulesQuery.isLoading ? (
              <div className="p-3 text-xs text-gray-500">Loading rules…</div>
            ) : filteredRules.length === 0 ? (
              <div className="p-3 text-xs text-gray-500">
                {allRules.length === 0 ? 'No enabled YARA rules. Add rules on the YARA Rules page.' : 'No rules match the filter.'}
              </div>
            ) : (
              <ul className="divide-y divide-gray-100 dark:divide-slate-700">
                {filteredRules.map(r => (
                  <li key={r.id} className="px-3 py-1.5 flex items-center gap-2 text-xs">
                    <input
                      type="checkbox"
                      checked={selectedRules.has(r.id)}
                      onChange={() => toggleRule(r.id)}
                      disabled={busy}
                      className="rounded"
                    />
                    <span className="font-mono text-gray-900 dark:text-slate-100 flex-1 truncate">{r.name}</span>
                    {r.source && r.source !== 'manual' && (
                      <span className="text-[10px] rounded bg-gray-200 dark:bg-slate-700 text-gray-600 dark:text-slate-400 px-1.5 py-0.5 font-mono">{r.source}</span>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>
          <p className="mt-1 text-[11px] text-gray-500 dark:text-slate-500">
            Leave empty to run with every enabled rule in the account. Selecting any rule limits the scan to just those.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => scan.mutate()}
            disabled={busy}
            className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50 flex items-center gap-2"
          >
            {busy ? <Loader2 className="w-4 h-4 animate-spin" /> : <ScanSearch className="w-4 h-4" />}
            {busy ? 'Scanning…' : 'Run scan'}
          </button>
          {result && (
            <span className="text-xs text-gray-500 dark:text-slate-400">
              Scanned at {result.scanned_at ? new Date(result.scanned_at).toLocaleTimeString() : '—'}
            </span>
          )}
        </div>

        {error && (
          <div className="flex items-start gap-2 rounded-lg border border-red-200 dark:border-red-800/50 bg-red-50 dark:bg-red-900/20 px-3 py-2 text-sm text-red-700 dark:text-red-400">
            <AlertTriangle className="w-4 h-4 flex-shrink-0 mt-0.5" />
            <div>{error}</div>
          </div>
        )}
      </div>

      {result && (
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-5">
          <div className="flex items-center gap-2 mb-4">
            {(result.match_count ?? 0) > 0 ? (
              <AlertTriangle className="w-5 h-5 text-amber-500" />
            ) : (
              <CheckCircle2 className="w-5 h-5 text-green-500" />
            )}
            <h2 className="text-sm font-semibold text-gray-900 dark:text-slate-100">
              {(result.match_count ?? 0) > 0
                ? `${result.match_count} matching rule${result.match_count === 1 ? '' : 's'}`
                : 'No matches'}
            </h2>
            <span className="ml-auto text-xs text-gray-500 dark:text-slate-400">
              Target: {result.pid ? `PID ${result.pid}` : result.path}
            </span>
          </div>

          {(result.match_count ?? 0) === 0 ? (
            <div className="text-sm text-gray-500 dark:text-slate-400 flex items-start gap-2">
              <Info className="w-4 h-4 flex-shrink-0 mt-0.5" />
              <div>
                Target scanned cleanly against the agent's currently loaded rule set. This is the expected
                result for benign processes/files. A non-zero match count surfaces the rule name, matched
                strings, offsets, and any metadata fields (severity, threat name, etc.) defined in the rule.
              </div>
            </div>
          ) : (
            <div className="space-y-3">
              {(result.matches || []).map((m, i) => (
                <div key={i} className="rounded-lg border border-amber-200 dark:border-amber-900/50 bg-amber-50/50 dark:bg-amber-900/10 p-3">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="font-mono text-sm font-semibold text-gray-900 dark:text-slate-100">
                      {m.Rule || 'rule'}
                    </span>
                    {m.Namespace && m.Namespace !== 'default' && (
                      <span className="text-xs text-gray-500 dark:text-slate-400 font-mono">[{m.Namespace}]</span>
                    )}
                    {(m.Tags || []).map(t => (
                      <span key={t} className="text-[11px] rounded bg-gray-100 dark:bg-slate-700 px-1.5 py-0.5 text-gray-600 dark:text-slate-400">{t}</span>
                    ))}
                  </div>
                  {(m.Metas || []).length > 0 && (
                    <div className="mt-2 text-xs text-gray-500 dark:text-slate-400 space-y-0.5">
                      {(m.Metas || []).map((meta, j) => (
                        <div key={j} className="font-mono">
                          <span className="text-gray-400 dark:text-slate-500">{meta.Identifier}:</span>{' '}
                          <span>{String(meta.Value)}</span>
                        </div>
                      ))}
                    </div>
                  )}
                  {(m.Strings || []).length > 0 && (
                    <details className="mt-2 text-xs">
                      <summary className="cursor-pointer text-gray-500 dark:text-slate-400">
                        {m.Strings?.length} matched string{m.Strings?.length === 1 ? '' : 's'}
                      </summary>
                      <div className="mt-2 space-y-1">
                        {(m.Strings || []).map((s, k) => (
                          <div key={k} className="font-mono text-[11px] text-gray-700 dark:text-slate-300">
                            <span className="text-gray-400 dark:text-slate-500">{s.Name || '$'}</span>
                            {s.Offset != null && (
                              <span className="text-gray-400 dark:text-slate-500"> @ 0x{s.Offset.toString(16)}</span>
                            )}
                            <span className="ml-2 break-all">{decodeMatchData(s.Data)}</span>
                          </div>
                        ))}
                      </div>
                    </details>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
