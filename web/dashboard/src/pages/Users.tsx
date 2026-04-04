import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type User, type UserGroup, type Organization } from '../lib/api'
import SlidePanel from '../components/SlidePanel'

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
  const [form, setForm] = useState({ email: '', name: '', password: '', role: 'viewer', org_restrictions: [] as string[], group_ids: [] as string[] })
  const [error, setError] = useState('')
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [editingUser, setEditingUser] = useState<User | null>(null)

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

  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: () => api.getGroups(),
  })
  const groups = (groupsData?.data || []) as (UserGroup & { members?: { id: string }[] })[]

  const { data: orgsData } = useQuery({
    queryKey: ['organizations'],
    queryFn: () => api.getOrganizations(),
  })
  const orgs = (orgsData?.data || []) as Organization[]

  // Build a map of userId -> group names
  const userGroupMap: Record<string, string[]> = {}
  groups.forEach(g => {
    (g.members || []).forEach(m => {
      if (!userGroupMap[m.id]) userGroupMap[m.id] = []
      userGroupMap[m.id].push(g.name)
    })
  })

  const isAdmin = currentUser?.role === 'admin' || currentUser?.role === 'root'

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
    setForm({ email: '', name: '', password: '', role: 'viewer', org_restrictions: [], group_ids: [] })
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
                <th className="px-6 py-3 font-medium text-gray-500">Groups</th>
                <th className="px-6 py-3 font-medium text-gray-500">2FA</th>
                <th className="px-6 py-3 font-medium text-gray-500">Created</th>
                <th className="px-6 py-3 font-medium text-gray-500">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading && (
                <tr><td colSpan={7} className="px-6 py-12 text-center text-gray-400">Loading...</td></tr>
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
                      <div className="flex flex-wrap gap-1">
                        {(userGroupMap[user.id] || []).map(gName => (
                          <span key={gName} className="inline-flex rounded-full bg-fibratus-50 text-fibratus-700 px-2 py-0.5 text-[10px] font-medium">
                            {gName}
                          </span>
                        ))}
                        {!(userGroupMap[user.id] || []).length && (
                          <span className="text-[10px] text-gray-400">--</span>
                        )}
                      </div>
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
                      <div className="flex items-center gap-3">
                        {isAdmin && !isSelf && (
                          <button
                            onClick={() => setEditingUser(user)}
                            className="text-xs text-fibratus-600 hover:underline"
                          >
                            Edit
                          </button>
                        )}
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
                      </div>
                    </td>
                  </tr>
                )
              })}
              {!isLoading && users.length === 0 && (
                <tr><td colSpan={7} className="px-6 py-12 text-center text-gray-400">No users yet.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Roles permission matrix */}
      <div className="mt-4 rounded-lg border border-gray-200 bg-white p-4">
        <h3 className="text-xs font-medium text-gray-500 uppercase tracking-wider mb-4">Role Permissions</h3>
        <div className="grid grid-cols-3 gap-6 text-xs">
          <RolePermissionCard
            role="Admin"
            badgeClass="bg-purple-100 text-purple-700"
            permissions={{
              Agents:           ['View', 'Manage', 'Delete'],
              Detections:       ['View'],
              Events:           ['View'],
              Rules:            ['View', 'Create/Edit/Delete'],
              Macros:           ['View', 'Create/Edit/Delete'],
              'Active Response': ['View', 'Execute'],
              Settings:         ['View', 'Manage'],
              'User Management': ['Manage Users'],
              Audit:            ['View'],
              Organizations:    ['Manage'],
            }}
          />
          <RolePermissionCard
            role="Analyst"
            badgeClass="bg-blue-100 text-blue-700"
            permissions={{
              Agents:           ['View'],
              Detections:       ['View'],
              Events:           ['View'],
              Rules:            ['View', 'Create/Edit/Delete'],
              Macros:           ['View', 'Create/Edit/Delete'],
              'Active Response': ['View'],
            }}
          />
          <RolePermissionCard
            role="Viewer"
            badgeClass="bg-gray-100 text-gray-600"
            permissions={{
              Agents:           ['View'],
              Detections:       ['View'],
              Events:           ['View'],
              Rules:            ['View'],
              Macros:           ['View'],
            }}
          />
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
                  {currentUser?.role === 'root' && <option value="root">Root</option>}
                </select>
              </div>
              {/* Org restrictions */}
              {orgs.length > 0 && (
                <div>
                  <label className="block text-xs font-medium text-gray-500 mb-1">Organization Access</label>
                  <div className="rounded-lg border border-gray-200 p-2 max-h-32 overflow-auto space-y-1">
                    <label className="flex items-center gap-2 text-xs text-gray-700 cursor-pointer">
                      <input type="checkbox" checked={form.org_restrictions.length === 0}
                        onChange={() => setForm(f => ({ ...f, org_restrictions: [] }))}
                        className="rounded border-gray-300" />
                      <span className="font-medium">All organizations</span>
                    </label>
                    {orgs.map(org => (
                      <label key={org.id} className="flex items-center gap-2 text-xs text-gray-600 cursor-pointer ml-4">
                        <input type="checkbox"
                          checked={form.org_restrictions.length === 0 || form.org_restrictions.includes(org.id)}
                          onChange={e => {
                            if (form.org_restrictions.length === 0) {
                              // Switching from "all" to specific — select only this one
                              setForm(f => ({ ...f, org_restrictions: [org.id] }))
                            } else if (e.target.checked) {
                              setForm(f => ({ ...f, org_restrictions: [...f.org_restrictions, org.id] }))
                            } else {
                              const updated = form.org_restrictions.filter(id => id !== org.id)
                              setForm(f => ({ ...f, org_restrictions: updated.length === 0 ? [] : updated }))
                            }
                          }}
                          className="rounded border-gray-300" />
                        {org.name}
                      </label>
                    ))}
                  </div>
                  <p className="mt-1 text-[10px] text-gray-400">Leave "All" checked for unrestricted access, or select specific orgs.</p>
                </div>
              )}
              {/* Group assignment */}
              {groups.length > 0 && (
                <div>
                  <label className="block text-xs font-medium text-gray-500 mb-1">Groups</label>
                  <div className="rounded-lg border border-gray-200 p-2 max-h-32 overflow-auto space-y-1">
                    {groups.map(g => (
                      <label key={g.id} className="flex items-center gap-2 text-xs text-gray-600 cursor-pointer">
                        <input type="checkbox"
                          checked={form.group_ids.includes(g.id)}
                          onChange={e => {
                            if (e.target.checked) {
                              setForm(f => ({ ...f, group_ids: [...f.group_ids, g.id] }))
                            } else {
                              setForm(f => ({ ...f, group_ids: f.group_ids.filter(id => id !== g.id) }))
                            }
                          }}
                          className="rounded border-gray-300" />
                        <span>{g.name}</span>
                        {g.description && <span className="text-gray-400">— {g.description}</span>}
                      </label>
                    ))}
                  </div>
                </div>
              )}
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

      {/* Edit user slide panel */}
      {editingUser && (
        <UserEditPanel
          user={editingUser}
          onClose={() => setEditingUser(null)}
          onDeleted={() => { setEditingUser(null) }}
        />
      )}
    </div>
  )
}

