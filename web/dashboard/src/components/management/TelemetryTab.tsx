import { useState, useRef } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../lib/api'
import type { SigmaConversionResult, SigmaBatchResult } from '../../lib/api'

// ================================================================
// Toggle Switch
// ================================================================

function ToggleSwitch({ enabled, onToggle, disabled }: { enabled: boolean; onToggle: () => void; disabled?: boolean }) {
  return (
    <button
      role="switch"
      aria-checked={enabled}
      onClick={onToggle}
      disabled={disabled}
      className={'relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-fibratus-500 focus:ring-offset-2 dark:focus:ring-offset-slate-800 disabled:opacity-50 disabled:cursor-not-allowed ' +
        (enabled ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-slate-600')
      }
    >
      <span
        className={'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ' +
          (enabled ? 'translate-x-5' : 'translate-x-0')
        }
      />
    </button>
  )
}

// ================================================================
// Event Log Channel Types & Constants
// ================================================================

interface EventLogChannel {
  name: string
  collect_all: boolean
  event_ids?: number[]
}

const RECOMMENDED_CHANNELS: EventLogChannel[] = [
  { name: 'Security', collect_all: true },
  { name: 'System', collect_all: true },
  { name: 'Microsoft-Windows-Sysmon/Operational', collect_all: true },
]

const ADDITIONAL_CHANNELS: EventLogChannel[] = [
  { name: 'Microsoft-Windows-PowerShell/Operational', collect_all: true },
  { name: 'Microsoft-Windows-Windows Defender/Operational', collect_all: true },
  { name: 'Application', collect_all: true },
  { name: 'Microsoft-Windows-CodeIntegrity/Operational', collect_all: true },
  { name: 'Microsoft-Windows-Windows Firewall With Advanced Security/Firewall', collect_all: true },
  { name: 'Microsoft-Windows-Bits-Client/Operational', collect_all: true },
  { name: 'Microsoft-Windows-TaskScheduler/Operational', collect_all: true },
  { name: 'Microsoft-Windows-WMI-Activity/Operational', collect_all: true },
  { name: 'Microsoft-Windows-TerminalServices-LocalSessionManager/Operational', collect_all: true },
  { name: 'Microsoft-Windows-NTLM/Operational', collect_all: true },
]

// ================================================================
// Telemetry Tab
// ================================================================

export default function TelemetryTab() {
  const queryClient = useQueryClient()

  // ── Event Log Collection state ──────────────────────────────
  const [showAddChannel, setShowAddChannel] = useState(false)
  const [customChannelName, setCustomChannelName] = useState('')

  // ── GitHub Sync state ───────────────────────────────────────
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ name: '', repo_url: '', branch: 'main', path: '', token: '', interval: 30, enabled: true, scope: 'account' })
  const [editId, setEditId] = useState<string | null>(null)
  const [syncResult, setSyncResult] = useState<Record<string, unknown> | null>(null)

  // ── SIGMA Converter state ────────────────────────────────────
  const [sigmaInput, setSigmaInput] = useState('')
  const [sigmaResult, setSigmaResult] = useState<SigmaConversionResult | null>(null)
  const [sigmaBatchResult, setSigmaBatchResult] = useState<SigmaBatchResult | null>(null)
  const [sigmaTab, setSigmaTab] = useState<'convert' | 'sigmahq'>('convert')
  const fileInputRef = useRef<HTMLInputElement>(null)

  // ── Event Log Queries ───────────────────────────────────────

  const { data: settingsData } = useQuery({
    queryKey: ['account-settings'],
    queryFn: () => api.getAccountSettings(),
  })
  const accountEnabled = (settingsData?.data as { eventlog_enabled?: boolean } | undefined)?.eventlog_enabled ?? false

  const accountToggleMutation = useMutation({
    mutationFn: (enabled: boolean) => api.updateAccountSettings({ eventlog_enabled: enabled }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['account-settings'] })
      queryClient.invalidateQueries({ queryKey: ['eventlog-policy'] })
    },
  })

  const { data: policyData } = useQuery({
    queryKey: ['eventlog-policy'],
    queryFn: () => api.getEventLogPolicy(),
  })

  const policy = policyData?.data as { enabled: boolean; channels: EventLogChannel[]; version?: number } | undefined
  const channels = policy?.channels ?? []

  const updatePolicyMutation = useMutation({
    mutationFn: (data: { enabled: boolean; channels: EventLogChannel[] }) => api.updateEventLogPolicy(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['eventlog-policy'] })
    },
  })

  // ── GitHub Sync Queries ─────────────────────────────────────

  const { data: syncConfigsData } = useQuery({
    queryKey: ['github-sync-configs'],
    queryFn: () => api.listGitHubSyncConfigs(),
  })
  const configs = (syncConfigsData?.data || []) as Record<string, unknown>[]

  const saveMutation = useMutation({
    mutationFn: () => api.saveGitHubSyncConfig({ ...form, id: editId || undefined }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['github-sync-configs'] })
      setShowAdd(false)
      setEditId(null)
      setForm({ name: '', repo_url: '', branch: 'main', path: '', token: '', interval: 30, enabled: true, scope: 'account' })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteGitHubSyncConfig(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['github-sync-configs'] }),
  })

  const syncAllMutation = useMutation({
    mutationFn: () => api.triggerGitHubSync(),
    onSuccess: (res) => {
      if (res.data) setSyncResult(res.data as Record<string, unknown>)
      queryClient.invalidateQueries({ queryKey: ['rules'] })
    },
  })

  const syncOneMutation = useMutation({
    mutationFn: (id: string) => api.triggerGitHubSyncOne(id),
    onSuccess: (res) => {
      if (res.data) setSyncResult(res.data as Record<string, unknown>)
      queryClient.invalidateQueries({ queryKey: ['rules'] })
    },
  })

  // ── SIGMA Converter Mutations ────────────────────────────────

  const convertMutation = useMutation({
    mutationFn: (yaml: string) => api.convertSigmaRule(yaml),
    onSuccess: (res) => {
      if (res.data) setSigmaResult(res.data as SigmaConversionResult)
    },
  })

  const importMutation = useMutation({
    mutationFn: (yaml: string) => api.importSigmaRule(yaml),
    onSuccess: (res) => {
      if (res.data) setSigmaResult(res.data as SigmaConversionResult)
      queryClient.invalidateQueries({ queryKey: ['rules'] })
    },
  })

  const importBatchMutation = useMutation({
    mutationFn: (rules: string[]) => api.importSigmaBatch(rules),
    onSuccess: (res) => {
      if (res.data) setSigmaBatchResult(res.data as SigmaBatchResult)
      queryClient.invalidateQueries({ queryKey: ['rules'] })
    },
  })

  // ── SIGMA Handlers ─────────────────────────────────────────

  const handleSigmaConvert = () => {
    if (!sigmaInput.trim()) return
    convertMutation.mutate(sigmaInput)
  }

  const handleSigmaImport = () => {
    if (!sigmaResult?.fibratus_yaml) return
    importMutation.mutate(sigmaInput)
  }

  const handleSigmaFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files
    if (!files || files.length === 0) return

    if (files.length === 1) {
      const reader = new FileReader()
      reader.onload = (ev) => {
        const content = ev.target?.result as string
        setSigmaInput(content)
        convertMutation.mutate(content)
      }
      reader.readAsText(files[0])
    } else {
      // Batch upload
      const promises = Array.from(files).map(file =>
        new Promise<string>((resolve) => {
          const reader = new FileReader()
          reader.onload = (ev) => resolve(ev.target?.result as string)
          reader.readAsText(file)
        })
      )
      Promise.all(promises).then(rules => {
        importBatchMutation.mutate(rules)
      })
    }
    // Reset file input
    if (fileInputRef.current) fileInputRef.current.value = ''
  }

  // ── Event Log Handlers ──────────────────────────────────────

  const handleEventLogToggle = () => {
    accountToggleMutation.mutate(!accountEnabled)
  }

  const handleChannelToggle = (channelName: string) => {
    const exists = channels.find(c => c.name === channelName)
    let newChannels: EventLogChannel[]
    if (exists) {
      newChannels = channels.filter(c => c.name !== channelName)
    } else {
      newChannels = [...channels, { name: channelName, collect_all: true }]
    }
    updatePolicyMutation.mutate({ enabled: accountEnabled, channels: newChannels })
  }

  const handlePreset = (preset: 'recommended' | 'sigma-full' | 'minimal') => {
    let newChannels: EventLogChannel[]
    switch (preset) {
      case 'recommended':
        newChannels = [...RECOMMENDED_CHANNELS]
        break
      case 'sigma-full':
        newChannels = [...RECOMMENDED_CHANNELS, ...ADDITIONAL_CHANNELS]
        break
      case 'minimal':
        newChannels = [
          { name: 'Security', collect_all: true },
          { name: 'System', collect_all: true },
        ]
        break
    }
    updatePolicyMutation.mutate({ enabled: true, channels: newChannels })
  }

  const handleAddCustomChannel = () => {
    if (!customChannelName.trim()) return
    if (channels.find(c => c.name === customChannelName.trim())) return
    const newChannels = [...channels, { name: customChannelName.trim(), collect_all: true }]
    updatePolicyMutation.mutate({ enabled: accountEnabled, channels: newChannels })
    setCustomChannelName('')
    setShowAddChannel(false)
  }

  const isChannelEnabled = (name: string) => channels.some(c => c.name === name)

  // ── GitHub Sync Handlers ────────────────────────────────────

  const startEdit = (cfg: Record<string, unknown>) => {
    setEditId(cfg.id as string)
    setForm({
      name: (cfg.name as string) || '',
      repo_url: (cfg.repo_url as string) || '',
      branch: (cfg.branch as string) || 'main',
      path: (cfg.path as string) || '',
      token: '',
      interval: (cfg.interval as number) || 30,
      enabled: (cfg.enabled as boolean) || false,
      scope: (cfg.scope as string) || 'account',
    })
    setShowAdd(true)
  }

  const inputCls = "w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 text-gray-900 dark:text-slate-100 px-3 py-2 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"

  // ── Render ──────────────────────────────────────────────────

  return (
    <div className="mt-6 space-y-8">
      {/* ══════════════════════════════════════════════════════════ */}
      {/* Event Log Collection                                      */}
      {/* ══════════════════════════════════════════════════════════ */}
      <div>
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-slate-100">
              Event Log Collection
            </h2>
            <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
              Collect Windows Event Logs from enrolled agents. Enables detection rules based on Security, System, Sysmon, and other log sources.
            </p>
          </div>
          <ToggleSwitch
            enabled={accountEnabled}
            onToggle={handleEventLogToggle}
            disabled={accountToggleMutation.isPending}
          />
        </div>

        {accountEnabled && (
          <div className="mt-4 space-y-4">
            {/* Presets */}
            <div className="flex items-center gap-2">
              <span className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wide">Presets:</span>
              <button onClick={() => handlePreset('recommended')} className="rounded-md bg-blue-50 dark:bg-blue-900/30 px-3 py-1.5 text-xs font-medium text-blue-700 dark:text-blue-400 hover:bg-blue-100 dark:hover:bg-blue-900/50 border border-blue-200 dark:border-blue-800">
                Recommended
              </button>
              <button onClick={() => handlePreset('sigma-full')} className="rounded-md bg-purple-50 dark:bg-purple-900/30 px-3 py-1.5 text-xs font-medium text-purple-700 dark:text-purple-400 hover:bg-purple-100 dark:hover:bg-purple-900/50 border border-purple-200 dark:border-purple-800">
                Sigma Full Coverage
              </button>
              <button onClick={() => handlePreset('minimal')} className="rounded-md bg-gray-50 dark:bg-slate-800 px-3 py-1.5 text-xs font-medium text-gray-600 dark:text-slate-400 hover:bg-gray-100 dark:hover:bg-slate-700 border border-gray-200 dark:border-slate-600">
                Minimal
              </button>
            </div>

            {/* Channel list */}
            <div className="rounded-lg border border-gray-200 dark:border-slate-700 overflow-hidden">
              <table className="min-w-full divide-y divide-gray-200 dark:divide-slate-700">
                <thead className="bg-gray-50 dark:bg-slate-800">
                  <tr>
                    <th className="px-4 py-2.5 text-left text-xs font-medium text-gray-500 dark:text-slate-400 uppercase">Channel</th>
                    <th className="px-4 py-2.5 text-left text-xs font-medium text-gray-500 dark:text-slate-400 uppercase">Mode</th>
                    <th className="px-4 py-2.5 text-center text-xs font-medium text-gray-500 dark:text-slate-400 uppercase">Collect</th>
                  </tr>
                </thead>
                <tbody className="bg-white dark:bg-slate-900 divide-y divide-gray-100 dark:divide-slate-800">
                  {/* Recommended channels */}
                  {RECOMMENDED_CHANNELS.map(ch => (
                    <tr key={ch.name} className="hover:bg-gray-50 dark:hover:bg-slate-800/50">
                      <td className="px-4 py-2.5 text-sm text-gray-900 dark:text-slate-200 font-mono">{ch.name}</td>
                      <td className="px-4 py-2.5 text-xs text-gray-500 dark:text-slate-400">All Events</td>
                      <td className="px-4 py-2.5 text-center">
                        <input
                          type="checkbox"
                          checked={isChannelEnabled(ch.name)}
                          onChange={() => handleChannelToggle(ch.name)}
                          disabled={updatePolicyMutation.isPending}
                          className="h-4 w-4 rounded border-gray-300 dark:border-slate-600 text-blue-600 focus:ring-blue-500 dark:bg-slate-700"
                        />
                      </td>
                    </tr>
                  ))}
                  {/* Additional channels */}
                  {ADDITIONAL_CHANNELS.map(ch => (
                    <tr key={ch.name} className="hover:bg-gray-50 dark:hover:bg-slate-800/50">
                      <td className="px-4 py-2.5 text-sm text-gray-900 dark:text-slate-200 font-mono">{ch.name}</td>
                      <td className="px-4 py-2.5 text-xs text-gray-500 dark:text-slate-400">All Events</td>
                      <td className="px-4 py-2.5 text-center">
                        <input
                          type="checkbox"
                          checked={isChannelEnabled(ch.name)}
                          onChange={() => handleChannelToggle(ch.name)}
                          disabled={updatePolicyMutation.isPending}
                          className="h-4 w-4 rounded border-gray-300 dark:border-slate-600 text-blue-600 focus:ring-blue-500 dark:bg-slate-700"
                        />
                      </td>
                    </tr>
                  ))}
                  {/* Custom channels (not in presets) */}
                  {channels
                    .filter(c => ![...RECOMMENDED_CHANNELS, ...ADDITIONAL_CHANNELS].find(p => p.name === c.name))
                    .map(ch => (
                      <tr key={ch.name} className="hover:bg-gray-50 dark:hover:bg-slate-800/50">
                        <td className="px-4 py-2.5 text-sm text-gray-900 dark:text-slate-200 font-mono">{ch.name}</td>
                        <td className="px-4 py-2.5 text-xs text-gray-500 dark:text-slate-400">All Events</td>
                        <td className="px-4 py-2.5 text-center">
                          <input
                            type="checkbox"
                            checked
                            onChange={() => handleChannelToggle(ch.name)}
                            disabled={updatePolicyMutation.isPending}
                            className="h-4 w-4 rounded border-gray-300 dark:border-slate-600 text-blue-600 focus:ring-blue-500 dark:bg-slate-700"
                          />
                        </td>
                      </tr>
                    ))}
                </tbody>
              </table>
            </div>

            {/* Add custom channel */}
            <div className="flex items-center gap-2">
              {showAddChannel ? (
                <>
                  <input
                    type="text"
                    value={customChannelName}
                    onChange={e => setCustomChannelName(e.target.value)}
                    placeholder="Microsoft-Windows-Provider/Operational"
                    className="flex-1 rounded-md border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-3 py-1.5 text-sm text-gray-900 dark:text-slate-200 font-mono placeholder-gray-400 dark:placeholder-slate-500"
                    onKeyDown={e => e.key === 'Enter' && handleAddCustomChannel()}
                  />
                  <button onClick={handleAddCustomChannel} className="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700">Add</button>
                  <button onClick={() => { setShowAddChannel(false); setCustomChannelName('') }} className="rounded-md border border-gray-300 dark:border-slate-600 px-3 py-1.5 text-xs font-medium text-gray-600 dark:text-slate-400 hover:bg-gray-100 dark:hover:bg-slate-700">Cancel</button>
                </>
              ) : (
                <button onClick={() => setShowAddChannel(true)} className="rounded-md border border-dashed border-gray-300 dark:border-slate-600 px-3 py-1.5 text-xs font-medium text-gray-500 dark:text-slate-400 hover:border-gray-400 dark:hover:border-slate-500 hover:text-gray-700 dark:hover:text-slate-300">
                  + Add Custom Channel
                </button>
              )}
            </div>

            {/* Status */}
            <div className="rounded-md bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 px-4 py-3">
              <p className="text-xs text-blue-700 dark:text-blue-400">
                <strong>{channels.length}</strong> channel{channels.length !== 1 ? 's' : ''} configured.
                Policy will be pushed to all enrolled agents.
                {policy?.version && <span className="ml-2 text-blue-500 dark:text-blue-500">(v{policy.version})</span>}
              </p>
            </div>
          </div>
        )}
      </div>

      {/* ══════════════════════════════════════════════════════════ */}
      {/* GitHub Sync (Detection as Code)                           */}
      {/* ══════════════════════════════════════════════════════════ */}
      <div>
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-slate-100">Detection as Code</h2>
            <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">Sync detection rules from GitHub repositories. Rules are validated before import.</p>
          </div>
          <div className="flex gap-2">
            {configs.length > 0 && (
              <button onClick={() => syncAllMutation.mutate()} disabled={syncAllMutation.isPending}
                className="rounded-lg border border-gray-300 dark:border-slate-600 px-3 py-1.5 text-sm font-medium text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700 disabled:opacity-50">
                {syncAllMutation.isPending ? 'Syncing...' : 'Sync All'}
              </button>
            )}
            <button onClick={() => { setShowAdd(true); setEditId(null); setForm({ name: '', repo_url: '', branch: 'main', path: '', token: '', interval: 30, enabled: true, scope: 'account' }) }}
              className="rounded-lg bg-fibratus-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-fibratus-700">
              Add Source
            </button>
          </div>
        </div>

        {/* Existing sources */}
        {configs.length > 0 && (
          <div className="mt-4 space-y-3">
            {configs.map((cfg) => (
              <div key={cfg.id as string} className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4 shadow-sm dark:shadow-slate-900/50">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <span className={'inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium ' + ((cfg.enabled as boolean) ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-400' : 'bg-gray-100 text-gray-600 dark:bg-slate-700 dark:text-slate-400')}>
                      {(cfg.enabled as boolean) ? 'Active' : 'Disabled'}
                    </span>
                    <span className="font-medium text-sm text-gray-900 dark:text-slate-100">{cfg.name as string || 'Unnamed'}</span>
                    <span className="inline-flex items-center rounded-full px-1.5 py-0.5 text-[9px] font-medium bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-400">
                      account-wide
                    </span>
                    <span className="text-xs font-mono text-gray-400 dark:text-slate-500">{cfg.branch as string}</span>
                    {(cfg.path as string) ? <span className="text-xs font-mono text-gray-400 dark:text-slate-500">/{cfg.path as string}</span> : null}
                  </div>
                  <div className="flex items-center gap-2">
                    <button onClick={() => syncOneMutation.mutate(cfg.id as string)} disabled={syncOneMutation.isPending}
                      className="rounded border border-gray-300 dark:border-slate-600 px-2 py-1 text-xs text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700 disabled:opacity-50">
                      Sync
                    </button>
                    <button onClick={() => startEdit(cfg)}
                      className="rounded border border-gray-300 dark:border-slate-600 px-2 py-1 text-xs text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700">
                      Edit
                    </button>
                    <button onClick={() => deleteMutation.mutate(cfg.id as string)}
                      className="rounded border border-red-300 dark:border-red-700 px-2 py-1 text-xs text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20">
                      Delete
                    </button>
                  </div>
                </div>
                <div className="mt-1 text-xs font-mono text-gray-500 dark:text-slate-400 truncate">{cfg.repo_url as string}</div>
              </div>
            ))}
          </div>
        )}

        {configs.length === 0 && !showAdd && (
          <div className="mt-4 rounded-xl border border-dashed border-gray-300 dark:border-slate-600 p-8 text-center text-sm text-gray-400 dark:text-slate-500">
            No GitHub sync sources configured. Click "Add Source" to connect a repository.
          </div>
        )}

        {/* Add/Edit form */}
        {showAdd && (
          <div className="mt-4 rounded-xl border border-fibratus-200 dark:border-fibratus-800 bg-fibratus-50/30 dark:bg-slate-800 p-6 shadow-sm space-y-4">
            <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">{editId ? 'Edit Source' : 'Add GitHub Source'}</h3>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-xs font-medium text-gray-700 dark:text-slate-300 mb-1">Name</label>
                <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} placeholder="My Rules" className={inputCls} />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-700 dark:text-slate-300 mb-1">Repository URL</label>
                <input value={form.repo_url} onChange={e => setForm({ ...form, repo_url: e.target.value })} placeholder="https://github.com/owner/repo" className={inputCls + ' font-mono'} />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-700 dark:text-slate-300 mb-1">Branch</label>
                <input value={form.branch} onChange={e => setForm({ ...form, branch: e.target.value })} placeholder="main" className={inputCls} />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-700 dark:text-slate-300 mb-1">Rules Path</label>
                <input value={form.path} onChange={e => setForm({ ...form, path: e.target.value })} placeholder="rules/" className={inputCls + ' font-mono'} />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-700 dark:text-slate-300 mb-1">GitHub Token (PAT)</label>
                <input type="password" value={form.token} onChange={e => setForm({ ...form, token: e.target.value })} placeholder={editId ? '(unchanged)' : 'ghp_...'} className={inputCls + ' font-mono'} />
              </div>
              <div className="flex items-end gap-4 flex-wrap">
                <div className="pb-1">
                  <span className="inline-flex items-center rounded-full bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400 px-2.5 py-0.5 text-[10px] font-medium">
                    Account-wide
                  </span>
                </div>
                <label className="flex items-center gap-2 cursor-pointer pb-1">
                  <input type="checkbox" checked={form.enabled} onChange={e => setForm({ ...form, enabled: e.target.checked })} className="rounded" />
                  <span className="text-sm text-gray-700 dark:text-slate-300">Auto-sync</span>
                </label>
                <div className="flex items-center gap-1 pb-1">
                  <span className="text-xs text-gray-500">every</span>
                  <input type="number" value={form.interval} onChange={e => setForm({ ...form, interval: Number(e.target.value) })} min={5}
                    className="w-14 rounded border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 text-gray-900 dark:text-slate-100 px-1.5 py-1 text-xs text-center" />
                  <span className="text-xs text-gray-500">min</span>
                </div>
              </div>
            </div>
            <div className="flex gap-3">
              <button onClick={() => saveMutation.mutate()} disabled={saveMutation.isPending || !form.repo_url}
                className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50">
                {saveMutation.isPending ? 'Saving...' : editId ? 'Update' : 'Add Source'}
              </button>
              <button onClick={() => { setShowAdd(false); setEditId(null) }}
                className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700">
                Cancel
              </button>
            </div>
          </div>
        )}

        {/* Sync result */}
        {syncResult && (
          <div className="mt-4 rounded-lg border border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-900 p-4">
            <div className="flex items-center justify-between">
              <h4 className="text-sm font-medium text-gray-900 dark:text-slate-100">Sync Result</h4>
              <button onClick={() => setSyncResult(null)} className="text-xs text-gray-400 hover:text-gray-600">dismiss</button>
            </div>
            <div className="mt-2 flex gap-6 text-sm">
              <span className="text-emerald-600 dark:text-emerald-400">{syncResult.created as number || 0} created</span>
              <span className="text-blue-600 dark:text-blue-400">{syncResult.updated as number || 0} updated</span>
              <span className="text-red-600 dark:text-red-400">{syncResult.deleted as number || 0} deleted</span>
              <span className="text-gray-500">{syncResult.skipped as number || 0} skipped</span>
            </div>
            {(syncResult.errors as string[])?.length > 0 && (
              <div className="mt-2 max-h-32 overflow-auto">
                {(syncResult.errors as string[]).map((err, i) => (
                  <div key={i} className="text-xs text-red-600 dark:text-red-400 py-0.5">{err}</div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>

      {/* ══════════════════════════════════════════════════════════ */}
      {/* SIGMA Rule Converter                                      */}
      {/* ══════════════════════════════════════════════════════════ */}
      <div>
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-slate-100">SIGMA Rule Converter</h2>
            <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
              Convert SIGMA detection rules to Fibratus format. Supports process creation, file, registry, network, DNS, image load, driver load, and Windows Event Log rules.
            </p>
          </div>
        </div>

        {/* Tab switcher */}
        <div className="mt-4 flex gap-1 rounded-lg bg-gray-100 dark:bg-slate-800 p-1 w-fit">
          <button
            onClick={() => setSigmaTab('convert')}
            className={'rounded-md px-4 py-1.5 text-sm font-medium transition-colors ' +
              (sigmaTab === 'convert'
                ? 'bg-white dark:bg-slate-700 text-gray-900 dark:text-slate-100 shadow-sm'
                : 'text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300')
            }
          >
            Convert Rule
          </button>
          <button
            onClick={() => setSigmaTab('sigmahq')}
            className={'rounded-md px-4 py-1.5 text-sm font-medium transition-colors ' +
              (sigmaTab === 'sigmahq'
                ? 'bg-white dark:bg-slate-700 text-gray-900 dark:text-slate-100 shadow-sm'
                : 'text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300')
            }
          >
            SigmaHQ Sync
          </button>
        </div>

        {/* Convert Tab */}
        {sigmaTab === 'convert' && (
          <div className="mt-4 space-y-4">
            {/* Input area */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <label className="text-xs font-medium text-gray-700 dark:text-slate-300">SIGMA Rule (YAML)</label>
                <div className="flex gap-2">
                  <input
                    ref={fileInputRef}
                    type="file"
                    accept=".yml,.yaml"
                    multiple
                    onChange={handleSigmaFileUpload}
                    className="hidden"
                  />
                  <button
                    onClick={() => fileInputRef.current?.click()}
                    className="rounded-md border border-gray-300 dark:border-slate-600 px-3 py-1 text-xs font-medium text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700"
                  >
                    Upload File(s)
                  </button>
                  <button
                    onClick={handleSigmaConvert}
                    disabled={convertMutation.isPending || !sigmaInput.trim()}
                    className="rounded-md bg-fibratus-600 px-4 py-1 text-xs font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
                  >
                    {convertMutation.isPending ? 'Converting...' : 'Convert'}
                  </button>
                </div>
              </div>
              <textarea
                value={sigmaInput}
                onChange={e => { setSigmaInput(e.target.value); setSigmaResult(null) }}
                placeholder={'title: Suspicious Process Creation\nid: 12345678-1234-1234-1234-123456789abc\nstatus: test\nlogsource:\n  category: process_creation\n  product: windows\ndetection:\n  selection:\n    Image|endswith: \'\\\\cmd.exe\'\n    CommandLine|contains: \'whoami\'\n  condition: selection\nlevel: medium'}
                className="w-full h-48 rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-900 text-gray-900 dark:text-slate-100 px-3 py-2 text-xs font-mono focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500 resize-y"
              />
            </div>

            {/* Conversion Result */}
            {sigmaResult && (
              <div className={'rounded-xl border p-4 shadow-sm ' + (
                sigmaResult.success
                  ? 'border-emerald-200 dark:border-emerald-800 bg-emerald-50/30 dark:bg-emerald-900/10'
                  : sigmaResult.unconvertible
                    ? 'border-amber-200 dark:border-amber-800 bg-amber-50/30 dark:bg-amber-900/10'
                    : 'border-red-200 dark:border-red-800 bg-red-50/30 dark:bg-red-900/10'
              )}>
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <span className={'inline-flex items-center rounded-full px-2.5 py-0.5 text-[10px] font-medium ' + (
                      sigmaResult.success
                        ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-400'
                        : sigmaResult.unconvertible
                          ? 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-400'
                          : 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-400'
                    )}>
                      {sigmaResult.success ? 'Converted' : sigmaResult.unconvertible ? 'Not Convertible' : 'Failed'}
                    </span>
                    {sigmaResult.sigma_title && (
                      <span className="text-sm font-medium text-gray-900 dark:text-slate-100">{sigmaResult.sigma_title}</span>
                    )}
                  </div>
                  {sigmaResult.success && (
                    <button
                      onClick={handleSigmaImport}
                      disabled={importMutation.isPending}
                      className="rounded-md bg-emerald-600 px-4 py-1.5 text-xs font-medium text-white hover:bg-emerald-700 disabled:opacity-50"
                    >
                      {importMutation.isPending ? 'Importing...' : 'Import as Rule'}
                    </button>
                  )}
                </div>

                {/* Errors */}
                {sigmaResult.errors && sigmaResult.errors.length > 0 && (
                  <div className="mb-3">
                    {sigmaResult.errors.map((err, i) => (
                      <div key={i} className="text-xs text-red-600 dark:text-red-400 py-0.5">{err}</div>
                    ))}
                  </div>
                )}

                {/* Reason (for unconvertible) */}
                {sigmaResult.reason && (
                  <div className="mb-3 text-xs text-amber-700 dark:text-amber-400">{sigmaResult.reason}</div>
                )}

                {/* Warnings */}
                {sigmaResult.warnings && sigmaResult.warnings.length > 0 && (
                  <div className="mb-3">
                    {sigmaResult.warnings.map((w, i) => (
                      <div key={i} className="text-xs text-amber-600 dark:text-amber-400 py-0.5">Warning: {w}</div>
                    ))}
                  </div>
                )}

                {/* Converted YAML */}
                {sigmaResult.fibratus_yaml && (
                  <div>
                    <div className="flex items-center justify-between mb-1">
                      <span className="text-xs font-medium text-gray-500 dark:text-slate-400">Fibratus Rule</span>
                      <button
                        onClick={() => navigator.clipboard.writeText(sigmaResult.fibratus_yaml || '')}
                        className="text-[10px] text-gray-400 hover:text-gray-600 dark:hover:text-slate-300"
                      >
                        Copy YAML
                      </button>
                    </div>
                    <pre className="rounded-lg bg-gray-900 dark:bg-black p-3 text-xs text-emerald-400 font-mono overflow-x-auto max-h-64 overflow-y-auto whitespace-pre-wrap">
                      {sigmaResult.fibratus_yaml}
                    </pre>
                  </div>
                )}

                {/* Metadata */}
                {sigmaResult.success && (
                  <div className="mt-3 flex gap-4 text-[10px] text-gray-500 dark:text-slate-500">
                    {sigmaResult.severity && <span>Severity: <strong className="text-gray-700 dark:text-slate-300">{sigmaResult.severity}</strong></span>}
                    {sigmaResult.labels?.['technique.id'] && <span>Technique: <strong className="text-gray-700 dark:text-slate-300">{sigmaResult.labels['technique.id']}</strong></span>}
                    {sigmaResult.labels?.['tactic.name'] && <span>Tactic: <strong className="text-gray-700 dark:text-slate-300">{sigmaResult.labels['tactic.name']}</strong></span>}
                  </div>
                )}
              </div>
            )}

            {/* Batch Result */}
            {sigmaBatchResult && (
              <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4 shadow-sm">
                <div className="flex items-center justify-between mb-3">
                  <h4 className="text-sm font-medium text-gray-900 dark:text-slate-100">Batch Import Result</h4>
                  <button onClick={() => setSigmaBatchResult(null)} className="text-xs text-gray-400 hover:text-gray-600">dismiss</button>
                </div>
                <div className="flex gap-6 text-sm">
                  <span className="text-gray-500 dark:text-slate-400">{sigmaBatchResult.total} total</span>
                  <span className="text-emerald-600 dark:text-emerald-400">{sigmaBatchResult.converted} converted</span>
                  <span className="text-red-600 dark:text-red-400">{sigmaBatchResult.failed} failed</span>
                  <span className="text-amber-600 dark:text-amber-400">{sigmaBatchResult.skipped} skipped</span>
                </div>
                {sigmaBatchResult.results.length > 0 && (
                  <div className="mt-3 max-h-48 overflow-auto space-y-1">
                    {sigmaBatchResult.results.map((res, i) => (
                      <div key={i} className="flex items-center gap-2 text-xs py-0.5">
                        <span className={'inline-flex w-16 justify-center rounded-full px-1.5 py-0.5 text-[9px] font-medium ' + (
                          res.success ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-400'
                            : res.unconvertible ? 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-400'
                              : 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-400'
                        )}>
                          {res.success ? 'OK' : res.unconvertible ? 'Skip' : 'Fail'}
                        </span>
                        <span className="text-gray-700 dark:text-slate-300 truncate">{res.sigma_title || res.sigma_id || `Rule ${i + 1}`}</span>
                        {res.errors && res.errors.length > 0 && (
                          <span className="text-red-500 dark:text-red-400 truncate">{res.errors[0]}</span>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Info box */}
            <div className="rounded-md bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 px-4 py-3">
              <p className="text-xs text-blue-700 dark:text-blue-400">
                <strong>Supported logsources:</strong> process_creation, file_event, file_delete, registry_event, registry_set, network_connection,
                dns_query, image_load, driver_load, process_access, create_remote_thread, and Windows Event Logs (Security, Sysmon, PowerShell, System, and more).
                Rules with unsupported logsources (Linux, macOS, cloud) will be flagged as unconvertible.
              </p>
            </div>
          </div>
        )}

        {/* SigmaHQ Sync Tab */}
        {sigmaTab === 'sigmahq' && (
          <div className="mt-4 space-y-4">
            <div className="rounded-md bg-purple-50 dark:bg-purple-900/20 border border-purple-200 dark:border-purple-800 px-4 py-3">
              <p className="text-xs text-purple-700 dark:text-purple-400">
                <strong>SigmaHQ Integration:</strong> Connect directly to the SigmaHQ GitHub repository to sync community detection rules.
                Use the Detection as Code section above to add <code className="bg-purple-100 dark:bg-purple-900/40 px-1 rounded">github.com/SigmaHQ/sigma</code> as a source.
                Set the rules path to <code className="bg-purple-100 dark:bg-purple-900/40 px-1 rounded">rules/windows</code> to pull all Windows-compatible SIGMA rules.
                Rules will be automatically converted to Fibratus format during sync.
              </p>
            </div>

            <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-5 shadow-sm">
              <h4 className="text-sm font-semibold text-gray-900 dark:text-slate-100 mb-3">Quick Setup: SigmaHQ</h4>
              <p className="text-xs text-gray-500 dark:text-slate-400 mb-4">
                Add the SigmaHQ repository as a Detection as Code source to automatically sync and convert SIGMA rules.
                Only convertible rules (Windows ETW + Event Log compatible) will be imported.
              </p>
              <div className="space-y-3">
                <div className="flex items-center gap-3">
                  <span className="w-24 text-xs font-medium text-gray-500 dark:text-slate-400">Repository:</span>
                  <code className="text-xs font-mono text-gray-700 dark:text-slate-300 bg-gray-100 dark:bg-slate-900 px-2 py-1 rounded">github.com/SigmaHQ/sigma</code>
                </div>
                <div className="flex items-center gap-3">
                  <span className="w-24 text-xs font-medium text-gray-500 dark:text-slate-400">Branch:</span>
                  <code className="text-xs font-mono text-gray-700 dark:text-slate-300 bg-gray-100 dark:bg-slate-900 px-2 py-1 rounded">master</code>
                </div>
                <div className="flex items-center gap-3">
                  <span className="w-24 text-xs font-medium text-gray-500 dark:text-slate-400">Rules Path:</span>
                  <code className="text-xs font-mono text-gray-700 dark:text-slate-300 bg-gray-100 dark:bg-slate-900 px-2 py-1 rounded">rules/windows</code>
                </div>
                <div className="pt-2 border-t border-gray-100 dark:border-slate-700">
                  <p className="text-[10px] text-gray-400 dark:text-slate-500">
                    Recommended paths for specific categories:
                  </p>
                  <div className="mt-1 grid grid-cols-2 gap-1">
                    <code className="text-[10px] font-mono text-gray-500 dark:text-slate-500">rules/windows/process_creation</code>
                    <code className="text-[10px] font-mono text-gray-500 dark:text-slate-500">rules/windows/registry</code>
                    <code className="text-[10px] font-mono text-gray-500 dark:text-slate-500">rules/windows/file</code>
                    <code className="text-[10px] font-mono text-gray-500 dark:text-slate-500">rules/windows/network_connection</code>
                    <code className="text-[10px] font-mono text-gray-500 dark:text-slate-500">rules/windows/dns_query</code>
                    <code className="text-[10px] font-mono text-gray-500 dark:text-slate-500">rules/windows/image_load</code>
                    <code className="text-[10px] font-mono text-gray-500 dark:text-slate-500">rules/windows/powershell</code>
                    <code className="text-[10px] font-mono text-gray-500 dark:text-slate-500">rules/windows/driver_load</code>
                  </div>
                </div>
              </div>
            </div>

            {/* Conversion format note */}
            <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-5 shadow-sm">
              <h4 className="text-sm font-semibold text-gray-900 dark:text-slate-100 mb-3">SIGMA to Fibratus Mapping</h4>
              <div className="space-y-2 text-xs">
                <div className="grid grid-cols-3 gap-2 text-gray-500 dark:text-slate-400 font-medium border-b border-gray-100 dark:border-slate-700 pb-1">
                  <span>SIGMA Logsource</span><span>Fibratus Event</span><span>Key Fields</span>
                </div>
                {[
                  ['process_creation', 'spawn_process', 'ps.exe, ps.cmdline, ps.parent.exe'],
                  ['file_event', 'create_file', 'file.name, ps.exe'],
                  ['registry_event', 'modify_registry', 'registry.path, registry.value'],
                  ['network_connection', 'connect_socket', 'net.dip, net.dport, ps.exe'],
                  ['dns_query', 'query_dns', 'dns.name, dns.rr, ps.exe'],
                  ['image_load', 'load_module', 'image.name, ps.exe'],
                  ['driver_load', 'load_driver', 'image.name, image.signature.*'],
                  ['process_access', 'open_process', 'ps.exe, ps.access.mask'],
                  ['Sysmon (generic)', 'eventlog_event', 'eventlog.channel, eventlog.event.id'],
                  ['PowerShell', 'eventlog_event', 'eventlog.data[ScriptBlockText]'],
                  ['Security Log', 'eventlog_event', 'eventlog.event.id, eventlog.data[*]'],
                ].map(([sigma, fibratus, fields]) => (
                  <div key={sigma} className="grid grid-cols-3 gap-2 text-gray-700 dark:text-slate-300">
                    <span className="font-mono">{sigma}</span>
                    <span className="font-mono text-fibratus-600 dark:text-fibratus-400">{fibratus}</span>
                    <span className="text-gray-500 dark:text-slate-400">{fields}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
