import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type User } from '../../lib/api'

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
// Account Tab
// ================================================================

interface AccountSettings {
  account_name: string
  plan: string
  require_2fa: boolean
  tamper_protection_enabled: boolean
  isolation_whitelist: string[]
  allowed_file_extensions: string[]
  telemetry_retention_days: number
  eventlog_enabled: boolean
  org_protection: Array<{
    id: string
    name: string
    tamper_protection_enabled: boolean
    telemetry_retention_days: number
  }>
}

export default function AccountTab() {
  const queryClient = useQueryClient()

  // ── Queries ──────────────────────────────────────────────────

  const { data: settingsData, isLoading } = useQuery({
    queryKey: ['account-settings'],
    queryFn: () => api.getAccountSettings(),
  })
  const settings = settingsData?.data as AccountSettings | undefined

  const { data: usersData } = useQuery({
    queryKey: ['users'],
    queryFn: () => api.getUsers(),
  })
  const users = (usersData?.data || []) as User[]
  const usersWith2FA = users.filter(u => u.totp_enabled).length
  const usersWithout2FA = users.filter(u => !u.totp_enabled).length

  // ── Mutations ────────────────────────────────────────────────

  const updateSettingsMut = useMutation({
    mutationFn: (data: { require_2fa?: boolean; tamper_protection_enabled?: boolean; isolation_whitelist?: string[]; allowed_file_extensions?: string[] }) =>
      api.updateAccountSettings(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['account-settings'] })
    },
  })

  const orgTamperMut = useMutation({
    mutationFn: ({ orgId, enabled }: { orgId: string; enabled: boolean }) =>
      api.updateOrgTamperProtection(orgId, enabled),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['account-settings'] })
      setOrgToggling(null)
    },
    onError: () => setOrgToggling(null),
  })

  // ── Local state ──────────────────────────────────────────────

  const [newWhitelistEntry, setNewWhitelistEntry] = useState('')
  const [tamperToggling, setTamperToggling] = useState(false)
  const [orgToggling, setOrgToggling] = useState<string | null>(null)

  // ── Derived ──────────────────────────────────────────────────

  const tamperEnabled = settings?.tamper_protection_enabled ?? false
  const whitelist = settings?.isolation_whitelist ?? []
  const orgProtection = settings?.org_protection ?? []

  // ── Handlers ─────────────────────────────────────────────────

  // Always include current values to prevent Go zero-value overwriting
  const currentSettings = () => ({
    require_2fa: settings?.require_2fa ?? false,
    tamper_protection_enabled: tamperEnabled,
    eventlog_enabled: settings?.eventlog_enabled ?? false,
    isolation_whitelist: whitelist,
  })


  const handleTamperToggle = async () => {
    setTamperToggling(true)
    try {
      await updateSettingsMut.mutateAsync({ ...currentSettings(), tamper_protection_enabled: !tamperEnabled })
    } finally {
      setTamperToggling(false)
    }
  }

  const handleOrgTamperToggle = (orgId: string, currentState: boolean) => {
    setOrgToggling(orgId)
    orgTamperMut.mutate({ orgId, enabled: !currentState })
  }

  const handleAddWhitelist = () => {
    const entry = newWhitelistEntry.trim()
    if (!entry || whitelist.includes(entry)) return
    updateSettingsMut.mutate({ ...currentSettings(), isolation_whitelist: [...whitelist, entry] })
    setNewWhitelistEntry('')
  }

  const handleRemoveWhitelist = (entry: string) => {
    updateSettingsMut.mutate({ ...currentSettings(), isolation_whitelist: whitelist.filter(e => e !== entry) })
  }

  // ── Loading ──────────────────────────────────────────────────

  if (isLoading) {
    return (
      <div className="mt-6 flex items-center justify-center py-12">
        <p className="text-sm text-gray-400 dark:text-slate-500">Loading account settings...</p>
      </div>
    )
  }

  // ── Render ───────────────────────────────────────────────────

  return (
    <div className="mt-6 space-y-6">
      {/* ── Account Information ──────────────────────────────── */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
        <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Account Information</h3>
        <div className="mt-4 grid grid-cols-2 gap-6">
          <div>
            <p className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wider">Account Name</p>
            <p className="mt-1 text-sm font-medium text-gray-900 dark:text-slate-100">{settings?.account_name || '-'}</p>
          </div>
          <div>
            <p className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wider">Plan</p>
            <span className="mt-1 inline-flex rounded-full bg-blue-50 px-2.5 py-0.5 text-xs font-medium text-blue-700">
              {settings?.plan || '-'}
            </span>
          </div>
        </div>
      </div>

      {/* ── Two-Factor Authentication ────────────────────────── */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
        <div>
          <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Two-Factor Authentication</h3>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
            Two-factor authentication is mandatory for all users. Users who have not set up 2FA will be prompted during login.
          </p>
        </div>

        <div className="mt-4 rounded-lg border border-emerald-200 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-900/20 px-4 py-3">
          <p className="text-xs text-emerald-800 dark:text-emerald-300">
            <strong>Always enforced:</strong> 2FA is required on all accounts. Users must set up an authenticator app on first login.
          </p>
        </div>

        {/* 2FA stats */}
        <div className="mt-5 grid grid-cols-2 gap-4">
          <div className="rounded-lg border border-gray-100 dark:border-slate-700 bg-gray-50 dark:bg-slate-900 px-4 py-3">
            <p className="text-xs font-medium text-gray-500 dark:text-slate-400">Users with 2FA</p>
            <div className="mt-1 flex items-baseline gap-2">
              <span className="text-xl font-bold text-emerald-600">{usersWith2FA}</span>
              {users.length > 0 && (
                <span className="text-xs text-gray-400 dark:text-slate-500">
                  ({Math.round((usersWith2FA / users.length) * 100)}%)
                </span>
              )}
            </div>
          </div>
          <div className="rounded-lg border border-gray-100 dark:border-slate-700 bg-gray-50 dark:bg-slate-900 px-4 py-3">
            <p className="text-xs font-medium text-gray-500 dark:text-slate-400">Users without 2FA</p>
            <div className="mt-1 flex items-baseline gap-2">
              <span className="text-xl font-bold text-gray-600 dark:text-slate-400">{usersWithout2FA}</span>
              {users.length > 0 && (
                <span className="text-xs text-gray-400 dark:text-slate-500">
                  ({Math.round((usersWithout2FA / users.length) * 100)}%)
                </span>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* ── Tamper Protection ────────────────────────────────── */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
        <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Tamper Protection</h3>
        <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
          Prevent unauthorized modification, termination, or uninstallation of the agent on endpoints.
        </p>

        {/* Account-wide toggle */}
        <div className="mt-4 flex items-center justify-between">
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium text-gray-900 dark:text-slate-100">Enable for all organizations</p>
            <p className="text-xs text-gray-500 dark:text-slate-400 mt-1">
              When enabled, all agents across all organizations will have tamper protection enforced. Individual agent and organization toggles will be locked.
            </p>
          </div>
          <ToggleSwitch
            enabled={tamperEnabled}
            onToggle={handleTamperToggle}
            disabled={tamperToggling || updateSettingsMut.isPending}
          />
        </div>

        {/* Per-org toggles */}
        {orgProtection.length > 0 && (
          <div className="border-t border-gray-100 dark:border-slate-700 pt-4 mt-5">
            <p className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wider mb-3">Per-Organization</p>
            <div className="space-y-3">
              {orgProtection.map(org => {
                const effectiveEnabled = tamperEnabled || org.tamper_protection_enabled
                const lockedByAccount = tamperEnabled
                return (
                  <div key={org.id} className="flex items-center justify-between py-2 px-3 rounded-lg bg-gray-50 dark:bg-slate-900/50">
                    <div className="flex items-center gap-3">
                      <span className="text-sm text-gray-900 dark:text-slate-200 font-medium">{org.name}</span>
                      {lockedByAccount && (
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 font-medium">
                          Locked by account policy
                        </span>
                      )}
                    </div>
                    <div className="flex items-center gap-4">
                      <ToggleSwitch
                        enabled={effectiveEnabled}
                        onToggle={() => handleOrgTamperToggle(org.id, org.tamper_protection_enabled)}
                        disabled={lockedByAccount || orgToggling === org.id || orgTamperMut.isPending}
                      />
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        )}
      </div>

      {/* ── Isolation Whitelist ───────────────────────────────── */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
        <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Isolation Whitelist</h3>
        <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
          These IPs/CIDRs will always be allowed during network isolation (e.g., RMM servers, jump boxes).
        </p>

        <div className="mt-4 flex gap-2">
          <input
            type="text"
            value={newWhitelistEntry}
            onChange={e => setNewWhitelistEntry(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') handleAddWhitelist() }}
            placeholder="IP address or CIDR (e.g., 10.0.0.5 or 192.168.1.0/24)"
            className="flex-1 rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-4 py-2 text-sm font-mono text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
          />
          <button
            onClick={handleAddWhitelist}
            disabled={!newWhitelistEntry.trim() || updateSettingsMut.isPending}
            className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
          >
            Add
          </button>
        </div>

        {whitelist.length > 0 ? (
          <div className="mt-4 space-y-2">
            {whitelist.map(entry => (
              <div key={entry} className="flex items-center justify-between rounded-lg border border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-900 px-4 py-2.5">
                <code className="text-sm font-mono text-gray-900 dark:text-slate-100">{entry}</code>
                <button
                  onClick={() => handleRemoveWhitelist(entry)}
                  disabled={updateSettingsMut.isPending}
                  className="text-xs font-medium text-red-600 dark:text-red-400 hover:text-red-800 dark:hover:text-red-300 disabled:opacity-50"
                >
                  Remove
                </button>
              </div>
            ))}
          </div>
        ) : (
          <p className="mt-4 text-sm text-gray-400 dark:text-slate-500">
            No whitelist entries configured. The fleet server will always be allowed automatically.
          </p>
        )}
      </div>

      {/* ── File Access Compliance ────────────────────────────── */}
      <FileAccessComplianceSection
        extensions={settings?.allowed_file_extensions || []}
        onSave={(exts) => updateSettingsMut.mutate({ ...currentSettings(), allowed_file_extensions: exts })}
        saving={updateSettingsMut.isPending}
      />

      {/* ── Your Security ─────────────────────────────────────── */}
      <SecuritySection />
    </div>
  )
}

// ================================================================
// File Access Compliance Section
// ================================================================

const PRESETS = {
  strict: {
    name: 'CMMC / HIPAA (Strict)',
    description: 'Only executables, scripts, logs, and forensic artifacts. Blocks all documents, images, archives, and data files.',
    extensions: ['.exe', '.dll', '.sys', '.drv', '.ps1', '.bat', '.cmd', '.lnk', '.log', '.evtx', '.dmp', '.pf', '.reg'],
  },
  standard: {
    name: 'Standard',
    description: 'Security-relevant files including configs, certificates, and system metadata. Blocks documents, images, and archives.',
    extensions: [
      '.exe', '.dll', '.sys', '.drv', '.ocx', '.cpl', '.scr', '.com',
      '.ps1', '.psm1', '.psd1', '.bat', '.cmd', '.vbs', '.vbe', '.js', '.jse', '.wsf', '.wsh', '.hta',
      '.lnk', '.url', '.reg', '.inf', '.ini', '.cfg', '.conf', '.config', '.xml', '.json', '.yaml', '.yml', '.toml',
      '.log', '.evtx', '.etl', '.dmp', '.mdmp', '.pf', '.manifest', '.cat', '.mum',
      '.cer', '.crt', '.pem', '.p7b', '.pfx', '.job',
    ],
  },
  permissive: {
    name: 'Permissive',
    description: 'No file type restrictions. All files can be downloaded. Not recommended for regulated environments.',
    extensions: [],
  },
}

function detectActivePreset(extensions: string[]): string {
  if (extensions.length === 0) return 'permissive'
  const sorted = [...extensions].sort().join(',')
  if (sorted === [...PRESETS.strict.extensions].sort().join(',')) return 'strict'
  if (sorted === [...PRESETS.standard.extensions].sort().join(',')) return 'standard'
  return 'custom'
}

function FileAccessComplianceSection({ extensions, onSave, saving }: { extensions: string[]; onSave: (exts: string[]) => void; saving: boolean }) {
  const [editing, setEditing] = useState(false)
  const [editExts, setEditExts] = useState<string[]>(extensions)
  const [newExt, setNewExt] = useState('')
  const [showExts, setShowExts] = useState(false)

  const activePreset = detectActivePreset(extensions)
  const presetInfo = activePreset === 'custom'
    ? { name: 'Custom Policy', description: `${extensions.length} allowed extensions (manually configured)` }
    : PRESETS[activePreset as keyof typeof PRESETS]

  const handlePreset = (key: string) => {
    const preset = PRESETS[key as keyof typeof PRESETS]
    setEditExts(preset.extensions)
    onSave(preset.extensions)
    setEditing(false)
  }

  const handleAddExt = () => {
    let ext = newExt.trim().toLowerCase()
    if (!ext) return
    if (!ext.startsWith('.')) ext = '.' + ext
    if (editExts.includes(ext)) return
    setEditExts([...editExts, ext].sort())
    setNewExt('')
  }

  const handleRemoveExt = (ext: string) => setEditExts(editExts.filter(e => e !== ext))

  const handleSave = () => { onSave(editExts); setEditing(false) }

  const presetColor = activePreset === 'strict'
    ? 'border-red-300 dark:border-red-700 bg-red-50 dark:bg-red-900/20'
    : activePreset === 'standard'
      ? 'border-blue-300 dark:border-blue-700 bg-blue-50 dark:bg-blue-900/20'
      : activePreset === 'permissive'
        ? 'border-amber-300 dark:border-amber-700 bg-amber-50 dark:bg-amber-900/20'
        : 'border-purple-300 dark:border-purple-700 bg-purple-50 dark:bg-purple-900/20'

  const presetTextColor = activePreset === 'strict'
    ? 'text-red-800 dark:text-red-300'
    : activePreset === 'standard'
      ? 'text-blue-800 dark:text-blue-300'
      : activePreset === 'permissive'
        ? 'text-amber-800 dark:text-amber-300'
        : 'text-purple-800 dark:text-purple-300'

  return (
    <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
      <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">File Access Compliance</h3>
      <p className="mt-1 text-xs text-gray-500 dark:text-slate-400">
        Restricts which file types analysts can download from endpoints. Applies to all organizations in this account.
      </p>

      {/* Active policy indicator */}
      <div className={'mt-4 rounded-lg border px-4 py-3 ' + presetColor}>
        <div className="flex items-center justify-between">
          <div>
            <p className={'text-sm font-semibold ' + presetTextColor}>{presetInfo.name}</p>
            <p className={'text-xs mt-0.5 ' + presetTextColor + ' opacity-80'}>{presetInfo.description}</p>
          </div>
          {!editing && (
            <button onClick={() => { setEditExts(extensions); setEditing(true) }}
              className="rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-1.5 text-xs font-medium text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-600">
              Change Policy
            </button>
          )}
        </div>
      </div>

      {/* Editing: preset selection */}
      {editing && (
        <div className="mt-4 space-y-4">
          <p className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wider">Select a policy</p>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            {Object.entries(PRESETS).map(([key, preset]) => (
              <button key={key} onClick={() => handlePreset(key)}
                className={'rounded-lg border p-4 text-left transition-all hover:shadow-md ' +
                  (key === 'strict' ? 'border-red-200 dark:border-red-800 hover:border-red-400' :
                   key === 'standard' ? 'border-blue-200 dark:border-blue-800 hover:border-blue-400' :
                   'border-amber-200 dark:border-amber-800 hover:border-amber-400')
                }>
                <p className={'text-sm font-semibold ' +
                  (key === 'strict' ? 'text-red-700 dark:text-red-400' :
                   key === 'standard' ? 'text-blue-700 dark:text-blue-400' :
                   'text-amber-700 dark:text-amber-400')
                }>{preset.name}</p>
                <p className="text-xs text-gray-500 dark:text-slate-400 mt-1">{preset.description}</p>
                <p className="text-[10px] text-gray-400 dark:text-slate-500 mt-2">
                  {preset.extensions.length === 0 ? 'No restrictions' : `${preset.extensions.length} extensions`}
                </p>
              </button>
            ))}
          </div>

          {/* Custom editing */}
          <div className="border-t border-gray-100 dark:border-slate-700 pt-4">
            <p className="text-xs font-medium text-gray-500 dark:text-slate-400 mb-2">Or customize extensions:</p>
            <div className="flex flex-wrap gap-1.5 mb-3">
              {editExts.map(ext => (
                <span key={ext} className="inline-flex items-center gap-1 rounded-full bg-gray-100 dark:bg-slate-700 px-2 py-0.5 text-xs font-mono text-gray-700 dark:text-slate-300">
                  {ext}
                  <button onClick={() => handleRemoveExt(ext)} className="text-red-400 hover:text-red-600">&times;</button>
                </span>
              ))}
            </div>
            <div className="flex gap-2">
              <input type="text" value={newExt} onChange={e => setNewExt(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleAddExt()} placeholder=".ext"
                className="w-32 rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-1.5 text-sm font-mono text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:outline-none" />
              <button onClick={handleAddExt} className="rounded-lg bg-gray-200 dark:bg-slate-600 px-3 py-1.5 text-xs font-medium text-gray-700 dark:text-slate-300 hover:bg-gray-300 dark:hover:bg-slate-500">Add</button>
              <div className="flex-1" />
              <button onClick={handleSave} disabled={saving} className="rounded-lg bg-fibratus-600 px-4 py-1.5 text-xs font-medium text-white hover:bg-fibratus-700 disabled:opacity-50">
                {saving ? 'Saving...' : 'Save Custom Policy'}
              </button>
              <button onClick={() => setEditing(false)} className="rounded-lg border border-gray-300 dark:border-slate-600 px-3 py-1.5 text-xs text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700">Cancel</button>
            </div>
          </div>
        </div>
      )}

      {/* Collapsible extension list (not editing) */}
      {!editing && extensions.length > 0 && (
        <div className="mt-3">
          <button onClick={() => setShowExts(!showExts)} className="text-xs text-gray-400 dark:text-slate-500 hover:text-gray-600 dark:hover:text-slate-300">
            {showExts ? 'Hide' : 'Show'} {extensions.length} allowed extensions
          </button>
          {showExts && (
            <div className="mt-2 flex flex-wrap gap-1">
              {extensions.map(ext => (
                <span key={ext} className="rounded bg-gray-100 dark:bg-slate-700 px-1.5 py-0.5 text-[10px] font-mono text-gray-500 dark:text-slate-400">{ext}</span>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ================================================================
// Security Section (Personal 2FA)
// ================================================================

function SecuritySection() {
  const queryClient = useQueryClient()

  const { data: userData } = useQuery({
    queryKey: ['current-user'],
    queryFn: () => api.getCurrentUser(),
  })
  const user = userData?.data as User | undefined

  const { data: totpData } = useQuery({
    queryKey: ['totp-status'],
    queryFn: () => api.getTOTPStatus(),
  })
  const totpEnabled = (totpData?.data as { enabled: boolean } | undefined)?.enabled ?? false

  const [setupStep, setSetupStep] = useState<'idle' | 'setup' | 'verify' | 'done'>('idle')
  const [totpSecret, setTotpSecret] = useState('')
  const [verifyCode, setVerifyCode] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const [setupError, setSetupError] = useState('')
  const [qrDataUrl, setQrDataUrl] = useState('')
  const [showDisable, setShowDisable] = useState(false)
  const [disablePassword, setDisablePassword] = useState('')
  const [disableError, setDisableError] = useState('')

  const setupMutation = useMutation({
    mutationFn: () => api.setupTOTP(),
    onSuccess: (res) => {
      if (res.error) { setSetupError(res.error.message); return }
      const data = res.data as { secret: string; uri: string; qr: string } | undefined
      if (data) {
        setTotpSecret(data.secret)
        setQrDataUrl(data.qr || '')
        setSetupStep('verify')
      }
    },
    onError: () => setSetupError('Failed to start 2FA setup'),
  })

  const verifyMutation = useMutation({
    mutationFn: () => api.verifyTOTP(verifyCode),
    onSuccess: (res) => {
      if (res.error) { setSetupError(res.error.message); return }
      const data = res.data as { enabled: boolean; recovery_codes: string[] } | undefined
      if (data) {
        setRecoveryCodes(data.recovery_codes || [])
        setSetupStep('done')
        queryClient.invalidateQueries({ queryKey: ['totp-status'] })
      }
    },
    onError: () => setSetupError('Failed to verify code'),
  })

  const disableMutation = useMutation({
    mutationFn: () => api.disableTOTP(disablePassword),
    onSuccess: (res) => {
      if (res.error) { setDisableError(res.error.message); return }
      setShowDisable(false)
      setDisablePassword('')
      setDisableError('')
      queryClient.invalidateQueries({ queryKey: ['totp-status'] })
    },
    onError: () => setDisableError('Failed to disable 2FA'),
  })

  const resetSetup = () => {
    setSetupStep('idle')
    setTotpSecret('')
    setQrDataUrl('')
    setVerifyCode('')
    setRecoveryCodes([])
    setSetupError('')
  }

  const roleLabel = (role: string) => {
    switch (role) {
      case 'admin': return 'Admin'
      case 'analyst': return 'Analyst'
      case 'viewer': return 'Viewer'
      default: return role
    }
  }

  const roleBadgeColor = (role: string) => {
    switch (role) {
      case 'admin': return 'text-purple-700 bg-purple-50 border-purple-200'
      case 'analyst': return 'text-blue-700 bg-blue-50 border-blue-200'
      case 'viewer': return 'text-gray-700 bg-gray-50 border-gray-200'
      default: return 'text-gray-700 bg-gray-50 border-gray-200'
    }
  }

  return (
    <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50 divide-y divide-gray-100 dark:divide-slate-700">
      <div className="px-6 py-4">
        <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Your Security</h3>
        <p className="text-xs text-gray-500 dark:text-slate-400 mt-1">Manage your personal security settings and two-factor authentication.</p>
      </div>

      {user && (
        <div className="px-6 py-5">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-gray-900 dark:text-slate-100">{user.name || user.email}</p>
              <p className="text-sm text-gray-500 dark:text-slate-400">{user.email}</p>
            </div>
            <span className={'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ' + roleBadgeColor(user.role)}>
              {roleLabel(user.role)}
            </span>
          </div>
        </div>
      )}

      <div className="px-6 py-5">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-gray-900 dark:text-slate-100">Two-Factor Authentication</p>
            <p className="text-sm text-gray-500 dark:text-slate-400">
              {totpEnabled ? 'Your account is protected with 2FA.' : 'Add an extra layer of security to your account.'}
            </p>
          </div>
          <div className="flex items-center gap-3">
            <span className={'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ' +
              (totpEnabled ? 'text-emerald-700 bg-emerald-50' : 'text-gray-500 bg-gray-100')}>
              {totpEnabled ? 'Enabled' : 'Disabled'}
            </span>
            {!totpEnabled && setupStep === 'idle' && (
              <button
                onClick={() => { setSetupError(''); setupMutation.mutate() }}
                disabled={setupMutation.isPending}
                className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
              >
                {setupMutation.isPending ? 'Setting up...' : 'Enable 2FA'}
              </button>
            )}
            {/* 2FA is mandatory — no disable option */}
          </div>
        </div>

        {setupError && (
          <div className="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700">{setupError}</div>
        )}

        {setupStep === 'verify' && (
          <div className="mt-4 rounded-lg border border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-900 p-4 space-y-4">
            <div>
              <p className="text-sm font-medium text-gray-900 dark:text-slate-100">1. Scan with your authenticator app</p>
              <p className="mt-1 text-xs text-gray-500">Scan this QR code with your authenticator app (Google Authenticator, Authy, etc.)</p>
            </div>
            <div className="flex flex-col items-center py-2">
              {qrDataUrl ? (
                <img src={qrDataUrl} alt="Scan with authenticator app" className="rounded-lg border border-gray-200 bg-white p-2" width={200} height={200} />
              ) : (
                <div className="flex h-[200px] w-[200px] items-center justify-center rounded-lg border border-gray-200 bg-white text-xs text-gray-400">Generating QR code...</div>
              )}
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-500 mb-1">Or enter the secret key manually</label>
              <div className="flex items-center gap-2">
                <code className="flex-1 select-all rounded-lg border border-gray-200 bg-white px-3 py-2 font-mono text-sm text-gray-900 break-all">{totpSecret}</code>
                <button onClick={() => navigator.clipboard.writeText(totpSecret)} className="rounded-lg border border-gray-300 px-3 py-2 text-xs font-medium text-gray-600 hover:bg-gray-100">Copy</button>
              </div>
            </div>
            <div>
              <p className="text-sm font-medium text-gray-900 dark:text-slate-100">2. Enter the verification code</p>
              <div className="mt-2 flex items-center gap-3">
                <input
                  type="text"
                  inputMode="numeric"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  value={verifyCode}
                  onChange={(e) => setVerifyCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                  className="w-40 rounded-lg border border-gray-300 px-3 py-2 text-sm text-center font-mono tracking-widest focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                  placeholder="000000"
                />
                <button
                  onClick={() => { setSetupError(''); verifyMutation.mutate() }}
                  disabled={verifyCode.length !== 6 || verifyMutation.isPending}
                  className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
                >
                  {verifyMutation.isPending ? 'Verifying...' : 'Verify'}
                </button>
                <button onClick={resetSetup} className="text-sm text-gray-500 hover:text-gray-700">Cancel</button>
              </div>
            </div>
          </div>
        )}

        {setupStep === 'done' && recoveryCodes.length > 0 && (
          <div className="mt-4 rounded-lg border border-emerald-200 bg-emerald-50 p-4 space-y-3">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-semibold text-emerald-800">Two-factor authentication enabled</p>
                <p className="text-xs text-emerald-600">Save these recovery codes in a secure location. Each code can only be used once.</p>
              </div>
              <button onClick={() => navigator.clipboard.writeText(recoveryCodes.join('\n'))} className="rounded-lg bg-emerald-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-emerald-700">Copy All</button>
            </div>
            <div className="grid grid-cols-2 gap-2">
              {recoveryCodes.map((code, i) => (
                <code key={i} className="rounded border border-emerald-200 bg-white px-3 py-1.5 font-mono text-sm text-gray-900 text-center">{code}</code>
              ))}
            </div>
            <button onClick={resetSetup} className="text-sm font-medium text-emerald-700 hover:text-emerald-800">Done</button>
          </div>
        )}

        {showDisable && (
          <div className="mt-4 rounded-lg border border-red-200 bg-red-50 p-4 space-y-3">
            <p className="text-sm font-medium text-red-800">Confirm password to disable 2FA</p>
            {disableError && (
              <div className="rounded-lg border border-red-300 bg-white px-3 py-2 text-sm text-red-700">{disableError}</div>
            )}
            <div className="flex items-center gap-3">
              <input
                type="password"
                value={disablePassword}
                onChange={(e) => setDisablePassword(e.target.value)}
                placeholder="Enter your password"
                className="w-64 rounded-lg border border-red-300 px-3 py-2 text-sm focus:border-red-500 focus:outline-none focus:ring-1 focus:ring-red-500"
              />
              <button
                onClick={() => { setDisableError(''); disableMutation.mutate() }}
                disabled={!disablePassword || disableMutation.isPending}
                className="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
              >
                {disableMutation.isPending ? 'Disabling...' : 'Disable 2FA'}
              </button>
              <button
                onClick={() => { setShowDisable(false); setDisablePassword(''); setDisableError('') }}
                className="text-sm text-gray-500 hover:text-gray-700"
              >
                Cancel
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
