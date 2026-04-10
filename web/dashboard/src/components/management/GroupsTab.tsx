import { useState, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Organization, type UserGroup, type User, type PermissionDef } from '../../lib/api'
import { useTableSort } from '../../hooks/useTableSort'
import SortableHeader from '../SortableHeader'

const categoryMeta: Record<string, { color: string; bg: string }> = {
  Pages:             { color: 'text-violet-700 dark:text-violet-400',  bg: 'bg-violet-50 dark:bg-violet-900/20' },
  Agents:            { color: 'text-blue-700 dark:text-blue-400',      bg: 'bg-blue-50 dark:bg-blue-900/20' },
  'Agent Detail':    { color: 'text-sky-700 dark:text-sky-400',        bg: 'bg-sky-50 dark:bg-sky-900/20' },
  Detections:        { color: 'text-amber-700 dark:text-amber-400',    bg: 'bg-amber-50 dark:bg-amber-900/20' },
  Events:            { color: 'text-cyan-700 dark:text-cyan-400',      bg: 'bg-cyan-50 dark:bg-cyan-900/20' },
  Rules:             { color: 'text-purple-700 dark:text-purple-400',  bg: 'bg-purple-50 dark:bg-purple-900/20' },
  'Active Response': { color: 'text-red-700 dark:text-red-400',        bg: 'bg-red-50 dark:bg-red-900/20' },
  'Response Actions': { color: 'text-rose-700 dark:text-rose-400',     bg: 'bg-rose-50 dark:bg-rose-900/20' },
  Captures:          { color: 'text-fuchsia-700 dark:text-fuchsia-400', bg: 'bg-fuchsia-50 dark:bg-fuchsia-900/20' },
  Telemetry:         { color: 'text-lime-700 dark:text-lime-400',      bg: 'bg-lime-50 dark:bg-lime-900/20' },
  Settings:          { color: 'text-gray-700 dark:text-slate-300',     bg: 'bg-gray-100 dark:bg-slate-700' },
  Enrollment:        { color: 'text-pink-700 dark:text-pink-400',      bg: 'bg-pink-50 dark:bg-pink-900/20' },
  'GitHub Sync':     { color: 'text-slate-700 dark:text-slate-300',    bg: 'bg-slate-100 dark:bg-slate-700' },
  'User Management': { color: 'text-emerald-700 dark:text-emerald-400', bg: 'bg-emerald-50 dark:bg-emerald-900/20' },
  Organizations:     { color: 'text-teal-700 dark:text-teal-400',      bg: 'bg-teal-50 dark:bg-teal-900/20' },
  Audit:             { color: 'text-orange-700 dark:text-orange-400',  bg: 'bg-orange-50 dark:bg-orange-900/20' },
}

function permBadgeClasses(category: string): string {
  const meta = categoryMeta[category] || { color: 'text-gray-700 dark:text-slate-300', bg: 'bg-gray-100 dark:bg-slate-700' }
  return `${meta.bg} ${meta.color}`
}

