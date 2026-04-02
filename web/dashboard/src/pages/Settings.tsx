import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api'

interface EnrollmentToken {
  id: string
  name: string
  org_id: string
  org_name: string
  max_uses: number
  uses_count: number
  expires_at: string
  created_by: string
  created_at: string
}

export default function Settings() {
  const queryClient = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [tokenName, setTokenName] = useState('')
  const [maxUses, setMaxUses] = useState(50)
  const [createdToken, setCreatedToken] = useState<string | null>(null)

  const { data } = useQuery({
    queryKey: ['enrollment-tokens'],
    queryFn: () => api.getOrgResource<EnrollmentToken[]>('/enrollment-tokens'),
  })

  const createMutation = useMutation({
    mutationFn: async () => {
      const token = localStorage.getItem('fleet_token')
      const orgId = localStorage.getItem('fleet_org_id')
      const res = await fetch(`/api/v1/orgs/${orgId}/enrollment-tokens`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`,
        },
        body: JSON.stringify({
          name: tokenName || 'enrollment-token',
          max_uses: maxUses,
          expires_in: 86400000000000, // 24h in nanoseconds (Go duration)
        }),
      })
      return res.json()
    },
    onSuccess: (data: { data?: EnrollmentToken }) => {
      if (data.data) {
        setCreatedToken(data.data.id)
        setShowCreate(false)
        setTokenName('')
        queryClient.invalidateQueries({ queryKey: ['enrollment-tokens'] })
      }
    },
  })

  const tokens = (data?.data || []) as EnrollmentToken[]
  const serverUrl = window.location.origin

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900">Settings</h1>

      {/* Enrollment Tokens Section */}
      <div className="mt-8">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold text-gray-900">Enrollment Tokens</h2>
            <p className="mt-1 text-sm text-gray-500">
              Create tokens to enroll new agents. Each token is scoped to this organization.
            </p>
          </div>
          <button
            onClick={() => { setShowCreate(!showCreate); setCreatedToken(null) }}
            className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
          >
            Create Token
          </button>
        </div>

        {/* Created token display */}
        {createdToken && (
          <div className="mt-4 rounded-xl border border-emerald-200 bg-emerald-50 p-6">
            <h3 className="text-sm font-semibold text-emerald-800">Token Created</h3>
            <p className="mt-1 text-xs text-emerald-600">Copy this token — it won't be shown again in full.</p>
            <div className="mt-3 flex items-center gap-2">
              <code className="flex-1 rounded-lg bg-white border border-emerald-200 px-4 py-2.5 font-mono text-sm text-gray-900 select-all">
                {createdToken}
              </code>
              <button
                onClick={() => navigator.clipboard.writeText(createdToken)}
                className="rounded-lg bg-emerald-600 px-3 py-2.5 text-sm font-medium text-white hover:bg-emerald-700"
              >
                Copy
              </button>
            </div>
            <div className="mt-4 rounded-lg bg-white border border-emerald-200 p-4">
              <p className="text-xs font-medium text-gray-500 mb-2">Run on the Windows endpoint:</p>
              <code className="text-sm font-mono text-gray-900 select-all">
                fibratus enroll --token {createdToken} --server {serverUrl}
              </code>
            </div>
          </div>
        )}

        {/* Create form */}
        {showCreate && (
          <div className="mt-4 rounded-xl border border-gray-200 bg-white p-6 shadow-sm">
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700">Name</label>
                <input
                  type="text"
                  value={tokenName}
                  onChange={(e) => setTokenName(e.target.value)}
                  placeholder="e.g., workstation-rollout"
                  className="mt-1 w-full rounded-lg border border-gray-300 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700">Max Uses</label>
                <input
                  type="number"
                  value={maxUses}
                  onChange={(e) => setMaxUses(Number(e.target.value))}
                  min={1}
                  className="mt-1 w-full rounded-lg border border-gray-300 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none"
                />
              </div>
            </div>
            <div className="mt-4">
              <button
                onClick={() => createMutation.mutate()}
                disabled={createMutation.isPending}
                className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
              >
                {createMutation.isPending ? 'Creating...' : 'Create Token'}
              </button>
            </div>
          </div>
        )}

        {/* Token list */}
        <div className="mt-4 rounded-xl border border-gray-200 bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-100 bg-gray-50/50">
                <tr>
                  <th className="px-6 py-3 font-medium text-gray-500">Token ID</th>
                  <th className="px-6 py-3 font-medium text-gray-500">Name</th>
                  <th className="px-6 py-3 font-medium text-gray-500">Uses</th>
                  <th className="px-6 py-3 font-medium text-gray-500">Expires</th>
                  <th className="px-6 py-3 font-medium text-gray-500">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {tokens.map((t) => {
                  const expired = new Date(t.expires_at) < new Date()
                  const exhausted = t.uses_count >= t.max_uses
                  const status = expired ? 'Expired' : exhausted ? 'Exhausted' : 'Active'
                  const statusColor = status === 'Active' ? 'text-emerald-700 bg-emerald-50' : 'text-gray-500 bg-gray-50'

                  return (
                    <tr key={t.id} className="hover:bg-gray-50/50">
                      <td className="px-6 py-3 font-mono text-xs text-gray-600">
                        {t.id.slice(0, 24)}...
                      </td>
                      <td className="px-6 py-3 text-gray-900">{t.name}</td>
                      <td className="px-6 py-3 text-gray-600">{t.uses_count} / {t.max_uses}</td>
                      <td className="px-6 py-3 text-gray-500">
                        {new Date(t.expires_at).toLocaleDateString()}
                      </td>
                      <td className="px-6 py-3">
                        <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${statusColor}`}>
                          {status}
                        </span>
                      </td>
                    </tr>
                  )
                })}
                {tokens.length === 0 && (
                  <tr>
                    <td colSpan={5} className="px-6 py-12 text-center text-gray-400">
                      No enrollment tokens yet. Create one to start enrolling agents.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  )
}
