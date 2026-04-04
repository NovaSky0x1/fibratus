import { useState, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Organization, type UserGroup, type User, type PermissionDef } from '../lib/api'
import Users from './Users'

type Tab = 'account' | 'organizations' | 'groups' | 'users'

// Category display order and colors (same as Groups page)
const categoryMeta: Record<string, { color: string; bg: string }> = {
  Agents:           { color: 'text-blue-700',    bg: 'bg-blue-50' },
  Detections:       { color: 'text-amber-700',   bg: 'bg-amber-50' },
  Events:           { color: 'text-cyan-700',    bg: 'bg-cyan-50' },
  Rules:            { color: 'text-purple-700',   bg: 'bg-purple-50' },
  Macros:           { color: 'text-indigo-700',  bg: 'bg-indigo-50' },
  'Active Response': { color: 'text-red-700',    bg: 'bg-red-50' },
  Settings:         { color: 'text-gray-700',    bg: 'bg-gray-100' },
  'User Management': { color: 'text-emerald-700', bg: 'bg-emerald-50' },
  Audit:            { color: 'text-orange-700',  bg: 'bg-orange-50' },
  Organizations:    { color: 'text-teal-700',    bg: 'bg-teal-50' },
}

function permBadgeClasses(category: string): string {
  const meta = categoryMeta[category] || { color: 'text-gray-700', bg: 'bg-gray-100' }
  return `${meta.bg} ${meta.color}`
}

export default function Management() {
  const [tab, setTab] = useState<Tab>('account')

  // Current user
  const { data: currentUserData } = useQuery({
    queryKey: ['current-user'],
    queryFn: () => api.getCurrentUser(),
  })
  const currentUser = currentUserData?.data as User | undefined
  const isAdmin = currentUser?.role === 'admin' || currentUser?.role === 'root'

  if (currentUser && !isAdmin) {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="text-center">
          <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-red-50">
            <svg className="h-8 w-8 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
            </svg>
          </div>
          <h2 className="text-lg font-semibold text-gray-900">Access Denied</h2>
          <p className="mt-1 text-sm text-gray-500">You need an admin or root role to access account management.</p>
        </div>
      </div>
    )
  }

  const tabs: { key: Tab; label: string }[] = [
    { key: 'account', label: 'Account' },
    { key: 'users', label: 'Users' },
    { key: 'organizations', label: 'Organizations' },
    { key: 'groups', label: 'User Groups' },
  ]

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Management</h1>
          <p className="mt-1 text-sm text-gray-500">Account settings, organizations, and user group management</p>
        </div>
      </div>

      {/* Tabs */}
      <div className="mt-6 flex gap-1 border-b border-gray-200">
        {tabs.map(t => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
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

      {tab === 'account' && <AccountSettingsTab />}
      {tab === 'users' && <UsersTab />}
      {tab === 'organizations' && <OrganizationsTab />}
      {tab === 'groups' && <UserGroupsTab />}
    </div>
  )
}

// ================================================================
// Account Settings Tab
// ================================================================

function UsersTab() {
  return <Users />
}

// ================================================================

