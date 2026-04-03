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
  const [form, setForm] = useState({ name: '', expr: '', description: '' })
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
      setForm({ name: '', expr: '', description: '' })
      setError('')
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

  const openEdit = (m: Macro) => {
    setEditMacro(m)
    setForm({ name: m.name, expr: m.expr, description: m.description || '' })
    setError('')
  }

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Macros</h1>
          <p className="mt-1 text-sm text-gray-500">
            {macros.length} macro(s) — reusable filter expressions for detection rules
          </p>
        </div>
        <button
          onClick={() => { setShowCreate(true); setForm({ name: '', expr: '', description: '' }); setError('') }}
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
                <th className="px-6 py-3 font-medium text-gray-500">Expression</th>
                <th className="px-6 py-3 font-medium text-gray-500">Description</th>
                <th className="px-6 py-3 font-medium text-gray-500">Updated</th>
                <th className="px-6 py-3 font-medium text-gray-500">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading && (
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400">Loading...</td></tr>
              )}
              {!isLoading && macros.map((m) => (
                <tr key={m.id} className="hover:bg-gray-50/50">
                  <td className="px-6 py-3 font-mono font-medium text-gray-900">{m.name}</td>
                  <td className="px-6 py-3">
                    <code className="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700 font-mono">
                      {m.expr.length > 80 ? m.expr.slice(0, 80) + '...' : m.expr}
                    </code>
                  </td>
                  <td className="px-6 py-3 text-gray-500 text-xs">{m.description || '-'}</td>
                  <td className="px-6 py-3 text-gray-500 text-xs">{new Date(m.updated_at).toLocaleDateString()}</td>
                  <td className="px-6 py-3">
                    <div className="flex items-center gap-3">
                      <button onClick={() => openEdit(m)} className="text-xs font-medium text-fibratus-600 hover:text-fibratus-800">Edit</button>
                      <button onClick={() => setDeleteTarget(m)} className="text-xs font-medium text-red-600 hover:text-red-800">Delete</button>
                    </div>
                  </td>
                </tr>
              ))}
              {!isLoading && macros.length === 0 && (
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400">
                  No macros defined. Macros are reusable filter expressions like <code className="bg-gray-100 px-1 rounded">spawn_process</code> that simplify rule conditions.
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
            <p className="mt-1 text-xs text-gray-400">This name is used in rule conditions</p>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Expression</label>
            <textarea
              value={form.expr}
              onChange={(e) => setForm({ ...form, expr: e.target.value })}
              rows={4}
              placeholder="e.g., kevt.name = 'CreateProcess'"
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm font-mono focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
            />
            <p className="mt-1 text-xs text-gray-400">Filter expression using Fibratus query language</p>
          </div>
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
                if (editMacro) {
                  updateMutation.mutate({ id: editMacro.id, data: form })
                } else {
                  createMutation.mutate(form)
                }
              }}
              disabled={!form.name.trim() || !form.expr.trim() || createMutation.isPending || updateMutation.isPending}
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
