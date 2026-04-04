import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type User } from '../lib/api'

const roleBadge: Record<string, string> = {
  admin: 'bg-purple-100 text-purple-700',
  analyst: 'bg-blue-100 text-blue-700',
  viewer: 'bg-gray-100 text-gray-600',
}

const passwordRules = [
  { label: '12+ characters', test: (p: string) => p.length >= 12 },
  { label: 'Uppercase letter', test: (p: string) => /[A-Z]/.test(p) },
  { label: 'Lowercase letter', test: (p: string) => /[a-z]/.test(p) },
  { label: 'Digit', test: (p: string) => /\d/.test(p) },
  { label: 'Special character', test: (p: string) => /[^A-Za-z0-9]/.test(p) },
]

export default function Users() {
  const queryClient = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState({ email: '', name: '', password: '', role: 'viewer' })
  const [error, setError] = useState('')
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const { data: currentUserData } = useQuery({
    queryKey: ['current-user'],
    queryFn: () => api.getCurrentUser(),
  })
  const currentUser = currentUserData?.data as User | undefined

  const { data, isLoading } = useQuery({
    queryKey: ['users'],
    queryFn: () => api.getUsers(),
  })
  const users = (data?.data || []) as User[]

  const isAdmin = currentUser?.role === 'admin'

  const roleMut = useMutation({
    mutationFn: ({ id, role }: { id: string; role: string }) => api.updateUserRole(id, role),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['users'] }),
  })

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deleteUser(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setDeleteId(null)
    },
  })

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    const allPass = passwordRules.every(r => r.test(form.password))
    if (!allPass) {
      setError('Password does not meet all requirements.')
      return
    }
    const resp = await api.createUser(form)
    if (resp.error) {
      setError(resp.error.message)
      return
    }
    queryClient.invalidateQueries({ queryKey: ['users'] })
    setShowCreate(false)
    setForm({ email: '', name: '', password: '', role: 'viewer' })
  }

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Users</h1>
          <p className="mt-1 text-sm text-gray-500">{users.length} user(s) in this organization</p>
        </div>
        {isAdmin && (
          <button
            onClick={() => setShowCreate(true)}
            className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
          >
            Add User
          </button>
        )}
      </div>

      {/* User table */}
      <div className="mt-6 rounded-xl border border-gray-200 bg-white shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 bg-gray-50/50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-500">Name</th>
                <th className="px-6 py-3 font-medium text-gray-500">Email</th>
                <th className="px-6 py-3 font-medium text-gray-500">Role</th>
                <th className="px-6 py-3 font-medium text-gray-500">2FA</th>
                <th className="px-6 py-3 font-medium text-gray-500">Created</th>
                <th className="px-6 py-3 font-medium text-gray-500">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading && (
                <tr><td colSpan={6} className="px-6 py-12 text-center text-gray-400">Loading...</td></tr>
              )}
              {!isLoading && users.map(user => {
                const isSelf = currentUser?.id === user.id
                return (
                  <tr key={user.id} className="hover:bg-gray-50/50">
                    <td className="px-6 py-3 font-medium text-gray-900">
                      {user.name}
                      {isSelf && <span className="ml-2 text-xs text-gray-400">(you)</span>}
                    </td>
                    <td className="px-6 py-3 text-gray-600">{user.email}</td>
                    <td className="px-6 py-3">
                      {isAdmin && !isSelf ? (
                        <select
                          value={user.role}
                          onChange={e => roleMut.mutate({ id: user.id, role: e.target.value })}
                          className={'rounded-full px-2.5 py-0.5 text-xs font-medium border-0 cursor-pointer ' +
                            (roleBadge[user.role] || roleBadge.viewer)}
                        >
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
                    <td className="px-6 py-3">
                      <span className={'rounded-full px-2 py-0.5 text-[10px] font-medium ' +
                        (user.totp_enabled ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-500')}>
                        {user.totp_enabled ? 'Enabled' : 'Off'}
                      </span>
                    </td>
                    <td className="px-6 py-3 text-gray-500 whitespace-nowrap">
                      {new Date(user.created_at).toLocaleDateString()}
                    </td>
                    <td className="px-6 py-3">
                      {isAdmin && !isSelf ? (
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
                              className="rounded bg-gray-200 px-2 py-1 text-xs text-gray-700 hover:bg-gray-300"
                            >
                              Cancel
                            </button>
                          </div>
                        ) : (
                          <button
                            onClick={() => setDeleteId(user.id)}
                            className="text-xs text-red-600 hover:underline"
                          >
                            Remove
                          </button>
                        )
                      ) : (
                        <span className="text-xs text-gray-300">-</span>
                      )}
                    </td>
                  </tr>
                )
              })}
              {!isLoading && users.length === 0 && (
                <tr><td colSpan={6} className="px-6 py-12 text-center text-gray-400">No users yet.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Roles legend */}
      <div className="mt-4 rounded-lg border border-gray-200 bg-white p-4">
        <h3 className="text-xs font-medium text-gray-500 uppercase tracking-wider mb-3">Role Permissions</h3>
        <div className="grid grid-cols-3 gap-4 text-xs">
          <div>
            <span className="rounded-full bg-purple-100 text-purple-700 px-2 py-0.5 font-medium">Admin</span>
            <p className="mt-1 text-gray-500">Full access -- manage users, rules, agents, settings, active response commands</p>
          </div>
          <div>
            <span className="rounded-full bg-blue-100 text-blue-700 px-2 py-0.5 font-medium">Analyst</span>
            <p className="mt-1 text-gray-500">Investigation -- view/manage detections, events, rules. No active response or settings</p>
          </div>
          <div>
            <span className="rounded-full bg-gray-100 text-gray-600 px-2 py-0.5 font-medium">Viewer</span>
            <p className="mt-1 text-gray-500">Read-only -- view detections, events, agents. No modifications</p>
          </div>
        </div>
      </div>

      {/* Create user modal */}
      {showCreate && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowCreate(false)}>
          <div className="bg-white rounded-xl shadow-xl w-full max-w-md p-6" onClick={e => e.stopPropagation()}>
            <h2 className="text-lg font-bold text-gray-900">Add User</h2>
            <form onSubmit={handleCreate} className="mt-4 space-y-3">
              <div>
                <label className="block text-xs font-medium text-gray-500 mb-1">Name</label>
                <input
                  value={form.name}
                  onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                  required
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 mb-1">Email</label>
                <input
                  type="email"
                  value={form.email}
                  onChange={e => setForm(f => ({ ...f, email: e.target.value }))}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                  required
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 mb-1">Password</label>
                <input
                  type="password"
                  value={form.password}
                  onChange={e => setForm(f => ({ ...f, password: e.target.value }))}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                  required
                  minLength={12}
                />
                <div className="mt-2 space-y-1">
                  {passwordRules.map(rule => (
                    <p key={rule.label} className={'text-[10px] ' + (rule.test(form.password) ? 'text-emerald-600' : 'text-gray-400')}>
                      {rule.test(form.password) ? '\u2713' : '\u2022'} {rule.label}
                    </p>
                  ))}
                </div>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 mb-1">Role</label>
                <select
                  value={form.role}
                  onChange={e => setForm(f => ({ ...f, role: e.target.value }))}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                >
                  <option value="viewer">Viewer</option>
                  <option value="analyst">Analyst</option>
                  <option value="admin">Admin</option>
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
                  Create User
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
