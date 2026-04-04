import { useState, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type UserGroup, type PermissionDef, type User, type Organization } from '../lib/api'

// Category display order and colors
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

export default function Groups() {
  const queryClient = useQueryClient()
  const [showModal, setShowModal] = useState(false)
  const [editingGroup, setEditingGroup] = useState<UserGroup | null>(null)
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const { data: groupsData, isLoading } = useQuery({
    queryKey: ['groups'],
    queryFn: () => api.getGroups(),
  })
  const groups = (groupsData?.data || []) as UserGroup[]

  const { data: permsData } = useQuery({
    queryKey: ['permissions'],
    queryFn: () => api.getPermissions(),
  })
  const permissions = (permsData?.data || []) as PermissionDef[]

  const { data: usersData } = useQuery({
    queryKey: ['users'],
    queryFn: () => api.getUsers(),
  })
  const users = (usersData?.data || []) as User[]

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
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">User Groups</h1>
          <p className="mt-1 text-sm text-gray-500">{groups.length} group(s) with granular permissions</p>
        </div>
        <button
          onClick={openCreate}
          className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
        >
          Create Group
        </button>
      </div>

      {/* Groups table */}
      <div className="mt-6 rounded-xl border border-gray-200 bg-white shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 bg-gray-50/50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-500">Name</th>
                <th className="px-6 py-3 font-medium text-gray-500">Description</th>
                <th className="px-6 py-3 font-medium text-gray-500">Permissions</th>
                <th className="px-6 py-3 font-medium text-gray-500">Org Access</th>
                <th className="px-6 py-3 font-medium text-gray-500">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading && (
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400">Loading...</td></tr>
              )}
              {!isLoading && groups.map(group => {
                const isExpanded = expandedId === group.id
                const orgRestrictions = group.org_restrictions || []
                const orgLabel = orgRestrictions.length === 0
                  ? 'All orgs'
                  : `${orgRestrictions.length} org(s)`
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
                            {orgLabel}
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

                      {/* Expanded detail */}
                      {isExpanded && (
                        <GroupDetail
                          group={group}
                          permissions={permissions}
                          permsByCategory={permsByCategory}
                          users={users}
                          orgs={orgs}
                        />
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

// ─────────────────────────────────────────────
// Group Detail (expanded row)
// ─────────────────────────────────────────────

function GroupDetail({
  group,
  permissions,
  permsByCategory,
  users,
  orgs,
}: {
  group: UserGroup
  permissions: PermissionDef[]
  permsByCategory: Record<string, PermissionDef[]>
  users: User[]
  orgs: Organization[]
}) {
  const queryClient = useQueryClient()
  const [addingMember, setAddingMember] = useState(false)
  const [selectedUserId, setSelectedUserId] = useState('')

  // Build a lookup for which permission IDs this group has
  const groupPermSet = useMemo(() => new Set(group.permissions || []), [group.permissions])

  // Members: for now, we show all users who have groups (the backend tracks membership).
  // Since the User type doesn't have a groups field yet, we'll store members locally
  // after fetching from a group members endpoint. For now, we'll use a simple query.
  const { data: groupData } = useQuery({
    queryKey: ['group-detail', group.id],
    queryFn: () => api.getGroups(),
  })
  // Extract members from group data if available
  const groupDetail = ((groupData?.data || []) as (UserGroup & { members?: User[] })[]).find(g => g.id === group.id)
  const members = groupDetail?.members || []

  const nonMembers = users.filter(u => !members.some(m => m.id === u.id))

  const addMemberMut = useMutation({
    mutationFn: (userId: string) => api.addGroupMember(group.id, userId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      queryClient.invalidateQueries({ queryKey: ['group-detail', group.id] })
      setAddingMember(false)
      setSelectedUserId('')
    },
  })

  const removeMemberMut = useMutation({
    mutationFn: (userId: string) => api.removeGroupMember(group.id, userId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      queryClient.invalidateQueries({ queryKey: ['group-detail', group.id] })
    },
  })

  const orgRestrictions = group.org_restrictions || []
  const restrictedOrgNames = orgRestrictions.length === 0
    ? ['All organizations']
    : orgRestrictions.map(id => orgs.find(o => o.id === id)?.name || id)

  return (
    <div className="border-t border-gray-100 bg-gray-50/30 px-8 py-5 space-y-5">
      {/* Permissions by category */}
      <div>
        <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-3">Permissions</h4>
        {Object.keys(permsByCategory).length === 0 ? (
          <p className="text-xs text-gray-400">No permissions defined.</p>
        ) : (
          <div className="grid grid-cols-2 gap-x-8 gap-y-3 lg:grid-cols-3">
            {Object.entries(permsByCategory).map(([category, perms]) => {
              const activePerms = perms.filter(p => groupPermSet.has(p.id))
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
          {restrictedOrgNames.map((name, i) => (
            <span key={i} className="inline-flex rounded-full bg-teal-50 text-teal-700 px-2.5 py-0.5 text-[10px] font-medium">
              {name}
            </span>
          ))}
        </div>
      </div>

      {/* Members */}
      <div>
        <div className="flex items-center gap-3 mb-2">
          <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Members</h4>
          <button
            onClick={() => setAddingMember(!addingMember)}
            className="text-[10px] font-medium text-fibratus-600 hover:underline"
          >
            {addingMember ? 'Cancel' : '+ Add Member'}
          </button>
        </div>

        {addingMember && (
          <div className="flex items-center gap-2 mb-3">
            <select
              value={selectedUserId}
              onChange={e => setSelectedUserId(e.target.value)}
              className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
            >
              <option value="">Select a user...</option>
              {nonMembers.map(u => (
                <option key={u.id} value={u.id}>{u.name || u.email} ({u.role})</option>
              ))}
            </select>
            <button
              onClick={() => selectedUserId && addMemberMut.mutate(selectedUserId)}
              disabled={!selectedUserId || addMemberMut.isPending}
              className="rounded-lg bg-fibratus-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
            >
              {addMemberMut.isPending ? 'Adding...' : 'Add'}
            </button>
          </div>
        )}

        {members.length === 0 ? (
          <p className="text-xs text-gray-400">No members in this group.</p>
        ) : (
          <div className="space-y-1.5">
            {members.map((m: User) => (
              <div key={m.id} className="flex items-center justify-between rounded-lg bg-white border border-gray-200 px-3 py-2">
                <div className="flex items-center gap-3">
                  <span className="text-sm font-medium text-gray-900">{m.name || m.email}</span>
                  <span className="text-xs text-gray-500">{m.email}</span>
                  <span className="inline-flex rounded-full bg-gray-100 text-gray-600 px-2 py-0.5 text-[10px] font-medium capitalize">
                    {m.role}
                  </span>
                </div>
                <button
                  onClick={() => removeMemberMut.mutate(m.id)}
                  disabled={removeMemberMut.isPending}
                  className="text-xs text-red-600 hover:underline disabled:opacity-50"
                >
                  Remove
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// ─────────────────────────────────────────────
// Create / Edit Group Modal
// ─────────────────────────────────────────────

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
