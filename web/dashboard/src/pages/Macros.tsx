import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Macro } from '../lib/api'
import SlidePanel from '../components/SlidePanel'
import ConfirmDialog from '../components/ConfirmDialog'

export default function Macros() {
  const queryClient = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [editMacro, setEditMacro] = useState<Macro | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Macro | null>(null)
  const [form, setForm] = useState({ name: '', expr: '', list: '', description: '' })
  const [macroType, setMacroType] = useState<'expr' | 'list'>('expr')
  const [error, setError] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['macros'],
    queryFn: () => api.getMacros(),
  })

  const createMutation = useMutation({
    mutationFn: (data: Partial<Macro>) => api.createMacro(data),
    onSuccess: (res) => {
      if (res.error) { setError(res.error.message); return }
      setShowCreate(false)
      resetForm()
      queryClient.invalidateQueries({ queryKey: ['macros'] })
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<Macro> }) => api.updateMacro(id, data),
    onSuccess: (res) => {
      if (res.error) { setError(res.error.message); return }
      setEditMacro(null)
      setError('')
      queryClient.invalidateQueries({ queryKey: ['macros'] })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteMacro(id),
    onSuccess: () => {
      setDeleteTarget(null)
      queryClient.invalidateQueries({ queryKey: ['macros'] })
    },
  })

  const macros = (data?.data || []) as Macro[]

  function resetForm() {
    setForm({ name: '', expr: '', list: '', description: '' })
    setMacroType('expr')
    setError('')
  }

  function openEdit(m: Macro) {
    setEditMacro(m)
    const hasList = m.list && m.list.length > 0
    setMacroType(hasList ? 'list' : 'expr')
    setForm({
      name: m.name,
      expr: m.expr || '',
      list: hasList ? m.list.join(', ') : '',
      description: m.description || '',
    })
    setError('')
  }

  function buildPayload(): Partial<Macro> {
    if (macroType === 'list') {
      return { name: form.name, expr: '', list: form.list.split(',').map(s => s.trim()).filter(Boolean), description: form.description }
    }
    return { name: form.name, expr: form.expr, list: [], description: form.description }
  }

  function macroDisplay(m: Macro): string {
    if (m.list && m.list.length > 0) {
      const preview = m.list.slice(0, 3).join(', ')
      return m.list.length > 3 ? `[${preview}, ...+${m.list.length - 3}]` : `[${preview}]`
    }
    if (m.expr) {
      return m.expr.length > 80 ? m.expr.slice(0, 80) + '...' : m.expr
    }
    return '-'
  }

  function macroTypeBadge(m: Macro): string {
    if (m.list && m.list.length > 0) return 'list'
    if (m.expr) return 'expr'
    return 'empty'
  }

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Macros</h1>
          <p className="mt-1 text-sm text-gray-500">
            {macros.length} macro(s) — reusable filter expressions and value lists for detection rules
          </p>
        </div>
        <button
          onClick={() => { setShowCreate(true); resetForm() }}
          className="rounded-lg bg-fibratus-600 px-4 py-2 text-sm font-medium text-white hover:bg-fibratus-700"
        >
          New Macro
        </button>
      </div>

      {/* Macros table */}
      <div className="mt-6 rounded-xl border border-gray-200 bg-white shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 bg-gray-50/50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-500">Name</th>
                <th className="px-6 py-3 font-medium text-gray-500 w-16">Type</th>
                <th className="px-6 py-3 font-medium text-gray-500">Value</th>
                <th className="px-6 py-3 font-medium text-gray-500">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading && (
                <tr><td colSpan={4} className="px-6 py-12 text-center text-gray-400">Loading...</td></tr>
              )}
              {!isLoading && macros.map((m) => (
                <tr key={m.id} className="hover:bg-gray-50/50">
                  <td className="px-6 py-3">
                    <span className="font-mono font-medium text-gray-900">{m.name}</span>
                    {m.description && <div className="text-xs text-gray-400 mt-0.5 line-clamp-1">{m.description}</div>}
                  </td>
                  <td className="px-6 py-3">
                    <span className={'rounded px-1.5 py-0.5 text-xs font-medium ' +
                      (macroTypeBadge(m) === 'list' ? 'bg-purple-50 text-purple-700' :
                       macroTypeBadge(m) === 'expr' ? 'bg-blue-50 text-blue-700' : 'bg-gray-100 text-gray-500')}>
                      {macroTypeBadge(m)}
                    </span>
                  </td>
                  <td className="px-6 py-3">
                    <code className="text-xs text-gray-600 font-mono">{macroDisplay(m)}</code>
                  </td>
                  <td className="px-6 py-3">
                    <div className="flex items-center gap-3">
                      <button onClick={() => openEdit(m)} className="text-xs font-medium text-fibratus-600 hover:text-fibratus-800">Edit</button>
                      <button onClick={() => setDeleteTarget(m)} className="text-xs font-medium text-red-600 hover:text-red-800">Delete</button>
                    </div>
                  </td>
                </tr>
              ))}
              {!isLoading && macros.length === 0 && (
                <tr><td colSpan={4} className="px-6 py-12 text-center text-gray-400">
                  No macros defined.
                </td></tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Create/Edit panel */}
      <SlidePanel
        open={showCreate || !!editMacro}
        title={editMacro ? 'Edit Macro: ' + editMacro.name : 'New Macro'}
        onClose={() => { setShowCreate(false); setEditMacro(null) }}
      >
        <div className="space-y-4">
          {error && (
            <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700">{error}</div>
          )}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
            <input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="e.g., spawn_process"
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm font-mono focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
            />
          </div>

          {/* Type toggle */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Type</label>
            <div className="flex gap-2">
              <button
                onClick={() => setMacroType('expr')}
                className={'px-3 py-1.5 text-xs rounded-lg font-medium border ' + (macroType === 'expr' ? 'bg-blue-50 text-blue-700 border-blue-300' : 'bg-white text-gray-500 border-gray-200')}
              >
                Expression
              </button>
              <button
                onClick={() => setMacroType('list')}
                className={'px-3 py-1.5 text-xs rounded-lg font-medium border ' + (macroType === 'list' ? 'bg-purple-50 text-purple-700 border-purple-300' : 'bg-white text-gray-500 border-gray-200')}
              >
                Value List
              </button>
            </div>
          </div>

          {macroType === 'expr' ? (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Expression</label>
              <textarea
                value={form.expr}
                onChange={(e) => setForm({ ...form, expr: e.target.value })}
                rows={4}
                placeholder="e.g., kevt.name = 'CreateProcess'"
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm font-mono focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
              />
            </div>
          ) : (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Values (comma-separated)</label>
              <textarea
                value={form.list}
                onChange={(e) => setForm({ ...form, list: e.target.value })}
                rows={6}
                placeholder={"chrome.exe, firefox.exe, msedge.exe, iexplore.exe,\nopera.exe, brave.exe, vivaldi.exe"}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm font-mono focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
              />
              <p className="mt-1 text-xs text-gray-400">
                {form.list ? form.list.split(',').map(s => s.trim()).filter(Boolean).length : 0} values
              </p>
            </div>
          )}

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Description (optional)</label>
            <input
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              placeholder="What this macro matches"
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
            />
          </div>

          <div className="flex gap-3 pt-2">
            <button
              onClick={() => {
                const payload = buildPayload()
                if (editMacro) {
                  updateMutation.mutate({ id: editMacro.id, data: payload })
                } else {
                  createMutation.mutate(payload)
                }
              }}
              disabled={!form.name.trim() || (!form.expr.trim() && !form.list.trim()) || createMutation.isPending || updateMutation.isPending}
              className="rounded-lg bg-fibratus-600 px-5 py-2 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
            >
              {createMutation.isPending || updateMutation.isPending ? 'Saving...' : editMacro ? 'Save' : 'Create'}
            </button>
            <button
              onClick={() => { setShowCreate(false); setEditMacro(null) }}
              className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
            >
              Cancel
            </button>
          </div>
        </div>
      </SlidePanel>

      <ConfirmDialog
        open={!!deleteTarget}
        title="Delete Macro"
        message={'Delete macro "' + (deleteTarget?.name || '') + '"? Rules referencing this macro will stop compiling.'}
        confirmLabel="Delete"
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}
