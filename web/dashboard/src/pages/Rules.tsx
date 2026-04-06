import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { api, type Rule, type ValidationError, type ApiResponse } from '../lib/api'
import SeverityBadge from '../components/SeverityBadge'
import SlidePanel from '../components/SlidePanel'
import ConfirmDialog from '../components/ConfirmDialog'
import { useTableSort } from '../hooks/useTableSort'
import SortableHeader from '../components/SortableHeader'

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
  const [validateAllPending, setValidateAllPending] = useState(false)

  // Editor validation state
  const [editorValidated, setEditorValidated] = useState(false)
  const [editorValidationResult, setEditorValidationResult] = useState<{
    valid: boolean
    errors?: ValidationError[]
    warnings?: string[]
  } | null>(null)

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
      setEditorValidated(false)
      setEditorValidationResult(null)
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
  const { sorted: sortedRules, sort, toggleSort } = useTableSort(rules, 'name', 'asc')
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
    setEditorValidated(false)
    setEditorValidationResult(null)
  }

  // Reset validation when YAML is edited
  const handleYamlChange = (value: string) => {
    setEditYaml(value)
    if (editorValidated) {
      setEditorValidated(false)
      setEditorValidationResult(null)
    }
  }

  // Extract condition from YAML for validation
  const extractCondition = (yaml: string): string => {
    const lines = yaml.split('\n')
    let condition = ''
    let inCondition = false
    for (const line of lines) {
      if (/^condition:\s*[>|]?\s*$/.test(line)) {
        inCondition = true
        continue
      }
      if (/^condition:\s+\S/.test(line)) {
        condition = line.replace(/^condition:\s+/, '').trim()
        inCondition = true
        continue
      }
      if (inCondition) {
        if (/^\S/.test(line) && !line.startsWith(' ') && !line.startsWith('\t')) {
          break
        }
        condition += (condition ? '\n' : '') + line.replace(/^\s{2}/, '')
      }
    }
    return condition.trim()
  }

  const handleValidate = async () => {
    const condition = extractCondition(editYaml)
    if (!condition) {
      setEditorValidationResult({ valid: false, errors: [{ type: 'syntax', message: 'No condition found in YAML' }] })
      setEditorValidated(true)
      return
    }
    try {
      const res = await api.validateRuleCondition(condition)
      const result = res.data as { valid: boolean; errors?: ValidationError[]; warnings?: string[] }
      setEditorValidationResult(result)
      setEditorValidated(true)
    } catch {
      setEditorValidationResult({ valid: false, errors: [{ type: 'syntax', message: 'Validation request failed' }] })
      setEditorValidated(true)
    }
  }

  const invalidCount = allRules.filter(r => r.validation_status === 'invalid').length
  const validCount = allRules.filter(r => r.validation_status === 'valid').length

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-slate-100">Rules</h1>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
            {total} rule(s) managed by fleet server
            {validCount > 0 && (
              <span className="ml-2 text-emerald-600 dark:text-emerald-400">
                {validCount} valid
              </span>
            )}
            {invalidCount > 0 && (
              <span className="ml-2 text-red-600 dark:text-red-400 font-medium">
                {invalidCount} invalid
              </span>
            )}
          </p>
        </div>
        <div className="flex gap-3">
          <button
            onClick={async () => {
              setValidateAllPending(true)
              try {
                await api.validateAllRules()
                queryClient.invalidateQueries({ queryKey: ['rules'] })
              } finally {
                setValidateAllPending(false)
              }
            }}
            disabled={validateAllPending}
            className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm font-medium text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700 disabled:opacity-50"
          >
            {validateAllPending ? 'Validating...' : 'Validate All'}
          </button>
          <button
            onClick={() => setShowUpload(!showUpload)}
            className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
          >
            {showUpload ? 'Cancel' : 'Upload Rule'}
          </button>
        </div>
      </div>

      {/* Upload panel */}
      {showUpload && (
        <div className="mt-6 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
          <h3 className="text-sm font-medium text-gray-900 dark:text-slate-100">Upload YAML Rule</h3>
          <p className="mt-1 text-xs text-gray-500 dark:text-slate-400">
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
            className="mt-3 w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-4 py-3 font-mono text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
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
            <label className="flex cursor-pointer items-center rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700">
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
          className="flex-1 rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-4 py-2 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
        />
        <span className="flex items-center text-sm text-gray-400 dark:text-slate-500">{rules.length} shown</span>
      </div>

      {/* Rules table */}
      <div className="mt-4 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
              <tr>
                <SortableHeader label="Name" sortKey="name" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Severity" sortKey="severity" sort={sort} onSort={toggleSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">MITRE</th>
                <SortableHeader label="Version" sortKey="version" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Status" sortKey="enabled" sort={sort} onSort={toggleSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {isLoading && (
                <tr>
                  <td colSpan={6} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">
                    Loading...
                  </td>
                </tr>
              )}
              {!isLoading &&
                sortedRules.map((rule) => (
                  <tr key={rule.id} className="hover:bg-gray-50/50 dark:hover:bg-slate-700/30">
                    <td className="px-6 py-3">
                      <div className="font-medium text-gray-900 dark:text-slate-100">{rule.name}</div>
                      {rule.description && (
                        <div className="mt-0.5 text-xs text-gray-500 dark:text-slate-400">
                          {rule.description}
                        </div>
                      )}
                    </td>
                    <td className="px-6 py-3">
                      <SeverityBadge severity={rule.severity} />
                    </td>
                    <td className="px-6 py-3 text-gray-600 dark:text-slate-400">
                      {rule.labels?.['technique.id'] && (
                        <span className="rounded bg-gray-100 dark:bg-slate-700 px-1.5 py-0.5 text-xs font-mono">
                          {rule.labels['technique.id']}
                        </span>
                      )}
                    </td>
                    <td className="px-6 py-3 font-mono text-xs text-gray-500 dark:text-slate-400">
                      {rule.version}
                    </td>
                    <td className="px-6 py-3">
                      <div className="flex items-center gap-2">
                        <span
                          className={
                            'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ' +
                            (rule.enabled
                              ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                              : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')
                          }
                        >
                          {rule.enabled ? 'Active' : 'Disabled'}
                        </span>
                        {rule.validation_status === 'valid' && (
                          <span className="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400">
                            Valid
                          </span>
                        )}
                        {rule.validation_status === 'invalid' && (
                          <span
                            className="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-400 cursor-pointer"
                            title="Click to see validation errors"
                            onClick={(e) => { e.stopPropagation(); openEditor(rule) }}
                          >
                            Invalid
                          </span>
                        )}
                        {rule.validation_status === 'pending' && (
                          <span className="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium bg-yellow-50 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400">
                            Pending
                          </span>
                        )}
                      </div>
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
                          onClick={() => {
                            if (!rule.enabled && rule.validation_status === 'invalid') {
                              openEditor(rule)
                              return
                            }
                            toggleMutation.mutate({
                              id: rule.id,
                              enabled: !rule.enabled,
                            })
                          }}
                          className={
                            'text-xs font-medium ' +
                            (rule.validation_status === 'invalid' && !rule.enabled
                              ? 'text-red-500 hover:text-red-700 cursor-not-allowed'
                              : 'text-gray-600 dark:text-slate-400 hover:text-gray-800 dark:hover:text-slate-200')
                          }
                          title={rule.validation_status === 'invalid' && !rule.enabled ? 'Fix validation errors before enabling' : ''}
                        >
                          {rule.enabled ? 'Disable' : rule.validation_status === 'invalid' ? 'Fix Errors' : 'Enable'}
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
              {!isLoading && sortedRules.length === 0 && (
                <tr>
                  <td colSpan={6} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">
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
              <span className="font-mono text-xs text-gray-400 dark:text-slate-500">{editRule.id}</span>
              {editRule.labels?.['technique.id'] && (
                <span className="rounded bg-gray-100 dark:bg-slate-700 px-1.5 py-0.5 text-xs font-mono text-gray-600 dark:text-slate-400">
                  {editRule.labels['technique.id']} — {editRule.labels?.['technique.name'] || ''}
                </span>
              )}
            </div>

            {editError && (
              <div className="mb-4 rounded-lg border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700">
                {editError}
              </div>
            )}

            {/* Show stored validation errors for invalid rules (before any edit) */}
            {!editorValidated && editRule.validation_status === 'invalid' && editRule.validation_errors && editRule.validation_errors.length > 0 && (
              <div className="mb-4 rounded-lg border border-red-200 dark:border-red-800/50 bg-red-50 dark:bg-red-900/20 px-4 py-3">
                <div className="flex items-center gap-2 mb-2">
                  <span className="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-400">
                    Validation Failed
                  </span>
                  <span className="text-xs text-red-600 dark:text-red-400">
                    This rule will not be synced to agents until errors are fixed
                  </span>
                </div>
                <ValidationErrorList errors={editRule.validation_errors as ValidationError[]} />
              </div>
            )}

            {/* Show live validation result */}
            {editorValidated && editorValidationResult && (
              <div className={
                'mb-4 rounded-lg border px-4 py-3 ' +
                (editorValidationResult.valid
                  ? 'border-emerald-200 dark:border-emerald-800/50 bg-emerald-50 dark:bg-emerald-900/20'
                  : 'border-red-200 dark:border-red-800/50 bg-red-50 dark:bg-red-900/20')
              }>
                {editorValidationResult.valid ? (
                  <div className="flex items-center gap-2">
                    <span className="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-400">
                      Validation Passed
                    </span>
                    <span className="text-xs text-emerald-600 dark:text-emerald-400">
                      Rule is valid and ready to save
                    </span>
                  </div>
                ) : (
                  <>
                    <div className="flex items-center gap-2 mb-2">
                      <span className="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-400">
                        Validation Failed
                      </span>
                      <span className="text-xs text-red-600 dark:text-red-400">
                        Fix the errors below before saving
                      </span>
                    </div>
                    {editorValidationResult.errors && (
                      <ValidationErrorList errors={editorValidationResult.errors} />
                    )}
                  </>
                )}
                {editorValidationResult.warnings && editorValidationResult.warnings.length > 0 && (
                  <div className="mt-2 space-y-1">
                    {editorValidationResult.warnings.map((w, i) => (
                      <div key={i} className="flex items-start gap-2 text-xs text-yellow-700 dark:text-yellow-400">
                        <span className="shrink-0 rounded bg-yellow-100 dark:bg-yellow-900/40 px-1.5 py-0.5 font-mono">warn</span>
                        <span>{w}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            <div className="flex-1 min-h-0">
              <textarea
                value={editYaml}
                onChange={(e) => handleYamlChange(e.target.value)}
                className="w-full h-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-4 py-3 font-mono text-sm text-gray-900 dark:text-slate-100 leading-relaxed focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500 resize-none"
                style={{ minHeight: 'calc(100vh - 300px)', tabSize: 2 }}
                spellCheck={false}
              />
            </div>

            <div className="mt-4 flex items-center gap-3">
              {/* Step 1: Validate button (always available when YAML changed) */}
              {!editorValidated && (
                <button
                  onClick={handleValidate}
                  disabled={!editYaml.trim()}
                  className="rounded-lg bg-fibratus-600 px-5 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
                >
                  Validate
                </button>
              )}
              {/* Step 2: Save button (only after successful validation) */}
              {editorValidated && editorValidationResult?.valid && (
                <button
                  onClick={() => editMutation.mutate({ id: editRule.id, yaml: editYaml })}
                  disabled={editMutation.isPending}
                  className="rounded-lg bg-emerald-600 px-5 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-50"
                >
                  {editMutation.isPending ? 'Saving...' : 'Save Rule'}
                </button>
              )}
              {/* Re-validate if validation failed */}
              {editorValidated && !editorValidationResult?.valid && (
                <button
                  onClick={handleValidate}
                  disabled={!editYaml.trim()}
                  className="rounded-lg bg-fibratus-600 px-5 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
                >
                  Re-validate
                </button>
              )}
              <button
                onClick={() => setEditRule(null)}
                className="rounded-lg border border-gray-300 dark:border-slate-600 px-4 py-2 text-sm font-medium text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700"
              >
                Cancel
              </button>
              <label className="ml-auto flex cursor-pointer items-center rounded-lg border border-gray-300 dark:border-slate-600 px-3 py-2 text-xs text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700">
                Import file
                <input
                  type="file"
                  accept=".yml,.yaml"
                  className="hidden"
                  onChange={(e) => {
                    const file = e.target.files?.[0]
                    if (file) file.text().then(handleYamlChange)
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

/** Renders a list of validation errors with type badges and fix suggestions */
function ValidationErrorList({ errors }: { errors: ValidationError[] }) {
  return (
    <div className="space-y-2">
      {errors.map((err, i) => (
        <div key={i} className="text-sm">
          <div className="flex items-start gap-2">
            <span className="shrink-0 rounded bg-red-100 dark:bg-red-900/40 px-1.5 py-0.5 text-xs font-mono text-red-700 dark:text-red-400">
              {err.type}
            </span>
            <span className="text-red-700 dark:text-red-300">{err.message}</span>
          </div>
          {err.suggestion && (
            <div className="ml-16 mt-1 text-xs text-red-600 dark:text-red-400 italic">
              Fix: {err.suggestion}
            </div>
          )}
        </div>
      ))}
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
