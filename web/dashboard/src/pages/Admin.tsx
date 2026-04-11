import { useState, useMemo, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Account, type Organization, type User } from '../lib/api'
import SlidePanel from '../components/SlidePanel'
import Groups from './Groups'
import DatabaseTab from '../components/management/DatabaseTab'
import { useTableSort } from '../hooks/useTableSort'
import SortableHeader from '../components/SortableHeader'

type Tab = 'accounts' | 'users' | 'organizations' | 'groups' | 'database'

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

const retentionOptions = [
  { value: 1, label: '1d' },
  { value: 3, label: '3d' },
  { value: 7, label: '7d' },
  { value: 14, label: '14d' },
  { value: 30, label: '30d' },
  { value: 90, label: '90d' },
  { value: 180, label: '180d' },
  { value: 365, label: '1yr' },
]

const orgRetentionOptions = [
  { value: 0, label: 'Account default' },
  ...retentionOptions,
]

const inputCls = 'w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none'
const btnPrimary = 'rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50'
const sectionHeader = 'text-sm font-semibold text-gray-900 dark:text-slate-100'
const labelCls = 'block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1'
const retentionSelectCls = 'bg-white dark:bg-slate-700 border border-gray-200 dark:border-slate-600 rounded px-2 py-0.5 text-xs text-gray-700 dark:text-slate-300 disabled:opacity-50'

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

function slugify(s: string): string {
  return s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
}

// ================================================================
// Main Component
// ================================================================

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
    { key: 'database', label: 'Database' },
  ]

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-slate-100">Super Admin</h1>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">Cross-account management — all accounts, organizations, users, and system databases</p>
        </div>
      </div>

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
      {tab === 'users' && <UsersTab currentUser={currentUser} />}
      {tab === 'organizations' && <OrganizationsTab filterAccountId={filterAccountId} onClearFilter={() => setFilterAccountId(null)} onAccountClick={handleAccountClick} />}
      {tab === 'groups' && <Groups />}
      {tab === 'database' && <DatabaseTab />}
    </div>
  )
}

// ================================================================
// Accounts Tab
// ================================================================

