import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type EnrollmentToken, type Organization, type User } from '../lib/api'
import ConfirmDialog from '../components/ConfirmDialog'
// QR code generated server-side — no client-side QR library needed

function CopyButton({ text, label = 'Copy' }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false)
  const handleCopy = () => {
    navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }
  return (
    <button
      onClick={handleCopy}
      className="shrink-0 rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-xs font-medium text-gray-600 dark:text-slate-400 hover:bg-gray-100 dark:hover:bg-slate-600"
    >
      {copied ? 'Copied!' : label}
    </button>
  )
}

export default function Settings() {
  const queryClient = useQueryClient()

  // Enrollment token state
  const [showCreateToken, setShowCreateToken] = useState(false)
  const [tokenName, setTokenName] = useState('')
  const [maxUses, setMaxUses] = useState(50)
  const [createdToken, setCreatedToken] = useState<string | null>(null)
  const [deleteTokenTarget, setDeleteTokenTarget] = useState<EnrollmentToken | null>(null)
  const [expandedTokenId, setExpandedTokenId] = useState<string | null>(null)

  // Organization state
  const [showCreateOrg, setShowCreateOrg] = useState(false)
  const [orgName, setOrgName] = useState('')
  const [orgSlug, setOrgSlug] = useState('')
  const [orgError, setOrgError] = useState('')
  const [deleteOrgTarget, setDeleteOrgTarget] = useState<Organization | null>(null)

  const { data: tokensData } = useQuery({
    queryKey: ['enrollment-tokens'],
    queryFn: () => api.getEnrollmentTokens(),
  })

  const { data: orgsData } = useQuery({
    queryKey: ['organizations'],
    queryFn: () => api.getOrganizations(),
  })

  const createTokenMutation = useMutation({
    mutationFn: () =>
      api.createEnrollmentToken({
        name: tokenName || 'enrollment-token',
        max_uses: maxUses,
        expires_in: 86400000000000,
      }),
    onSuccess: (res) => {
      const data = res.data as EnrollmentToken | undefined
      if (data) {
        setCreatedToken(data.id)
        setShowCreateToken(false)
        setTokenName('')
        queryClient.invalidateQueries({ queryKey: ['enrollment-tokens'] })
      }
    },
  })

  const deleteTokenMutation = useMutation({
    mutationFn: (id: string) => api.deleteEnrollmentToken(id),
    onSuccess: () => {
      setDeleteTokenTarget(null)
      queryClient.invalidateQueries({ queryKey: ['enrollment-tokens'] })
    },
  })

  const createOrgMutation = useMutation({
    mutationFn: () => api.createOrganization({ name: orgName, slug: orgSlug }),
    onSuccess: (res) => {
      if (res.error) {
        setOrgError(res.error.message)
        return
      }
      setShowCreateOrg(false)
      setOrgName('')
      setOrgSlug('')
      setOrgError('')
      queryClient.invalidateQueries({ queryKey: ['organizations'] })
    },
  })

  const deleteOrgMutation = useMutation({
    mutationFn: (id: string) => api.deleteOrganization(id),
    onSuccess: () => {
      setDeleteOrgTarget(null)
      queryClient.invalidateQueries({ queryKey: ['organizations'] })
    },
  })

  const tokens = (tokensData?.data || []) as EnrollmentToken[]
  const orgs = (orgsData?.data || []) as Organization[]
  const serverUrl = window.location.origin

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 dark:text-slate-100">Settings</h1>

      {/* ──────────────────────────────────────────── */}
      {/* Agent Deployment Section */}
      {/* ──────────────────────────────────────────── */}
      <div className="mt-8">
        <div>
          <h2 className="text-lg font-semibold text-gray-900 dark:text-slate-100">
            Agent Deployment
          </h2>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
            Deploy the Fibratus agent to Windows endpoints with a single
            PowerShell command. The installer downloads the agent, enrolls it
            with this server, and starts the service automatically.
          </p>
        </div>

        <div className="mt-4 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
          <div className="space-y-4">
            <div>
              <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">
                Prerequisites
              </h3>
              <ul className="mt-2 space-y-1 text-sm text-gray-600 dark:text-slate-400">
                <li className="flex items-start gap-2">
                  <span className="mt-0.5 text-fibratus-500">&#9679;</span>
                  <span>
                    <strong>Run as Administrator</strong> -- the installer
                    requires elevated privileges to install the Windows service
                  </span>
                </li>
                <li className="flex items-start gap-2">
                  <span className="mt-0.5 text-fibratus-500">&#9679;</span>
                  <span>
                    <strong>PowerShell 5.1+</strong> -- included in Windows 10
                    and later
                  </span>
                </li>
                <li className="flex items-start gap-2">
                  <span className="mt-0.5 text-fibratus-500">&#9679;</span>
                  <span>
                    <strong>Network access</strong> -- the endpoint must be able
                    to reach <code className="rounded bg-gray-100 dark:bg-slate-700 px-1 py-0.5 text-xs font-mono">{serverUrl}</code> over HTTPS
                  </span>
                </li>
              </ul>
            </div>

            <div>
              <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">
                Install Command
              </h3>
              <p className="mt-1 text-xs text-gray-500">
                Replace <code className="rounded bg-gray-100 dark:bg-slate-700 px-1 py-0.5 font-mono">{'<ENROLLMENT_TOKEN_ID>'}</code> with
                a valid enrollment token from the section below. Run this in an{' '}
                <strong>elevated PowerShell</strong> window.
              </p>
              <div className="mt-2 flex items-center gap-2">
                <div className="flex-1 rounded-lg bg-gray-900 px-4 py-3">
                  <code className="select-all font-mono text-sm text-emerald-400">
                    irm {serverUrl}/install/{'<ENROLLMENT_TOKEN_ID>'} | iex
                  </code>
                </div>
                <CopyButton
                  text={`irm ${serverUrl}/install/<ENROLLMENT_TOKEN_ID> | iex`}
                  label="Copy"
                />
              </div>
            </div>

            <div className="rounded-lg border border-amber-200 dark:border-amber-800/50 bg-amber-50 dark:bg-amber-900/20 px-4 py-3">
              <p className="text-xs text-amber-800 dark:text-amber-300">
                <strong>Important:</strong> Each enrollment token is scoped to an
                organization and has a usage limit. Create a token below, then
                use its ID in the install command. The token is embedded in the
                URL -- agents enroll automatically during installation.
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* ──────────────────────────────────────────── */}
      {/* Tamper Protection & Isolation Whitelist */}
      {/* ──────────────────────────────────────────── */}
      <ProtectionSettingsSection />

      {/* ──────────────────────────────────────────── */}
      {/* Enrollment Tokens Section */}
      {/* ──────────────────────────────────────────── */}
      <div className="mt-8">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-slate-100">
              Enrollment Tokens
            </h2>
            <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
              Create tokens to enroll new agents. Each token is scoped to this
              organization.
            </p>
          </div>
          <button
            onClick={() => {
              setShowCreateToken(!showCreateToken)
              setCreatedToken(null)
            }}
            className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
          >
            Create Token
          </button>
        </div>

        {/* Created token display */}
        {createdToken && (
          <div className="mt-4 rounded-xl border border-emerald-300 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-900/20 p-6">
            <h3 className="text-sm font-semibold text-emerald-800 dark:text-emerald-400">
              Token Created
            </h3>
            <p className="mt-1 text-xs text-emerald-600 dark:text-emerald-500">
              Copy this token — it will not be shown again in full.
            </p>
            <div className="mt-3 flex items-center gap-2">
              <code className="flex-1 select-all rounded-lg border border-emerald-200 dark:border-emerald-800 bg-white dark:bg-slate-800 px-4 py-2.5 font-mono text-sm text-gray-900 dark:text-slate-100">
                {createdToken}
              </code>
              <CopyButton text={createdToken} label="Copy Token" />
            </div>

            <div className="mt-4 space-y-3">
              <div className="rounded-lg border border-emerald-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
                <p className="mb-2 text-xs font-semibold text-gray-700 dark:text-slate-300">
                  One-liner install (elevated PowerShell):
                </p>
                <div className="flex items-center gap-2">
                  <div className="flex-1 rounded-lg bg-gray-900 px-4 py-3">
                    <code className="select-all font-mono text-sm text-emerald-400">
                      irm {serverUrl}/install/{createdToken} | iex
                    </code>
                  </div>
                  <CopyButton
                    text={`irm ${serverUrl}/install/${createdToken} | iex`}
                    label="Copy"
                  />
                </div>
              </div>

              <div className="rounded-lg border border-emerald-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
                <p className="mb-2 text-xs font-semibold text-gray-700 dark:text-slate-300">
                  Manual enrollment (if agent is already installed):
                </p>
                <div className="flex items-center gap-2">
                  <div className="flex-1 rounded-lg bg-gray-900 px-4 py-3">
                    <code className="select-all font-mono text-sm text-gray-300">
                      fibratus enroll --token {createdToken} --server {serverUrl}
                    </code>
                  </div>
                  <CopyButton
                    text={`fibratus enroll --token ${createdToken} --server ${serverUrl}`}
                    label="Copy"
                  />
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Create form */}
        {showCreateToken && (
          <div className="mt-4 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-slate-300">
                  Name
                </label>
                <input
                  type="text"
                  value={tokenName}
                  onChange={(e) => setTokenName(e.target.value)}
                  placeholder="e.g., workstation-rollout"
                  className="mt-1 w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 text-gray-900 dark:text-slate-100 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-slate-300">
                  Max Uses
                </label>
                <input
                  type="number"
                  value={maxUses}
                  onChange={(e) => setMaxUses(Number(e.target.value))}
                  min={1}
                  className="mt-1 w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 text-gray-900 dark:text-slate-100 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none"
                />
              </div>
            </div>
            <div className="mt-4">
              <button
                onClick={() => createTokenMutation.mutate()}
                disabled={createTokenMutation.isPending}
                className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
              >
                {createTokenMutation.isPending ? 'Creating...' : 'Create Token'}
              </button>
            </div>
          </div>
        )}

        {/* Token list */}
        <div className="mt-4 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
                <tr>
                  <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">
                    Token ID
                  </th>
                  <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Name</th>
                  <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Uses</th>
                  <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">
                    Expires
                  </th>
                  <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">
                    Status
                  </th>
                  <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
                {tokens.map((t) => {
                  const expired = new Date(t.expires_at) < new Date()
                  const exhausted = t.uses_count >= t.max_uses
                  const status = expired
                    ? 'Expired'
                    : exhausted
                      ? 'Exhausted'
                      : 'Active'
                  const statusColor =
                    status === 'Active'
                      ? 'text-emerald-700 bg-emerald-50'
                      : 'text-gray-500 bg-gray-50'
                  const installCmd = `irm ${serverUrl}/install/${t.id} | iex`
                  const isExpanded = expandedTokenId === t.id

                  return (
                    <tr key={t.id} className="group">
                      <td colSpan={6} className="p-0">
                        <div className="flex items-center hover:bg-gray-50/50 dark:hover:bg-slate-700/50 px-6 py-3">
                          <div className="w-[200px] font-mono text-xs text-gray-600 dark:text-slate-400 shrink-0">
                            {t.id.slice(0, 24)}...
                          </div>
                          <div className="w-[140px] text-gray-900 dark:text-slate-100 shrink-0">
                            {t.name}
                          </div>
                          <div className="w-[80px] text-gray-600 shrink-0">
                            {t.uses_count} / {t.max_uses}
                          </div>
                          <div className="w-[100px] text-gray-500 shrink-0">
                            {new Date(t.expires_at).toLocaleDateString()}
                          </div>
                          <div className="w-[80px] shrink-0">
                            <span
                              className={
                                'inline-flex rounded-full px-2 py-0.5 text-xs font-medium ' +
                                statusColor
                              }
                            >
                              {status}
                            </span>
                          </div>
                          <div className="flex items-center gap-3 ml-auto shrink-0">
                            {status === 'Active' && (
                              <button
                                onClick={() =>
                                  setExpandedTokenId(isExpanded ? null : t.id)
                                }
                                className="text-xs font-medium text-fibratus-600 hover:text-fibratus-800"
                              >
                                {isExpanded ? 'Hide Command' : 'Install Command'}
                              </button>
                            )}
                            <button
                              onClick={() => setDeleteTokenTarget(t)}
                              className="text-xs font-medium text-red-600 hover:text-red-800"
                            >
                              Delete
                            </button>
                          </div>
                        </div>
                        {isExpanded && (
                          <div className="px-6 pb-4">
                            <div className="rounded-lg border border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-900 p-4 space-y-3">
                              <div>
                                <p className="text-xs font-semibold text-gray-700 dark:text-slate-300 mb-1.5">
                                  Run in an elevated PowerShell on the target
                                  endpoint:
                                </p>
                                <div className="flex items-center gap-2">
                                  <div className="flex-1 rounded-lg bg-gray-900 px-4 py-3">
                                    <code className="select-all font-mono text-sm text-emerald-400 break-all">
                                      {installCmd}
                                    </code>
                                  </div>
                                  <CopyButton text={installCmd} label="Copy" />
                                </div>
                              </div>
                              <p className="text-xs text-gray-500">
                                This command downloads and installs the Fibratus
                                agent, enrolls it with this server, and starts
                                the Windows service. Must be run as
                                Administrator.
                              </p>
                            </div>
                          </div>
                        )}
                      </td>
                    </tr>
                  )
                })}
                {tokens.length === 0 && (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-6 py-12 text-center text-gray-400 dark:text-slate-500"
                    >
                      No enrollment tokens yet. Create one to start enrolling
                      agents.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {/* ──────────────────────────────────────────── */}
      {/* Organizations Section */}
      {/* ──────────────────────────────────────────── */}
      <div className="mt-12">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-slate-100">
              Organizations
            </h2>
            <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
              Manage organizations within your account.
            </p>
          </div>
          <button
            onClick={() => setShowCreateOrg(!showCreateOrg)}
            className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
          >
            {showCreateOrg ? 'Cancel' : 'Create Organization'}
          </button>
        </div>

        {/* Create org form */}
        {showCreateOrg && (
          <div className="mt-4 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
            {orgError && (
              <div className="mb-4 rounded-lg border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700">
                {orgError}
              </div>
            )}
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-slate-300">
                  Name
                </label>
                <input
                  type="text"
                  value={orgName}
                  onChange={(e) => setOrgName(e.target.value)}
                  placeholder="e.g., Engineering"
                  className="mt-1 w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 text-gray-900 dark:text-slate-100 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-slate-300">
                  Slug
                </label>
                <input
                  type="text"
                  value={orgSlug}
                  onChange={(e) => setOrgSlug(e.target.value)}
                  placeholder="e.g., engineering"
                  className="mt-1 w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 text-gray-900 dark:text-slate-100 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none"
                />
              </div>
            </div>
            <div className="mt-4">
              <button
                onClick={() => createOrgMutation.mutate()}
                disabled={
                  !orgName.trim() ||
                  !orgSlug.trim() ||
                  createOrgMutation.isPending
                }
                className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
              >
                {createOrgMutation.isPending
                  ? 'Creating...'
                  : 'Create Organization'}
              </button>
            </div>
          </div>
        )}

        {/* Organization list */}
        <div className="mt-4 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
                <tr>
                  <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Name</th>
                  <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Slug</th>
                  <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">
                    Agents
                  </th>
                  <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">ID</th>
                  <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
                {orgs.map((org) => (
                  <tr key={org.id} className="hover:bg-gray-50/50 dark:hover:bg-slate-700/50">
                    <td className="px-6 py-3 font-medium text-gray-900 dark:text-slate-100">
                      {org.name}
                    </td>
                    <td className="px-6 py-3 text-gray-600">{org.slug}</td>
                    <td className="px-6 py-3 text-gray-600">
                      {org.agent_count}
                    </td>
                    <td className="px-6 py-3 font-mono text-xs text-gray-400 dark:text-slate-500 dark:text-slate-500">
                      {org.id}
                    </td>
                    <td className="px-6 py-3">
                      <button
                        onClick={() => setDeleteOrgTarget(org)}
                        className="text-xs font-medium text-red-600 hover:text-red-800"
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
                {orgs.length === 0 && (
                  <tr>
                    <td
                      colSpan={5}
                      className="px-6 py-12 text-center text-gray-400 dark:text-slate-500"
                    >
                      No organizations found.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {/* ──────────────────────────────────────────── */}
      {/* Security — User Profile & 2FA */}
      {/* ──────────────────────────────────────────── */}
      <SecuritySection />

      {/* ──────────────────────────────────────────── */}
      {/* Detection as Code — GitHub Sync */}
      {/* ──────────────────────────────────────────── */}
      <GitHubSyncSection />

      {/* Delete token confirmation dialog */}
      <ConfirmDialog
        open={!!deleteTokenTarget}
        title="Delete Enrollment Token"
        message={
          'Are you sure you want to delete the token "' +
          (deleteTokenTarget?.name || '') +
          '"? Agents already enrolled with this token will not be affected.'
        }
        confirmLabel="Delete"
        onConfirm={() => {
          if (deleteTokenTarget) deleteTokenMutation.mutate(deleteTokenTarget.id)
        }}
        onCancel={() => setDeleteTokenTarget(null)}
      />

      {/* Delete organization confirmation dialog */}
      <ConfirmDialog
        open={!!deleteOrgTarget}
        title="Delete Organization"
        message={
          'Are you sure you want to delete the organization "' +
          (deleteOrgTarget?.name || '') +
          '"? This will permanently remove all agents, rules, detections, and data associated with this organization. This action cannot be undone.'
        }
        confirmLabel="Delete Organization"
        onConfirm={() => {
          if (deleteOrgTarget) deleteOrgMutation.mutate(deleteOrgTarget.id)
        }}
        onCancel={() => setDeleteOrgTarget(null)}
      />
    </div>
  )
}

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

function ProtectionSettingsSection() {
  const queryClient = useQueryClient()

  const { data: settingsData } = useQuery({
    queryKey: ['account-settings'],
    queryFn: () => api.getAccountSettings(),
  })
  const settings = settingsData?.data as { tamper_protection_enabled: boolean; isolation_whitelist: string[]; org_protection?: Array<{ id: string; name: string; tamper_protection_enabled: boolean }> } | undefined
  const tamperEnabled = settings?.tamper_protection_enabled ?? false
  const whitelist = settings?.isolation_whitelist ?? []
  const orgProtection = settings?.org_protection ?? []

  const [newWhitelistEntry, setNewWhitelistEntry] = useState('')
  const [tamperToggling, setTamperToggling] = useState(false)
  const [orgToggling, setOrgToggling] = useState<string | null>(null)

  const tamperMutation = useMutation({
    mutationFn: (enabled: boolean) => api.updateAccountSettings({ tamper_protection_enabled: enabled }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['account-settings'] })
    },
  })

  const orgTamperMutation = useMutation({
    mutationFn: ({ orgId, enabled }: { orgId: string; enabled: boolean }) => api.updateOrgTamperProtection(orgId, enabled),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['account-settings'] })
      setOrgToggling(null)
    },
    onError: () => setOrgToggling(null),
  })

  const whitelistMutation = useMutation({
    mutationFn: (updatedList: string[]) => api.updateAccountSettings({ isolation_whitelist: updatedList }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['account-settings'] })
    },
  })

  const handleTamperToggle = async () => {
    setTamperToggling(true)
    try {
      await tamperMutation.mutateAsync(!tamperEnabled)
    } finally {
      setTamperToggling(false)
    }
  }

  const handleOrgTamperToggle = async (orgId: string, currentState: boolean) => {
    setOrgToggling(orgId)
    orgTamperMutation.mutate({ orgId, enabled: !currentState })
  }

  const handleAddWhitelist = () => {
    const entry = newWhitelistEntry.trim()
    if (!entry || whitelist.includes(entry)) return
    whitelistMutation.mutate([...whitelist, entry])
    setNewWhitelistEntry('')
  }

  const handleRemoveWhitelist = (entry: string) => {
    whitelistMutation.mutate(whitelist.filter(e => e !== entry))
  }

  return (
    <div className="mt-8 space-y-8">
      {/* Tamper Protection */}
      <div>
        <div>
          <h2 className="text-lg font-semibold text-gray-900 dark:text-slate-100">Tamper Protection</h2>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
            Prevent unauthorized modification, termination, or uninstallation of the agent on endpoints.
          </p>
        </div>

        {/* Account-wide toggle */}
        <div className="mt-4 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50 space-y-5">
          <div className="flex items-center justify-between">
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-gray-900 dark:text-slate-100">Enable for all organizations</p>
              <p className="text-xs text-gray-500 dark:text-slate-400 mt-1">
                When enabled, all agents across all organizations will have tamper protection enforced. Individual agent and organization toggles will be locked.
              </p>
            </div>
            <ToggleSwitch enabled={tamperEnabled} onToggle={handleTamperToggle} disabled={tamperToggling || tamperMutation.isPending} />
          </div>

          {/* Per-org toggles */}
          {orgProtection.length > 0 && (
            <div className="border-t border-gray-100 dark:border-slate-700 pt-4">
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
                          <span className="text-[10px] px-1.5 py-0.5 rounded bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 font-medium">Locked by account policy</span>
                        )}
                      </div>
                      <ToggleSwitch
                        enabled={effectiveEnabled}
                        onToggle={() => handleOrgTamperToggle(org.id, org.tamper_protection_enabled)}
                        disabled={lockedByAccount || orgToggling === org.id || orgTamperMutation.isPending}
                      />
                    </div>
                  )
                })}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Isolation Whitelist */}
      <div>
        <div>
          <h2 className="text-lg font-semibold text-gray-900 dark:text-slate-100">Isolation Whitelist</h2>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
            These IPs/CIDRs will always be allowed during network isolation (e.g., RMM servers, jump boxes).
          </p>
        </div>

        <div className="mt-4 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
          <div className="flex gap-2">
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
              disabled={!newWhitelistEntry.trim() || whitelistMutation.isPending}
              className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
            >
              Add
            </button>
          </div>

          {whitelist.length > 0 ? (
            <div className="mt-4 space-y-2">
              {whitelist.map((entry) => (
                <div key={entry} className="flex items-center justify-between rounded-lg border border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-900 px-4 py-2.5">
                  <code className="text-sm font-mono text-gray-900 dark:text-slate-100">{entry}</code>
                  <button
                    onClick={() => handleRemoveWhitelist(entry)}
                    disabled={whitelistMutation.isPending}
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
      </div>
    </div>
  )
}

function SecuritySection() {
  const queryClient = useQueryClient()

  // User profile
  const { data: userData } = useQuery({
    queryKey: ['current-user'],
    queryFn: () => api.getCurrentUser(),
  })
  const user = userData?.data as User | undefined

  // TOTP status
  const { data: totpData } = useQuery({
    queryKey: ['totp-status'],
    queryFn: () => api.getTOTPStatus(),
  })
  const totpEnabled = (totpData?.data as { enabled: boolean } | undefined)?.enabled ?? false

  // Setup flow state
  const [setupStep, setSetupStep] = useState<'idle' | 'setup' | 'verify' | 'done'>('idle')
  const [totpSecret, setTotpSecret] = useState('')
  // totpUri removed — QR code generated server-side
  const [verifyCode, setVerifyCode] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const [setupError, setSetupError] = useState('')

  // QR code
  const [qrDataUrl, setQrDataUrl] = useState('')

  // QR data URL now comes from the server (no client-side QR library)

  // Disable flow state
  const [showDisable, setShowDisable] = useState(false)
  const [disablePassword, setDisablePassword] = useState('')
  const [disableError, setDisableError] = useState('')

  const setupMutation = useMutation({
    mutationFn: () => api.setupTOTP(),
    onSuccess: (res) => {
      if (res.error) {
        setSetupError(res.error.message)
        return
      }
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
      if (res.error) {
        setSetupError(res.error.message)
        return
      }
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
      if (res.error) {
        setDisableError(res.error.message)
        return
      }
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

  const copyRecoveryCodes = () => {
    navigator.clipboard.writeText(recoveryCodes.join('\n'))
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
    <div className="mt-12">
      <div>
        <h2 className="text-lg font-semibold text-gray-900 dark:text-slate-100">Security</h2>
        <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
          Manage your account security and two-factor authentication.
        </p>
      </div>

      <div className="mt-4 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50 divide-y divide-gray-100 dark:divide-slate-700">
        {/* User Profile */}
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

        {/* 2FA Status & Controls */}
        <div className="px-6 py-5">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div>
                <p className="text-sm font-medium text-gray-900 dark:text-slate-100">Two-Factor Authentication</p>
                <p className="text-sm text-gray-500 dark:text-slate-400">
                  {totpEnabled
                    ? 'Your account is protected with 2FA.'
                    : 'Add an extra layer of security to your account.'}
                </p>
              </div>
            </div>
            <div className="flex items-center gap-3">
              <span className={
                'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ' +
                (totpEnabled ? 'text-emerald-700 bg-emerald-50' : 'text-gray-500 bg-gray-100')
              }>
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
              {totpEnabled && !showDisable && (
                <button
                  onClick={() => setShowDisable(true)}
                  className="rounded-lg border border-red-300 px-4 py-2 text-sm font-medium text-red-600 hover:bg-red-50"
                >
                  Disable 2FA
                </button>
              )}
            </div>
          </div>

          {/* Setup error */}
          {setupError && (
            <div className="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700">
              {setupError}
            </div>
          )}

          {/* Setup Step: Verify -- show QR code, secret & code input */}
          {setupStep === 'verify' && (
            <div className="mt-4 rounded-lg border border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-900 p-4 space-y-4">
              <div>
                <p className="text-sm font-medium text-gray-900 dark:text-slate-100">1. Scan with your authenticator app</p>
                <p className="mt-1 text-xs text-gray-500">
                  Scan this QR code with your authenticator app (Google Authenticator, Authy, etc.)
                </p>
              </div>
              <div className="flex flex-col items-center py-2">
                {qrDataUrl ? (
                  <img src={qrDataUrl} alt="Scan with authenticator app" className="rounded-lg border border-gray-200 bg-white p-2" width={200} height={200} />
                ) : (
                  <div className="flex h-[200px] w-[200px] items-center justify-center rounded-lg border border-gray-200 bg-white text-xs text-gray-400 dark:text-slate-500">
                    Generating QR code...
                  </div>
                )}
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 mb-1">Or enter the secret key manually</label>
                <div className="flex items-center gap-2">
                  <code className="flex-1 select-all rounded-lg border border-gray-200 bg-white px-3 py-2 font-mono text-sm text-gray-900 dark:text-slate-100 break-all">
                    {totpSecret}
                  </code>
                  <button
                    onClick={() => navigator.clipboard.writeText(totpSecret)}
                    className="rounded-lg border border-gray-300 px-3 py-2 text-xs font-medium text-gray-600 hover:bg-gray-100"
                  >
                    Copy
                  </button>
                </div>
              </div>
              <div>
                <p className="text-sm font-medium text-gray-900 dark:text-slate-100">2. Enter the verification code</p>
                <p className="mt-1 text-xs text-gray-500">
                  Enter the 6-digit code shown in your authenticator app to verify setup.
                </p>
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
                  <button
                    onClick={resetSetup}
                    className="text-sm text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            </div>
          )}

          {/* Setup Step: Done -- show recovery codes */}
          {setupStep === 'done' && recoveryCodes.length > 0 && (
            <div className="mt-4 rounded-lg border border-emerald-200 bg-emerald-50 p-4 space-y-3">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-semibold text-emerald-800">Two-factor authentication enabled</p>
                  <p className="text-xs text-emerald-600">
                    Save these recovery codes in a secure location. Each code can only be used once.
                  </p>
                </div>
                <button
                  onClick={copyRecoveryCodes}
                  className="rounded-lg bg-emerald-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-emerald-700"
                >
                  Copy All
                </button>
              </div>
              <div className="grid grid-cols-2 gap-2">
                {recoveryCodes.map((code, i) => (
                  <code key={i} className="rounded border border-emerald-200 bg-white px-3 py-1.5 font-mono text-sm text-gray-900 dark:text-slate-100 text-center">
                    {code}
                  </code>
                ))}
              </div>
              <button
                onClick={resetSetup}
                className="text-sm font-medium text-emerald-700 hover:text-emerald-800"
              >
                Done
              </button>
            </div>
          )}

          {/* Disable 2FA */}
          {showDisable && (
            <div className="mt-4 rounded-lg border border-red-200 bg-red-50 p-4 space-y-3">
              <p className="text-sm font-medium text-red-800">Confirm password to disable 2FA</p>
              {disableError && (
                <div className="rounded-lg border border-red-300 bg-white px-3 py-2 text-sm text-red-700">
                  {disableError}
                </div>
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
                  className="text-sm text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function GitHubSyncSection() {
  const queryClient = useQueryClient()
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ name: '', repo_url: '', branch: 'main', path: '', token: '', interval: 30, enabled: true, scope: 'account' })
  const [editId, setEditId] = useState<string | null>(null)
  const [syncResult, setSyncResult] = useState<Record<string, unknown> | null>(null)

  const { data } = useQuery({
    queryKey: ['github-sync-configs'],
    queryFn: () => api.listGitHubSyncConfigs(),
  })
  const configs = (data?.data || []) as Record<string, unknown>[]

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

  return (
    <div className="mt-8">
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
                    {(cfg.scope as string) === 'org' ? 'org' : 'account'}
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
              <div>
                <label className="block text-xs font-medium text-gray-700 dark:text-slate-300 mb-1">Scope</label>
                <select value={form.scope} onChange={e => setForm({ ...form, scope: e.target.value })}
                  className="rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 text-gray-900 dark:text-slate-100 px-2 py-2 text-xs">
                  <option value="account">Account-wide (all orgs)</option>
                  <option value="org">This org only</option>
                </select>
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
  )
}
