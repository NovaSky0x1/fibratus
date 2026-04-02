import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Rule, type ApiResponse } from '../lib/api'
import SeverityBadge from '../components/SeverityBadge'
import ConfirmDialog from '../components/ConfirmDialog'

export default function Rules() {
  const queryClient = useQueryClient()
  const [showUpload, setShowUpload] = useState(false)
  const [yamlContent, setYamlContent] = useState('')
  const [uploadError, setUploadError] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<Rule | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['rules'],
    queryFn: () => api.getRules(),
  })

  const uploadMutation = useMutation({
    mutationFn: (yaml: string) => api.createRule(yaml),
    onSuccess: (res: ApiResponse<Rule>) => {
      if (res.error) {
        setUploadError(res.error.message)
        return
      }
      setShowUpload(false)
      setYamlContent('')
      setUploadError('')
      queryClient.invalidateQueries({ queryKey: ['rules'] })
    },
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      api.updateRule(id, { enabled }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['rules'] })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteRule(id),
    onSuccess: () => {
      setDeleteTarget(null)
      queryClient.invalidateQueries({ queryKey: ['rules'] })
    },
  })

  const rules = (data?.data || []) as Rule[]
  const total = data?.meta?.total ?? rules.length

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Rules</h1>
          <p className="mt-1 text-sm text-gray-500">
            {total} rule(s) managed by fleet server
          </p>
        </div>
        <button
          onClick={() => setShowUpload(!showUpload)}
          className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
        >
          {showUpload ? 'Cancel' : 'Upload Rule'}
        </button>
      </div>

      {/* Upload panel */}
      {showUpload && (
        <div className="mt-6 rounded-xl border border-gray-200 bg-white p-6 shadow-sm">
          <h3 className="text-sm font-medium text-gray-900">Upload YAML Rule</h3>
          <p className="mt-1 text-xs text-gray-500">
            Paste a Fibratus detection rule in YAML format. Same format as
            rules/*.yml files.
          </p>
          {uploadError && (
            <div className="mt-3 rounded-lg border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700">
              {uploadError}
            </div>
          )}
          <textarea
            value={yamlContent}
            onChange={(e) => setYamlContent(e.target.value)}
            rows={15}
            className="mt-3 w-full rounded-lg border border-gray-300 px-4 py-3 font-mono text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
            placeholder={'name: My Detection Rule\nid: 550e8400-e29b-41d4-a716-446655440000\nversion: 1.0.0\ndescription: Detects suspicious activity\ncondition: >\n  spawn_process and ps.name imatches \'*.exe\'\noutput: "Suspicious process: %ps.name"\nseverity: high\nlabels:\n  tactic.id: TA0002\n  tactic.name: Execution'}
          />
          <div className="mt-3 flex gap-3">
            <button
              onClick={() => uploadMutation.mutate(yamlContent)}
              disabled={!yamlContent.trim() || uploadMutation.isPending}
              className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
            >
              {uploadMutation.isPending ? 'Uploading...' : 'Upload'}
            </button>
            <label className="flex cursor-pointer items-center rounded-lg border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50">
              Import from file
              <input
                type="file"
                accept=".yml,.yaml"
                className="hidden"
                onChange={(e) => {
                  const file = e.target.files?.[0]
                  if (file) {
                    file.text().then(setYamlContent)
                  }
                }}
              />
            </label>
          </div>
        </div>
      )}

      {/* Rules table */}
      <div className="mt-6 rounded-xl border border-gray-200 bg-white shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 bg-gray-50/50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-500">Name</th>
                <th className="px-6 py-3 font-medium text-gray-500">Severity</th>
                <th className="px-6 py-3 font-medium text-gray-500">MITRE</th>
                <th className="px-6 py-3 font-medium text-gray-500">Version</th>
                <th className="px-6 py-3 font-medium text-gray-500">Status</th>
                <th className="px-6 py-3 font-medium text-gray-500">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading && (
                <tr>
                  <td colSpan={6} className="px-6 py-12 text-center text-gray-400">
                    Loading...
                  </td>
                </tr>
              )}
              {!isLoading &&
                rules.map((rule) => (
                  <tr key={rule.id} className="hover:bg-gray-50/50">
                    <td className="px-6 py-3">
                      <div className="font-medium text-gray-900">{rule.name}</div>
                      {rule.description && (
                        <div className="mt-0.5 text-xs text-gray-500 line-clamp-1">
                          {rule.description}
                        </div>
                      )}
                    </td>
                    <td className="px-6 py-3">
                      <SeverityBadge severity={rule.severity} />
                    </td>
                    <td className="px-6 py-3 text-gray-600">
                      {rule.labels?.['technique.id'] && (
                        <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-mono">
                          {rule.labels['technique.id']}
                        </span>
                      )}
                    </td>
                    <td className="px-6 py-3 font-mono text-xs text-gray-500">
                      {rule.version}
                    </td>
                    <td className="px-6 py-3">
                      <span
                        className={
                          'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ' +
                          (rule.enabled
                            ? 'bg-emerald-50 text-emerald-700'
                            : 'bg-gray-100 text-gray-500')
                        }
                      >
                        {rule.enabled ? 'Active' : 'Disabled'}
                      </span>
                    </td>
                    <td className="px-6 py-3">
                      <div className="flex items-center gap-3">
                        <button
                          onClick={() =>
                            toggleMutation.mutate({
                              id: rule.id,
                              enabled: !rule.enabled,
                            })
                          }
                          className="text-xs font-medium text-fibratus-600 hover:text-fibratus-800"
                        >
                          {rule.enabled ? 'Disable' : 'Enable'}
                        </button>
                        <button
                          onClick={() => setDeleteTarget(rule)}
                          className="text-xs font-medium text-red-600 hover:text-red-800"
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              {!isLoading && rules.length === 0 && (
                <tr>
                  <td colSpan={6} className="px-6 py-12 text-center text-gray-400">
                    No rules configured. Upload detection rules above.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Delete confirmation dialog */}
      <ConfirmDialog
        open={!!deleteTarget}
        title="Delete Rule"
        message={
          'Are you sure you want to delete the rule "' +
          (deleteTarget?.name || '') +
          '"? This action cannot be undone.'
        }
        confirmLabel="Delete"
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.id)
        }}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}