function AccountsTab({ onAccountClick }: { onAccountClick: (id: string) => void }) {
  const queryClient = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [createForm, setCreateForm] = useState({ name: '', plan: 'standard', email: '', password: '', user_name: '', org_name: '', user_role: 'member' })
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
    mutationFn: (data: Record<string, string>) => api.adminCreateAccount(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      setShowCreate(false)
      setCreateForm({ name: '', plan: 'standard', email: '', password: '', user_name: '', org_name: '', user_role: 'member' })
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

  const retentionMut = useMutation({
    mutationFn: (data: { accountId: string; days: number }) =>
      api.adminUpdateAccount(data.accountId, { telemetry_retention_days: data.days }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin-accounts'] }),
  })

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    if (!createForm.name.trim()) { setError('Account name is required'); return }
    createMut.mutate(createForm)
  }

  return (
    <div className="mt-6">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-4">
          <p className="text-sm text-gray-500 dark:text-slate-400">{filtered.length} account(s)</p>
          <input
            type="text"
            placeholder="Search by name or plan..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-1.5 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
          />
        </div>
        <button onClick={() => setShowCreate(true)} className={btnPrimary}>Create Account</button>
      </div>

      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
              <tr>
                <SortableHeader label="Name" sortKey="name" sort={accountSort} onSort={toggleAccountSort} />
                <SortableHeader label="Plan" sortKey="plan" sort={accountSort} onSort={toggleAccountSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Retention</th>
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
                    <button onClick={() => onAccountClick(acct.id)} className="font-medium text-fibratus-600 hover:underline">{acct.name}</button>
                  </td>
                  <td className="px-6 py-3">
                    <span className="inline-flex rounded-full bg-blue-50 dark:bg-blue-900/30 px-2.5 py-0.5 text-xs font-medium text-blue-700 dark:text-blue-400">{acct.plan}</span>
                  </td>
                  <td className="px-6 py-3">
                    <select
                      value={acct.telemetry_retention_days || 7}
                      onChange={e => retentionMut.mutate({ accountId: acct.id, days: Number(e.target.value) })}
                      disabled={retentionMut.isPending}
                      className={retentionSelectCls}
                    >
                      {retentionOptions.map(o => (
                        <option key={o.value} value={o.value}>{o.label}</option>
                      ))}
                    </select>
                  </td>
                  <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{acct.org_count}</td>
                  <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{acct.user_count}</td>
                  <td className="px-6 py-3 text-gray-500 dark:text-slate-400 whitespace-nowrap">{new Date(acct.created_at).toLocaleDateString()}</td>
                  <td className="px-6 py-3">
                    <div className="flex items-center gap-3">
                      <button onClick={() => setEditingAccount(acct)} className="text-xs text-fibratus-600 hover:underline">Edit</button>
                      {deleteId === acct.id ? (
                        <div className="flex items-center gap-2">
                          <button onClick={() => deleteMut.mutate(acct.id)} className="rounded bg-red-600 px-2 py-1 text-xs text-white hover:bg-red-700">Confirm</button>
                          <button onClick={() => setDeleteId(null)} className="rounded bg-gray-200 dark:bg-slate-600 px-2 py-1 text-xs text-gray-700 dark:text-slate-300 hover:bg-gray-300 dark:hover:bg-slate-500">Cancel</button>
                        </div>
                      ) : (
                        <button onClick={() => setDeleteId(acct.id)} className="text-xs text-red-600 hover:underline">Delete</button>
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
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelCls}>Account Name *</label>
                  <input value={createForm.name} onChange={e => setCreateForm(f => ({ ...f, name: e.target.value }))} className={inputCls} required />
                </div>
                <div>
                  <label className={labelCls}>Organization Name</label>
                  <input value={createForm.org_name} onChange={e => setCreateForm(f => ({ ...f, org_name: e.target.value }))} className={inputCls} placeholder={createForm.name || 'Same as account'} />
                </div>
              </div>
              <div>
                <label className={labelCls}>Plan</label>
                <select value={createForm.plan} onChange={e => setCreateForm(f => ({ ...f, plan: e.target.value }))} className={inputCls}>
                  <option value="trial">Trial</option>
                  <option value="standard">Standard</option>
                  <option value="professional">Professional</option>
                  <option value="enterprise">Enterprise</option>
                </select>
              </div>
              <hr className="border-gray-200 dark:border-slate-700" />
              <p className="text-xs font-medium text-gray-500 dark:text-slate-400">Initial Admin User</p>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelCls}>Email *</label>
                  <input type="email" value={createForm.email} onChange={e => setCreateForm(f => ({ ...f, email: e.target.value }))} className={inputCls} required />
                </div>
                <div>
                  <label className={labelCls}>Display Name</label>
                  <input value={createForm.user_name} onChange={e => setCreateForm(f => ({ ...f, user_name: e.target.value }))} className={inputCls} placeholder="Admin" />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelCls}>Password *</label>
                  <input type="password" value={createForm.password} onChange={e => setCreateForm(f => ({ ...f, password: e.target.value }))} className={inputCls} required minLength={8} />
                </div>
                <div>
                  <label className={labelCls}>Role</label>
                  <select value={createForm.user_role} onChange={e => setCreateForm(f => ({ ...f, user_role: e.target.value }))} className={inputCls}>
                    <option value="member">Member (standard)</option>
                    <option value="root">Root (super admin)</option>
                  </select>
                </div>
              </div>
              {error && <p className="text-xs text-red-600">{error}</p>}
              <div className="flex justify-end gap-2 pt-2">
                <button type="button" onClick={() => { setShowCreate(false); setError('') }} className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700">Cancel</button>
                <button type="submit" disabled={createMut.isPending} className={btnPrimary}>{createMut.isPending ? 'Creating...' : 'Create Account'}</button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Edit Account Modal */}
      {editingAccount && <EditAccountModal account={editingAccount} onClose={() => setEditingAccount(null)} />}
    </div>
  )
}

// ================================================================
// Edit Account Modal
// ================================================================

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

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-white dark:bg-slate-800 rounded-xl shadow-xl w-full max-w-md p-6" onClick={e => e.stopPropagation()}>
        <h2 className="text-lg font-bold text-gray-900 dark:text-slate-100">Edit Account</h2>
        <form onSubmit={e => { e.preventDefault(); if (name.trim()) updateMut.mutate() }} className="mt-4 space-y-3">
          <div>
            <label className={labelCls}>Account Name</label>
            <input value={name} onChange={e => setName(e.target.value)} className={inputCls} required />
          </div>
          <div>
            <label className={labelCls}>Plan</label>
            <select value={plan} onChange={e => setPlan(e.target.value)} className={inputCls}>
              <option value="trial">Trial</option>
              <option value="standard">Standard</option>
              <option value="professional">Professional</option>
              <option value="enterprise">Enterprise</option>
            </select>
          </div>
          {error && <p className="text-xs text-red-600">{error}</p>}
          {success && <p className="text-xs text-emerald-600">{success}</p>}
          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700">Cancel</button>
            <button type="submit" disabled={updateMut.isPending} className={btnPrimary}>{updateMut.isPending ? 'Saving...' : 'Save Changes'}</button>
          </div>
        </form>
      </div>
    </div>
  )
}

// ================================================================
// Users Tab (cross-account)
// ================================================================

