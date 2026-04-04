import { useState, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Account, type Organization, type User } from '../lib/api'
import SlidePanel from '../components/SlidePanel'

type Tab = 'accounts' | 'users' | 'organizations'

const roleBadge: Record<string, string> = {
  root: 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-400',
  admin: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400',
  analyst: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  viewer: 'bg-gray-100 text-gray-600 dark:bg-slate-700 dark:text-slate-400',
}

const passwordRules = [
  { label: '12+ characters', test: (p: string) => p.length >= 12 },
  { label: 'Uppercase letter', test: (p: string) => /[A-Z]/.test(p) },
  { label: 'Lowercase letter', test: (p: string) => /[a-z]/.test(p) },
  { label: 'Digit', test: (p: string) => /\d/.test(p) },
  { label: 'Special character', test: (p: string) => /[^A-Za-z0-9]/.test(p) },
]

export default function Admin() {
  const _queryClient = useQueryClient()
  const [tab, setTab] = useState<Tab>('accounts')
  const [filterAccountId, setFilterAccountId] = useState<string | null>(null)

  const { data: currentUserData } = useQuery({
    queryKey: ['current-user'],
    queryFn: () => api.getCurrentUser(),
  })
  const currentUser = currentUserData?.data as User | undefined

  if (currentUser && currentUser.role !== 'root') {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <p className="text-gray-500 dark:text-slate-400">You do not have permission to access this page.</p>
      </div>
    )
  }

  const handleAccountClick = (accountId: string) => {
    setFilterAccountId(accountId)
    setTab('organizations')
  }

  const tabs: { key: Tab; label: string }[] = [
    { key: 'accounts', label: 'Accounts' },
    { key: 'users', label: 'Users' },
    { key: 'organizations', label: 'Organizations' },
  ]

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-slate-100">Admin Panel</h1>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">Manage all accounts, organizations, and users across the platform</p>
        </div>
      </div>

      {/* Tabs */}
      <div className="mt-6 flex gap-1 border-b border-gray-200 dark:border-slate-700">
        {tabs.map(t => (
          <button
            key={t.key}
            onClick={() => { setTab(t.key); if (t.key !== 'organizations') setFilterAccountId(null) }}
            className={
              'px-4 py-2.5 text-sm font-medium border-b-2 transition-colors -mb-px ' +
              (tab === t.key
                ? 'border-fibratus-600 text-fibratus-600'
                : 'border-transparent text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300 hover:border-gray-300 dark:hover:border-slate-600')
            }
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'accounts' && <AccountsTab onAccountClick={handleAccountClick} />}
      {tab === 'users' && <UsersTab />}
      {tab === 'organizations' && <OrganizationsTab filterAccountId={filterAccountId} onClearFilter={() => setFilterAccountId(null)} />}
    </div>
  )
}

// ================================================================
// Accounts Tab
// ================================================================

