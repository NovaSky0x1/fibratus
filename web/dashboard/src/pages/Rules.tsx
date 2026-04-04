import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { api, type Rule, type ApiResponse } from '../lib/api'
import SeverityBadge from '../components/SeverityBadge'
import SlidePanel from '../components/SlidePanel'
import ConfirmDialog from '../components/ConfirmDialog'

export default function Rules() {
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()
  const [showUpload, setShowUpload] = useState(false)
  const [yamlContent, setYamlContent] = useState('')
  const [uploadError, setUploadError] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<Rule | null>(null)
  const [editRule, setEditRule] = useState<Rule | null>(null)
  const [editYaml, setEditYaml] = useState('')
  const [editError, setEditError] = useState('')
  const [search, setSearch] = useState('')

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

  const editMutation = useMutation({
    mutationFn: ({ id, yaml }: { id: string; yaml: string }) => api.updateRuleYaml(id, yaml),
    onSuccess: (res: ApiResponse<Rule>) => {
      if (res.error) {
        setEditError(res.error.message)
        return
      }
      setEditRule(null)
      setEditYaml('')
      setEditError('')
      queryClient.invalidateQueries({ queryKey: ['rules'] })
    },
  })

  const allRules = (data?.data || []) as Rule[]
  const rules = search
    ? allRules.filter(r =>
        r.name.toLowerCase().includes(search.toLowerCase()) ||
        r.description?.toLowerCase().includes(search.toLowerCase()) ||
        r.condition?.toLowerCase().includes(search.toLowerCase()) ||
        r.labels?.['technique.id']?.toLowerCase().includes(search.toLowerCase()))
    : allRules
  const total = data?.meta?.total ?? allRules.length

  // Auto-open rule from URL ?rule=ID (linked from detection page)
  useEffect(() => {
    const ruleId = searchParams.get('rule')
    if (ruleId && allRules.length > 0) {
      const found = allRules.find(r => r.id === ruleId)
      if (found) {
        openEditor(found)
        setSearchParams({}, { replace: true })
      }
    }
  }, [searchParams, allRules])

  const openEditor = (rule: Rule) => {
    setEditRule(rule)
    setEditYaml(rule.raw_yaml || buildYamlFromRule(rule))
    setEditError('')
  }

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

      {/* Search */}
      <div className="mt-6 flex gap-4">
        <input
          type="text"
          placeholder="Search rules by name, technique, condition..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="flex-1 rounded-lg border border-gray-300 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
        />
        <span className="flex items-center text-sm text-gray-400">{rules.length} shown</span>
      </div>

      {/* Rules table */}
      <div className="mt-4 rounded-xl border border-gray-200 bg-white shadow-sm">
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
                        <div className="mt-0.5 text-xs text-gray-500">
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
                          onClick={() => openEditor(rule)}
                          className="text-xs font-medium text-fibratus-600 hover:text-fibratus-800"
                        >
                          Edit
                        </button>
                        <button
                          onClick={() =>
                            toggleMutation.mutate({
                              id: rule.id,
                              enabled: !rule.enabled,
                            })
                          }
                          className="text-xs font-medium text-gray-600 hover:text-gray-800"
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
                    {search ? 'No rules match your search.' : 'No rules configured. Upload detection rules above.'}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Rule editor slide panel */}
      <SlidePanel open={!!editRule} title={'Edit: ' + (editRule?.name || '')} onClose={() => setEditRule(null)} wide>
        {editRule && (
          <div className="flex flex-col h-full">
            <div className="flex items-center gap-4 mb-4">
              <SeverityBadge severity={editRule.severity} />
              <span className="font-mono text-xs text-gray-400">{editRule.id}</span>
              {editRule.labels?.['technique.id'] && (
                <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-mono text-gray-600">
                  {editRule.labels['technique.id']} — {editRule.labels?.['technique.name'] || ''}
                </span>
              )}
            </div>

            {editError && (
              <div className="mb-4 rounded-lg border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700">
                {editError}
              </div>
            )}

            <div className="flex-1 min-h-0">
              <textarea
                value={editYaml}
                onChange={(e) => setEditYaml(e.target.value)}
                className="w-full h-full rounded-lg border border-gray-300 px-4 py-3 font-mono text-sm leading-relaxed focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500 resize-none"
                style={{ minHeight: 'calc(100vh - 300px)', tabSize: 2 }}
                spellCheck={false}
              />
            </div>

            <div className="mt-4 flex items-center gap-3">
              <button
                onClick={() => editMutation.mutate({ id: editRule.id, yaml: editYaml })}
                disabled={editMutation.isPending || !editYaml.trim()}
                className="rounded-lg bg-fibratus-600 px-5 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
              >
                {editMutation.isPending ? 'Saving...' : 'Save Rule'}
              </button>
              <button
                onClick={() => setEditRule(null)}
                className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
              >
                Cancel
              </button>
              <label className="ml-auto flex cursor-pointer items-center rounded-lg border border-gray-300 px-3 py-2 text-xs hover:bg-gray-50">
                Import file
                <input
                  type="file"
                  accept=".yml,.yaml"
                  className="hidden"
                  onChange={(e) => {
                    const file = e.target.files?.[0]
                    if (file) file.text().then(setEditYaml)
                  }}
                />
              </label>
            </div>
          </div>
        )}
      </SlidePanel>

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

/** Build minimal YAML from rule fields when raw_yaml is not available */
function buildYamlFromRule(rule: Rule): string {
  const lines: string[] = []
  lines.push(`name: ${rule.name}`)
  lines.push(`id: ${rule.id}`)
  lines.push(`version: ${rule.version || '1.0.0'}`)
  if (rule.description) lines.push(`description: ${rule.description}`)
  if (rule.condition) lines.push(`condition: >\n  ${rule.condition}`)
  if (rule.output) lines.push(`output: "${rule.output}"`)
  lines.push(`severity: ${rule.severity || 'medium'}`)
  if (rule.labels && Object.keys(rule.labels).length > 0) {
    lines.push('labels:')
    for (const [k, v] of Object.entries(rule.labels)) {
      lines.push(`  ${k}: ${v}`)
    }
  }
  if (rule.tags && rule.tags.length > 0) {
    lines.push('tags:')
    for (const t of rule.tags) {
      lines.push(`  - ${t}`)
    }
  }
  return lines.join('\n') + '\n'
}