function UsersTab({ currentUser }: { currentUser?: User }) {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [editingUser, setEditingUser] = useState<(User & { account_name?: string }) | null>(null)
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [createForm, setCreateForm] = useState({ email: '', name: '', password: '', role: 'member', account_id: '' })
  const [createError, setCreateError] = useState('')

  const { data: usersData, isLoading } = useQuery({
    queryKey: ['admin-users'],
    queryFn: () => api.adminGetAllUsers(),
    enabled: currentUser?.role === 'root',
  })
  const allUsers = (usersData?.data || []) as (User & { account_name?: string })[]

  const { data: accountsData } = useQuery({
    queryKey: ['admin-accounts'],
    queryFn: () => api.adminGetAccounts(),
    enabled: currentUser?.role === 'root',
  })
  const accounts = (accountsData?.data || []) as Account[]

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

  const createMut = useMutation({
    mutationFn: (data: typeof createForm) => api.adminCreateUser(data),
    onSuccess: (res) => {
      if (res.error) { setCreateError(res.error.message); return }
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
      setShowCreate(false)
      setCreateForm({ email: '', name: '', password: '', role: 'member', account_id: '' })
      setCreateError('')
    },
    onError: () => setCreateError('Failed to create user'),
  })

  return (
    <div className="mt-6">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-4">
          <p className="text-sm text-gray-500 dark:text-slate-400">{filtered.length} user(s) across all accounts</p>
          <input
            type="text"
            placeholder="Search by name, email, role, account..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-1.5 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none w-72"
          />
        </div>
        <button onClick={() => setShowCreate(true)} className={btnPrimary}>Create User</button>
      </div>

      {/* Create User Modal */}
      {showCreate && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowCreate(false)}>
          <div className="bg-white dark:bg-slate-800 rounded-xl shadow-xl w-full max-w-md p-6" onClick={e => e.stopPropagation()}>
            <h2 className="text-lg font-bold text-gray-900 dark:text-slate-100">Create User</h2>
            <form onSubmit={e => { e.preventDefault(); setCreateError(''); if (!createForm.account_id) { setCreateError('Select an account'); return } createMut.mutate(createForm) }} className="mt-4 space-y-3">
              <div>
                <label className={labelCls}>Account *</label>
                <select value={createForm.account_id} onChange={e => setCreateForm(f => ({ ...f, account_id: e.target.value }))} className={inputCls} required>
                  <option value="">Select account...</option>
                  {accounts.map(a => <option key={a.id} value={a.id}>{a.name}</option>)}
                </select>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelCls}>Email *</label>
                  <input type="email" value={createForm.email} onChange={e => setCreateForm(f => ({ ...f, email: e.target.value }))} className={inputCls} required />
                </div>
                <div>
                  <label className={labelCls}>Display Name</label>
                  <input value={createForm.name} onChange={e => setCreateForm(f => ({ ...f, name: e.target.value }))} className={inputCls} placeholder="Admin" />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelCls}>Password *</label>
                  <input type="password" value={createForm.password} onChange={e => setCreateForm(f => ({ ...f, password: e.target.value }))} className={inputCls} required minLength={8} />
                </div>
                <div>
                  <label className={labelCls}>Role</label>
                  <select value={createForm.role} onChange={e => setCreateForm(f => ({ ...f, role: e.target.value }))} className={inputCls}>
                    <option value="member">Member</option>
                    <option value="root">Root (super admin)</option>
                  </select>
                </div>
              </div>
              {createError && <p className="text-xs text-red-600">{createError}</p>}
              <div className="flex justify-end gap-2 pt-2">
                <button type="button" onClick={() => { setShowCreate(false); setCreateError('') }} className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700">Cancel</button>
                <button type="submit" disabled={createMut.isPending} className={btnPrimary}>{createMut.isPending ? 'Creating...' : 'Create User'}</button>
              </div>
            </form>
          </div>
        </div>
      )}

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
                      {isSelf && <span className="ml-2 text-xs text-gray-400">(you)</span>}
                    </td>
                    <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{user.email}</td>
                    <td className="px-6 py-3">
                      <span className={'inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium ' + (roleBadge[user.role] || roleBadge.viewer)}>{user.role}</span>
                    </td>
                    <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{user.account_name || '-'}</td>
                    <td className="px-6 py-3">
                      <span className={'rounded-full px-2 py-0.5 text-[10px] font-medium ' + (orgRestrictions.length === 0 ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400' : 'bg-gray-100 dark:bg-slate-700 text-gray-600 dark:text-slate-400')}>
                        {orgRestrictions.length === 0 ? 'All orgs' : `${orgRestrictions.length} org(s)`}
                      </span>
                    </td>
                    <td className="px-6 py-3">
                      <span className={'rounded-full px-2 py-0.5 text-[10px] font-medium ' + (user.totp_enabled ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400' : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')}>
                        {user.totp_enabled ? 'Enabled' : 'Off'}
                      </span>
                    </td>
                    <td className="px-6 py-3">
                      <span className={'rounded-full px-2 py-0.5 text-[10px] font-medium ' + (user.is_locked ? 'bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-400' : 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400')}>
                        {user.is_locked ? 'Locked' : 'Active'}
                      </span>
                    </td>
                    <td className="px-6 py-3 text-gray-500 dark:text-slate-400 whitespace-nowrap">{new Date(user.created_at).toLocaleDateString()}</td>
                    <td className="px-6 py-3">
                      <div className="flex items-center gap-2">
                        {!isSelf && (
                          <button onClick={() => setEditingUser(user)} className="text-xs text-fibratus-600 hover:underline">Edit</button>
                        )}
                        {!isSelf && (deleteId === user.id ? (
                          <div className="flex items-center gap-2">
                            <button onClick={() => deleteMut.mutate(user.id)} className="rounded bg-red-600 px-2 py-1 text-xs text-white hover:bg-red-700">Confirm</button>
                            <button onClick={() => setDeleteId(null)} className="rounded bg-gray-200 dark:bg-slate-600 px-2 py-1 text-xs text-gray-700 dark:text-slate-300">Cancel</button>
                          </div>
                        ) : (
                          <button onClick={() => setDeleteId(user.id)} className="text-xs text-red-600 hover:underline">Delete</button>
                        ))}
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

      {editingUser && <AdminUserEditPanel user={editingUser} onClose={() => setEditingUser(null)} />}
    </div>
  )
}

// ================================================================
// Admin User Edit Panel (SlidePanel)
// ================================================================

function AdminUserEditPanel({ user, onClose }: { user: User & { account_name?: string }; onClose: () => void }) {
  const queryClient = useQueryClient()
  const { data: currentUserData } = useQuery({ queryKey: ['current-user'], queryFn: () => api.getCurrentUser() })
  const currentUser = currentUserData?.data as User | undefined

  // Section 1: Profile
  const [profileName, setProfileName] = useState(user.name)
  const [profileEmail, setProfileEmail] = useState(user.email)
  const [profileMsg, setProfileMsg] = useState('')
  const [profileError, setProfileError] = useState('')

  // Section 2: Account Assignment
  const [selectedAccountId, setSelectedAccountId] = useState(user.account_id)
  const [accountMsg, setAccountMsg] = useState('')
  const [accountError, setAccountError] = useState('')

  // Section 3: Role
  const [selectedRole, setSelectedRole] = useState(user.role)
  const [roleMsg, setRoleMsg] = useState('')
  const [roleError, setRoleError] = useState('')

  // Section 4: Org Access
  const currentRestrictions = parseOrgRestrictions(user.org_restrictions)
  const [allOrgsAccess, setAllOrgsAccess] = useState(currentRestrictions.length === 0)
  const [selectedOrgIds, setSelectedOrgIds] = useState<string[]>(currentRestrictions)
  const [orgMsg, setOrgMsg] = useState('')
  const [orgError, setOrgError] = useState('')

  // Section 5: Security
  const [newPassword, setNewPassword] = useState('')
  const [passwordMsg, setPasswordMsg] = useState('')
  const [passwordError, setPasswordError] = useState('')
  const [totpConfirm, setTotpConfirm] = useState(false)
  const [totpMsg, setTotpMsg] = useState('')
  const [totpError, setTotpError] = useState('')

  // Section 6: Danger Zone
  const [deleteConfirm, setDeleteConfirm] = useState(false)

  // Data queries
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

  // Reset org selections when account changes
  useEffect(() => {
    if (selectedAccountId !== user.account_id) {
      setAllOrgsAccess(true)
      setSelectedOrgIds([])
    }
  }, [selectedAccountId, user.account_id])

  // Auto-clear messages
  const autoClear = (setter: (v: string) => void) => setTimeout(() => setter(''), 3000)

  // Mutations
  const updateProfileMut = useMutation({
    mutationFn: () => api.adminUpdateUser(user.id, { name: profileName, email: profileEmail }),
    onSuccess: (res) => {
      if (res.error) { setProfileError(res.error.message); return }
      setProfileMsg('Profile updated.')
      setProfileError('')
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      autoClear(setProfileMsg)
    },
    onError: () => setProfileError('Failed to update profile.'),
  })

  const updateAccountMut = useMutation({
    mutationFn: () => api.adminUpdateUser(user.id, { account_id: selectedAccountId }),
    onSuccess: (res) => {
      if (res.error) { setAccountError(res.error.message); return }
      setAccountMsg('Account updated.')
      setAccountError('')
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
      autoClear(setAccountMsg)
    },
    onError: () => setAccountError('Failed to update account.'),
  })

  const updateRoleMut = useMutation({
    mutationFn: () => api.adminUpdateUser(user.id, { role: selectedRole }),
    onSuccess: (res) => {
      if (res.error) { setRoleError(res.error.message); return }
      setRoleMsg('Role updated.')
      setRoleError('')
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      autoClear(setRoleMsg)
    },
    onError: () => setRoleError('Failed to update role.'),
  })

  const updateOrgAccessMut = useMutation({
    mutationFn: () => api.adminUpdateUser(user.id, { org_restrictions: allOrgsAccess ? [] : selectedOrgIds, set_org_restrictions: true }),
    onSuccess: (res) => {
      if (res.error) { setOrgError(res.error.message); return }
      setOrgMsg('Org access updated.')
      setOrgError('')
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      autoClear(setOrgMsg)
    },
    onError: () => setOrgError('Failed to update org access.'),
  })

  const resetPasswordMut = useMutation({
    mutationFn: () => api.adminResetPassword(user.id, newPassword),
    onSuccess: (res) => {
      if (res.error) { setPasswordError(res.error.message); return }
      setPasswordMsg('Password reset successfully.')
      setPasswordError('')
      setNewPassword('')
      autoClear(setPasswordMsg)
    },
    onError: () => setPasswordError('Failed to reset password.'),
  })

  const disableTotpMut = useMutation({
    mutationFn: () => api.adminDisableTOTP(user.id),
    onSuccess: (res) => {
      if (res.error) { setTotpError(res.error.message); return }
      setTotpMsg('2FA disabled.')
      setTotpError('')
      setTotpConfirm(false)
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
      autoClear(setTotpMsg)
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
  const toggleOrgId = (orgId: string) => setSelectedOrgIds(prev => prev.includes(orgId) ? prev.filter(id => id !== orgId) : [...prev, orgId])

  return (
    <SlidePanel open={true} title={user.name || user.email} onClose={onClose}>
      <div className="space-y-6">
        {/* Section 1: Profile */}
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
            <button onClick={() => updateProfileMut.mutate()} disabled={updateProfileMut.isPending} className={btnPrimary}>
              {updateProfileMut.isPending ? 'Saving...' : 'Save Profile'}
            </button>
          </div>
        </section>

        {/* Section 2: Account Assignment */}
        <section className="rounded-lg border border-gray-200 dark:border-slate-700 p-4">
          <h3 className={sectionHeader}>Account Assignment</h3>
          <div className="mt-3 space-y-3">
            <div>
              <label className={labelCls}>Account</label>
              <select value={selectedAccountId} onChange={e => setSelectedAccountId(e.target.value)} className={inputCls}>
                {accounts.map(a => (
                  <option key={a.id} value={a.id}>{a.name} ({a.plan})</option>
                ))}
              </select>
            </div>
            {accountError && <p className="text-xs text-red-600">{accountError}</p>}
            {accountMsg && <p className="text-xs text-emerald-600">{accountMsg}</p>}
            <button onClick={() => updateAccountMut.mutate()} disabled={updateAccountMut.isPending || selectedAccountId === user.account_id} className={btnPrimary}>
              {updateAccountMut.isPending ? 'Saving...' : 'Save Account'}
            </button>
          </div>
        </section>

        {/* Section 3: Role Assignment */}
        <section className="rounded-lg border border-gray-200 dark:border-slate-700 p-4">
          <h3 className={sectionHeader}>Role Assignment</h3>
          <div className="mt-3 space-y-3">
            <div>
              <label className={labelCls}>Role</label>
              <select value={selectedRole} onChange={e => setSelectedRole(e.target.value)} className={inputCls}>
                {currentUser?.role === 'root' && <option value="root">Root</option>}
                <option value="admin">Admin</option>
                <option value="analyst">Analyst</option>
                <option value="viewer">Viewer</option>
              </select>
            </div>
            {roleError && <p className="text-xs text-red-600">{roleError}</p>}
            {roleMsg && <p className="text-xs text-emerald-600">{roleMsg}</p>}
            <button onClick={() => updateRoleMut.mutate()} disabled={updateRoleMut.isPending || selectedRole === user.role} className={btnPrimary}>
              {updateRoleMut.isPending ? 'Saving...' : 'Save Role'}
            </button>
          </div>
        </section>

        {/* Section 4: Organization Access */}
        <section className="rounded-lg border border-gray-200 dark:border-slate-700 p-4">
          <h3 className={sectionHeader}>Organization Access</h3>
          <div className="mt-3 space-y-3">
            <label className="flex items-center gap-3 cursor-pointer">
              <button
                type="button"
                onClick={() => setAllOrgsAccess(!allOrgsAccess)}
                className={'relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ' + (allOrgsAccess ? 'bg-emerald-500' : 'bg-gray-200 dark:bg-slate-600')}
                role="switch"
                aria-checked={allOrgsAccess}
              >
                <span className={'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ' + (allOrgsAccess ? 'translate-x-4' : 'translate-x-0')} />
              </button>
              <span className="text-sm text-gray-700 dark:text-slate-300">All organizations</span>
            </label>
            {!allOrgsAccess && (
              <div className="rounded-lg border border-gray-200 dark:border-slate-600 bg-gray-50 dark:bg-slate-700/50 p-3 max-h-48 overflow-y-auto space-y-2">
                {accountOrgs.length === 0 && (
                  <p className="text-xs text-gray-400 dark:text-slate-500">No organizations found for this account.</p>
                )}
                {accountOrgs.map(org => (
                  <label key={org.id} className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={selectedOrgIds.includes(org.id)}
                      onChange={() => toggleOrgId(org.id)}
                      className="h-4 w-4 rounded border-gray-300 text-fibratus-600 focus:ring-fibratus-500"
                    />
                    <span className="text-sm text-gray-700 dark:text-slate-300">{org.name}</span>
                  </label>
                ))}
              </div>
            )}
            {!allOrgsAccess && selectedOrgIds.length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {selectedOrgIds.map(oid => {
                  const org = accountOrgs.find(o => o.id === oid)
                  return (
                    <span key={oid} className="inline-flex items-center rounded-full bg-fibratus-50 dark:bg-fibratus-900/30 px-2 py-0.5 text-[10px] font-medium text-fibratus-700 dark:text-fibratus-400">
                      {org?.name || oid}
                    </span>
                  )
                })}
              </div>
            )}
            {orgError && <p className="text-xs text-red-600">{orgError}</p>}
            {orgMsg && <p className="text-xs text-emerald-600">{orgMsg}</p>}
            <button onClick={() => updateOrgAccessMut.mutate()} disabled={updateOrgAccessMut.isPending} className={btnPrimary}>
              {updateOrgAccessMut.isPending ? 'Saving...' : 'Save Org Access'}
            </button>
          </div>
        </section>

        {/* Section 5: Security */}
        <section className="rounded-lg border border-gray-200 dark:border-slate-700 p-4">
          <h3 className={sectionHeader}>Security</h3>
          <div className="mt-3 space-y-4">
            {/* 2FA */}
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <span className="text-sm text-gray-600 dark:text-slate-400">2FA:</span>
                <span className={'inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium ' + (user.totp_enabled ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400' : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')}>
                  {user.totp_enabled ? 'Enabled' : 'Disabled'}
                </span>
              </div>
              {user.totp_enabled && !totpConfirm && (
                <button onClick={() => setTotpConfirm(true)} className="rounded-lg border border-red-300 dark:border-red-800 px-3 py-1.5 text-xs font-medium text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20">
                  Disable 2FA
                </button>
              )}
            </div>
            {totpConfirm && (
              <div className="rounded-lg border border-red-200 dark:border-red-900/50 bg-red-50 dark:bg-red-900/20 p-3 space-y-2">
                <p className="text-xs text-red-800 dark:text-red-300">Remove TOTP for this user?</p>
                {totpError && <p className="text-xs text-red-600">{totpError}</p>}
                <div className="flex gap-2">
                  <button onClick={() => disableTotpMut.mutate()} disabled={disableTotpMut.isPending} className="rounded-lg bg-red-600 px-3 py-1.5 text-xs text-white hover:bg-red-700 disabled:opacity-50">Confirm</button>
                  <button onClick={() => setTotpConfirm(false)} className="rounded-lg border border-gray-300 dark:border-slate-600 px-3 py-1.5 text-xs text-gray-600 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700">Cancel</button>
                </div>
              </div>
            )}
            {totpMsg && <p className="text-xs text-emerald-600">{totpMsg}</p>}

            <hr className="border-gray-200 dark:border-slate-700" />

            {/* Lock Status */}
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <span className="text-sm text-gray-600 dark:text-slate-400">Status:</span>
                {user.is_locked ? (
                  <span className="inline-flex rounded-full bg-red-50 dark:bg-red-900/30 px-2.5 py-0.5 text-xs font-medium text-red-700 dark:text-red-400">Locked</span>
                ) : (
                  <span className="inline-flex rounded-full bg-emerald-50 dark:bg-emerald-900/30 px-2.5 py-0.5 text-xs font-medium text-emerald-700 dark:text-emerald-400">Active</span>
                )}
              </div>
              {user.is_locked && (
                <button onClick={() => unlockMut.mutate()} disabled={unlockMut.isPending} className="rounded-lg bg-amber-600 px-3 py-1.5 text-xs text-white hover:bg-amber-700 disabled:opacity-50">
                  {unlockMut.isPending ? 'Unlocking...' : 'Unlock'}
                </button>
              )}
            </div>

            <hr className="border-gray-200 dark:border-slate-700" />

            {/* Password Reset */}
            <div>
              <h4 className="text-xs font-semibold text-gray-700 dark:text-slate-300">Reset Password</h4>
              <div className="mt-2 space-y-2">
                <input
                  type="password"
                  value={newPassword}
                  onChange={e => setNewPassword(e.target.value)}
                  className={inputCls}
                  placeholder="Enter new password"
                />
                {newPassword && (
                  <>
                    <div className="grid grid-cols-2 gap-x-4 gap-y-0.5">
                      {passwordRules.map(rule => (
                        <p key={rule.label} className={'text-[10px] ' + (rule.test(newPassword) ? 'text-emerald-600' : 'text-gray-400 dark:text-slate-500')}>
                          {rule.test(newPassword) ? '\u2713' : '\u2022'} {rule.label}
                        </p>
                      ))}
                    </div>
                    <div className="h-1.5 w-full rounded-full bg-gray-200 dark:bg-slate-600 overflow-hidden">
                      <div
                        className={'h-full rounded-full transition-all duration-300 ' + (
                          passwordRules.filter(r => r.test(newPassword)).length <= 2 ? 'bg-red-500' :
                          passwordRules.filter(r => r.test(newPassword)).length <= 4 ? 'bg-amber-500' : 'bg-emerald-500'
                        )}
                        style={{ width: `${(passwordRules.filter(r => r.test(newPassword)).length / passwordRules.length) * 100}%` }}
                      />
                    </div>
                  </>
                )}
                {passwordError && <p className="text-xs text-red-600">{passwordError}</p>}
                {passwordMsg && <p className="text-xs text-emerald-600">{passwordMsg}</p>}
                <button onClick={() => resetPasswordMut.mutate()} disabled={!allPasswordRulesPass || resetPasswordMut.isPending} className={btnPrimary}>
                  {resetPasswordMut.isPending ? 'Resetting...' : 'Reset Password'}
                </button>
              </div>
            </div>
          </div>
        </section>

        {/* Section 6: Danger Zone */}
        <section className="rounded-lg border border-red-200 dark:border-red-900/50 p-4">
          <h3 className="text-sm font-semibold text-red-600 dark:text-red-400">Danger Zone</h3>
          <div className="mt-3">
            {!deleteConfirm ? (
              <button onClick={() => setDeleteConfirm(true)} className="rounded-lg border border-red-300 dark:border-red-800 bg-white dark:bg-slate-800 px-4 py-2 text-sm font-medium text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20">
                Delete User
              </button>
            ) : (
              <div className="rounded-lg border border-red-200 dark:border-red-900/50 bg-red-50 dark:bg-red-900/20 p-3 space-y-2">
                <p className="text-sm text-red-800 dark:text-red-300">Permanently delete <strong>{user.name || user.email}</strong>?</p>
                <div className="flex gap-2">
                  <button onClick={() => deleteMut.mutate()} disabled={deleteMut.isPending} className="rounded-lg bg-red-600 px-4 py-2 text-sm text-white hover:bg-red-700 disabled:opacity-50">
                    {deleteMut.isPending ? 'Deleting...' : 'Yes, Delete'}
                  </button>
                  <button onClick={() => setDeleteConfirm(false)} className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm text-gray-600 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700">
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
// Organizations Tab (cross-account)
// ================================================================

function OrganizationsTab({ filterAccountId, onClearFilter, onAccountClick }: { filterAccountId: string | null; onClearFilter: () => void; onAccountClick: (id: string) => void }) {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [createName, setCreateName] = useState('')
  const [createSlug, setCreateSlug] = useState('')
  const [createError, setCreateError] = useState('')

  const { data: currentUserData } = useQuery({ queryKey: ['current-user'], queryFn: () => api.getCurrentUser() })
  const currentUser = currentUserData?.data as User | undefined
  const { data: accountsData } = useQuery({ queryKey: ['admin-accounts'], queryFn: () => api.adminGetAccounts(), enabled: currentUser?.role === 'root' })
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
    let list = filterAccountId ? allOrgs.filter(o => o.account_id === filterAccountId) : allOrgs
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

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deleteOrganization(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-all-orgs'] })
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
      setDeleteId(null)
    },
  })

  const createMut = useMutation({
    mutationFn: (data: { name: string; slug: string }) => api.createOrganization(data),
    onSuccess: (res) => {
      if (res.error) { setCreateError(res.error.message); return }
      queryClient.invalidateQueries({ queryKey: ['admin-all-orgs'] })
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
      setShowCreate(false)
      setCreateName('')
      setCreateSlug('')
      setCreateError('')
    },
    onError: () => setCreateError('Failed to create organization.'),
  })

  const retentionMut = useMutation({
    mutationFn: (data: { orgId: string; days: number }) => api.updateOrgRetention(data.orgId, data.days),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin-all-orgs'] }),
  })

  const filterAccountName = filterAccountId ? accounts.find(a => a.id === filterAccountId)?.name : null

  const handleCreateSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setCreateError('')
    if (!createName.trim()) { setCreateError('Organization name is required'); return }
    const slug = createSlug.trim() || slugify(createName)
    createMut.mutate({ name: createName.trim(), slug })
  }

  return (
    <div className="mt-6">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-4">
          <p className="text-sm text-gray-500 dark:text-slate-400">
            {filtered.length} organization(s)
            {filterAccountName && <span className="ml-1 text-fibratus-600">in {filterAccountName}</span>}
          </p>
          {filterAccountId && (
            <button onClick={onClearFilter} className="text-xs text-fibratus-600 hover:underline">Show all</button>
          )}
          <input
            type="text"
            placeholder="Search organizations..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-1.5 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
          />
        </div>
        <button onClick={() => setShowCreate(true)} className={btnPrimary}>Create Organization</button>
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
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Retention</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {isLoading && (
                <tr><td colSpan={6} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">Loading...</td></tr>
              )}
              {!isLoading && sortedOrgs.map(org => (
                <tr key={org.id} className="hover:bg-gray-50/50 dark:hover:bg-slate-700/50">
                  <td className="px-6 py-3 font-medium text-gray-900 dark:text-slate-100">{org.name}</td>
                  <td className="px-6 py-3">
                    <button onClick={() => onAccountClick(org.account_id)} className="text-fibratus-600 hover:underline text-sm">
                      {org.account_name || '-'}
                    </button>
                  </td>
                  <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{org.agent_count}</td>
                  <td className="px-6 py-3 font-mono text-xs text-gray-500 dark:text-slate-500">{org.slug}</td>
                  <td className="px-6 py-3">
                    <select
                      value={org.telemetry_retention_days || 0}
                      onChange={e => retentionMut.mutate({ orgId: org.id, days: Number(e.target.value) })}
                      disabled={retentionMut.isPending}
                      className={retentionSelectCls}
                    >
                      {orgRetentionOptions.map(o => (
                        <option key={o.value} value={o.value}>{o.label}</option>
                      ))}
                    </select>
                  </td>
                  <td className="px-6 py-3">
                    {deleteId === org.id ? (
                      <div className="flex items-center gap-2">
                        <button onClick={() => deleteMut.mutate(org.id)} className="rounded bg-red-600 px-2 py-1 text-xs text-white hover:bg-red-700">Confirm</button>
                        <button onClick={() => setDeleteId(null)} className="rounded bg-gray-200 dark:bg-slate-600 px-2 py-1 text-xs text-gray-700 dark:text-slate-300 hover:bg-gray-300 dark:hover:bg-slate-500">Cancel</button>
                      </div>
                    ) : (
                      <button onClick={() => setDeleteId(org.id)} className="text-xs text-red-600 hover:underline">Delete</button>
                    )}
                  </td>
                </tr>
              ))}
              {!isLoading && filtered.length === 0 && (
                <tr><td colSpan={6} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">No organizations found.</td></tr>
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
            <form onSubmit={handleCreateSubmit} className="mt-4 space-y-3">
              <div>
                <label className={labelCls}>Organization Name</label>
                <input
                  value={createName}
                  onChange={e => { setCreateName(e.target.value); if (!createSlug || createSlug === slugify(createName)) setCreateSlug(slugify(e.target.value)) }}
                  className={inputCls}
                  required
                />
              </div>
              <div>
                <label className={labelCls}>Slug</label>
                <input
                  value={createSlug}
                  onChange={e => setCreateSlug(e.target.value)}
                  className={inputCls}
                  placeholder="auto-generated from name"
                />
                <p className="mt-1 text-[10px] text-gray-400 dark:text-slate-500">URL-friendly identifier. Leave blank to auto-generate.</p>
              </div>
              {createError && <p className="text-xs text-red-600">{createError}</p>}
              <div className="flex justify-end gap-2 pt-2">
                <button type="button" onClick={() => { setShowCreate(false); setCreateError('') }} className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700">Cancel</button>
                <button type="submit" disabled={createMut.isPending} className={btnPrimary}>{createMut.isPending ? 'Creating...' : 'Create Organization'}</button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