function AccountsTab({ onAccountClick }: { onAccountClick: (id: string) => void }) {
  const queryClient = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [createForm, setCreateForm] = useState({ name: '', plan: 'standard' })
  const [error, setError] = useState('')
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [editingAccount, setEditingAccount] = useState<Account | null>(null)
  const [search, setSearch] = useState('')

  const { data: currentUserData } = useQuery({
    queryKey: ['current-user'],
    queryFn: () => api.getCurrentUser(),
  })
  const currentUser = currentUserData?.data as User | undefined

  const { data: accountsData, isLoading } = useQuery({
    queryKey: ['admin-accounts'],
    queryFn: () => api.adminGetAccounts(),
    enabled: currentUser?.role === 'root',
  })
  const accounts = (accountsData?.data || []) as Account[]

  const filtered = useMemo(() => {
    if (!search.trim()) return accounts
    const q = search.toLowerCase()
    return accounts.filter(a =>
      a.name.toLowerCase().includes(q) || a.plan.toLowerCase().includes(q)
    )
  }, [accounts, search])

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

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.adminDeleteAccount(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
      queryClient.invalidateQueries({ queryKey: ['admin-all-orgs'] })
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      setDeleteId(null)
    },
  })

  const toggle2FAMut = useMutation({
    mutationFn: (data: { accountId: string; require_2fa: boolean }) =>
      api.adminUpdateAccount(data.accountId, { require_2fa: data.require_2fa }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin-accounts'] }),
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

  return (
    <div className="mt-6">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-4">
          <p className="text-sm text-gray-500 dark:text-slate-400">{filtered.length} account(s)</p>
          <input
            type="text"
            placeholder="Search accounts..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-1.5 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
          />
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
        >
          Create Account
        </button>
      </div>

      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Name</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Plan</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">2FA Required</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Orgs</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Users</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Created</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {isLoading && (
                <tr><td colSpan={7} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">Loading...</td></tr>
              )}
              {!isLoading && filtered.map(acct => (
                <tr key={acct.id} className="hover:bg-gray-50/50 dark:hover:bg-slate-700/50">
                  <td className="px-6 py-3">
                    <button
                      onClick={() => onAccountClick(acct.id)}
                      className="font-medium text-fibratus-600 hover:underline"
                    >
                      {acct.name}
                    </button>
                  </td>
                  <td className="px-6 py-3">
                    <span className="inline-flex rounded-full bg-blue-50 dark:bg-blue-900/30 px-2.5 py-0.5 text-xs font-medium text-blue-700 dark:text-blue-400">
                      {acct.plan}
                    </span>
                  </td>
                  <td className="px-6 py-3">
                    <button
                      onClick={() => toggle2FAMut.mutate({ accountId: acct.id, require_2fa: !acct.require_2fa })}
                      disabled={toggle2FAMut.isPending}
                      className={
                        'relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none disabled:opacity-50 ' +
                        (acct.require_2fa ? 'bg-emerald-500' : 'bg-gray-200 dark:bg-slate-600')
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
                  <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{acct.org_count}</td>
                  <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{acct.user_count}</td>
                  <td className="px-6 py-3 text-gray-500 dark:text-slate-400 whitespace-nowrap">
                    {new Date(acct.created_at).toLocaleDateString()}
                  </td>
                  <td className="px-6 py-3">
                    <div className="flex items-center gap-3">
                      <button
                        onClick={() => setEditingAccount(acct)}
                        className="text-xs text-fibratus-600 hover:underline"
                      >
                        Edit
                      </button>
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
                            className="rounded bg-gray-200 dark:bg-slate-600 px-2 py-1 text-xs text-gray-700 dark:text-slate-300 hover:bg-gray-300 dark:hover:bg-slate-500"
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
                    </div>
                  </td>
                </tr>
              ))}
              {!isLoading && filtered.length === 0 && (
                <tr><td colSpan={7} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">No accounts found.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Create Account Modal */}
      {showCreate && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowCreate(false)}>
          <div className="bg-white dark:bg-slate-800 rounded-xl shadow-xl w-full max-w-md p-6" onClick={e => e.stopPropagation()}>
            <h2 className="text-lg font-bold text-gray-900 dark:text-slate-100">Create Account</h2>
            <form onSubmit={handleCreate} className="mt-4 space-y-3">
              <div>
                <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Account Name</label>
                <input
                  value={createForm.name}
                  onChange={e => setCreateForm(f => ({ ...f, name: e.target.value }))}
                  className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                  required
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Plan</label>
                <select
                  value={createForm.plan}
                  onChange={e => setCreateForm(f => ({ ...f, plan: e.target.value }))}
                  className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
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
                  className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={createMut.isPending}
                  className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
                >
                  {createMut.isPending ? 'Creating...' : 'Create Account'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Edit Account Modal */}
      {editingAccount && (
        <EditAccountModal
          account={editingAccount}
          onClose={() => setEditingAccount(null)}
        />
      )}
    </div>
  )
}

function EditAccountModal({ account, onClose }: { account: Account; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(account.name)
  const [plan, setPlan] = useState(account.plan)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  const updateMut = useMutation({
    mutationFn: () => api.adminUpdateAccount(account.id, { name, plan }),
    onSuccess: (res) => {
      if (res.error) { setError(res.error.message); return }
      setSuccess('Account updated.')
      setError('')
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
      setTimeout(() => onClose(), 1000)
    },
    onError: () => setError('Failed to update account.'),
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setSuccess('')
    if (!name.trim()) { setError('Name is required'); return }
    updateMut.mutate()
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-white dark:bg-slate-800 rounded-xl shadow-xl w-full max-w-md p-6" onClick={e => e.stopPropagation()}>
        <h2 className="text-lg font-bold text-gray-900 dark:text-slate-100">Edit Account</h2>
        <form onSubmit={handleSubmit} className="mt-4 space-y-3">
          <div>
            <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Account Name</label>
            <input
              value={name}
              onChange={e => setName(e.target.value)}
              className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
              required
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Plan</label>
            <select
              value={plan}
              onChange={e => setPlan(e.target.value)}
              className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
            >
              <option value="trial">Trial</option>
              <option value="standard">Standard</option>
              <option value="professional">Professional</option>
              <option value="enterprise">Enterprise</option>
            </select>
          </div>
          {error && <p className="text-xs text-red-600">{error}</p>}
          {success && <p className="text-xs text-emerald-600">{success}</p>}
          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={updateMut.isPending}
              className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
            >
              {updateMut.isPending ? 'Saving...' : 'Save Changes'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// ================================================================
// Users Tab
// ================================================================

function UsersTab() {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [editingUser, setEditingUser] = useState<(User & { account_name?: string }) | null>(null)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const { data: currentUserData } = useQuery({
    queryKey: ['current-user'],
    queryFn: () => api.getCurrentUser(),
  })
  const currentUser = currentUserData?.data as User | undefined

  const { data: usersData, isLoading } = useQuery({
    queryKey: ['admin-users'],
    queryFn: () => api.adminGetAllUsers(),
    enabled: currentUser?.role === 'root',
  })
  const allUsers = (usersData?.data || []) as (User & { account_name?: string })[]

  const filtered = useMemo(() => {
    if (!search.trim()) return allUsers
    const q = search.toLowerCase()
    return allUsers.filter(u =>
      u.name.toLowerCase().includes(q) ||
      u.email.toLowerCase().includes(q) ||
      u.role.toLowerCase().includes(q) ||
      (u.account_name || '').toLowerCase().includes(q)
    )
  }, [allUsers, search])

  const roleMut = useMutation({
    mutationFn: ({ id, role }: { id: string; role: string }) => api.adminUpdateUser(id, { role }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin-users'] }),
  })

  const unlockMut = useMutation({
    mutationFn: (id: string) => api.adminUnlockUser(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin-users'] }),
  })

  const disableTotpMut = useMutation({
    mutationFn: (id: string) => api.adminDisableTOTP(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin-users'] }),
  })

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.adminDeleteUser(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
      setDeleteId(null)
    },
  })

  return (
    <div className="mt-6">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-4">
          <p className="text-sm text-gray-500 dark:text-slate-400">{filtered.length} user(s) across all accounts</p>
          <input
            type="text"
            placeholder="Search by name, email, role..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-1.5 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none w-72"
          />
        </div>
      </div>

      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Name</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Email</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Role</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Account</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">2FA</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Status</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Created</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {isLoading && (
                <tr><td colSpan={8} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">Loading...</td></tr>
              )}
              {!isLoading && filtered.map(user => {
                const isSelf = currentUser?.id === user.id
                return (
                  <tr key={user.id} className="hover:bg-gray-50/50 dark:hover:bg-slate-700/50">
                    <td className="px-6 py-3 font-medium text-gray-900 dark:text-slate-100">
                      {user.name}
                      {isSelf && <span className="ml-2 text-xs text-gray-400 dark:text-slate-500">(you)</span>}
                      {user.locked && (
                        <span className="ml-2 inline-flex items-center rounded-full bg-red-50 dark:bg-red-900/30 px-2 py-0.5 text-[10px] font-medium text-red-700 dark:text-red-400">
                          <svg className="mr-0.5 h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                          </svg>
                          locked
                        </span>
                      )}
                    </td>
                    <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{user.email}</td>
                    <td className="px-6 py-3">
                      {!isSelf ? (
                        <select
                          value={user.role}
                          onChange={e => roleMut.mutate({ id: user.id, role: e.target.value })}
                          className={'rounded-full px-2.5 py-0.5 text-xs font-medium border-0 cursor-pointer ' +
                            (roleBadge[user.role] || roleBadge.viewer)}
                        >
                          <option value="root">Root</option>
                          <option value="admin">Admin</option>
                          <option value="analyst">Analyst</option>
                          <option value="viewer">Viewer</option>
                        </select>
                      ) : (
                        <span className={'inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium ' +
                          (roleBadge[user.role] || roleBadge.viewer)}>
                          {user.role}
                        </span>
                      )}
                    </td>
                    <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{user.account_name || '-'}</td>
                    <td className="px-6 py-3">
                      <span className={'rounded-full px-2 py-0.5 text-[10px] font-medium ' +
                        (user.totp_enabled ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400' : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')}>
                        {user.totp_enabled ? 'Enabled' : 'Off'}
                      </span>
                    </td>
                    <td className="px-6 py-3">
                      <span className={'rounded-full px-2 py-0.5 text-[10px] font-medium ' +
                        (user.locked
                          ? 'bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-400'
                          : 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400')}>
                        {user.locked ? 'Locked' : 'Active'}
                      </span>
                    </td>
                    <td className="px-6 py-3 text-gray-500 dark:text-slate-400 whitespace-nowrap">
                      {new Date(user.created_at).toLocaleDateString()}
                    </td>
                    <td className="px-6 py-3">
                      <div className="flex items-center gap-2">
                        {!isSelf && (
                          <button
                            onClick={() => setEditingUser(user)}
                            className="text-xs text-fibratus-600 hover:underline"
                          >
                            Edit
                          </button>
                        )}
                        {!isSelf && user.locked && (
                          <button
                            onClick={() => unlockMut.mutate(user.id)}
                            disabled={unlockMut.isPending}
                            className="text-xs text-amber-600 hover:underline"
                          >
                            Unlock
                          </button>
                        )}
                        {!isSelf && user.totp_enabled && (
                          <button
                            onClick={() => disableTotpMut.mutate(user.id)}
                            disabled={disableTotpMut.isPending}
                            className="text-xs text-amber-600 hover:underline"
                          >
                            Disable 2FA
                          </button>
                        )}
                        {!isSelf && (
                          deleteId === user.id ? (
                            <div className="flex items-center gap-2">
                              <button
                                onClick={() => deleteMut.mutate(user.id)}
                                className="rounded bg-red-600 px-2 py-1 text-xs text-white hover:bg-red-700"
                              >
                                Confirm
                              </button>
                              <button
                                onClick={() => setDeleteId(null)}
                                className="rounded bg-gray-200 dark:bg-slate-600 px-2 py-1 text-xs text-gray-700 dark:text-slate-300 hover:bg-gray-300 dark:hover:bg-slate-500"
                              >
                                Cancel
                              </button>
                            </div>
                          ) : (
                            <button
                              onClick={() => setDeleteId(user.id)}
                              className="text-xs text-red-600 hover:underline"
                            >
                              Delete
                            </button>
                          )
                        )}
                      </div>
                    </td>
                  </tr>
                )
              })}
              {!isLoading && filtered.length === 0 && (
                <tr><td colSpan={8} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">No users found.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Edit User SlidePanel */}
      {editingUser && (
        <AdminUserEditPanel
          user={editingUser}
          onClose={() => setEditingUser(null)}
        />
      )}
    </div>
  )
}

function AdminUserEditPanel({ user, onClose }: { user: User & { account_name?: string }; onClose: () => void }) {
  const queryClient = useQueryClient()

  // Profile
  const [profileName, setProfileName] = useState(user.name)
  const [profileEmail, setProfileEmail] = useState(user.email)
  const [profileMsg, setProfileMsg] = useState('')
  const [profileError, setProfileError] = useState('')

  // Password
  const [newPassword, setNewPassword] = useState('')
  const [passwordMsg, setPasswordMsg] = useState('')
  const [passwordError, setPasswordError] = useState('')

  // 2FA
  const [totpConfirm, setTotpConfirm] = useState(false)
  const [totpMsg, setTotpMsg] = useState('')
  const [totpError, setTotpError] = useState('')

  // Delete
  const [deleteConfirm, setDeleteConfirm] = useState(false)

  const updateProfileMut = useMutation({
    mutationFn: () => api.adminUpdateUser(user.id, { name: profileName, email: profileEmail }),
    onSuccess: (res) => {
      if (res.error) { setProfileError(res.error.message); return }
      setProfileMsg('Profile updated.')
      setProfileError('')
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      setTimeout(() => setProfileMsg(''), 3000)
    },
    onError: () => setProfileError('Failed to update profile.'),
  })

  const resetPasswordMut = useMutation({
    mutationFn: () => api.adminResetPassword(user.id, newPassword),
    onSuccess: (res) => {
      if (res.error) { setPasswordError(res.error.message); return }
      setPasswordMsg('Password has been reset.')
      setPasswordError('')
      setNewPassword('')
      setTimeout(() => setPasswordMsg(''), 3000)
    },
    onError: () => setPasswordError('Failed to reset password.'),
  })

  const disableTotpMut = useMutation({
    mutationFn: () => api.adminDisableTOTP(user.id),
    onSuccess: (res) => {
      if (res.error) { setTotpError(res.error.message); return }
      setTotpMsg('2FA has been disabled for this user.')
      setTotpError('')
      setTotpConfirm(false)
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      setTimeout(() => setTotpMsg(''), 3000)
    },
    onError: () => setTotpError('Failed to disable 2FA.'),
  })

  const unlockMut = useMutation({
    mutationFn: () => api.adminUnlockUser(user.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
    },
  })

  const deleteMut = useMutation({
    mutationFn: () => api.adminDeleteUser(user.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
      onClose()
    },
  })

  const allPasswordRulesPass = passwordRules.every(r => r.test(newPassword))

  return (
    <SlidePanel open={true} title={`${user.name || user.email} ${user.account_name ? `(${user.account_name})` : ''}`} onClose={onClose}>
      <div className="space-y-8">
        {/* Profile Section */}
        <div>
          <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Profile</h3>
          <div className="mt-3 space-y-3">
            <div>
              <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Name</label>
              <input
                value={profileName}
                onChange={e => setProfileName(e.target.value)}
                className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Email</label>
              <input
                type="email"
                value={profileEmail}
                onChange={e => setProfileEmail(e.target.value)}
                className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Role</label>
              <select
                value={user.role}
                disabled
                className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-gray-50 dark:bg-slate-700/50 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:outline-none cursor-not-allowed"
              >
                <option>{user.role}</option>
              </select>
              <p className="mt-1 text-[10px] text-gray-400 dark:text-slate-500">Change role via the inline dropdown in the users table.</p>
            </div>
            {profileError && <p className="text-xs text-red-600">{profileError}</p>}
            {profileMsg && <p className="text-xs text-emerald-600">{profileMsg}</p>}
            <button
              onClick={() => { setProfileError(''); updateProfileMut.mutate() }}
              disabled={updateProfileMut.isPending || (!profileName.trim() && !profileEmail.trim())}
              className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
            >
              {updateProfileMut.isPending ? 'Saving...' : 'Save Profile'}
            </button>
          </div>
        </div>

        <hr className="border-gray-200 dark:border-slate-700" />

        {/* Locked status */}
        {user.locked && (
          <>
            <div>
              <h3 className="text-sm font-semibold text-amber-600">Account Locked</h3>
              <p className="mt-1 text-xs text-gray-500 dark:text-slate-400">This user's account is locked, likely due to too many failed login attempts.</p>
              <div className="mt-3">
                <button
                  onClick={() => unlockMut.mutate()}
                  disabled={unlockMut.isPending}
                  className="rounded-lg bg-amber-600 px-4 py-2 text-sm font-medium text-white hover:bg-amber-700 disabled:opacity-50"
                >
                  {unlockMut.isPending ? 'Unlocking...' : 'Unlock Account'}
                </button>
              </div>
            </div>
            <hr className="border-gray-200 dark:border-slate-700" />
          </>
        )}

        {/* Password Section */}
        <div>
          <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Reset Password</h3>
          <p className="mt-1 text-xs text-gray-500 dark:text-slate-400">Set a new password for this user. They will need to use the new password on next login.</p>
          <div className="mt-3 space-y-3">
            <div>
              <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">New Password</label>
              <input
                type="password"
                value={newPassword}
                onChange={e => setNewPassword(e.target.value)}
                className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                placeholder="Enter new password"
                minLength={12}
              />
              <div className="mt-2 space-y-1">
                {passwordRules.map(rule => (
                  <p key={rule.label} className={'text-[10px] ' + (rule.test(newPassword) ? 'text-emerald-600' : 'text-gray-400')}>
                    {rule.test(newPassword) ? '\u2713' : '\u2022'} {rule.label}
                  </p>
                ))}
              </div>
            </div>
            {passwordError && <p className="text-xs text-red-600">{passwordError}</p>}
            {passwordMsg && <p className="text-xs text-emerald-600">{passwordMsg}</p>}
            <button
              onClick={() => { setPasswordError(''); resetPasswordMut.mutate() }}
              disabled={!allPasswordRulesPass || resetPasswordMut.isPending}
              className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
            >
              {resetPasswordMut.isPending ? 'Resetting...' : 'Reset Password'}
            </button>
          </div>
        </div>

        <hr className="border-gray-200 dark:border-slate-700" />

        {/* 2FA Section */}
        <div>
          <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Two-Factor Authentication</h3>
          <div className="mt-3 flex items-center gap-3">
            <span className="text-sm text-gray-600 dark:text-slate-400">Status:</span>
            <span className={'inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium ' +
              (user.totp_enabled ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400' : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')}>
              {user.totp_enabled ? 'Enabled' : 'Disabled'}
            </span>
          </div>
          {user.totp_enabled && (
            <div className="mt-3">
              {!totpConfirm ? (
                <button
                  onClick={() => setTotpConfirm(true)}
                  className="rounded-lg border border-red-300 dark:border-red-800 px-4 py-2 text-sm font-medium text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20"
                >
                  Disable 2FA
                </button>
              ) : (
                <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20 p-4 space-y-3">
                  <p className="text-sm text-red-800 dark:text-red-300">
                    Are you sure you want to disable 2FA for this user? This will remove their TOTP configuration.
                  </p>
                  {totpError && <p className="text-xs text-red-600">{totpError}</p>}
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => { setTotpError(''); disableTotpMut.mutate() }}
                      disabled={disableTotpMut.isPending}
                      className="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
                    >
                      {disableTotpMut.isPending ? 'Disabling...' : 'Yes, Disable 2FA'}
                    </button>
                    <button
                      onClick={() => setTotpConfirm(false)}
                      className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              )}
              {totpMsg && <p className="mt-2 text-xs text-emerald-600">{totpMsg}</p>}
            </div>
          )}
          {!user.totp_enabled && (
            <p className="mt-2 text-xs text-gray-500 dark:text-slate-400">This user has not enabled two-factor authentication.</p>
          )}
        </div>

        <hr className="border-gray-200 dark:border-slate-700" />

        {/* Danger Zone */}
        <div>
          <h3 className="text-sm font-semibold text-red-600">Danger Zone</h3>
          <p className="mt-1 text-xs text-gray-500 dark:text-slate-400">Permanently delete this user account. This action cannot be undone.</p>
          <div className="mt-3">
            {!deleteConfirm ? (
              <button
                onClick={() => setDeleteConfirm(true)}
                className="rounded-lg border border-red-300 dark:border-red-800 bg-white dark:bg-slate-800 px-4 py-2 text-sm font-medium text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20"
              >
                Delete User
              </button>
            ) : (
              <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20 p-4 space-y-3">
                <p className="text-sm text-red-800 dark:text-red-300">
                  Are you sure you want to permanently delete <strong>{user.name || user.email}</strong>?
                </p>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => deleteMut.mutate()}
                    disabled={deleteMut.isPending}
                    className="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
                  >
                    {deleteMut.isPending ? 'Deleting...' : 'Yes, Delete User'}
                  </button>
                  <button
                    onClick={() => setDeleteConfirm(false)}
                    className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </SlidePanel>
  )
}

// ================================================================
// Organizations Tab
// ================================================================

function OrganizationsTab({ filterAccountId, onClearFilter }: { filterAccountId: string | null; onClearFilter: () => void }) {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [createForm, setCreateForm] = useState({ name: '', slug: '' })
  const [createError, setCreateError] = useState('')
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const { data: currentUserData } = useQuery({
    queryKey: ['current-user'],
    queryFn: () => api.getCurrentUser(),
  })
  const currentUser = currentUserData?.data as User | undefined

  const { data: accountsData } = useQuery({
    queryKey: ['admin-accounts'],
    queryFn: () => api.adminGetAccounts(),
    enabled: currentUser?.role === 'root',
  })
  const accounts = (accountsData?.data || []) as Account[]

  const { data: allOrgsData, isLoading } = useQuery({
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

  const filtered = useMemo(() => {
    let list = filterAccountId
      ? allOrgs.filter(o => o.account_id === filterAccountId)
      : allOrgs
    if (search.trim()) {
      const q = search.toLowerCase()
      list = list.filter(o =>
        o.name.toLowerCase().includes(q) ||
        (o.account_name || '').toLowerCase().includes(q) ||
        o.slug.toLowerCase().includes(q)
      )
    }
    return list
  }, [allOrgs, filterAccountId, search])

  const createMut = useMutation({
    mutationFn: (data: { name: string; slug: string }) => api.createOrganization(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-all-orgs'] })
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
      setShowCreate(false)
      setCreateForm({ name: '', slug: '' })
      setCreateError('')
    },
    onError: () => setCreateError('Failed to create organization'),
  })

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deleteOrganization(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-all-orgs'] })
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
      setDeleteId(null)
    },
  })

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault()
    setCreateError('')
    if (!createForm.name.trim()) { setCreateError('Name is required'); return }
    const slug = createForm.slug.trim() || createForm.name.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-')
    createMut.mutate({ name: createForm.name, slug })
  }

  const filterAccountName = filterAccountId ? accounts.find(a => a.id === filterAccountId)?.name : null

  return (
    <div className="mt-6">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-4">
          <p className="text-sm text-gray-500 dark:text-slate-400">
            {filtered.length} organization(s)
            {filterAccountName && (
              <span className="ml-1 text-fibratus-600">in {filterAccountName}</span>
            )}
          </p>
          {filterAccountId && (
            <button
              onClick={onClearFilter}
              className="text-xs text-fibratus-600 hover:underline"
            >
              Show all
            </button>
          )}
          <input
            type="text"
            placeholder="Search organizations..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-1.5 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
          />
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
        >
          Create Organization
        </button>
      </div>

      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Name</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Account</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Agents</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Slug</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {isLoading && (
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">Loading...</td></tr>
              )}
              {!isLoading && filtered.map(org => (
                <tr key={org.id} className="hover:bg-gray-50/50 dark:hover:bg-slate-700/50">
                  <td className="px-6 py-3 font-medium text-gray-900 dark:text-slate-100">{org.name}</td>
                  <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{org.account_name || '-'}</td>
                  <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{org.agent_count}</td>
                  <td className="px-6 py-3 text-gray-500 dark:text-slate-400 font-mono text-xs">{org.slug}</td>
                  <td className="px-6 py-3">
                    {deleteId === org.id ? (
                      <div className="flex items-center gap-2">
                        <button
                          onClick={() => deleteMut.mutate(org.id)}
                          className="rounded bg-red-600 px-2 py-1 text-xs text-white hover:bg-red-700"
                        >
                          Confirm
                        </button>
                        <button
                          onClick={() => setDeleteId(null)}
                          className="rounded bg-gray-200 dark:bg-slate-600 px-2 py-1 text-xs text-gray-700 dark:text-slate-300 hover:bg-gray-300 dark:hover:bg-slate-500"
                        >
                          Cancel
                        </button>
                      </div>
                    ) : (
                      <button
                        onClick={() => setDeleteId(org.id)}
                        className="text-xs text-red-600 hover:underline"
                      >
                        Delete
                      </button>
                    )}
                  </td>
                </tr>
              ))}
              {!isLoading && filtered.length === 0 && (
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">No organizations found.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Create Organization Modal */}
      {showCreate && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowCreate(false)}>
          <div className="bg-white dark:bg-slate-800 rounded-xl shadow-xl w-full max-w-md p-6" onClick={e => e.stopPropagation()}>
            <h2 className="text-lg font-bold text-gray-900 dark:text-slate-100">Create Organization</h2>
            <form onSubmit={handleCreate} className="mt-4 space-y-3">
              <div>
                <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Organization Name</label>
                <input
                  value={createForm.name}
                  onChange={e => setCreateForm(f => ({ ...f, name: e.target.value }))}
                  className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                  required
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Slug (optional)</label>
                <input
                  value={createForm.slug}
                  onChange={e => setCreateForm(f => ({ ...f, slug: e.target.value }))}
                  placeholder="auto-generated from name"
                  className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                />
              </div>
              {createError && <p className="text-xs text-red-600">{createError}</p>}
              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => { setShowCreate(false); setCreateError('') }}
                  className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={createMut.isPending}
                  className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
                >
                  {createMut.isPending ? 'Creating...' : 'Create Organization'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