function AccountSettingsTab() {
  const queryClient = useQueryClient()

  const { data: settingsData, isLoading } = useQuery({
    queryKey: ['account-settings'],
    queryFn: () => api.getAccountSettings(),
  })
  const settings = settingsData?.data

  const { data: usersData } = useQuery({
    queryKey: ['users'],
    queryFn: () => api.getUsers(),
  })
  const users = (usersData?.data || []) as User[]
  const usersWith2FA = users.filter(u => u.totp_enabled).length
  const usersWithout2FA = users.filter(u => !u.totp_enabled).length

  const updateMut = useMutation({
    mutationFn: (data: { require_2fa: boolean }) => api.updateAccountSettings(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['account-settings'] })
    },
  })

  const handleToggle2FA = () => {
    if (!settings) return
    updateMut.mutate({ require_2fa: !settings.require_2fa })
  }

  if (isLoading) {
    return (
      <div className="mt-6 flex items-center justify-center py-12">
        <p className="text-sm text-gray-400">Loading account settings...</p>
      </div>
    )
  }

  return (
    <div className="mt-6 space-y-6">
      {/* Account Info */}
      <div className="rounded-xl border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="text-sm font-semibold text-gray-900">Account Information</h3>
        <div className="mt-4 grid grid-cols-2 gap-6">
          <div>
            <p className="text-xs font-medium text-gray-500 uppercase tracking-wider">Account Name</p>
            <p className="mt-1 text-sm font-medium text-gray-900">{settings?.account_name || '-'}</p>
          </div>
          <div>
            <p className="text-xs font-medium text-gray-500 uppercase tracking-wider">Plan</p>
            <span className="mt-1 inline-flex rounded-full bg-blue-50 px-2.5 py-0.5 text-xs font-medium text-blue-700">
              {settings?.plan || '-'}
            </span>
          </div>
        </div>
      </div>

      {/* 2FA Enforcement */}
      <div className="rounded-xl border border-gray-200 bg-white p-6 shadow-sm">
        <div className="flex items-start justify-between">
          <div>
            <h3 className="text-sm font-semibold text-gray-900">Two-Factor Authentication Enforcement</h3>
            <p className="mt-1 text-sm text-gray-500">
              Require all users in this account to set up 2FA
            </p>
          </div>
          <button
            onClick={handleToggle2FA}
            disabled={updateMut.isPending}
            className={
              'relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 ' +
              (settings?.require_2fa ? 'bg-emerald-500 focus:ring-emerald-500' : 'bg-gray-200 focus:ring-gray-400')
            }
            role="switch"
            aria-checked={settings?.require_2fa || false}
          >
            <span
              className={
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ' +
                (settings?.require_2fa ? 'translate-x-5' : 'translate-x-0')
              }
            />
          </button>
        </div>

        {settings?.require_2fa && (
          <div className="mt-4 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3">
            <p className="text-xs text-amber-800">
              <strong>Enforcement active:</strong> All users without 2FA will be prompted to set it up on next login.
            </p>
          </div>
        )}

        {/* 2FA stats */}
        <div className="mt-5 grid grid-cols-2 gap-4">
          <div className="rounded-lg border border-gray-100 bg-gray-50 px-4 py-3">
            <p className="text-xs font-medium text-gray-500">Users with 2FA</p>
            <div className="mt-1 flex items-baseline gap-2">
              <span className="text-xl font-bold text-emerald-600">{usersWith2FA}</span>
              {users.length > 0 && (
                <span className="text-xs text-gray-400">
                  ({Math.round((usersWith2FA / users.length) * 100)}%)
                </span>
              )}
            </div>
          </div>
          <div className="rounded-lg border border-gray-100 bg-gray-50 px-4 py-3">
            <p className="text-xs font-medium text-gray-500">Users without 2FA</p>
            <div className="mt-1 flex items-baseline gap-2">
              <span className="text-xl font-bold text-gray-600">{usersWithout2FA}</span>
              {users.length > 0 && (
                <span className="text-xs text-gray-400">
                  ({Math.round((usersWithout2FA / users.length) * 100)}%)
                </span>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// ================================================================
// Organizations Tab
// ================================================================

function OrganizationsTab() {
  const queryClient = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [orgName, setOrgName] = useState('')
  const [orgSlug, setOrgSlug] = useState('')
  const [error, setError] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<Organization | null>(null)

  const { data: orgsData, isLoading } = useQuery({
    queryKey: ['organizations'],
    queryFn: () => api.getOrganizations(),
  })
  const orgs = (orgsData?.data || []) as Organization[]

  const createMut = useMutation({
    mutationFn: () => api.createOrganization({ name: orgName, slug: orgSlug }),
    onSuccess: (res) => {
      if (res.error) {
        setError(res.error.message)
        return
      }
      setShowCreate(false)
      setOrgName('')
      setOrgSlug('')
      setError('')
      queryClient.invalidateQueries({ queryKey: ['organizations'] })
    },
    onError: () => setError('Failed to create organization'),
  })

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deleteOrganization(id),
    onSuccess: () => {
      setDeleteTarget(null)
      queryClient.invalidateQueries({ queryKey: ['organizations'] })
    },
  })

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    if (!orgName.trim()) {
      setError('Organization name is required')
      return
    }
    createMut.mutate()
  }

  return (
    <div className="mt-6">
      <div className="flex items-center justify-between mb-4">
        <p className="text-sm text-gray-500">{orgs.length} organization(s)</p>
        <button
          onClick={() => setShowCreate(true)}
          className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
        >
          Create Organization
        </button>
      </div>

      <div className="rounded-xl border border-gray-200 bg-white shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 bg-gray-50/50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-500">Name</th>
                <th className="px-6 py-3 font-medium text-gray-500">Slug</th>
                <th className="px-6 py-3 font-medium text-gray-500">Agents</th>
                <th className="px-6 py-3 font-medium text-gray-500">Created</th>
                <th className="px-6 py-3 font-medium text-gray-500">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading && (
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400">Loading...</td></tr>
              )}
              {!isLoading && orgs.map(org => (
                <tr key={org.id} className="hover:bg-gray-50/50">
                  <td className="px-6 py-3 font-medium text-gray-900">{org.name}</td>
                  <td className="px-6 py-3 font-mono text-xs text-gray-500">{org.slug}</td>
                  <td className="px-6 py-3 text-gray-600">{org.agent_count}</td>
                  <td className="px-6 py-3 text-gray-500 whitespace-nowrap">-</td>
                  <td className="px-6 py-3">
                    {deleteTarget?.id === org.id ? (
                      <div className="flex items-center gap-2">
                        <button
                          onClick={() => deleteMut.mutate(org.id)}
                          className="rounded bg-red-600 px-2 py-1 text-xs text-white hover:bg-red-700"
                        >
                          Confirm
                        </button>
                        <button
                          onClick={() => setDeleteTarget(null)}
                          className="rounded bg-gray-200 px-2 py-1 text-xs text-gray-700 hover:bg-gray-300"
                        >
                          Cancel
                        </button>
                      </div>
                    ) : (
                      <button
                        onClick={() => setDeleteTarget(org)}
                        className="text-xs text-red-600 hover:underline"
                      >
                        Delete
                      </button>
                    )}
                  </td>
                </tr>
              ))}
              {!isLoading && orgs.length === 0 && (
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400">No organizations found.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Create Org Modal */}
      {showCreate && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowCreate(false)}>
          <div className="bg-white rounded-xl shadow-xl w-full max-w-md p-6" onClick={e => e.stopPropagation()}>
            <h2 className="text-lg font-bold text-gray-900">Create Organization</h2>
            <form onSubmit={handleCreate} className="mt-4 space-y-3">
              <div>
                <label className="block text-xs font-medium text-gray-500 mb-1">Organization Name</label>
                <input
                  value={orgName}
                  onChange={e => {
                    setOrgName(e.target.value)
                    setOrgSlug(e.target.value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, ''))
                  }}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                  placeholder="e.g., Production"
                  required
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 mb-1">Slug</label>
                <input
                  value={orgSlug}
                  onChange={e => setOrgSlug(e.target.value)}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm font-mono focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                  placeholder="e.g., production"
                  required
                />
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

// ================================================================
// User Groups Tab
// ================================================================

function UserGroupsTab() {
  const queryClient = useQueryClient()
  const [showModal, setShowModal] = useState(false)
  const [editingGroup, setEditingGroup] = useState<UserGroup | null>(null)
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const { data: groupsData, isLoading } = useQuery({
    queryKey: ['groups'],
    queryFn: () => api.getGroups(),
  })
  const groups = (groupsData?.data || []) as (UserGroup & { members?: User[] })[]

  const { data: permsData } = useQuery({
    queryKey: ['permissions'],
    queryFn: () => api.getPermissions(),
  })
  const permissions = (permsData?.data || []) as PermissionDef[]

  const { data: orgsData } = useQuery({
    queryKey: ['organizations'],
    queryFn: () => api.getOrganizations(),
  })
  const orgs = (orgsData?.data || []) as Organization[]

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deleteGroup(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      setDeleteId(null)
    },
  })

  const permsByCategory = useMemo(() => {
    const map: Record<string, PermissionDef[]> = {}
    permissions.forEach(p => {
      if (!map[p.category]) map[p.category] = []
      map[p.category].push(p)
    })
    return map
  }, [permissions])

  const openCreate = () => {
    setEditingGroup(null)
    setShowModal(true)
  }

  const openEdit = (g: UserGroup) => {
    setEditingGroup(g)
    setShowModal(true)
  }

  const toggleExpand = (id: string) => {
    setExpandedId(prev => prev === id ? null : id)
  }

  return (
    <div className="mt-6">
      <div className="flex items-center justify-between mb-4">
        <p className="text-sm text-gray-500">{groups.length} group(s) with granular permissions</p>
        <button
          onClick={openCreate}
          className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
        >
          Create Group
        </button>
      </div>

      <div className="rounded-xl border border-gray-200 bg-white shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 bg-gray-50/50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-500">Name</th>
                <th className="px-6 py-3 font-medium text-gray-500">Description</th>
                <th className="px-6 py-3 font-medium text-gray-500">Permissions</th>
                <th className="px-6 py-3 font-medium text-gray-500">Members</th>
                <th className="px-6 py-3 font-medium text-gray-500">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading && (
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400">Loading...</td></tr>
              )}
              {!isLoading && groups.map(group => {
                const isExpanded = expandedId === group.id
                const memberCount = group.members?.length || 0
                return (
                  <tr key={group.id} className="group">
                    <td colSpan={5} className="p-0">
                      {/* Main row */}
                      <div
                        className="flex items-center hover:bg-gray-50/50 cursor-pointer"
                        onClick={() => toggleExpand(group.id)}
                      >
                        <div className="px-6 py-3 font-medium text-gray-900 w-48 flex items-center gap-2">
                          <svg className={`h-4 w-4 text-gray-400 transition-transform ${isExpanded ? 'rotate-90' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                          </svg>
                          {group.name}
                        </div>
                        <div className="px-6 py-3 text-gray-600 flex-1 truncate">{group.description}</div>
                        <div className="px-6 py-3 w-32">
                          <span className="inline-flex rounded-full bg-fibratus-50 text-fibratus-700 px-2 py-0.5 text-xs font-medium">
                            {(group.permissions || []).length} perms
                          </span>
                        </div>
                        <div className="px-6 py-3 w-32">
                          <span className="inline-flex rounded-full bg-gray-100 text-gray-600 px-2 py-0.5 text-xs font-medium">
                            {memberCount} member{memberCount !== 1 ? 's' : ''}
                          </span>
                        </div>
                        <div className="px-6 py-3 w-40" onClick={e => e.stopPropagation()}>
                          <div className="flex items-center gap-3">
                            <button
                              onClick={() => openEdit(group)}
                              className="text-xs text-fibratus-600 hover:underline"
                            >
                              Edit
                            </button>
                            {deleteId === group.id ? (
                              <div className="flex items-center gap-2">
                                <button
                                  onClick={() => deleteMut.mutate(group.id)}
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
                                onClick={() => setDeleteId(group.id)}
                                className="text-xs text-red-600 hover:underline"
                              >
                                Delete
                              </button>
                            )}
                          </div>
                        </div>
                      </div>

                      {/* Expanded permissions detail */}
                      {isExpanded && (
                        <div className="border-t border-gray-100 bg-gray-50/30 px-8 py-5 space-y-4">
                          <div>
                            <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-3">Permissions</h4>
                            {Object.keys(permsByCategory).length === 0 ? (
                              <p className="text-xs text-gray-400">No permissions defined.</p>
                            ) : (
                              <div className="grid grid-cols-2 gap-x-8 gap-y-3 lg:grid-cols-3">
                                {Object.entries(permsByCategory).map(([category, perms]) => {
                                  const activePerms = perms.filter(p => (group.permissions || []).includes(p.id))
                                  if (activePerms.length === 0) return null
                                  return (
                                    <div key={category}>
                                      <p className="text-xs font-medium text-gray-700 mb-1.5">{category}</p>
                                      <div className="flex flex-wrap gap-1.5">
                                        {activePerms.map(p => (
                                          <span
                                            key={p.id}
                                            className={`inline-flex rounded-full px-2 py-0.5 text-[10px] font-medium ${permBadgeClasses(category)}`}
                                          >
                                            {p.name}
                                          </span>
                                        ))}
                                      </div>
                                    </div>
                                  )
                                })}
                                {permissions.length > 0 && (group.permissions || []).length === 0 && (
                                  <p className="text-xs text-gray-400 col-span-full">No permissions assigned to this group.</p>
                                )}
                              </div>
                            )}
                          </div>

                          {/* Org restrictions */}
                          <div>
                            <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-2">Organization Access</h4>
                            <div className="flex flex-wrap gap-1.5">
                              {(group.org_restrictions || []).length === 0 ? (
                                <span className="inline-flex rounded-full bg-teal-50 text-teal-700 px-2.5 py-0.5 text-[10px] font-medium">
                                  All organizations
                                </span>
                              ) : (
                                (group.org_restrictions || []).map(id => (
                                  <span key={id} className="inline-flex rounded-full bg-teal-50 text-teal-700 px-2.5 py-0.5 text-[10px] font-medium">
                                    {orgs.find(o => o.id === id)?.name || id}
                                  </span>
                                ))
                              )}
                            </div>
                          </div>
                        </div>
                      )}
                    </td>
                  </tr>
                )
              })}
              {!isLoading && groups.length === 0 && (
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400">No groups yet. Create one to assign granular permissions.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Create/Edit modal */}
      {showModal && (
        <GroupModal
          group={editingGroup}
          permissions={permissions}
          permsByCategory={permsByCategory}
          orgs={orgs}
          onClose={() => { setShowModal(false); setEditingGroup(null) }}
        />
      )}
    </div>
  )
}

// ================================================================
// Group Create/Edit Modal
// ================================================================

function GroupModal({
  group,
  permissions: _permissions,
  permsByCategory,
  orgs,
  onClose,
}: {
  group: UserGroup | null
  permissions: PermissionDef[]
  permsByCategory: Record<string, PermissionDef[]>
  orgs: Organization[]
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const isEdit = !!group

  const [name, setName] = useState(group?.name || '')
  const [description, setDescription] = useState(group?.description || '')
  const [selectedPerms, setSelectedPerms] = useState<Set<string>>(new Set(group?.permissions || []))
  const [allOrgs, setAllOrgs] = useState(!group || (group.org_restrictions || []).length === 0)
  const [selectedOrgIds, setSelectedOrgIds] = useState<Set<string>>(new Set(group?.org_restrictions || []))
  const [error, setError] = useState('')

  const togglePerm = (id: string) => {
    setSelectedPerms(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const toggleCategory = (category: string) => {
    const permsInCat = permsByCategory[category] || []
    const allSelected = permsInCat.every(p => selectedPerms.has(p.id))
    setSelectedPerms(prev => {
      const next = new Set(prev)
      permsInCat.forEach(p => {
        if (allSelected) next.delete(p.id)
        else next.add(p.id)
      })
      return next
    })
  }

  const toggleOrg = (id: string) => {
    setSelectedOrgIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const createMut = useMutation({
    mutationFn: (data: { name: string; description: string; permissions: string[]; org_restrictions: string[] }) =>
      api.createGroup(data),
    onSuccess: (res) => {
      if (res.error) { setError(res.error.message); return }
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      onClose()
    },
    onError: () => setError('Failed to create group.'),
  })

  const updateMut = useMutation({
    mutationFn: (data: { name: string; description: string; permissions: string[]; org_restrictions: string[] }) =>
      api.updateGroup(group!.id, data),
    onSuccess: (res) => {
      if (res.error) { setError(res.error.message); return }
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      onClose()
    },
    onError: () => setError('Failed to update group.'),
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    if (!name.trim()) { setError('Name is required.'); return }
    const data = {
      name: name.trim(),
      description: description.trim(),
      permissions: Array.from(selectedPerms),
      org_restrictions: allOrgs ? [] : Array.from(selectedOrgIds),
    }
    if (isEdit) updateMut.mutate(data)
    else createMut.mutate(data)
  }

  const isPending = createMut.isPending || updateMut.isPending

  return (
    <div className="fixed inset-0 bg-black/50 flex items-start justify-center z-50 overflow-y-auto py-8" onClick={onClose}>
      <div className="bg-white rounded-xl shadow-xl w-full max-w-2xl" onClick={e => e.stopPropagation()}>
        <div className="border-b border-gray-200 px-6 py-4 flex items-center justify-between">
          <h2 className="text-lg font-bold text-gray-900">{isEdit ? 'Edit Group' : 'Create Group'}</h2>
          <button onClick={onClose} className="rounded-lg p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600">
            <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-6 space-y-5 max-h-[calc(100vh-12rem)] overflow-y-auto">
          {/* Name */}
          <div>
            <label className="block text-xs font-medium text-gray-500 mb-1">Name</label>
            <input
              value={name}
              onChange={e => setName(e.target.value)}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
              placeholder="e.g. SOC Analysts"
              required
            />
          </div>

          {/* Description */}
          <div>
            <label className="block text-xs font-medium text-gray-500 mb-1">Description</label>
            <input
              value={description}
              onChange={e => setDescription(e.target.value)}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
              placeholder="What this group is for"
            />
          </div>

          {/* Permissions checklist */}
          <div>
            <label className="block text-xs font-medium text-gray-500 mb-3">Permissions</label>
            {Object.keys(permsByCategory).length === 0 ? (
              <p className="text-xs text-gray-400">No permissions available. The server has not returned permission definitions yet.</p>
            ) : (
              <div className="space-y-4">
                {Object.entries(permsByCategory).map(([category, perms]) => {
                  const allChecked = perms.every(p => selectedPerms.has(p.id))
                  const someChecked = perms.some(p => selectedPerms.has(p.id))
                  return (
                    <div key={category} className="rounded-lg border border-gray-200 p-3">
                      <label className="flex items-center gap-2 cursor-pointer mb-2">
                        <input
                          type="checkbox"
                          checked={allChecked}
                          ref={el => { if (el) el.indeterminate = someChecked && !allChecked }}
                          onChange={() => toggleCategory(category)}
                          className="rounded border-gray-300 text-fibratus-600 focus:ring-fibratus-500"
                        />
                        <span className="text-xs font-semibold text-gray-700">{category}</span>
                      </label>
                      <div className="grid grid-cols-2 gap-x-4 gap-y-1.5 ml-6">
                        {perms.map(p => (
                          <label key={p.id} className="flex items-center gap-2 cursor-pointer">
                            <input
                              type="checkbox"
                              checked={selectedPerms.has(p.id)}
                              onChange={() => togglePerm(p.id)}
                              className="rounded border-gray-300 text-fibratus-600 focus:ring-fibratus-500"
                            />
                            <span className="text-xs text-gray-600">{p.name}</span>
                          </label>
                        ))}
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </div>

          {/* Org Restrictions */}
          <div>
            <label className="block text-xs font-medium text-gray-500 mb-2">Organization Access</label>
            <label className="flex items-center gap-2 cursor-pointer mb-3">
              <input
                type="checkbox"
                checked={allOrgs}
                onChange={() => {
                  setAllOrgs(!allOrgs)
                  if (!allOrgs) setSelectedOrgIds(new Set())
                }}
                className="rounded border-gray-300 text-fibratus-600 focus:ring-fibratus-500"
              />
              <span className="text-xs font-medium text-gray-700">All organizations</span>
            </label>
            {!allOrgs && (
              <div className="rounded-lg border border-gray-200 p-3 space-y-1.5 max-h-40 overflow-y-auto">
                {orgs.length === 0 ? (
                  <p className="text-xs text-gray-400">No organizations available.</p>
                ) : (
                  orgs.map(org => (
                    <label key={org.id} className="flex items-center gap-2 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={selectedOrgIds.has(org.id)}
                        onChange={() => toggleOrg(org.id)}
                        className="rounded border-gray-300 text-fibratus-600 focus:ring-fibratus-500"
                      />
                      <span className="text-xs text-gray-600">{org.name}</span>
                    </label>
                  ))
                )}
              </div>
            )}
          </div>

          {error && <p className="text-xs text-red-600">{error}</p>}

          <div className="flex justify-end gap-2 pt-2 border-t border-gray-100">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isPending}
              className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
            >
              {isPending ? 'Saving...' : isEdit ? 'Update Group' : 'Create Group'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
