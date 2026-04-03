import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type EnrollmentToken, type Organization } from '../lib/api'
import ConfirmDialog from '../components/ConfirmDialog'

export default function Settings() {
  const queryClient = useQueryClient()

  // Enrollment token state
  const [showCreateToken, setShowCreateToken] = useState(false)
  const [tokenName, setTokenName] = useState('')
  const [maxUses, setMaxUses] = useState(50)
  const [createdToken, setCreatedToken] = useState<string | null>(null)
  const [deleteTokenTarget, setDeleteTokenTarget] = useState<EnrollmentToken | null>(null)

  // Organization state
  const [showCreateOrg, setShowCreateOrg] = useState(false)
  const [orgName, setOrgName] = useState('')
  const [orgSlug, setOrgSlug] = useState('')
  const [orgError, setOrgError] = useState('')

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

  const tokens = (tokensData?.data || []) as EnrollmentToken[]
  const orgs = (orgsData?.data || []) as Organization[]
  const serverUrl = window.location.origin

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900">Settings</h1>

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
              <button
                onClick={() => navigator.clipboard.writeText(createdToken)}
                className="rounded-lg bg-emerald-600 px-3 py-2.5 text-sm font-medium text-white hover:bg-emerald-700"
              >
                Copy
              </button>
            </div>
            <div className="mt-4 rounded-lg border border-emerald-200 bg-white p-4">
              <p className="mb-2 text-xs font-medium text-gray-500">
                Run on the Windows endpoint:
              </p>
              <code className="select-all font-mono text-sm text-gray-900">
                fibratus enroll --token {createdToken} --server {serverUrl}
              </code>
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

                  return (
                    <tr key={t.id} className="hover:bg-gray-50/50">
                      <td className="px-6 py-3 font-mono text-xs text-gray-600">
                        {t.id.slice(0, 24)}...
                      </td>
                      <td className="px-6 py-3 text-gray-900">{t.name}</td>
                      <td className="px-6 py-3 text-gray-600">
                        {t.uses_count} / {t.max_uses}
                      </td>
                      <td className="px-6 py-3 text-gray-500">
                        {new Date(t.expires_at).toLocaleDateString()}
                      </td>
                      <td className="px-6 py-3">
                        <span
                          className={
                            'inline-flex rounded-full px-2 py-0.5 text-xs font-medium ' +
                            statusColor
                          }
                        >
                          {status}
                        </span>
                      </td>
                      <td className="px-6 py-3">
                        <button
                          onClick={() => setDeleteTokenTarget(t)}
                          className="text-xs font-medium text-red-600 hover:text-red-800"
                        >
                          Delete
                        </button>
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
                  </tr>
                ))}
                {orgs.length === 0 && (
                  <tr>
                    <td
                      colSpan={4}
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