export default function GroupsTab() {
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

  const groupsWithCounts = useMemo(() =>
    groups.map(g => ({
      ...g,
      _permCount: (g.permissions || []).length,
      _memberCount: g.members?.length || 0,
    })),
  [groups])

  const { sorted: sortedGroups, sort: groupSort, toggleSort: toggleGroupSort } = useTableSort(groupsWithCounts, 'name', 'asc')

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
        <p className="text-sm text-gray-500 dark:text-slate-400">{groups.length} group(s) with granular permissions</p>
        <button
          onClick={openCreate}
          className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
        >
          Create Group
        </button>
      </div>

      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
              <tr>
                <SortableHeader label="Name" sortKey="name" sort={groupSort} onSort={toggleGroupSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Description</th>
                <SortableHeader label="Permissions" sortKey="_permCount" sort={groupSort} onSort={toggleGroupSort} />
                <SortableHeader label="Members" sortKey="_memberCount" sort={groupSort} onSort={toggleGroupSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {isLoading && (
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">Loading...</td></tr>
              )}
              {!isLoading && sortedGroups.map(group => {
                const isExpanded = expandedId === group.id
                const memberCount = group.members?.length || 0
                return (
                  <tr key={group.id} className="group">
                    <td colSpan={5} className="p-0">
                      <div
                        className="flex items-center hover:bg-gray-50/50 dark:hover:bg-slate-700/50 cursor-pointer"
                        onClick={() => toggleExpand(group.id)}
                      >
                        <div className="px-6 py-3 font-medium text-gray-900 dark:text-slate-100 w-48 flex items-center gap-2">
                          <svg className={`h-4 w-4 text-gray-400 dark:text-slate-500 transition-transform ${isExpanded ? 'rotate-90' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                          </svg>
                          {group.name}
                        </div>
                        <div className="px-6 py-3 text-gray-600 dark:text-slate-400 flex-1 truncate">{group.description}</div>
                        <div className="px-6 py-3 w-32">
                          <span className="inline-flex rounded-full bg-fibratus-50 text-fibratus-700 px-2 py-0.5 text-xs font-medium">
                            {(group.permissions || []).length} perms
                          </span>
                        </div>
                        <div className="px-6 py-3 w-32">
                          <span className="inline-flex rounded-full bg-gray-100 text-gray-600 dark:text-slate-400 px-2 py-0.5 text-xs font-medium">
                            {memberCount} member{memberCount !== 1 ? 's' : ''}
                          </span>
                        </div>
                        <div className="px-6 py-3 w-40" onClick={e => e.stopPropagation()}>
                          <div className="flex items-center gap-3">
                            <button onClick={() => openEdit(group)} className="text-xs text-fibratus-600 hover:underline">Edit</button>
                            {deleteId === group.id ? (
                              <div className="flex items-center gap-2">
                                <button onClick={() => deleteMut.mutate(group.id)} className="rounded bg-red-600 px-2 py-1 text-xs text-white hover:bg-red-700">Confirm</button>
                                <button onClick={() => setDeleteId(null)} className="rounded bg-gray-200 dark:bg-slate-600 px-2 py-1 text-xs text-gray-700 dark:text-slate-300 hover:bg-gray-300 dark:hover:bg-slate-500">Cancel</button>
                              </div>
                            ) : (
                              <button onClick={() => setDeleteId(group.id)} className="text-xs text-red-600 hover:underline">Delete</button>
                            )}
                          </div>
                        </div>
                      </div>

                      {isExpanded && (
                        <div className="border-t border-gray-100 dark:border-slate-700 bg-gray-50/30 dark:bg-slate-800/30 px-8 py-5 space-y-4">
                          <div>
                            <h4 className="text-xs font-semibold text-gray-500 dark:text-slate-400 uppercase tracking-wider mb-3">Permissions</h4>
                            {Object.keys(permsByCategory).length === 0 ? (
                              <p className="text-xs text-gray-400 dark:text-slate-500">No permissions defined.</p>
                            ) : (
                              <div className="grid grid-cols-2 gap-x-8 gap-y-3 lg:grid-cols-3">
                                {Object.entries(permsByCategory).map(([category, perms]) => {
                                  const activePerms = perms.filter(p => (group.permissions || []).includes(p.id))
                                  if (activePerms.length === 0) return null
                                  return (
                                    <div key={category}>
                                      <p className="text-xs font-medium text-gray-700 dark:text-slate-300 mb-1.5">{category}</p>
                                      <div className="flex flex-wrap gap-1.5">
                                        {activePerms.map(p => (
                                          <span key={p.id} className={`inline-flex rounded-full px-2 py-0.5 text-[10px] font-medium ${permBadgeClasses(category)}`}>
                                            {p.name}
                                          </span>
                                        ))}
                                      </div>
                                    </div>
                                  )
                                })}
                                {permissions.length > 0 && (group.permissions || []).length === 0 && (
                                  <p className="text-xs text-gray-400 dark:text-slate-500 col-span-full">No permissions assigned to this group.</p>
                                )}
                              </div>
                            )}
                          </div>
                          <div>
                            <h4 className="text-xs font-semibold text-gray-500 dark:text-slate-400 uppercase tracking-wider mb-2">Organization Access</h4>
                            <div className="flex flex-wrap gap-1.5">
                              {(group.org_restrictions || []).length === 0 ? (
                                <span className="inline-flex rounded-full bg-teal-50 text-teal-700 px-2.5 py-0.5 text-[10px] font-medium">All organizations</span>
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
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">No groups yet. Create one to assign granular permissions.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

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
      <div className="bg-white dark:bg-slate-800 rounded-xl shadow-xl w-full max-w-2xl" onClick={e => e.stopPropagation()}>
        <div className="border-b border-gray-200 dark:border-slate-700 px-6 py-4 flex items-center justify-between">
          <h2 className="text-lg font-bold text-gray-900 dark:text-slate-100">{isEdit ? 'Edit Group' : 'Create Group'}</h2>
          <button onClick={onClose} className="rounded-lg p-1 text-gray-400 dark:text-slate-500 hover:bg-gray-100 dark:hover:bg-slate-700 hover:text-gray-600 dark:hover:text-slate-300">
            <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-6 space-y-5 max-h-[calc(100vh-12rem)] overflow-y-auto">
          <div>
            <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Name</label>
            <input
              value={name}
              onChange={e => setName(e.target.value)}
              className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
              placeholder="e.g. SOC Analysts"
              required
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Description</label>
            <input
              value={description}
              onChange={e => setDescription(e.target.value)}
              className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
              placeholder="What this group is for"
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-3">Permissions</label>
            {Object.keys(permsByCategory).length === 0 ? (
              <p className="text-xs text-gray-400 dark:text-slate-500">No permissions available.</p>
            ) : (
              <div className="space-y-4">
                {Object.entries(permsByCategory).map(([category, perms]) => {
                  const allChecked = perms.every(p => selectedPerms.has(p.id))
                  const someChecked = perms.some(p => selectedPerms.has(p.id))
                  return (
                    <div key={category} className="rounded-lg border border-gray-200 dark:border-slate-700 p-3">
                      <label className="flex items-center gap-2 cursor-pointer mb-2">
                        <input
                          type="checkbox"
                          checked={allChecked}
                          ref={el => { if (el) el.indeterminate = someChecked && !allChecked }}
                          onChange={() => toggleCategory(category)}
                          className="rounded border-gray-300 text-fibratus-600 focus:ring-fibratus-500"
                        />
                        <span className="text-xs font-semibold text-gray-700 dark:text-slate-300">{category}</span>
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
                            <span className="text-xs text-gray-600 dark:text-slate-400">{p.name}</span>
                          </label>
                        ))}
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </div>

          <div>
            <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-2">Organization Access</label>
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
              <span className="text-xs font-medium text-gray-700 dark:text-slate-300">All organizations</span>
            </label>
            {!allOrgs && (
              <div className="rounded-lg border border-gray-200 dark:border-slate-700 p-3 space-y-1.5 max-h-40 overflow-y-auto">
                {orgs.length === 0 ? (
                  <p className="text-xs text-gray-400 dark:text-slate-500">No organizations available.</p>
                ) : (
                  orgs.map(org => (
                    <label key={org.id} className="flex items-center gap-2 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={selectedOrgIds.has(org.id)}
                        onChange={() => toggleOrg(org.id)}
                        className="rounded border-gray-300 text-fibratus-600 focus:ring-fibratus-500"
                      />
                      <span className="text-xs text-gray-600 dark:text-slate-400">{org.name}</span>
                    </label>
                  ))
                )}
              </div>
            )}
          </div>

          {error && <p className="text-xs text-red-600">{error}</p>}

          <div className="flex justify-end gap-2 pt-2 border-t border-gray-100 dark:border-slate-700">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700"
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
