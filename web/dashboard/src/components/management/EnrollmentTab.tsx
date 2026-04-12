import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type EnrollmentToken } from '../../lib/api'
import ConfirmDialog from '../../components/ConfirmDialog'

interface AccountSettings {
  latest_agent_version?: string
  latest_agent_msi_url?: string
  auto_update_agents?: boolean
  agent_update_repo?: string
}

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

export default function EnrollmentTab() {
  const queryClient = useQueryClient()

  const [showCreateToken, setShowCreateToken] = useState(false)
  const [tokenName, setTokenName] = useState('')
  const [maxUses, setMaxUses] = useState(50)
  const [createdToken, setCreatedToken] = useState<string | null>(null)
  const [deleteTokenTarget, setDeleteTokenTarget] = useState<EnrollmentToken | null>(null)
  const [expandedTokenId, setExpandedTokenId] = useState<string | null>(null)

  const { data: tokensData } = useQuery({
    queryKey: ['enrollment-tokens'],
    queryFn: () => api.getEnrollmentTokens(),
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

  const { data: accountData } = useQuery({
    queryKey: ['account-settings'],
    queryFn: () => api.getAccountSettings(),
  })

  const tokens = (tokensData?.data || []) as EnrollmentToken[]
  const serverUrl = window.location.origin
  const account = accountData?.data as AccountSettings | undefined
  const latestVersion = account?.latest_agent_version
  const autoUpdate = account?.auto_update_agents

  return (
    <div>
      {/* Release version banner */}
      {latestVersion && (
        <div className={`mb-6 rounded-xl border p-4 ${autoUpdate ? 'border-emerald-200 dark:border-emerald-800/50 bg-emerald-50 dark:bg-emerald-900/20' : 'border-amber-200 dark:border-amber-800/50 bg-amber-50 dark:bg-amber-900/20'}`}>
          <div className="flex items-center justify-between">
            <div>
              <p className={`text-sm font-semibold ${autoUpdate ? 'text-emerald-800 dark:text-emerald-300' : 'text-amber-800 dark:text-amber-300'}`}>
                Agent Download Version: <span className="font-mono">v{latestVersion}</span>
              </p>
              <p className={`mt-0.5 text-xs ${autoUpdate ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-600 dark:text-amber-400'}`}>
                {autoUpdate
                  ? 'Auto-update is enabled — new installs and existing agents will use the latest release automatically.'
                  : 'Auto-update is off — new installs use this version. Enable auto-update in Account settings to keep agents current.'}
              </p>
            </div>
            <span className={`shrink-0 rounded-full px-3 py-1 text-xs font-medium ${autoUpdate ? 'bg-emerald-100 dark:bg-emerald-800/50 text-emerald-700 dark:text-emerald-300' : 'bg-amber-100 dark:bg-amber-800/50 text-amber-700 dark:text-amber-300'}`}>
              {autoUpdate ? 'Auto-Update ON' : 'Auto-Update OFF'}
            </span>
          </div>
        </div>
      )}

      {/* ──────────────────────────────────────────── */}
      {/* Agent Deployment Section */}
      {/* ──────────────────────────────────────────── */}
      <div>
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

            <div>
              <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">
                Manual Enrollment
              </h3>
              <p className="mt-1 text-xs text-gray-500">
                If the agent is already installed, enroll it manually:
              </p>
              <div className="mt-2 flex items-center gap-2">
                <div className="flex-1 rounded-lg bg-gray-900 px-4 py-3">
                  <code className="select-all font-mono text-sm text-gray-300">
                    fibratus enroll --token {'<ENROLLMENT_TOKEN_ID>'} --server {serverUrl}
                  </code>
                </div>
                <CopyButton
                  text={`fibratus enroll --token <ENROLLMENT_TOKEN_ID> --server ${serverUrl}`}
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

      {/* Delete token confirmation */}
      <ConfirmDialog
        open={!!deleteTokenTarget}
        title="Delete Enrollment Token"
        message={`Are you sure you want to delete the token "${deleteTokenTarget?.name}"? Agents already enrolled with this token will not be affected.`}
        confirmLabel="Delete"
        onConfirm={() => {
          if (deleteTokenTarget) deleteTokenMutation.mutate(deleteTokenTarget.id)
        }}
        onCancel={() => setDeleteTokenTarget(null)}
      />
    </div>
  )
}
