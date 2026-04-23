import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type YaraRule } from '../lib/api'
import { AlertTriangle, CheckCircle2, Plus, Trash2, Save, X, ScanSearch } from 'lucide-react'

const SAMPLE_RULE = `rule ExampleMarker
{
    meta:
        description = "Example YARA rule — replace with your own"
        threat_name = "Example"
        severity = "low"
        score = 10
        author = "YourName"
    strings:
        $marker = "EXAMPLE_MARKER" ascii
    condition:
        $marker
}
`

export default function YaraRules() {
  const qc = useQueryClient()
  const [editing, setEditing] = useState<YaraRule | null>(null)
  const [draft, setDraft] = useState<{ name: string; description: string; content: string; enabled: boolean }>({
    name: '', description: '', content: '', enabled: true,
  })
  const [error, setError] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['yara-rules'],
    queryFn: () => api.listYaraRules(),
    refetchInterval: 30_000,
  })
  const rules: YaraRule[] = (data?.data as YaraRule[] | undefined) ?? []

  const create = useMutation({
    mutationFn: () => api.createYaraRule(draft),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['yara-rules'] })
      setEditing(null); setError('')
    },
    onError: (e: Error) => setError(e.message),
  })

  const update = useMutation({
    mutationFn: () => {
      if (!editing) throw new Error('no rule selected')
      return api.updateYaraRule(editing.id, draft)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['yara-rules'] })
      setEditing(null); setError('')
    },
    onError: (e: Error) => setError(e.message),
  })

  const del = useMutation({
    mutationFn: (id: string) => api.deleteYaraRule(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['yara-rules'] }),
    onError: (e: Error) => setError(e.message),
  })

  const beginNew = () => {
    setEditing({ id: '', account_id: '', name: '', description: '', content: SAMPLE_RULE, enabled: true, validation_status: 'pending', created_at: '', updated_at: '' } as YaraRule)
    setDraft({ name: '', description: '', content: SAMPLE_RULE, enabled: true })
    setError('')
  }

  const beginEdit = (rule: YaraRule) => {
    setEditing(rule)
    setDraft({ name: rule.name, description: rule.description || '', content: rule.content, enabled: rule.enabled })
    setError('')
  }

  const save = () => {
    if (!draft.name.trim() || !draft.content.trim()) {
      setError('name and content are required')
      return
    }
    if (editing && editing.id) update.mutate()
    else create.mutate()
  }

  return (
    <div className="p-6">
      <div className="mb-6 flex items-start justify-between">
        <div>
          <div className="flex items-center gap-2">
            <ScanSearch className="w-5 h-5 text-fibratus-600" />
            <h1 className="text-2xl font-semibold text-gray-900 dark:text-slate-100">YARA Rules</h1>
          </div>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
            Server-managed YARA rule set. Rules are <strong>account-scoped</strong> (shared across every organization
            in your account) and embedded inline in <code className="font-mono text-xs">yara_scan</code> active-response
            commands at creation time — no agent-side sync is required.
          </p>
        </div>
        <button
          onClick={beginNew}
          className="rounded-lg bg-fibratus-600 px-3 py-2 text-sm font-medium text-white hover:bg-fibratus-700 flex items-center gap-2"
        >
          <Plus className="w-4 h-4" /> New rule
        </button>
      </div>

      {error && (
        <div className="mb-4 rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800/50 px-4 py-3 text-sm text-red-700 dark:text-red-400">
          {error}
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-5 gap-6">
        {/* Rule list */}
        <div className="lg:col-span-2 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 overflow-hidden">
          {isLoading ? (
            <div className="p-8 text-center text-sm text-gray-500 dark:text-slate-400">Loading…</div>
          ) : rules.length === 0 ? (
            <div className="p-8 text-center text-sm text-gray-500 dark:text-slate-400">No YARA rules yet. Click <strong>New rule</strong>.</div>
          ) : (
            <ul className="divide-y divide-gray-100 dark:divide-slate-700">
              {rules.map(r => (
                <li
                  key={r.id}
                  className={`px-4 py-3 cursor-pointer hover:bg-gray-50 dark:hover:bg-slate-900/40 ${editing?.id === r.id ? 'bg-fibratus-50 dark:bg-fibratus-900/20' : ''}`}
                  onClick={() => beginEdit(r)}
                >
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-sm text-gray-900 dark:text-slate-100 truncate">{r.name}</span>
                        {r.enabled ? (
                          <span className="text-[10px] rounded bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400 px-1.5 py-0.5">enabled</span>
                        ) : (
                          <span className="text-[10px] rounded bg-gray-100 dark:bg-slate-700 text-gray-600 dark:text-slate-400 px-1.5 py-0.5">disabled</span>
                        )}
                        {r.validation_status === 'invalid' && (
                          <span className="text-[10px] rounded bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400 px-1.5 py-0.5 flex items-center gap-1">
                            <AlertTriangle className="w-3 h-3" /> invalid
                          </span>
                        )}
                      </div>
                      {r.description && (
                        <div className="text-xs text-gray-500 dark:text-slate-400 truncate mt-0.5">{r.description}</div>
                      )}
                    </div>
                    <button
                      onClick={(e) => { e.stopPropagation(); if (confirm(`Delete rule "${r.name}"?`)) del.mutate(r.id) }}
                      className="text-gray-400 hover:text-red-600 dark:text-slate-500"
                      title="Delete"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* Editor */}
        <div className="lg:col-span-3 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-5 space-y-4">
          {editing ? (
            <>
              <div className="flex items-start justify-between gap-2">
                <h2 className="text-sm font-semibold text-gray-900 dark:text-slate-100">
                  {editing.id ? 'Edit rule' : 'New rule'}
                </h2>
                <button onClick={() => setEditing(null)} className="text-gray-400 hover:text-gray-700 dark:hover:text-slate-200">
                  <X className="w-4 h-4" />
                </button>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Name</label>
                <input
                  type="text"
                  value={draft.name}
                  onChange={e => setDraft({ ...draft, name: e.target.value })}
                  placeholder="SuspiciousBeacon"
                  className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm font-mono text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Description (optional)</label>
                <input
                  type="text"
                  value={draft.description}
                  onChange={e => setDraft({ ...draft, description: e.target.value })}
                  className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100"
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Rule content</label>
                <textarea
                  value={draft.content}
                  onChange={e => setDraft({ ...draft, content: e.target.value })}
                  rows={18}
                  className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-gray-50 dark:bg-slate-900 px-3 py-2 text-xs font-mono text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                  spellCheck={false}
                />
              </div>
              <div className="flex items-center gap-3">
                <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-slate-300">
                  <input type="checkbox" checked={draft.enabled} onChange={e => setDraft({ ...draft, enabled: e.target.checked })} />
                  Enabled
                </label>
                {editing.validation_status === 'invalid' && editing.validation_errors && (
                  <span className="text-xs text-red-600 dark:text-red-400">
                    Last validation: {editing.validation_errors}
                  </span>
                )}
                {editing.validation_status === 'valid' && (
                  <span className="text-xs text-green-600 dark:text-green-400 flex items-center gap-1">
                    <CheckCircle2 className="w-3 h-3" /> syntax valid
                  </span>
                )}
              </div>
              <div className="flex gap-2">
                <button
                  onClick={save}
                  disabled={create.isPending || update.isPending}
                  className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50 flex items-center gap-2"
                >
                  <Save className="w-4 h-4" /> Save
                </button>
                <button
                  onClick={() => setEditing(null)}
                  className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700"
                >
                  Cancel
                </button>
              </div>
            </>
          ) : (
            <div className="text-center text-sm text-gray-500 dark:text-slate-400 py-8">
              Select a rule on the left to edit, or click <strong>New rule</strong>.
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
