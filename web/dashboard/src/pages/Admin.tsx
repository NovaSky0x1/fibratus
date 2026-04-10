import { useState, useMemo, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Account, type Organization, type User } from '../lib/api'
import SlidePanel from '../components/SlidePanel'
import Groups from './Groups'
import { useTableSort } from '../hooks/useTableSort'
import SortableHeader from '../components/SortableHeader'

type Tab = 'accounts' | 'users' | 'organizations' | 'groups' | 'system'

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
    { key: 'groups', label: 'User Groups' },
    { key: 'system', label: 'System' },
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
      {tab === 'groups' && <GroupsTab />}
      {tab === 'system' && <SystemTab />}
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

  const { sorted: sortedAccounts, sort: accountSort, toggleSort: toggleAccountSort } = useTableSort<Account>(filtered, 'name', 'asc')

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
                <SortableHeader label="Name" sortKey="name" sort={accountSort} onSort={toggleAccountSort} />
                <SortableHeader label="Plan" sortKey="plan" sort={accountSort} onSort={toggleAccountSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">2FA Required</th>
                <SortableHeader label="Orgs" sortKey="org_count" sort={accountSort} onSort={toggleAccountSort} />
                <SortableHeader label="Users" sortKey="user_count" sort={accountSort} onSort={toggleAccountSort} />
                <SortableHeader label="Created" sortKey="created_at" sort={accountSort} onSort={toggleAccountSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {isLoading && (
                <tr><td colSpan={7} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">Loading...</td></tr>
              )}
              {!isLoading && sortedAccounts.map(acct => (
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
// Helpers
// ================================================================

function parseOrgRestrictions(raw: string | string[] | null | undefined): string[] {
  if (!raw) return []
  if (Array.isArray(raw)) return raw.filter(Boolean)
  if (typeof raw === 'string') {
    try {
      const parsed = JSON.parse(raw)
      if (Array.isArray(parsed)) return parsed.filter(Boolean)
    } catch { /* ignore */ }
    if (raw.trim()) return [raw]
  }
  return []
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

  const { sorted: sortedUsers, sort: userSort, toggleSort: toggleUserSort } = useTableSort<User & { account_name?: string }>(filtered, 'name', 'asc')

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
                <SortableHeader label="Name" sortKey="name" sort={userSort} onSort={toggleUserSort} />
                <SortableHeader label="Email" sortKey="email" sort={userSort} onSort={toggleUserSort} />
                <SortableHeader label="Role" sortKey="role" sort={userSort} onSort={toggleUserSort} />
                <SortableHeader label="Account" sortKey="account_name" sort={userSort} onSort={toggleUserSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Org Access</th>
                <SortableHeader label="2FA" sortKey="totp_enabled" sort={userSort} onSort={toggleUserSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Status</th>
                <SortableHeader label="Created" sortKey="created_at" sort={userSort} onSort={toggleUserSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {isLoading && (
                <tr><td colSpan={9} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">Loading...</td></tr>
              )}
              {!isLoading && sortedUsers.map(user => {
                const isSelf = currentUser?.id === user.id
                const orgRestrictions = parseOrgRestrictions(user.org_restrictions)
                return (
                  <tr key={user.id} className="hover:bg-gray-50/50 dark:hover:bg-slate-700/50">
                    <td className="px-6 py-3 font-medium text-gray-900 dark:text-slate-100">
                      {user.name}
                      {isSelf && <span className="ml-2 text-xs text-gray-400 dark:text-slate-500">(you)</span>}
                    </td>
                    <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{user.email}</td>
                    <td className="px-6 py-3">
                      <span className={'inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium ' +
                        (roleBadge[user.role] || roleBadge.viewer)}>
                        {user.role}
                      </span>
                    </td>
                    <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{user.account_name || '-'}</td>
                    <td className="px-6 py-3">
                      <span className={'rounded-full px-2 py-0.5 text-[10px] font-medium ' +
                        (orgRestrictions.length === 0
                          ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400'
                          : 'bg-gray-100 dark:bg-slate-700 text-gray-600 dark:text-slate-400')}>
                        {orgRestrictions.length === 0 ? 'All orgs' : `${orgRestrictions.length} org(s)`}
                      </span>
                    </td>
                    <td className="px-6 py-3">
                      <span className={'rounded-full px-2 py-0.5 text-[10px] font-medium ' +
                        (user.totp_enabled ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400' : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')}>
                        {user.totp_enabled ? 'Enabled' : 'Off'}
                      </span>
                    </td>
                    <td className="px-6 py-3">
                      <span className={'rounded-full px-2 py-0.5 text-[10px] font-medium ' +
                        (user.is_locked
                          ? 'bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-400'
                          : 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400')}>
                        {user.is_locked ? 'Locked' : 'Active'}
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
                <tr><td colSpan={9} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">No users found.</td></tr>
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
  const { data: currentUserData } = useQuery({
    queryKey: ['current-user'],
    queryFn: () => api.getCurrentUser(),
  })
  const currentUser = currentUserData?.data as User | undefined
  const inputCls = 'w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none'
  const btnPrimary = 'rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50'
  const sectionHeader = 'text-sm font-semibold text-gray-900 dark:text-slate-100'
  const labelCls = 'block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1'

  // ── Profile ──
  const [profileName, setProfileName] = useState(user.name)
  const [profileEmail, setProfileEmail] = useState(user.email)
  const [profileMsg, setProfileMsg] = useState('')
  const [profileError, setProfileError] = useState('')

  // ── Account ──
  const [selectedAccountId, setSelectedAccountId] = useState(user.account_id)
  const [accountMsg, setAccountMsg] = useState('')
  const [accountError, setAccountError] = useState('')

  // ── Role ──
  const [selectedRole, setSelectedRole] = useState(user.role)
  const [roleMsg, setRoleMsg] = useState('')
  const [roleError, setRoleError] = useState('')

  // ── Org Access ──
  const currentRestrictions = parseOrgRestrictions(user.org_restrictions)
  const [allOrgsAccess, setAllOrgsAccess] = useState(currentRestrictions.length === 0)
  const [selectedOrgIds, setSelectedOrgIds] = useState<string[]>(currentRestrictions)
  const [orgMsg, setOrgMsg] = useState('')
  const [orgError, setOrgError] = useState('')

  // ── Password ──
  const [newPassword, setNewPassword] = useState('')
  const [passwordMsg, setPasswordMsg] = useState('')
  const [passwordError, setPasswordError] = useState('')

  // ── 2FA ──
  const [totpConfirm, setTotpConfirm] = useState(false)
  const [totpMsg, setTotpMsg] = useState('')
  const [totpError, setTotpError] = useState('')

  // ── Delete ──
  const [deleteConfirm, setDeleteConfirm] = useState(false)

  // ── Data Queries ──
  const { data: accountsData } = useQuery({
    queryKey: ['admin-accounts'],
    queryFn: () => api.adminGetAccounts(),
  })
  const accounts = (accountsData?.data || []) as Account[]

  const { data: accountOrgsData } = useQuery({
    queryKey: ['admin-account-orgs', selectedAccountId],
    queryFn: () => api.adminGetAccountOrgs(selectedAccountId),
    enabled: !!selectedAccountId,
  })
  const accountOrgs = (accountOrgsData?.data || []) as Organization[]

  // When account changes, reset org selections
  useEffect(() => {
    if (selectedAccountId !== user.account_id) {
      setAllOrgsAccess(true)
      setSelectedOrgIds([])
    }
  }, [selectedAccountId, user.account_id])

  // ── Mutations ──
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

  const updateAccountMut = useMutation({
    mutationFn: () => api.adminUpdateUser(user.id, { account_id: selectedAccountId }),
    onSuccess: (res) => {
      if (res.error) { setAccountError(res.error.message); return }
      setAccountMsg('Account assignment updated.')
      setAccountError('')
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
      setTimeout(() => setAccountMsg(''), 3000)
    },
    onError: () => setAccountError('Failed to update account assignment.'),
  })

  const updateRoleMut = useMutation({
    mutationFn: () => api.adminUpdateUser(user.id, { role: selectedRole }),
    onSuccess: (res) => {
      if (res.error) { setRoleError(res.error.message); return }
      setRoleMsg('Role updated.')
      setRoleError('')
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      setTimeout(() => setRoleMsg(''), 3000)
    },
    onError: () => setRoleError('Failed to update role.'),
  })

  const updateOrgAccessMut = useMutation({
    mutationFn: () => api.adminUpdateUser(user.id, {
      org_restrictions: allOrgsAccess ? [] : selectedOrgIds,
      set_org_restrictions: true,
    }),
    onSuccess: (res) => {
      if (res.error) { setOrgError(res.error.message); return }
      setOrgMsg('Organization access updated.')
      setOrgError('')
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      setTimeout(() => setOrgMsg(''), 3000)
    },
    onError: () => setOrgError('Failed to update organization access.'),
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
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin-users'] }),
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

  const toggleOrgId = (orgId: string) => {
    setSelectedOrgIds(prev =>
      prev.includes(orgId) ? prev.filter(id => id !== orgId) : [...prev, orgId]
    )
  }

  return (
    <SlidePanel open={true} title={`${user.name || user.email}`} onClose={onClose}>
      <div className="space-y-6">

        {/* ── 1. Profile ── */}
        <section className="rounded-lg border border-gray-200 dark:border-slate-700 p-4">
          <h3 className={sectionHeader}>Profile</h3>
          <div className="mt-3 space-y-3">
            <div>
              <label className={labelCls}>Full Name</label>
              <input value={profileName} onChange={e => setProfileName(e.target.value)} className={inputCls} />
            </div>
            <div>
              <label className={labelCls}>Email</label>
              <input type="email" value={profileEmail} onChange={e => setProfileEmail(e.target.value)} className={inputCls} />
            </div>
            {profileError && <p className="text-xs text-red-600">{profileError}</p>}
            {profileMsg && <p className="text-xs text-emerald-600">{profileMsg}</p>}
            <button
              onClick={() => { setProfileError(''); updateProfileMut.mutate() }}
              disabled={updateProfileMut.isPending || (!profileName.trim() && !profileEmail.trim())}
              className={btnPrimary}
            >
              {updateProfileMut.isPending ? 'Saving...' : 'Save Profile'}
            </button>
          </div>
        </section>

        {/* ── 2. Account Assignment ── */}
        <section className="rounded-lg border border-gray-200 dark:border-slate-700 p-4">
          <h3 className={sectionHeader}>Account Assignment</h3>
          <p className="mt-1 text-xs text-gray-500 dark:text-slate-400">
            Current account: <span className="font-medium text-gray-700 dark:text-slate-300">{user.account_name || 'Unknown'}</span>
          </p>
          <div className="mt-3 space-y-3">
            <div>
              <label className={labelCls}>Account</label>
              <select
                value={selectedAccountId}
                onChange={e => setSelectedAccountId(e.target.value)}
                className={inputCls}
              >
                {accounts.map(a => (
                  <option key={a.id} value={a.id}>{a.name} ({a.plan})</option>
                ))}
              </select>
            </div>
            {accountError && <p className="text-xs text-red-600">{accountError}</p>}
            {accountMsg && <p className="text-xs text-emerald-600">{accountMsg}</p>}
            <button
              onClick={() => { setAccountError(''); updateAccountMut.mutate() }}
              disabled={updateAccountMut.isPending || selectedAccountId === user.account_id}
              className={btnPrimary}
            >
              {updateAccountMut.isPending ? 'Saving...' : 'Save Account'}
            </button>
          </div>
        </section>

        {/* ── 3. Role Assignment ── */}
        <section className="rounded-lg border border-gray-200 dark:border-slate-700 p-4">
          <h3 className={sectionHeader}>Role Assignment</h3>
          <div className="mt-3 space-y-3">
            <div>
              <label className={labelCls}>Role</label>
              <select
                value={selectedRole}
                onChange={e => setSelectedRole(e.target.value)}
                className={inputCls}
              >
                {currentUser?.role === 'root' && <option value="root">Root</option>}
                <option value="admin">Admin</option>
                <option value="analyst">Analyst</option>
                <option value="viewer">Viewer</option>
              </select>
            </div>
            {roleError && <p className="text-xs text-red-600">{roleError}</p>}
            {roleMsg && <p className="text-xs text-emerald-600">{roleMsg}</p>}
            <button
              onClick={() => { setRoleError(''); updateRoleMut.mutate() }}
              disabled={updateRoleMut.isPending || selectedRole === user.role}
              className={btnPrimary}
            >
              {updateRoleMut.isPending ? 'Saving...' : 'Save Role'}
            </button>
          </div>
        </section>

        {/* ── 4. Organization Access ── */}
        <section className="rounded-lg border border-gray-200 dark:border-slate-700 p-4">
          <h3 className={sectionHeader}>Organization Access</h3>
          <p className="mt-1 text-xs text-gray-500 dark:text-slate-400">
            Controls which organizations this user can view data for within their account.
          </p>
          <div className="mt-3 space-y-3">
            {/* All orgs toggle */}
            <label className="flex items-center gap-3 cursor-pointer">
              <button
                type="button"
                onClick={() => setAllOrgsAccess(!allOrgsAccess)}
                className={
                  'relative inline-flex h-5 w-9 shrink-0 rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ' +
                  (allOrgsAccess ? 'bg-emerald-500' : 'bg-gray-200 dark:bg-slate-600')
                }
                role="switch"
                aria-checked={allOrgsAccess}
              >
                <span className={
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ' +
                  (allOrgsAccess ? 'translate-x-4' : 'translate-x-0')
                } />
              </button>
              <span className="text-sm text-gray-700 dark:text-slate-300">All organizations</span>
            </label>

            {/* Org checkboxes (shown when not "all orgs") */}
            {!allOrgsAccess && (
              <div className="rounded-lg border border-gray-200 dark:border-slate-600 bg-gray-50 dark:bg-slate-700/50 p-3 max-h-48 overflow-y-auto space-y-2">
                {accountOrgs.length === 0 && (
                  <p className="text-xs text-gray-400 dark:text-slate-500">No organizations found in this account.</p>
                )}
                {accountOrgs.map(org => (
                  <label key={org.id} className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={selectedOrgIds.includes(org.id)}
                      onChange={() => toggleOrgId(org.id)}
                      className="h-4 w-4 rounded border-gray-300 dark:border-slate-600 text-fibratus-600 focus:ring-fibratus-500"
                    />
                    <span className="text-sm text-gray-700 dark:text-slate-300">{org.name}</span>
                    <span className="text-[10px] text-gray-400 dark:text-slate-500">({org.agent_count} agents)</span>
                  </label>
                ))}
              </div>
            )}

            {/* Current restrictions display */}
            {currentRestrictions.length > 0 && (
              <div className="text-xs text-gray-500 dark:text-slate-400">
                Currently restricted to {currentRestrictions.length} org(s):
                <div className="mt-1 flex flex-wrap gap-1">
                  {currentRestrictions.map(id => {
                    const org = accountOrgs.find(o => o.id === id)
                    return (
                      <span key={id} className="inline-flex rounded-full bg-gray-100 dark:bg-slate-700 px-2 py-0.5 text-[10px] text-gray-600 dark:text-slate-400">
                        {org ? org.name : id.slice(0, 8) + '...'}
                      </span>
                    )
                  })}
                </div>
              </div>
            )}

            {orgError && <p className="text-xs text-red-600">{orgError}</p>}
            {orgMsg && <p className="text-xs text-emerald-600">{orgMsg}</p>}
            <button
              onClick={() => { setOrgError(''); updateOrgAccessMut.mutate() }}
              disabled={updateOrgAccessMut.isPending}
              className={btnPrimary}
            >
              {updateOrgAccessMut.isPending ? 'Saving...' : 'Save Org Access'}
            </button>
          </div>
        </section>

        {/* ── 5. Security ── */}
        <section className="rounded-lg border border-gray-200 dark:border-slate-700 p-4">
          <h3 className={sectionHeader}>Security</h3>
          <div className="mt-3 space-y-4">

            {/* 2FA Status */}
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <span className="text-sm text-gray-600 dark:text-slate-400">2FA Status:</span>
                <span className={'inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium ' +
                  (user.totp_enabled ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400' : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')}>
                  {user.totp_enabled ? 'Enabled' : 'Disabled'}
                </span>
              </div>
              {user.totp_enabled && !totpConfirm && (
                <button
                  onClick={() => setTotpConfirm(true)}
                  className="rounded-lg border border-red-300 dark:border-red-800 px-3 py-1.5 text-xs font-medium text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20"
                >
                  Disable 2FA
                </button>
              )}
            </div>
            {user.totp_enabled && totpConfirm && (
              <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20 p-3 space-y-2">
                <p className="text-xs text-red-800 dark:text-red-300">
                  This will remove the user's TOTP configuration. They will need to re-enroll.
                </p>
                {totpError && <p className="text-xs text-red-600">{totpError}</p>}
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => { setTotpError(''); disableTotpMut.mutate() }}
                    disabled={disableTotpMut.isPending}
                    className="rounded-lg bg-red-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-700 disabled:opacity-50"
                  >
                    {disableTotpMut.isPending ? 'Disabling...' : 'Confirm Disable'}
                  </button>
                  <button
                    onClick={() => setTotpConfirm(false)}
                    className="rounded-lg border border-gray-300 dark:border-slate-600 px-3 py-1.5 text-xs text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}
            {totpMsg && <p className="text-xs text-emerald-600">{totpMsg}</p>}

            <hr className="border-gray-200 dark:border-slate-700" />

            {/* Lock Status */}
            <div className="flex items-center justify-between">
              <div>
                <span className="text-sm text-gray-600 dark:text-slate-400">Account Status: </span>
                {user.is_locked ? (
                  <span className="inline-flex rounded-full bg-red-50 dark:bg-red-900/30 px-2.5 py-0.5 text-xs font-medium text-red-700 dark:text-red-400">
                    Locked{user.login_attempts ? ` (${user.login_attempts} failed attempts)` : ''}
                  </span>
                ) : (
                  <span className="inline-flex rounded-full bg-emerald-50 dark:bg-emerald-900/30 px-2.5 py-0.5 text-xs font-medium text-emerald-700 dark:text-emerald-400">
                    Active
                  </span>
                )}
              </div>
              {user.is_locked && (
                <button
                  onClick={() => unlockMut.mutate()}
                  disabled={unlockMut.isPending}
                  className="rounded-lg bg-amber-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-amber-700 disabled:opacity-50"
                >
                  {unlockMut.isPending ? 'Unlocking...' : 'Unlock Account'}
                </button>
              )}
            </div>

            <hr className="border-gray-200 dark:border-slate-700" />

            {/* Password Reset */}
            <div>
              <h4 className="text-xs font-semibold text-gray-700 dark:text-slate-300">Reset Password</h4>
              <p className="mt-1 text-[10px] text-gray-500 dark:text-slate-400">User will need the new password on next login.</p>
              <div className="mt-2 space-y-2">
                <input
                  type="password"
                  value={newPassword}
                  onChange={e => setNewPassword(e.target.value)}
                  className={inputCls}
                  placeholder="Enter new password"
                  minLength={12}
                />
                <div className="grid grid-cols-2 gap-x-4 gap-y-0.5">
                  {passwordRules.map(rule => (
                    <p key={rule.label} className={'text-[10px] ' + (rule.test(newPassword) ? 'text-emerald-600' : 'text-gray-400')}>
                      {rule.test(newPassword) ? '\u2713' : '\u2022'} {rule.label}
                    </p>
                  ))}
                </div>
                {passwordError && <p className="text-xs text-red-600">{passwordError}</p>}
                {passwordMsg && <p className="text-xs text-emerald-600">{passwordMsg}</p>}
                <button
                  onClick={() => { setPasswordError(''); resetPasswordMut.mutate() }}
                  disabled={!allPasswordRulesPass || resetPasswordMut.isPending}
                  className={btnPrimary}
                >
                  {resetPasswordMut.isPending ? 'Resetting...' : 'Reset Password'}
                </button>
              </div>
            </div>
          </div>
        </section>

        {/* ── 6. Danger Zone ── */}
        <section className="rounded-lg border border-red-200 dark:border-red-900/50 p-4">
          <h3 className="text-sm font-semibold text-red-600">Danger Zone</h3>
          <p className="mt-1 text-xs text-gray-500 dark:text-slate-400">Permanently delete this user. This action cannot be undone.</p>
          <div className="mt-3">
            {!deleteConfirm ? (
              <button
                onClick={() => setDeleteConfirm(true)}
                className="rounded-lg border border-red-300 dark:border-red-800 bg-white dark:bg-slate-800 px-4 py-2 text-sm font-medium text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20"
              >
                Delete User
              </button>
            ) : (
              <div className="rounded-lg border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-900/20 p-3 space-y-2">
                <p className="text-sm text-red-800 dark:text-red-300">
                  Permanently delete <strong>{user.name || user.email}</strong>?
                </p>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => deleteMut.mutate()}
                    disabled={deleteMut.isPending}
                    className="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
                  >
                    {deleteMut.isPending ? 'Deleting...' : 'Yes, Delete'}
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
        </section>

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

  const { sorted: sortedOrgs, sort: orgSort, toggleSort: toggleOrgSort } = useTableSort<Organization & { account_name?: string }>(filtered, 'name', 'asc')

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
                <SortableHeader label="Name" sortKey="name" sort={orgSort} onSort={toggleOrgSort} />
                <SortableHeader label="Account" sortKey="account_name" sort={orgSort} onSort={toggleOrgSort} />
                <SortableHeader label="Agents" sortKey="agent_count" sort={orgSort} onSort={toggleOrgSort} />
                <SortableHeader label="Slug" sortKey="slug" sort={orgSort} onSort={toggleOrgSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {isLoading && (
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">Loading...</td></tr>
              )}
              {!isLoading && sortedOrgs.map(org => (
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

// ═══════════════════════════════════════════════════
// Groups Tab — Reuse the Groups page component
// ═══════════════════════════════════════════════════

function GroupsTab() {
  return <Groups />
}

// ═══════════════════════════════════════════════════
// System Tab — Server & database management
// ═══════════════════════════════════════════════════

function SystemTab() {
  const [dbType, setDbType] = useState<'postgres' | 'clickhouse'>('postgres')
  const [query, setQuery] = useState('')
  const [result, setResult] = useState<{ columns: string[]; rows: unknown[][]; affected_rows?: number; error?: string } | null>(null)
  const [loading, setLoading] = useState(false)
  const [activeTable, setActiveTable] = useState<string | null>(null)
  const [tableColumns, setTableColumns] = useState<{ columns: string[]; rows: unknown[][] } | null>(null)
  const [tableData, setTableData] = useState<{ columns: string[]; rows: unknown[][] } | null>(null)
  const [tableLoading, setTableLoading] = useState(false)
  const [editCell, setEditCell] = useState<{ row: number; col: number; value: string } | null>(null)
  const [editSaving, setEditSaving] = useState(false)
  const [browseView, setBrowseView] = useState<'tables' | 'browse' | 'query'>('tables')
  const [browseOffset, setBrowseOffset] = useState(0)
  const [selectedRows, setSelectedRows] = useState<Set<number>>(new Set())
  const [deleting, setDeleting] = useState(false)
  const browseLimit = 50

  const { data: pgTables } = useQuery({ queryKey: ['pg-tables'], queryFn: () => api.dbTablesPostgres(), staleTime: 30000 })
  const { data: chTables } = useQuery({ queryKey: ['ch-tables'], queryFn: () => api.dbTablesClickhouse(), staleTime: 30000 })
  const { data: settingsData } = useQuery({ queryKey: ['account-settings'], queryFn: () => api.getAccountSettings() })
  const settings = (settingsData as { data?: { telemetry_retention_days?: number } })?.data
  const [retentionDays, setRetentionDays] = useState(0)
  const [retentionSaving, setRetentionSaving] = useState(false)
  useEffect(() => {
    if (settings?.telemetry_retention_days && retentionDays === 0) setRetentionDays(settings.telemetry_retention_days)
  }, [settings]) // eslint-disable-line

  const executeQuery = async () => {
    if (!query.trim()) return
    setLoading(true)
    setResult(null)
    try {
      const res = dbType === 'postgres' ? await api.dbQueryPostgres(query) : await api.dbQueryClickhouse(query)
      if (res.data) setResult(res.data as typeof result)
      if (res.error) setResult({ columns: [], rows: [], error: res.error.message })
    } catch (err) {
      setResult({ columns: [], rows: [], error: String(err) })
    }
    setLoading(false)
  }

  const openTable = async (tableName: string) => {
    setActiveTable(tableName)
    setBrowseView('browse')
    setBrowseOffset(0)
    setTableLoading(true)
    setEditCell(null)
    setSelectedRows(new Set())
    try {
      // Fetch columns/schema
      const schemaQuery = dbType === 'postgres'
        ? `SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_schema = 'public' AND table_name = '${tableName}' ORDER BY ordinal_position`
        : `SELECT name AS column_name, type AS data_type, default_expression AS column_default FROM system.columns WHERE database = 'fibratus' AND table = '${tableName}' ORDER BY position`
      const schemaRes = dbType === 'postgres' ? await api.dbQueryPostgres(schemaQuery) : await api.dbQueryClickhouse(schemaQuery)
      if (schemaRes.data) setTableColumns(schemaRes.data as typeof tableColumns)

      // Fetch data
      const dataQuery = dbType === 'postgres'
        ? `SELECT * FROM "${tableName}" LIMIT ${browseLimit}`
        : `SELECT * FROM ${tableName} LIMIT ${browseLimit}`
      const dataRes = dbType === 'postgres' ? await api.dbQueryPostgres(dataQuery) : await api.dbQueryClickhouse(dataQuery)
      if (dataRes.data) setTableData(dataRes.data as typeof tableData)
    } catch { /* ignore */ }
    setTableLoading(false)
  }

  const loadPage = async (offset: number) => {
    if (!activeTable) return
    setTableLoading(true)
    setBrowseOffset(offset)
    setEditCell(null)
    setSelectedRows(new Set())
    try {
      const q = dbType === 'postgres'
        ? `SELECT * FROM "${activeTable}" LIMIT ${browseLimit} OFFSET ${offset}`
        : `SELECT * FROM ${activeTable} LIMIT ${browseLimit} OFFSET ${offset}`
      const res = dbType === 'postgres' ? await api.dbQueryPostgres(q) : await api.dbQueryClickhouse(q)
      if (res.data) setTableData(res.data as typeof tableData)
    } catch { /* ignore */ }
    setTableLoading(false)
  }

  const saveCell = async () => {
    if (!editCell || !activeTable || !tableData) return
    setEditSaving(true)
    const col = tableData.columns[editCell.col]
    const pkCol = tableData.columns[0] // assume first column is the PK
    const pkVal = tableData.rows[editCell.row][0]
    const escaped = editCell.value.replace(/'/g, "''")
    const pkEscaped = String(pkVal).replace(/'/g, "''")
    const updateQuery = `UPDATE "${activeTable}" SET "${col}" = '${escaped}' WHERE "${pkCol}" = '${pkEscaped}'`
    try {
      const res = await api.dbQueryPostgres(updateQuery)
      if (res.data && !(res.data as { error?: string }).error) {
        // Update local state
        const newRows = [...tableData.rows]
        newRows[editCell.row] = [...newRows[editCell.row]]
        newRows[editCell.row][editCell.col] = editCell.value
        setTableData({ ...tableData, rows: newRows })
        setEditCell(null)
      }
    } catch { /* ignore */ }
    setEditSaving(false)
  }

  const deleteSelected = async () => {
    if (!activeTable || !tableData || selectedRows.size === 0 || dbType !== 'postgres') return
    const pkCol = tableData.columns[0]
    const ids = Array.from(selectedRows).map(ri => {
      const val = String(tableData.rows[ri][0]).replace(/'/g, "''")
      return `'${val}'`
    })
    setDeleting(true)
    try {
      await api.dbQueryPostgres(`DELETE FROM "${activeTable}" WHERE "${pkCol}" IN (${ids.join(',')})`)
      setSelectedRows(new Set())
      await openTable(activeTable)
    } catch { /* ignore */ }
    setDeleting(false)
  }

  const deleteAllRows = async () => {
    if (!activeTable) return
    if (!confirm(`Delete ALL rows from "${activeTable}"? This cannot be undone.`)) return
    setDeleting(true)
    try {
      const q = dbType === 'postgres'
        ? `DELETE FROM "${activeTable}"`
        : `TRUNCATE TABLE ${activeTable}`
      const fn = dbType === 'postgres' ? api.dbQueryPostgres : api.dbQueryClickhouse
      await fn(q)
      setSelectedRows(new Set())
      await openTable(activeTable)
    } catch { /* ignore */ }
    setDeleting(false)
  }

  const tables = dbType === 'postgres' ? pgTables?.data : chTables?.data
  const tablesArr = tables as { columns?: string[]; rows?: unknown[][] } | undefined

  return (
    <div className="space-y-4">
      {/* Server Info */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
        <div className="grid grid-cols-4 gap-4 text-sm">
          <div><span className="text-gray-500 dark:text-slate-400 text-xs">Server</span><p className="font-mono text-gray-900 dark:text-slate-100 text-xs">{window.location.origin}</p></div>
          <div><span className="text-gray-500 dark:text-slate-400 text-xs">Version</span><p className="font-mono text-gray-900 dark:text-slate-100 text-xs">Fleet v1.0</p></div>
          <div>
            <span className="text-gray-500 dark:text-slate-400 text-xs">ClickHouse Telemetry Retention</span>
            <div className="flex items-center gap-2 mt-1">
              <select
                value={retentionDays || 7}
                onChange={e => setRetentionDays(Number(e.target.value))}
                className="bg-white dark:bg-slate-700 border border-gray-300 dark:border-slate-600 rounded px-2 py-0.5 text-xs font-mono text-gray-900 dark:text-slate-100"
              >
                <option value={1}>1 day</option>
                <option value={3}>3 days</option>
                <option value={7}>7 days</option>
                <option value={14}>14 days</option>
                <option value={30}>30 days</option>
                <option value={90}>90 days</option>
                <option value={180}>180 days</option>
                <option value={365}>365 days</option>
              </select>
              {retentionDays > 0 && retentionDays !== (settings?.telemetry_retention_days || 7) && (
                <button
                  disabled={retentionSaving}
                  onClick={async () => {
                    setRetentionSaving(true)
                    try { await api.updateTelemetryRetention(retentionDays) } catch {}
                    setRetentionSaving(false)
                  }}
                  className="rounded bg-blue-600 px-2 py-0.5 text-[10px] font-medium text-white hover:bg-blue-700 disabled:opacity-50"
                >
                  {retentionSaving ? 'Saving...' : 'Save'}
                </button>
              )}
            </div>
          </div>
          <div><span className="text-gray-500 dark:text-slate-400 text-xs">Environment</span><p className="font-mono text-gray-900 dark:text-slate-100 text-xs">Production</p></div>
        </div>
      </div>

      {/* DB Toggle + View Toggle */}
      <div className="flex items-center gap-3">
        <div className="flex items-center gap-1 rounded-lg bg-gray-100 dark:bg-slate-700 p-0.5">
          <button onClick={() => { setDbType('postgres'); setResult(null); setActiveTable(null); setBrowseView('tables') }}
            className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${dbType === 'postgres' ? 'bg-blue-600 text-white shadow-sm' : 'text-gray-600 dark:text-slate-400 hover:text-gray-800 dark:hover:text-slate-200'}`}>
            PostgreSQL
          </button>
          <button onClick={() => { setDbType('clickhouse'); setResult(null); setActiveTable(null); setBrowseView('tables') }}
            className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${dbType === 'clickhouse' ? 'bg-amber-600 text-white shadow-sm' : 'text-gray-600 dark:text-slate-400 hover:text-gray-800 dark:hover:text-slate-200'}`}>
            ClickHouse
          </button>
        </div>
        <div className="h-5 w-px bg-gray-200 dark:bg-slate-600" />
        <div className="flex items-center gap-1 rounded-lg bg-gray-100 dark:bg-slate-700 p-0.5">
          <button onClick={() => { setBrowseView('tables'); setActiveTable(null) }}
            className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${browseView === 'tables' ? 'bg-white dark:bg-slate-600 text-gray-900 dark:text-slate-100 shadow-sm' : 'text-gray-500 dark:text-slate-400'}`}>
            Browse
          </button>
          <button onClick={() => setBrowseView('query')}
            className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${browseView === 'query' ? 'bg-white dark:bg-slate-600 text-gray-900 dark:text-slate-100 shadow-sm' : 'text-gray-500 dark:text-slate-400'}`}>
            Query
          </button>
        </div>
        {activeTable && browseView === 'browse' && (
          <>
            <div className="h-5 w-px bg-gray-200 dark:bg-slate-600" />
            <button onClick={() => { setActiveTable(null); setBrowseView('tables') }}
              className="text-xs text-fibratus-600 hover:underline flex items-center gap-1">
              <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" /></svg>
              All Tables
            </button>
            <span className="text-xs font-mono font-medium text-gray-900 dark:text-slate-100">{activeTable}</span>
          </>
        )}
      </div>

      {/* ═════ Tables List View ═════ */}
      {browseView === 'tables' && tablesArr?.rows && tablesArr.rows.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {tablesArr.rows.map((row, i) => {
            const name = String(row[0])
            const size = row[1] != null ? String(row[1]) : ''
            const extra = row[2] != null ? String(row[2]) : ''
            const extra2 = row[3] != null ? String(row[3]) : ''
            return (
              <button key={i} onClick={() => openTable(name)}
                className="text-left rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4 hover:border-blue-300 dark:hover:border-blue-600 hover:shadow-md transition-all group">
                <div className="flex items-center gap-2">
                  <svg className="w-4 h-4 text-gray-400 dark:text-slate-500 group-hover:text-blue-500 transition-colors" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
                  </svg>
                  <span className="font-mono text-sm font-medium text-gray-900 dark:text-slate-100 group-hover:text-blue-600 dark:group-hover:text-blue-400 transition-colors">{name}</span>
                </div>
                <div className="mt-2 flex items-center gap-3 text-[10px] text-gray-400 dark:text-slate-500">
                  {size && <span>{size}</span>}
                  {extra && <span>{tablesArr.columns?.[2]}: {extra}</span>}
                  {extra2 && <span>{tablesArr.columns?.[3]}: {extra2}</span>}
                </div>
              </button>
            )
          })}
        </div>
      )}
      {browseView === 'tables' && (!tablesArr?.rows || tablesArr.rows.length === 0) && (
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-12 text-center text-gray-400 dark:text-slate-500 text-sm">
          No tables found
        </div>
      )}

      {/* ═════ Table Browse View ═════ */}
      {browseView === 'browse' && activeTable && (
        <div className="space-y-4">
          {/* Schema */}
          {tableColumns && tableColumns.rows && tableColumns.rows.length > 0 && (
            <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 overflow-hidden">
              <div className="px-4 py-2 border-b border-gray-100 dark:border-slate-700 bg-gray-50 dark:bg-slate-900/40 flex items-center justify-between">
                <span className="text-xs font-semibold text-gray-500 dark:text-slate-400 uppercase">Schema — {tableColumns.rows.length} columns</span>
                <button onClick={() => openTable(activeTable)} className="text-[10px] text-fibratus-600 hover:underline">Refresh</button>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full text-xs">
                  <thead className="bg-gray-50/50 dark:bg-slate-900/30">
                    <tr>
                      {tableColumns.columns.map(c => (
                        <th key={c} className="px-3 py-1.5 text-left font-medium text-gray-500 dark:text-slate-400 whitespace-nowrap">{c}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-100 dark:divide-slate-700/50">
                    {tableColumns.rows.map((row, i) => (
                      <tr key={i} className="hover:bg-blue-50/30 dark:hover:bg-slate-800/30">
                        {(row as unknown[]).map((cell, j) => (
                          <td key={j} className="px-3 py-1 font-mono text-gray-700 dark:text-slate-300 whitespace-nowrap">
                            {j === 0 ? <span className="text-blue-600 dark:text-blue-400 font-medium">{String(cell ?? '')}</span>
                              : j === 1 ? <span className="text-amber-600 dark:text-amber-400">{String(cell ?? '')}</span>
                              : <span>{cell === null ? <span className="text-gray-300 dark:text-slate-600 italic">NULL</span> : String(cell)}</span>}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* Data */}
          {tableLoading ? (
            <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-12 text-center text-gray-400 dark:text-slate-500 text-sm">Loading...</div>
          ) : tableData && tableData.columns && tableData.columns.length > 0 ? (
            <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 overflow-hidden">
              <div className="px-4 py-2 border-b border-gray-100 dark:border-slate-700 bg-gray-50 dark:bg-slate-900/40 flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <span className="text-xs font-semibold text-gray-500 dark:text-slate-400">
                    Rows {browseOffset + 1}–{browseOffset + (tableData.rows?.length || 0)}
                    {dbType === 'postgres' && <span className="ml-1 text-[10px] text-gray-400 dark:text-slate-500">(click cell to edit)</span>}
                  </span>
                  {selectedRows.size > 0 && dbType === 'postgres' && (
                    <button onClick={deleteSelected} disabled={deleting}
                      className="rounded bg-red-600 px-2 py-0.5 text-[10px] font-medium text-white hover:bg-red-700 disabled:opacity-50">
                      {deleting ? 'Deleting...' : `Delete ${selectedRows.size} selected`}
                    </button>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  {dbType === 'postgres' && (
                    <button onClick={deleteAllRows} disabled={deleting}
                      className="rounded border border-red-300 dark:border-red-700 px-2 py-0.5 text-[10px] text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-30">
                      Delete All
                    </button>
                  )}
                  <button disabled={browseOffset === 0} onClick={() => loadPage(Math.max(0, browseOffset - browseLimit))}
                    className="rounded border border-gray-300 dark:border-slate-600 px-2 py-0.5 text-[10px] text-gray-600 dark:text-slate-400 disabled:opacity-30 hover:bg-gray-50 dark:hover:bg-slate-700">Prev</button>
                  <button disabled={(tableData.rows?.length || 0) < browseLimit} onClick={() => loadPage(browseOffset + browseLimit)}
                    className="rounded border border-gray-300 dark:border-slate-600 px-2 py-0.5 text-[10px] text-gray-600 dark:text-slate-400 disabled:opacity-30 hover:bg-gray-50 dark:hover:bg-slate-700">Next</button>
                </div>
              </div>
              <div className="overflow-x-auto max-h-[600px] overflow-y-auto">
                <table className="w-full text-xs">
                  <thead className="bg-gray-50 dark:bg-slate-900/50 sticky top-0 z-10">
                    <tr>
                      {dbType === 'postgres' && (
                        <th className="px-2 py-2 w-8">
                          <input type="checkbox"
                            checked={tableData.rows?.length > 0 && selectedRows.size === tableData.rows.length}
                            onChange={e => {
                              if (e.target.checked) setSelectedRows(new Set(tableData.rows.map((_, i) => i)))
                              else setSelectedRows(new Set())
                            }}
                            className="rounded border-gray-300 dark:border-slate-600 text-red-600 focus:ring-red-500" />
                        </th>
                      )}
                      <th className="px-2 py-2 text-left font-medium text-gray-400 dark:text-slate-500 w-8">#</th>
                      {tableData.columns.map(c => (
                        <th key={c} className="px-3 py-2 text-left font-medium text-gray-500 dark:text-slate-400 whitespace-nowrap">{c}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-100 dark:divide-slate-700/50">
                    {tableData.rows?.map((row, ri) => (
                      <tr key={ri} className={'hover:bg-blue-50/30 dark:hover:bg-slate-800/20 group' + (selectedRows.has(ri) ? ' bg-red-50/50 dark:bg-red-900/10' : '')}>
                        {dbType === 'postgres' && (
                          <td className="px-2 py-1">
                            <input type="checkbox" checked={selectedRows.has(ri)}
                              onChange={e => {
                                const next = new Set(selectedRows)
                                if (e.target.checked) next.add(ri); else next.delete(ri)
                                setSelectedRows(next)
                              }}
                              className="rounded border-gray-300 dark:border-slate-600 text-red-600 focus:ring-red-500" />
                          </td>
                        )}
                        <td className="px-2 py-1 text-[10px] text-gray-300 dark:text-slate-600 tabular-nums">{browseOffset + ri + 1}</td>
                        {(row as unknown[]).map((cell, ci) => {
                          const isEditing = editCell?.row === ri && editCell?.col === ci
                          return (
                            <td key={ci}
                              className={'px-3 py-1 font-mono text-gray-700 dark:text-slate-300 max-w-[300px] ' +
                                (dbType === 'postgres' ? 'cursor-pointer hover:bg-blue-100/40 dark:hover:bg-blue-900/20' : '') +
                                (ci === 0 ? ' text-blue-600 dark:text-blue-400 font-medium' : '')}
                              onClick={() => {
                                if (dbType === 'postgres' && !isEditing) {
                                  setEditCell({ row: ri, col: ci, value: cell === null ? '' : String(cell) })
                                }
                              }}
                              title={cell === null ? 'NULL' : String(cell)}>
                              {isEditing ? (
                                <div className="flex items-center gap-1" onClick={e => e.stopPropagation()}>
                                  <input
                                    autoFocus
                                    value={editCell.value}
                                    onChange={e => setEditCell({ ...editCell, value: e.target.value })}
                                    onKeyDown={e => { if (e.key === 'Enter') saveCell(); if (e.key === 'Escape') setEditCell(null) }}
                                    className="w-full rounded border border-blue-400 dark:border-blue-600 bg-white dark:bg-slate-700 px-1.5 py-0.5 text-xs text-gray-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-blue-500"
                                  />
                                  <button onClick={saveCell} disabled={editSaving}
                                    className="rounded bg-blue-600 px-1.5 py-0.5 text-[9px] text-white hover:bg-blue-700 flex-shrink-0">
                                    {editSaving ? '...' : 'Save'}
                                  </button>
                                  <button onClick={() => setEditCell(null)}
                                    className="rounded bg-gray-200 dark:bg-slate-600 px-1.5 py-0.5 text-[9px] text-gray-600 dark:text-slate-300 flex-shrink-0">
                                    Esc
                                  </button>
                                </div>
                              ) : (
                                <span className="truncate block">
                                  {cell === null ? <span className="text-gray-300 dark:text-slate-600 italic">NULL</span> : String(cell)}
                                </span>
                              )}
                            </td>
                          )
                        })}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ) : (
            <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-12 text-center text-gray-400 dark:text-slate-500 text-sm">
              No data in this table
            </div>
          )}
        </div>
      )}

      {/* ═════ Query View ═════ */}
      {browseView === 'query' && (
        <>
          {/* Quick table buttons */}
          {tablesArr?.rows && tablesArr.rows.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {tablesArr.rows.map((row, i) => (
                <button key={i} onClick={() => setQuery(`SELECT * FROM ${dbType === 'postgres' ? `"${row[0]}"` : row[0]} LIMIT 100`)}
                  className="rounded-md border border-gray-200 dark:border-slate-600 bg-white dark:bg-slate-800 px-2 py-1 text-[10px] font-mono text-gray-600 dark:text-slate-400 hover:border-blue-300 dark:hover:border-blue-600 hover:text-blue-600 dark:hover:text-blue-400 transition-colors">
                  {String(row[0])}
                </button>
              ))}
            </div>
          )}

          <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4 space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold text-gray-500 dark:text-slate-400 uppercase">SQL Query — {dbType === 'postgres' ? 'PostgreSQL' : 'ClickHouse'}</span>
              <span className="text-[10px] text-red-500 dark:text-red-400">Root access only. Use with caution.</span>
            </div>
            <textarea
              value={query}
              onChange={e => setQuery(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) executeQuery() }}
              placeholder={dbType === 'postgres'
                ? 'SELECT * FROM users LIMIT 10;\n\n-- Ctrl+Enter to execute'
                : 'SELECT count() FROM telemetry_events;\n\n-- Ctrl+Enter to execute'}
              className="w-full h-32 rounded-lg border border-gray-300 dark:border-slate-600 bg-gray-50 dark:bg-black px-4 py-3 font-mono text-sm text-gray-900 dark:text-cyan-400 placeholder-gray-400 dark:placeholder-slate-600 focus:border-blue-500 dark:focus:border-cyan-500 focus:outline-none resize-y"
            />
            <div className="flex items-center gap-2">
              <button onClick={executeQuery} disabled={loading || !query.trim()}
                className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50">
                {loading ? 'Executing...' : 'Execute (Ctrl+Enter)'}
              </button>
              <button onClick={() => { setQuery(''); setResult(null) }}
                className="rounded-lg border border-gray-300 dark:border-slate-600 px-3 py-2 text-sm text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700">
                Clear
              </button>
            </div>
          </div>

          {result && (
            <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 overflow-hidden">
              <div className="px-4 py-2 border-b border-gray-100 dark:border-slate-700 bg-gray-50 dark:bg-slate-900/40 flex items-center justify-between">
                <span className="text-xs font-semibold text-gray-500 dark:text-slate-400">
                  {result.error ? 'Error' : `Results — ${result.rows?.length || 0} rows`}
                  {result.affected_rows !== undefined && !result.error && ` (${result.affected_rows} affected)`}
                </span>
              </div>
              {result.error ? (
                <div className="p-4 text-sm text-red-600 dark:text-red-400 font-mono break-all">{result.error}</div>
              ) : result.columns && result.columns.length > 0 ? (
                <div className="overflow-x-auto max-h-[500px] overflow-y-auto">
                  <table className="w-full text-xs">
                    <thead className="bg-gray-50 dark:bg-slate-900/50 sticky top-0">
                      <tr>{result.columns.map(c => <th key={c} className="px-3 py-2 text-left font-medium text-gray-500 dark:text-slate-400 whitespace-nowrap">{c}</th>)}</tr>
                    </thead>
                    <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
                      {result.rows?.map((row, i) => (
                        <tr key={i} className="hover:bg-blue-50/30 dark:hover:bg-slate-800/30">
                          {(row as unknown[]).map((cell, j) => (
                            <td key={j} className="px-3 py-1.5 font-mono text-gray-700 dark:text-slate-300 whitespace-nowrap max-w-[400px] truncate" title={String(cell ?? '')}>
                              {cell === null ? <span className="text-gray-300 dark:text-slate-600 italic">NULL</span> : String(cell)}
                            </td>
                          ))}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <div className="p-4 text-sm text-gray-500 dark:text-slate-400">No results</div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  )
}
