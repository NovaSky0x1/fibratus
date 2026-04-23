import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type PendingUser } from '../lib/api'
import { useState } from 'react'

export default function PendingUsers() {
  const qc = useQueryClient()
  const [error, setError] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['admin', 'pending-users'],
    queryFn: () => api.adminListPendingUsers(),
    refetchInterval: 30_000,
  })

  const approve = useMutation({
    mutationFn: (id: string) => api.adminApprovePendingUser(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin', 'pending-users'] }),
    onError: (e: Error) => setError(e.message),
  })

  const reject = useMutation({
    mutationFn: (id: string) => api.adminRejectPendingUser(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin', 'pending-users'] }),
    onError: (e: Error) => setError(e.message),
  })

  const users: PendingUser[] = (data?.data as PendingUser[] | undefined) ?? []

  return (
    <div className="p-6">
      <div className="mb-6">
        <h1 className="text-2xl font-semibold text-gray-900 dark:text-slate-100">Pending Signups</h1>
        <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
          New signups are held until a root admin approves them. Rejected users cannot log in.
        </p>
      </div>

      {error && (
        <div className="mb-4 rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800/50 px-4 py-3 text-sm text-red-700 dark:text-red-400">
          {error}
        </div>
      )}

      {data?.error && (
        <div className="mb-4 rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800/50 px-4 py-3 text-sm text-red-700 dark:text-red-400">
          {data.error.message}
        </div>
      )}

      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 overflow-hidden">
        {isLoading ? (
          <div className="p-8 text-center text-sm text-gray-500 dark:text-slate-400">Loading…</div>
        ) : users.length === 0 ? (
          <div className="p-8 text-center text-sm text-gray-500 dark:text-slate-400">
            No signups awaiting approval.
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-slate-900/40 text-left text-xs uppercase text-gray-500 dark:text-slate-400">
              <tr>
                <th className="px-4 py-3 font-medium">Email</th>
                <th className="px-4 py-3 font-medium">Name</th>
                <th className="px-4 py-3 font-medium">Account ID</th>
                <th className="px-4 py-3 font-medium">Requested</th>
                <th className="px-4 py-3 font-medium text-right">Action</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {users.map((u) => (
                <tr key={u.id} className="text-gray-900 dark:text-slate-100">
                  <td className="px-4 py-3 font-mono text-xs">{u.email}</td>
                  <td className="px-4 py-3">{u.name}</td>
                  <td className="px-4 py-3 font-mono text-[11px] text-gray-500 dark:text-slate-400">{u.account_id}</td>
                  <td className="px-4 py-3 text-gray-500 dark:text-slate-400">
                    {new Date(u.created_at).toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="inline-flex gap-2">
                      <button
                        disabled={approve.isPending || reject.isPending}
                        onClick={() => approve.mutate(u.id)}
                        className="rounded-md bg-green-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-green-700 disabled:opacity-50"
                      >
                        Approve
                      </button>
                      <button
                        disabled={approve.isPending || reject.isPending}
                        onClick={() => {
                          if (confirm(`Reject signup for ${u.email}? They will not be able to log in.`)) {
                            reject.mutate(u.id)
                          }
                        }}
                        className="rounded-md bg-red-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-700 disabled:opacity-50"
                      >
                        Reject
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
