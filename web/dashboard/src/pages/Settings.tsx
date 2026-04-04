import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type EnrollmentToken, type Organization, type User } from '../lib/api'
import ConfirmDialog from '../components/ConfirmDialog'

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
      className="shrink-0 rounded-lg border border-gray-300 bg-white px-3 py-2 text-xs font-medium text-gray-600 hover:bg-gray-100"
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
      <h1 className="text-2xl font-bold text-gray-900">Settings</h1>

      {/* ──────────────────────────────────────────── */}
      {/* Agent Deployment Section */}
      {/* ──────────────────────────────────────────── */}
      <div className="mt-8">
        <div>
          <h2 className="text-lg font-semibold text-gray-900">
            Agent Deployment
          </h2>
          <p className="mt-1 text-sm text-gray-500">
            Deploy the Fibratus agent to Windows endpoints with a single
            PowerShell command. The installer downloads the agent, enrolls it
            with this server, and starts the service automatically.
          </p>
        </div>

        <div className="mt-4 rounded-xl border border-gray-200 bg-white p-6 shadow-sm">
          <div className="space-y-4">
            <div>
              <h3 className="text-sm font-semibold text-gray-900">
                Prerequisites
              </h3>
              <ul className="mt-2 space-y-1 text-sm text-gray-600">
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
                    to reach <code className="rounded bg-gray-100 px-1 py-0.5 text-xs font-mono">{serverUrl}</code> over HTTPS
                  </span>
                </li>
              </ul>
            </div>

            <div>
              <h3 className="text-sm font-semibold text-gray-900">
                Install Command
              </h3>
              <p className="mt-1 text-xs text-gray-500">
                Replace <code className="rounded bg-gray-100 px-1 py-0.5 font-mono">{'<ENROLLMENT_TOKEN_ID>'}</code> with
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

            <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3">
              <p className="text-xs text-amber-800">
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
      {/* Enrollment Tokens Section */}
      {/* ──────────────────────────────────────────── */}
      <div className="mt-8">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold text-gray-900">
              Enrollment Tokens
            </h2>
            <p className="mt-1 text-sm text-gray-500">
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
          <div className="mt-4 rounded-xl border border-emerald-200 bg-emerald-50 p-6">
            <h3 className="text-sm font-semibold text-emerald-800">
              Token Created
            </h3>
            <p className="mt-1 text-xs text-emerald-600">
              Copy this token -- it will not be shown again in full.
            </p>
            <div className="mt-3 flex items-center gap-2">
              <code className="flex-1 select-all rounded-lg border border-emerald-200 bg-white px-4 py-2.5 font-mono text-sm text-gray-900">
                {createdToken}
              </code>
              <CopyButton text={createdToken} label="Copy Token" />
            </div>

            <div className="mt-4 space-y-3">
              <div className="rounded-lg border border-emerald-200 bg-white p-4">
                <p className="mb-2 text-xs font-semibold text-gray-700">
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

              <div className="rounded-lg border border-emerald-200 bg-white p-4">
                <p className="mb-2 text-xs font-semibold text-gray-700">
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
          <div className="mt-4 rounded-xl border border-gray-200 bg-white p-6 shadow-sm">
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700">
                  Name
                </label>
                <input
                  type="text"
                  value={tokenName}
                  onChange={(e) => setTokenName(e.target.value)}
                  placeholder="e.g., workstation-rollout"
                  className="mt-1 w-full rounded-lg border border-gray-300 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700">
                  Max Uses
                </label>
                <input
                  type="number"
                  value={maxUses}
                  onChange={(e) => setMaxUses(Number(e.target.value))}
                  min={1}
                  className="mt-1 w-full rounded-lg border border-gray-300 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none"
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
        <div className="mt-4 rounded-xl border border-gray-200 bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-100 bg-gray-50/50">
                <tr>
                  <th className="px-6 py-3 font-medium text-gray-500">
                    Token ID
                  </th>
                  <th className="px-6 py-3 font-medium text-gray-500">Name</th>
                  <th className="px-6 py-3 font-medium text-gray-500">Uses</th>
                  <th className="px-6 py-3 font-medium text-gray-500">
                    Expires
                  </th>
                  <th className="px-6 py-3 font-medium text-gray-500">
                    Status
                  </th>
                  <th className="px-6 py-3 font-medium text-gray-500">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
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
                        <div className="flex items-center hover:bg-gray-50/50 px-6 py-3">
                          <div className="w-[200px] font-mono text-xs text-gray-600 shrink-0">
                            {t.id.slice(0, 24)}...
                          </div>
                          <div className="w-[140px] text-gray-900 shrink-0">
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
                            <div className="rounded-lg border border-gray-200 bg-gray-50 p-4 space-y-3">
                              <div>
                                <p className="text-xs font-semibold text-gray-700 mb-1.5">
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
                      className="px-6 py-12 text-center text-gray-400"
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
            <h2 className="text-lg font-semibold text-gray-900">
              Organizations
            </h2>
            <p className="mt-1 text-sm text-gray-500">
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
          <div className="mt-4 rounded-xl border border-gray-200 bg-white p-6 shadow-sm">
            {orgError && (
              <div className="mb-4 rounded-lg border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700">
                {orgError}
              </div>
            )}
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700">
                  Name
                </label>
                <input
                  type="text"
                  value={orgName}
                  onChange={(e) => setOrgName(e.target.value)}
                  placeholder="e.g., Engineering"
                  className="mt-1 w-full rounded-lg border border-gray-300 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700">
                  Slug
                </label>
                <input
                  type="text"
                  value={orgSlug}
                  onChange={(e) => setOrgSlug(e.target.value)}
                  placeholder="e.g., engineering"
                  className="mt-1 w-full rounded-lg border border-gray-300 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none"
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
        <div className="mt-4 rounded-xl border border-gray-200 bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-100 bg-gray-50/50">
                <tr>
                  <th className="px-6 py-3 font-medium text-gray-500">Name</th>
                  <th className="px-6 py-3 font-medium text-gray-500">Slug</th>
                  <th className="px-6 py-3 font-medium text-gray-500">
                    Agents
                  </th>
                  <th className="px-6 py-3 font-medium text-gray-500">ID</th>
                  <th className="px-6 py-3 font-medium text-gray-500">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {orgs.map((org) => (
                  <tr key={org.id} className="hover:bg-gray-50/50">
                    <td className="px-6 py-3 font-medium text-gray-900">
                      {org.name}
                    </td>
                    <td className="px-6 py-3 text-gray-600">{org.slug}</td>
                    <td className="px-6 py-3 text-gray-600">
                      {org.agent_count}
                    </td>
                    <td className="px-6 py-3 font-mono text-xs text-gray-400">
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
                      className="px-6 py-12 text-center text-gray-400"
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
  const [totpUri, setTotpUri] = useState('')
  const [verifyCode, setVerifyCode] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const [setupError, setSetupError] = useState('')

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
      const data = res.data as { secret: string; uri: string } | undefined
      if (data) {
        setTotpSecret(data.secret)
        setTotpUri(data.uri)
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
    setTotpUri('')
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
        <h2 className="text-lg font-semibold text-gray-900">Security</h2>
        <p className="mt-1 text-sm text-gray-500">
          Manage your account security and two-factor authentication.
        </p>
      </div>

      <div className="mt-4 rounded-xl border border-gray-200 bg-white shadow-sm divide-y divide-gray-100">
        {/* User Profile */}
        {user && (
          <div className="px-6 py-5">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-900">{user.name || user.email}</p>
                <p className="text-sm text-gray-500">{user.email}</p>
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
                <p className="text-sm font-medium text-gray-900">Two-Factor Authentication</p>
                <p className="text-sm text-gray-500">
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

          {/* Setup Step: Verify -- show secret & URI, code input */}
          {setupStep === 'verify' && (
            <div className="mt-4 rounded-lg border border-gray-200 bg-gray-50 p-4 space-y-4">
              <div>
                <p className="text-sm font-medium text-gray-900">1. Add to your authenticator app</p>
                <p className="mt-1 text-xs text-gray-500">
                  Copy the secret key below into your authenticator app (Google Authenticator, Authy, etc.)
                </p>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 mb-1">Secret Key</label>
                <div className="flex items-center gap-2">
                  <code className="flex-1 select-all rounded-lg border border-gray-200 bg-white px-3 py-2 font-mono text-sm text-gray-900 break-all">
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
                <label className="block text-xs font-medium text-gray-500 mb-1">Provisioning URI</label>
                <div className="flex items-center gap-2">
                  <code className="flex-1 select-all rounded-lg border border-gray-200 bg-white px-3 py-2 font-mono text-xs text-gray-600 break-all">
                    {totpUri}
                  </code>
                  <button
                    onClick={() => navigator.clipboard.writeText(totpUri)}
                    className="rounded-lg border border-gray-300 px-3 py-2 text-xs font-medium text-gray-600 hover:bg-gray-100"
                  >
                    Copy
                  </button>
                </div>
              </div>
              <div>
                <p className="text-sm font-medium text-gray-900">2. Enter the verification code</p>
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
                    className="text-sm text-gray-500 hover:text-gray-700"
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
                  <code key={i} className="rounded border border-emerald-200 bg-white px-3 py-1.5 font-mono text-sm text-gray-900 text-center">
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
                  className="text-sm text-gray-500 hover:text-gray-700"
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
  const [form, setForm] = useState({ repo_url: '', branch: 'main', path: 'rules/', token: '', interval: 30, enabled: false })
  const [loaded, setLoaded] = useState(false)
  const [syncResult, setSyncResult] = useState<{ created: number; updated: number; skipped: number; errors: string[]; duration: string } | null>(null)

  const { data } = useQuery({
    queryKey: ['github-sync-config'],
    queryFn: () => api.getGitHubSyncConfig(),
  })

  // Load config into form on first fetch
  if (data?.data && !loaded) {
    const cfg = data.data as Record<string, unknown>
    setForm({
      repo_url: (cfg.repo_url as string) || '',
      branch: (cfg.branch as string) || 'main',
      path: (cfg.path as string) || 'rules/',
      token: (cfg.token as string) === '***configured***' ? '' : '',
      interval: (cfg.interval as number) || 30,
      enabled: (cfg.enabled as boolean) || false,
    })
    setLoaded(true)
  }

  const saveMutation = useMutation({
    mutationFn: () => api.saveGitHubSyncConfig(form),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['github-sync-config'] })
    },
  })

  const syncMutation = useMutation({
    mutationFn: () => api.triggerGitHubSync(),
    onSuccess: (res) => {
      if (res.data) setSyncResult(res.data as typeof syncResult)
      queryClient.invalidateQueries({ queryKey: ['rules'] })
    },
  })

  return (
    <div className="mt-8">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-gray-900">Detection as Code</h2>
          <p className="mt-1 text-sm text-gray-500">Sync detection rules from a GitHub repository. Rules are validated before import.</p>
        </div>
      </div>

      <div className="mt-4 rounded-xl border border-gray-200 bg-white p-6 shadow-sm space-y-4">
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Repository API URL</label>
            <input
              value={form.repo_url}
              onChange={e => setForm({ ...form, repo_url: e.target.value })}
              placeholder="https://api.github.com/repos/owner/repo"
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm font-mono focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
            />
            <p className="mt-1 text-xs text-gray-400">GitHub API URL for the repository containing detection rules</p>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Branch</label>
            <input
              value={form.branch}
              onChange={e => setForm({ ...form, branch: e.target.value })}
              placeholder="main"
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Rules Path</label>
            <input
              value={form.path}
              onChange={e => setForm({ ...form, path: e.target.value })}
              placeholder="rules/"
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm font-mono focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">GitHub Token (PAT)</label>
            <input
              type="password"
              value={form.token}
              onChange={e => setForm({ ...form, token: e.target.value })}
              placeholder="ghp_..."
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm font-mono focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
            />
            <p className="mt-1 text-xs text-gray-400">Optional for public repos. Required for private repos.</p>
          </div>
        </div>

        <div className="flex items-center gap-4">
          <label className="flex items-center gap-2 cursor-pointer">
            <input type="checkbox" checked={form.enabled} onChange={e => setForm({ ...form, enabled: e.target.checked })} className="rounded" />
            <span className="text-sm text-gray-700">Enable automatic sync</span>
          </label>
          <div className="flex items-center gap-2">
            <span className="text-sm text-gray-500">every</span>
            <input
              type="number"
              value={form.interval}
              onChange={e => setForm({ ...form, interval: Number(e.target.value) })}
              min={5}
              className="w-16 rounded border border-gray-300 px-2 py-1 text-sm text-center"
            />
            <span className="text-sm text-gray-500">minutes</span>
          </div>
        </div>

        <div className="flex gap-3 pt-2">
          <button
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending}
            className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
          >
            {saveMutation.isPending ? 'Saving...' : 'Save Configuration'}
          </button>
          <button
            onClick={() => syncMutation.mutate()}
            disabled={syncMutation.isPending || !form.repo_url}
            className="rounded-lg border border-fibratus-300 bg-fibratus-50 px-4 py-2 text-sm font-medium text-fibratus-700 hover:bg-fibratus-100 disabled:opacity-50"
          >
            {syncMutation.isPending ? 'Syncing...' : 'Sync Now'}
          </button>
        </div>

        {syncResult && (
          <div className="mt-4 rounded-lg border border-gray-200 bg-gray-50 p-4">
            <h4 className="text-sm font-medium text-gray-900">Sync Result</h4>
            <div className="mt-2 flex gap-6 text-sm">
              <span className="text-emerald-700">{syncResult.created} created</span>
              <span className="text-blue-700">{syncResult.updated} updated</span>
              <span className="text-gray-500">{syncResult.skipped} skipped</span>
              <span className="text-gray-400">{syncResult.duration}</span>
            </div>
            {syncResult.errors && syncResult.errors.length > 0 && (
              <div className="mt-2 max-h-32 overflow-auto">
                {syncResult.errors.map((err, i) => (
                  <div key={i} className="text-xs text-red-600 py-0.5">{err}</div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