const permCategoryColors: Record<string, { bg: string; text: string }> = {
  Agents:           { bg: 'bg-blue-50',    text: 'text-blue-700' },
  Detections:       { bg: 'bg-amber-50',   text: 'text-amber-700' },
  Events:           { bg: 'bg-cyan-50',    text: 'text-cyan-700' },
  Rules:            { bg: 'bg-purple-50',   text: 'text-purple-700' },
  Macros:           { bg: 'bg-indigo-50',  text: 'text-indigo-700' },
  'Active Response': { bg: 'bg-red-50',    text: 'text-red-700' },
  Settings:         { bg: 'bg-gray-100',   text: 'text-gray-700' },
  'User Management': { bg: 'bg-emerald-50', text: 'text-emerald-700' },
  Audit:            { bg: 'bg-orange-50',  text: 'text-orange-700' },
  Organizations:    { bg: 'bg-teal-50',    text: 'text-teal-700' },
}

function RolePermissionCard({ role, badgeClass, permissions }: {
  role: string
  badgeClass: string
  permissions: Record<string, string[]>
}) {
  return (
    <div>
      <span className={`rounded-full px-2 py-0.5 font-medium ${badgeClass}`}>{role}</span>
      <div className="mt-2 space-y-1.5">
        {Object.entries(permissions).map(([category, perms]) => {
          const colors = permCategoryColors[category] || { bg: 'bg-gray-100', text: 'text-gray-700' }
          return (
            <div key={category} className="flex items-start gap-1.5">
              <span className="text-[10px] text-gray-500 w-24 shrink-0 pt-0.5">{category}:</span>
              <div className="flex flex-wrap gap-1">
                {perms.map(p => (
                  <span key={p} className={`inline-flex rounded-full px-1.5 py-0 text-[10px] font-medium ${colors.bg} ${colors.text}`}>
                    {p}
                  </span>
                ))}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function UserEditPanel({ user, onClose, onDeleted }: { user: User; onClose: () => void; onDeleted: () => void }) {
  const queryClient = useQueryClient()

  // Profile form
  const [profileName, setProfileName] = useState(user.name)
  const [profileEmail, setProfileEmail] = useState(user.email)
  const [profileMsg, setProfileMsg] = useState('')
  const [profileError, setProfileError] = useState('')

  // Password form
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
    mutationFn: () => api.updateUser(user.id, { name: profileName, email: profileEmail }),
    onSuccess: (res) => {
      if (res.error) { setProfileError(res.error.message); return }
      setProfileMsg('Profile updated.')
      setProfileError('')
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setTimeout(() => setProfileMsg(''), 3000)
    },
    onError: () => setProfileError('Failed to update profile.'),
  })

  const resetPasswordMut = useMutation({
    mutationFn: () => api.resetUserPassword(user.id, newPassword),
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
    mutationFn: () => api.disableUserTOTP(user.id),
    onSuccess: (res) => {
      if (res.error) { setTotpError(res.error.message); return }
      setTotpMsg('2FA has been disabled for this user.')
      setTotpError('')
      setTotpConfirm(false)
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setTimeout(() => setTotpMsg(''), 3000)
    },
    onError: () => setTotpError('Failed to disable 2FA.'),
  })

  const deleteMut = useMutation({
    mutationFn: () => api.deleteUser(user.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      onDeleted()
    },
  })

  const allPasswordRulesPass = passwordRules.every(r => r.test(newPassword))

  return (
    <SlidePanel open={true} title={user.name || user.email} onClose={onClose}>
      <div className="space-y-8">
        {/* Profile Section */}
        <div>
          <h3 className="text-sm font-semibold text-gray-900">Profile</h3>
          <div className="mt-3 space-y-3">
            <div>
              <label className="block text-xs font-medium text-gray-500 mb-1">Name</label>
              <input
                value={profileName}
                onChange={e => setProfileName(e.target.value)}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-500 mb-1">Email</label>
              <input
                type="email"
                value={profileEmail}
                onChange={e => setProfileEmail(e.target.value)}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
              />
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

        <hr className="border-gray-200" />

        {/* Password Section */}
        <div>
          <h3 className="text-sm font-semibold text-gray-900">Password</h3>
          <p className="mt-1 text-xs text-gray-500">Reset this user's password. They will need to use the new password on next login.</p>
          <div className="mt-3 space-y-3">
            <div>
              <label className="block text-xs font-medium text-gray-500 mb-1">New Password</label>
              <input
                type="password"
                value={newPassword}
                onChange={e => setNewPassword(e.target.value)}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
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

        <hr className="border-gray-200" />

        {/* 2FA Section */}
        <div>
          <h3 className="text-sm font-semibold text-gray-900">Two-Factor Authentication</h3>
          <div className="mt-3 flex items-center gap-3">
            <span className="text-sm text-gray-600">Status:</span>
            <span className={'inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium ' +
              (user.totp_enabled ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-500')}>
              {user.totp_enabled ? 'Enabled' : 'Disabled'}
            </span>
          </div>
          {user.totp_enabled && (
            <div className="mt-3">
              {!totpConfirm ? (
                <button
                  onClick={() => setTotpConfirm(true)}
                  className="rounded-lg border border-red-300 px-4 py-2 text-sm font-medium text-red-600 hover:bg-red-50"
                >
                  Disable 2FA
                </button>
              ) : (
                <div className="rounded-lg border border-red-200 bg-red-50 p-4 space-y-3">
                  <p className="text-sm text-red-800">
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
                      className="rounded-lg border border-gray-300 px-4 py-2 text-sm text-gray-600 hover:bg-gray-50"
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
            <p className="mt-2 text-xs text-gray-500">This user has not enabled two-factor authentication.</p>
          )}
        </div>

        <hr className="border-gray-200" />

        {/* Danger Zone */}
        <div>
          <h3 className="text-sm font-semibold text-red-600">Danger Zone</h3>
          <p className="mt-1 text-xs text-gray-500">Permanently delete this user account. This action cannot be undone.</p>
          <div className="mt-3">
            {!deleteConfirm ? (
              <button
                onClick={() => setDeleteConfirm(true)}
                className="rounded-lg border border-red-300 bg-white px-4 py-2 text-sm font-medium text-red-600 hover:bg-red-50"
              >
                Delete User
              </button>
            ) : (
              <div className="rounded-lg border border-red-200 bg-red-50 p-4 space-y-3">
                <p className="text-sm text-red-800">
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
                    className="rounded-lg border border-gray-300 px-4 py-2 text-sm text-gray-600 hover:bg-gray-50"
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
