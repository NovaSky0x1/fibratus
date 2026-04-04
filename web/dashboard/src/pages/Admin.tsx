import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Account, type Organization, type User } from '../lib/api'

type Tab = 'accounts' | 'organizations' | 'users'

export default function Admin() {
  const queryClient = useQueryClient()
  const [tab, setTab] = useState<Tab>('accounts')
  const [showCreate, setShowCreate] = useState(false)
  const [createForm, setCreateForm] = useState({ name: '', plan: 'standard' })
  const [error, setError] = useState('')
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [filterAccountId, setFilterAccountId] = useState<string | null>(null)

  // Check if current user is root
  const { data: currentUserData } = useQuery({
    queryKey: ['current-user'],
    queryFn: () => api.getCurrentUser(),
  })
  const currentUser = currentUserData?.data as User | undefined

  // Accounts
  const { data: accountsData, isLoading: accountsLoading } = useQuery({
    queryKey: ['admin-accounts'],
    queryFn: () => api.adminGetAccounts(),
    enabled: currentUser?.role === 'root',
  })
  const accounts = (accountsData?.data || []) as Account[]

  // All users
  const { data: usersData, isLoading: usersLoading } = useQuery({
    queryKey: ['admin-users'],
    queryFn: () => api.adminGetAllUsers(),
    enabled: currentUser?.role === 'root',
  })
  const allUsers = (usersData?.data || []) as (User & { account_name?: string })[]

  // Orgs per account — collect from all accounts
  const { data: allOrgsData, isLoading: orgsLoading } = useQuery({
    queryKey: ['admin-all-orgs', accounts.map(a => a.id).join(',')],
    queryFn: async () => {
      const results: (Organization & { account_name?: string })[] = []
      for (const acct of accounts) {
        const res = await api.adminGetAccountOrgs(acct.id)
        const orgs = (res.data || []) as Organization[]
        orgs.forEach(o => results.push({ ...o, account_name: acct.name }))
      }
      return results
    },
    enabled: currentUser?.role === 'root' && accounts.length > 0,
  })
  const allOrgs = allOrgsData || []

  const filteredOrgs = filterAccountId
    ? allOrgs.filter(o => o.account_id === filterAccountId)
    : allOrgs

  // Create account
  const createMut = useMutation({
    mutationFn: (data: { name: string; plan: string }) => api.adminCreateAccount(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
      setShowCreate(false)
      setCreateForm({ name: '', plan: 'standard' })
      setError('')
    },
    onError: () => setError('Failed to create account'),
  })

  // Delete account
  const deleteMut = useMutation({
    mutationFn: (id: string) => api.adminDeleteAccount(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
      queryClient.invalidateQueries({ queryKey: ['admin-all-orgs'] })
      setDeleteId(null)
    },
  })

  // Toggle 2FA enforcement per account
  const toggle2FAMut = useMutation({
    mutationFn: (data: { accountId: string; require_2fa: boolean }) =>
      api.adminUpdateAccount(data.accountId, { require_2fa: data.require_2fa }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
    },
  })

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    if (!createForm.name.trim()) {
      setError('Account name is required')
      return
    }
    createMut.mutate(createForm)
  }

  const handleAccountClick = (accountId: string) => {
    setFilterAccountId(accountId)
    setTab('organizations')
  }

  if (currentUser && currentUser.role !== 'root') {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <p className="text-gray-500">You do not have permission to access this page.</p>
      </div>
    )
  }

  const tabs: { key: Tab; label: string }[] = [
    { key: 'accounts', label: 'Accounts' },
    { key: 'organizations', label: 'Organizations' },
    { key: 'users', label: 'Users' },
  ]

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Admin Panel</h1>
          <p className="mt-1 text-sm text-gray-500">Manage all accounts, organizations, and users</p>
        </div>
      </div>

      {/* Tabs */}
      <div className="mt-6 flex gap-1 border-b border-gray-200">
        {tabs.map(t => (
          <button
            key={t.key}
            onClick={() => { setTab(t.key); if (t.key !== 'organizations') setFilterAccountId(null) }}
            className={
              'px-4 py-2.5 text-sm font-medium border-b-2 transition-colors -mb-px ' +
              (tab === t.key
                ? 'border-fibratus-600 text-fibratus-600'
                : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300')
            }
          >
            {t.label}
          </button>
        ))}
      </div>

      {/* Accounts Tab */}
      {tab === 'accounts' && (
        <div className="mt-6">
          <div className="flex items-center justify-between mb-4">
            <p className="text-sm text-gray-500">{accounts.length} account(s)</p>
            <button
              onClick={() => setShowCreate(true)}
              className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
            >
              Create Account
            </button>
          </div>
          <div className="rounded-xl border border-gray-200 bg-white shadow-sm">
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead className="border-b border-gray-100 bg-gray-50/50">
                  <tr>
                    <th className="px-6 py-3 font-medium text-gray-500">Name</th>
                    <th className="px-6 py-3 font-medium text-gray-500">Plan</th>
                    <th className="px-6 py-3 font-medium text-gray-500">2FA Required</th>
                    <th className="px-6 py-3 font-medium text-gray-500">Orgs</th>
                    <th className="px-6 py-3 font-medium text-gray-500">Users</th>
                    <th className="px-6 py-3 font-medium text-gray-500">Created</th>
                    <th className="px-6 py-3 font-medium text-gray-500">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {accountsLoading && (
                    <tr><td colSpan={7} className="px-6 py-12 text-center text-gray-400">Loading...</td></tr>
                  )}
                  {!accountsLoading && accounts.map(acct => (
                    <tr key={acct.id} className="hover:bg-gray-50/50">
                      <td className="px-6 py-3">
                        <button
                          onClick={() => handleAccountClick(acct.id)}
                          className="font-medium text-fibratus-600 hover:underline"
                        >
                          {acct.name}
                        </button>
                      </td>
                      <td className="px-6 py-3">
                        <span className="inline-flex rounded-full bg-blue-50 px-2.5 py-0.5 text-xs font-medium text-blue-700">
                          {acct.plan}
                        </span>
                      </td>
                      <td className="px-6 py-3">
                        <button
                          onClick={() => toggle2FAMut.mutate({ accountId: acct.id, require_2fa: !acct.require_2fa })}
                          disabled={toggle2FAMut.isPending}
                          className={
                            'relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none disabled:opacity-50 ' +
                            (acct.require_2fa ? 'bg-emerald-500' : 'bg-gray-200')
                          }
                          role="switch"
                          aria-checked={acct.require_2fa || false}
                          title={acct.require_2fa ? '2FA enforced - click to disable' : '2FA not enforced - click to enable'}
                        >
                          <span
                            className={
                              'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ' +
                              (acct.require_2fa ? 'translate-x-4' : 'translate-x-0')
                            }
                          />
                        </button>
                      </td>
                      <td className="px-6 py-3 text-gray-600">{acct.org_count}</td>
                      <td className="px-6 py-3 text-gray-600">{acct.user_count}</td>
                      <td className="px-6 py-3 text-gray-500 whitespace-nowrap">
                        {new Date(acct.created_at).toLocaleDateString()}
                      </td>
                      <td className="px-6 py-3">
                        {deleteId === acct.id ? (
                          <div className="flex items-center gap-2">
                            <button
                              onClick={() => deleteMut.mutate(acct.id)}
                              className="rounded bg-red-600 px-2 py-1 text-xs text-white hover:bg-red-700"
                            >
                              Confirm
                            </button>
                            <button
                              onClick={() => setDeleteId(null)}
                              className="rounded bg-gray-200 px-2 py-1 text-xs text-gray-700 hover:bg-gray-300"
                            >
                              Cancel
                            </button>
                          </div>
                        ) : (
                          <button
                            onClick={() => setDeleteId(acct.id)}
                            className="text-xs text-red-600 hover:underline"
                          >
                            Delete
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                  {!accountsLoading && accounts.length === 0 && (
                    <tr><td colSpan={7} className="px-6 py-12 text-center text-gray-400">No accounts found.</td></tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {/* Organizations Tab */}
      {tab === 'organizations' && (
        <div className="mt-6">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-3">
              <p className="text-sm text-gray-500">{filteredOrgs.length} organization(s)</p>
              {filterAccountId && (
                <button
                  onClick={() => setFilterAccountId(null)}
                  className="text-xs text-fibratus-600 hover:underline"
                >
                  Show all
                </button>
              )}
            </div>
          </div>
          <div className="rounded-xl border border-gray-200 bg-white shadow-sm">
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead className="border-b border-gray-100 bg-gray-50/50">
                  <tr>
                    <th className="px-6 py-3 font-medium text-gray-500">Name</th>
                    <th className="px-6 py-3 font-medium text-gray-500">Account</th>
                    <th className="px-6 py-3 font-medium text-gray-500">Agents</th>
                    <th className="px-6 py-3 font-medium text-gray-500">Slug</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {orgsLoading && (
                    <tr><td colSpan={4} className="px-6 py-12 text-center text-gray-400">Loading...</td></tr>
                  )}
                  {!orgsLoading && filteredOrgs.map(org => (
                    <tr key={org.id} className="hover:bg-gray-50/50">
                      <td className="px-6 py-3 font-medium text-gray-900">{org.name}</td>
                      <td className="px-6 py-3 text-gray-600">{org.account_name || '-'}</td>
                      <td className="px-6 py-3 text-gray-600">{org.agent_count}</td>
                      <td className="px-6 py-3 text-gray-500 font-mono text-xs">{org.slug}</td>
                    </tr>
                  ))}
                  {!orgsLoading && filteredOrgs.length === 0 && (
                    <tr><td colSpan={4} className="px-6 py-12 text-center text-gray-400">No organizations found.</td></tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {/* Users Tab */}
      {tab === 'users' && (
        <div className="mt-6">
          <div className="flex items-center justify-between mb-4">
            <p className="text-sm text-gray-500">{allUsers.length} user(s) across all accounts</p>
          </div>
          <div className="rounded-xl border border-gray-200 bg-white shadow-sm">
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead className="border-b border-gray-100 bg-gray-50/50">
                  <tr>
                    <th className="px-6 py-3 font-medium text-gray-500">Name</th>
                    <th className="px-6 py-3 font-medium text-gray-500">Email</th>
                    <th className="px-6 py-3 font-medium text-gray-500">Role</th>
                    <th className="px-6 py-3 font-medium text-gray-500">Account</th>
                    <th className="px-6 py-3 font-medium text-gray-500">2FA</th>
                    <th className="px-6 py-3 font-medium text-gray-500">Created</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {usersLoading && (
                    <tr><td colSpan={6} className="px-6 py-12 text-center text-gray-400">Loading...</td></tr>
                  )}
                  {!usersLoading && allUsers.map(user => (
                    <tr key={user.id} className="hover:bg-gray-50/50">
                      <td className="px-6 py-3 font-medium text-gray-900">{user.name}</td>
                      <td className="px-6 py-3 text-gray-600">{user.email}</td>
                      <td className="px-6 py-3">
                        <span className={
                          'inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium ' +
                          (user.role === 'root' ? 'bg-red-50 text-red-700' :
                           user.role === 'admin' ? 'bg-purple-100 text-purple-700' :
                           user.role === 'analyst' ? 'bg-blue-100 text-blue-700' :
                           'bg-gray-100 text-gray-600')
                        }>
                          {user.role}
                        </span>
                      </td>
                      <td className="px-6 py-3 text-gray-600">{user.account_name || '-'}</td>
                      <td className="px-6 py-3">
                        <span className={'rounded-full px-2 py-0.5 text-[10px] font-medium ' +
                          (user.totp_enabled ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-500')}>
                          {user.totp_enabled ? 'Enabled' : 'Off'}
                        </span>
                      </td>
                      <td className="px-6 py-3 text-gray-500 whitespace-nowrap">
                        {new Date(user.created_at).toLocaleDateString()}
                      </td>
                    </tr>
                  ))}
                  {!usersLoading && allUsers.length === 0 && (
                    <tr><td colSpan={6} className="px-6 py-12 text-center text-gray-400">No users found.</td></tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {/* Create Account Modal */}
      {showCreate && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowCreate(false)}>
          <div className="bg-white rounded-xl shadow-xl w-full max-w-md p-6" onClick={e => e.stopPropagation()}>
            <h2 className="text-lg font-bold text-gray-900">Create Account</h2>
            <form onSubmit={handleCreate} className="mt-4 space-y-3">
              <div>
                <label className="block text-xs font-medium text-gray-500 mb-1">Account Name</label>
                <input
                  value={createForm.name}
                  onChange={e => setCreateForm(f => ({ ...f, name: e.target.value }))}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                  required
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 mb-1">Plan</label>
                <select
                  value={createForm.plan}
                  onChange={e => setCreateForm(f => ({ ...f, plan: e.target.value }))}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                >
                  <option value="trial">Trial</option>
                  <option value="standard">Standard</option>
                  <option value="professional">Professional</option>
                  <option value="enterprise">Enterprise</option>
                </select>
              </div>
              {error && <p className="text-xs text-red-600">{error}</p>}
              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => { setShowCreate(false); setError('') }}
                  className="rounded-lg border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
                >
                  Create Account
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
