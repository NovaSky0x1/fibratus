import { useState, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Organization, type Account, type User } from '../../lib/api'
import { useTableSort } from '../../hooks/useTableSort'
import SortableHeader from '../SortableHeader'

export default function OrganizationsTab() {
  const queryClient = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [orgName, setOrgName] = useState('')
  const [orgSlug, setOrgSlug] = useState('')
  const [error, setError] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<Organization | null>(null)
  const [search, setSearch] = useState('')
  const [filterAccountId, setFilterAccountId] = useState<string | null>(null)

  const { data: currentUserData } = useQuery({
    queryKey: ['current-user'],
    queryFn: () => api.getCurrentUser(),
  })
  const currentUser = currentUserData?.data as User | undefined
  const isRoot = currentUser?.role === 'root'

  // For admin: org-scoped list
  const { data: orgsData, isLoading: orgsLoading } = useQuery({
    queryKey: ['organizations'],
    queryFn: () => api.getOrganizations(),
    enabled: !isRoot,
  })

  // For root: cross-account org list
  const { data: accountsData } = useQuery({
    queryKey: ['admin-accounts'],
    queryFn: () => api.adminGetAccounts(),
    enabled: isRoot,
  })
  const accounts = (accountsData?.data || []) as Account[]

  const { data: allOrgsData, isLoading: allOrgsLoading } = useQuery({
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
    enabled: isRoot && accounts.length > 0,
  })

  const orgs = isRoot
    ? (allOrgsData || []) as (Organization & { account_name?: string })[]
    : ((orgsData?.data || []) as Organization[])
  const isLoading = isRoot ? allOrgsLoading : orgsLoading

  const filtered = useMemo(() => {
    let list = filterAccountId
      ? orgs.filter(o => o.account_id === filterAccountId)
      : orgs
    if (search.trim()) {
      const q = search.toLowerCase()
      list = list.filter(o =>
        o.name.toLowerCase().includes(q) ||
        o.slug.toLowerCase().includes(q) ||
        ((o as Organization & { account_name?: string }).account_name || '').toLowerCase().includes(q)
      )
    }
    return list
  }, [orgs, filterAccountId, search])

  const { sorted: sortedOrgs, sort: orgSort, toggleSort: toggleOrgSort } = useTableSort<Organization & { account_name?: string }>(filtered, 'name', 'asc')

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
      queryClient.invalidateQueries({ queryKey: ['admin-all-orgs'] })
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
    },
    onError: () => setError('Failed to create organization'),
  })

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deleteOrganization(id),
    onSuccess: () => {
      setDeleteTarget(null)
      queryClient.invalidateQueries({ queryKey: ['organizations'] })
      queryClient.invalidateQueries({ queryKey: ['admin-all-orgs'] })
      queryClient.invalidateQueries({ queryKey: ['admin-accounts'] })
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
              onClick={() => setFilterAccountId(null)}
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
                {isRoot && <SortableHeader label="Account" sortKey="account_name" sort={orgSort} onSort={toggleOrgSort} />}
                <SortableHeader label="Slug" sortKey="slug" sort={orgSort} onSort={toggleOrgSort} />
                <SortableHeader label="Agents" sortKey="agent_count" sort={orgSort} onSort={toggleOrgSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {isLoading && (
                <tr><td colSpan={isRoot ? 5 : 4} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">Loading...</td></tr>
              )}
              {!isLoading && sortedOrgs.map(org => (
                <tr key={org.id} className="hover:bg-gray-50/50 dark:hover:bg-slate-700/50">
                  <td className="px-6 py-3 font-medium text-gray-900 dark:text-slate-100">{org.name}</td>
                  {isRoot && (
                    <td className="px-6 py-3 text-gray-600 dark:text-slate-400">
                      <button
                        onClick={() => setFilterAccountId(org.account_id)}
                        className="text-fibratus-600 hover:underline"
                      >
                        {(org as Organization & { account_name?: string }).account_name || '-'}
                      </button>
                    </td>
                  )}
                  <td className="px-6 py-3 font-mono text-xs text-gray-500">{org.slug}</td>
                  <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{org.agent_count}</td>
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
                          className="rounded bg-gray-200 dark:bg-slate-600 px-2 py-1 text-xs text-gray-700 dark:text-slate-300 hover:bg-gray-300 dark:hover:bg-slate-500"
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
              {!isLoading && filtered.length === 0 && (
                <tr><td colSpan={isRoot ? 5 : 4} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">No organizations found.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Create Org Modal */}
      {showCreate && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowCreate(false)}>
          <div className="bg-white dark:bg-slate-800 rounded-xl shadow-xl w-full max-w-md p-6" onClick={e => e.stopPropagation()}>
            <h2 className="text-lg font-bold text-gray-900 dark:text-slate-100">Create Organization</h2>
            <form onSubmit={handleCreate} className="mt-4 space-y-3">
              <div>
                <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Organization Name</label>
                <input
                  value={orgName}
                  onChange={e => {
                    setOrgName(e.target.value)
                    setOrgSlug(e.target.value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, ''))
                  }}
                  className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                  placeholder="e.g., Production"
                  required
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Slug</label>
                <input
                  value={orgSlug}
                  onChange={e => setOrgSlug(e.target.value)}
                  className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm font-mono text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none"
                  placeholder="e.g., production"
                  required
                />
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
