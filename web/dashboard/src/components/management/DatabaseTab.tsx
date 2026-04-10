import { useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../../lib/api'

type DbType = 'postgres' | 'clickhouse'
type BrowseView = 'tables' | 'browse' | 'query'

interface QueryResult {
  columns: string[]
  rows: unknown[][]
  affected_rows?: number
  error?: string
}

const browseLimit = 50

export default function DatabaseTab() {
  const [dbType, setDbType] = useState<DbType>('postgres')
  const [query, setQuery] = useState('')
  const [result, setResult] = useState<QueryResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [activeTable, setActiveTable] = useState<string | null>(null)
  const [tableColumns, setTableColumns] = useState<QueryResult | null>(null)
  const [tableData, setTableData] = useState<QueryResult | null>(null)
  const [tableLoading, setTableLoading] = useState(false)
  const [editCell, setEditCell] = useState<{ row: number; col: number; value: string } | null>(null)
  const [editSaving, setEditSaving] = useState(false)
  const [browseView, setBrowseView] = useState<BrowseView>('tables')
  const [browseOffset, setBrowseOffset] = useState(0)
  const [selectedRows, setSelectedRows] = useState<Set<number>>(new Set())
  const [deleting, setDeleting] = useState(false)
  const [retentionDays, setRetentionDays] = useState<number>(30)
  const [retentionSaving, setRetentionSaving] = useState(false)

  const pgTables = useQuery({
    queryKey: ['pg-tables'],
    queryFn: () => api.dbTablesPostgres(),
    staleTime: 30000,
  })

  const chTables = useQuery({
    queryKey: ['ch-tables'],
    queryFn: () => api.dbTablesClickhouse(),
    staleTime: 30000,
  })

  const accountSettings = useQuery({
    queryKey: ['account-settings'],
    queryFn: () => api.getAccountSettings(),
  })

  useEffect(() => {
    if (accountSettings.data?.data) {
      const settings = accountSettings.data.data as Record<string, unknown>
      if (typeof settings.telemetry_retention_days === 'number') {
        setRetentionDays(settings.telemetry_retention_days)
      }
    }
  }, [accountSettings.data])

  const tables = dbType === 'postgres' ? pgTables : chTables

  async function executeQuery() {
    if (!query.trim()) return
    setLoading(true)
    setResult(null)
    try {
      const fn = dbType === 'postgres' ? api.dbQueryPostgres : api.dbQueryClickhouse
      const res = await fn(query)
      if (res.data) {
        setResult(res.data)
      } else if (res.error) {
        setResult({ columns: [], rows: [], error: res.error.message })
      }
    } catch (err) {
      setResult({ columns: [], rows: [], error: String(err) })
    } finally {
      setLoading(false)
    }
  }

  async function openTable(tableName: string) {
    setActiveTable(tableName)
    setBrowseView('browse')
    setBrowseOffset(0)
    setSelectedRows(new Set())
    setTableLoading(true)
    setTableColumns(null)
    setTableData(null)
    try {
      const fn = dbType === 'postgres' ? api.dbQueryPostgres : api.dbQueryClickhouse
      const schemaQuery = dbType === 'postgres'
        ? `SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_schema = 'public' AND table_name = '${tableName}' ORDER BY ordinal_position`
        : `SELECT name AS column_name, type AS data_type, default_expression AS column_default FROM system.columns WHERE database = 'fibratus' AND table = '${tableName}' ORDER BY position`
      const dataQuery = dbType === 'postgres'
        ? `SELECT * FROM "${tableName}" LIMIT ${browseLimit}`
        : `SELECT * FROM ${tableName} LIMIT ${browseLimit}`
      const [schemaRes, dataRes] = await Promise.all([fn(schemaQuery), fn(dataQuery)])
      if (schemaRes.data) setTableColumns(schemaRes.data)
      if (dataRes.data) setTableData(dataRes.data)
    } catch {
      // silently handle
    } finally {
      setTableLoading(false)
    }
  }

  async function loadPage(offset: number) {
    if (!activeTable) return
    setBrowseOffset(offset)
    setSelectedRows(new Set())
    setTableLoading(true)
    try {
      const fn = dbType === 'postgres' ? api.dbQueryPostgres : api.dbQueryClickhouse
      const dataQuery = dbType === 'postgres'
        ? `SELECT * FROM "${activeTable}" LIMIT ${browseLimit} OFFSET ${offset}`
        : `SELECT * FROM ${activeTable} LIMIT ${browseLimit} OFFSET ${offset}`
      const res = await fn(dataQuery)
      if (res.data) setTableData(res.data)
    } catch {
      // silently handle
    } finally {
      setTableLoading(false)
    }
  }

  async function saveCell() {
    if (!editCell || !activeTable || !tableData || dbType !== 'postgres') return
    setEditSaving(true)
    try {
      const pkCol = tableData.columns[0]
      const pkVal = tableData.rows[editCell.row][0]
      const col = tableData.columns[editCell.col]
      const escaped = editCell.value.replace(/'/g, "''")
      const updateQuery = `UPDATE "${activeTable}" SET "${col}" = '${escaped}' WHERE "${pkCol}" = '${pkVal}'`
      await api.dbQueryPostgres(updateQuery)
      // Refresh data
      const dataQuery = `SELECT * FROM "${activeTable}" LIMIT ${browseLimit} OFFSET ${browseOffset}`
      const res = await api.dbQueryPostgres(dataQuery)
      if (res.data) setTableData(res.data)
      setEditCell(null)
    } catch {
      // silently handle
    } finally {
      setEditSaving(false)
    }
  }

  async function deleteSelected() {
    if (!activeTable || selectedRows.size === 0 || !tableData || dbType !== 'postgres') return
    if (!confirm(`Delete ${selectedRows.size} selected row(s)?`)) return
    setDeleting(true)
    try {
      const pkCol = tableData.columns[0]
      const pkVals = Array.from(selectedRows).map(i => `'${tableData.rows[i][0]}'`).join(', ')
      await api.dbQueryPostgres(`DELETE FROM "${activeTable}" WHERE "${pkCol}" IN (${pkVals})`)
      setSelectedRows(new Set())
      await loadPage(browseOffset)
    } catch {
      // silently handle
    } finally {
      setDeleting(false)
    }
  }

  async function deleteAllRows() {
    if (!activeTable) return
    if (!confirm(`Delete ALL rows from "${activeTable}"? This cannot be undone.`)) return
    setDeleting(true)
    try {
      const fn = dbType === 'postgres' ? api.dbQueryPostgres : api.dbQueryClickhouse
      const q = dbType === 'postgres' ? `TRUNCATE "${activeTable}"` : `TRUNCATE TABLE ${activeTable}`
      await fn(q)
      setSelectedRows(new Set())
      await loadPage(0)
    } catch {
      // silently handle
    } finally {
      setDeleting(false)
    }
  }

  async function saveRetention() {
    setRetentionSaving(true)
    try {
      await api.updateTelemetryRetention(retentionDays)
      accountSettings.refetch()
    } catch {
      // silently handle
    } finally {
      setRetentionSaving(false)
    }
  }

  const currentRetention = (accountSettings.data?.data as Record<string, unknown>)?.telemetry_retention_days as number | undefined
  const retentionChanged = currentRetention !== undefined && retentionDays !== currentRetention

  function formatCellValue(val: unknown): string {
    if (val === null || val === undefined) return 'NULL'
    if (typeof val === 'object') return JSON.stringify(val)
    return String(val)
  }

  const tableList = tables.data?.data

  return (
    <div className="space-y-4">
      {/* Server Info Bar */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
        <div className="grid grid-cols-4 gap-4">
          <div>
            <div className="text-xs text-gray-500 dark:text-slate-400 mb-1">Server URL</div>
            <div className="text-sm font-medium text-gray-900 dark:text-slate-100 truncate">{window.location.origin}</div>
          </div>
          <div>
            <div className="text-xs text-gray-500 dark:text-slate-400 mb-1">Version</div>
            <div className="text-sm font-medium text-gray-900 dark:text-slate-100">Fleet v1.0</div>
          </div>
          <div>
            <div className="text-xs text-gray-500 dark:text-slate-400 mb-1">ClickHouse Telemetry Retention</div>
            <div className="flex items-center gap-2">
              <select
                value={retentionDays}
                onChange={e => setRetentionDays(Number(e.target.value))}
                className="text-sm rounded border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 text-gray-900 dark:text-slate-100 px-2 py-1"
              >
                {[7, 14, 30, 60, 90, 180, 365].map(d => (
                  <option key={d} value={d}>{d} days</option>
                ))}
              </select>
              {retentionChanged && (
                <button
                  onClick={saveRetention}
                  disabled={retentionSaving}
                  className="text-xs px-2 py-1 rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
                >
                  {retentionSaving ? 'Saving...' : 'Save'}
                </button>
              )}
            </div>
          </div>
          <div>
            <div className="text-xs text-gray-500 dark:text-slate-400 mb-1">Environment</div>
            <div className="text-sm font-medium text-gray-900 dark:text-slate-100">Production</div>
          </div>
        </div>
      </div>

      {/* DB Toggle + View Toggle */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="rounded-lg bg-gray-100 dark:bg-slate-700 p-0.5 flex">
            <button
              onClick={() => { setDbType('postgres'); setActiveTable(null); setBrowseView('tables') }}
              className={`px-3 py-1.5 text-sm font-medium rounded-md transition-all ${dbType === 'postgres' ? 'bg-blue-600 text-white shadow-sm' : 'text-gray-600 dark:text-slate-300 hover:text-gray-900 dark:hover:text-slate-100'}`}
            >
              PostgreSQL
            </button>
            <button
              onClick={() => { setDbType('clickhouse'); setActiveTable(null); setBrowseView('tables') }}
              className={`px-3 py-1.5 text-sm font-medium rounded-md transition-all ${dbType === 'clickhouse' ? 'bg-amber-600 text-white shadow-sm' : 'text-gray-600 dark:text-slate-300 hover:text-gray-900 dark:hover:text-slate-100'}`}
            >
              ClickHouse
            </button>
          </div>
          <div className="rounded-lg bg-gray-100 dark:bg-slate-700 p-0.5 flex">
            <button
              onClick={() => { setBrowseView('tables'); setActiveTable(null) }}
              className={`px-3 py-1.5 text-sm font-medium rounded-md transition-all ${browseView === 'tables' || browseView === 'browse' ? 'bg-white dark:bg-slate-600 text-gray-900 dark:text-slate-100 shadow-sm' : 'text-gray-600 dark:text-slate-300'}`}
            >
              Browse
            </button>
            <button
              onClick={() => setBrowseView('query')}
              className={`px-3 py-1.5 text-sm font-medium rounded-md transition-all ${browseView === 'query' ? 'bg-white dark:bg-slate-600 text-gray-900 dark:text-slate-100 shadow-sm' : 'text-gray-600 dark:text-slate-300'}`}
            >
              Query
            </button>
          </div>
          {activeTable && browseView === 'browse' && (
            <div className="flex items-center gap-1 text-sm text-gray-500 dark:text-slate-400">
              <button onClick={() => { setBrowseView('tables'); setActiveTable(null) }} className="hover:text-blue-600 dark:hover:text-blue-400">
                Tables
              </button>
              <span>/</span>
              <span className="text-gray-900 dark:text-slate-100 font-medium">{activeTable}</span>
            </div>
          )}
        </div>
      </div>

      {/* Tables List View */}
      {browseView === 'tables' && (
        <div>
          {tables.isLoading ? (
            <div className="text-center py-12 text-gray-500 dark:text-slate-400">Loading tables...</div>
          ) : tableList && tableList.columns && tableList.rows ? (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
              {tableList.rows.map((row, i) => {
                const tableName = String(row[0])
                return (
                  <button
                    key={i}
                    onClick={() => openTable(tableName)}
                    className="text-left rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4 hover:border-blue-300 dark:hover:border-blue-600 hover:shadow-md transition-all group"
                  >
                    <div className="flex items-center gap-2 mb-2">
                      <svg className="w-4 h-4 text-gray-400 dark:text-slate-500 group-hover:text-blue-500 transition-colors" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 7v10c0 2 8 2 8 2s8 0 8-2V7M4 7c0 2 8 2 8 2s8 0 8-2M4 7c0-2 8-2 8-2s8 0 8 2m-8 5c-4 0-8-.5-8-2m8 2c4 0 8-.5 8-2" />
                      </svg>
                      <span className="font-medium text-gray-900 dark:text-slate-100 group-hover:text-blue-600 dark:group-hover:text-blue-400 transition-colors">{tableName}</span>
                    </div>
                    <div className="flex items-center gap-3 text-xs text-gray-500 dark:text-slate-400">
                      {row[1] !== null && row[1] !== undefined && (
                        <span>Size: {String(row[1])}</span>
                      )}
                      {tableList.columns.slice(2).map((col, ci) => (
                        row[ci + 2] !== null && row[ci + 2] !== undefined ? (
                          <span key={ci}>{col}: {String(row[ci + 2])}</span>
                        ) : null
                      ))}
                    </div>
                  </button>
                )
              })}
            </div>
          ) : (
            <div className="text-center py-12 text-gray-500 dark:text-slate-400">No tables found</div>
          )}
        </div>
      )}

      {/* Table Browse View */}
      {browseView === 'browse' && activeTable && (
        <div className="space-y-4">
          {tableLoading ? (
            <div className="text-center py-12 text-gray-500 dark:text-slate-400">Loading table...</div>
          ) : (
            <>
              {/* Schema */}
              {tableColumns && tableColumns.rows.length > 0 && (
                <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 overflow-hidden">
                  <div className="px-4 py-2.5 border-b border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-800/50">
                    <span className="text-sm font-medium text-gray-700 dark:text-slate-300">Schema</span>
                  </div>
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-gray-200 dark:border-slate-700">
                        {tableColumns.columns.map((col, i) => (
                          <th key={i} className="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-slate-400 uppercase">{col}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {tableColumns.rows.map((row, ri) => (
                        <tr key={ri} className="border-b border-gray-100 dark:border-slate-700/50">
                          {row.map((val, ci) => (
                            <td key={ci} className="px-4 py-1.5">
                              {ci === 0 ? (
                                <span className="text-blue-600 dark:text-blue-400 font-medium">{formatCellValue(val)}</span>
                              ) : ci === 1 ? (
                                <span className="text-amber-600 dark:text-amber-400">{formatCellValue(val)}</span>
                              ) : val === null || val === undefined ? (
                                <span className="text-gray-300 dark:text-slate-600 italic">NULL</span>
                              ) : (
                                <span className="text-gray-700 dark:text-slate-300">{formatCellValue(val)}</span>
                              )}
                            </td>
                          ))}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}

              {/* Data */}
              {tableData && (
                <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 overflow-hidden">
                  <div className="px-4 py-2.5 border-b border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-800/50 flex items-center justify-between">
                    <span className="text-sm font-medium text-gray-700 dark:text-slate-300">
                      Data {tableData.rows.length > 0 && `(${browseOffset + 1}-${browseOffset + tableData.rows.length})`}
                    </span>
                    <div className="flex items-center gap-2">
                      <button
                        onClick={() => loadPage(Math.max(0, browseOffset - browseLimit))}
                        disabled={browseOffset === 0 || tableLoading}
                        className="px-2 py-1 text-xs rounded border border-gray-300 dark:border-slate-600 text-gray-700 dark:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-700 disabled:opacity-30"
                      >
                        Prev
                      </button>
                      <button
                        onClick={() => loadPage(browseOffset + browseLimit)}
                        disabled={tableData.rows.length < browseLimit || tableLoading}
                        className="px-2 py-1 text-xs rounded border border-gray-300 dark:border-slate-600 text-gray-700 dark:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-700 disabled:opacity-30"
                      >
                        Next
                      </button>
                      {dbType === 'postgres' && selectedRows.size > 0 && (
                        <button
                          onClick={deleteSelected}
                          disabled={deleting}
                          className="px-2 py-1 text-xs rounded bg-red-600 text-white hover:bg-red-700 disabled:opacity-50"
                        >
                          {deleting ? 'Deleting...' : `Delete ${selectedRows.size} selected`}
                        </button>
                      )}
                      {dbType === 'postgres' && (
                        <button
                          onClick={deleteAllRows}
                          disabled={deleting}
                          className="px-2 py-1 text-xs rounded border border-red-300 dark:border-red-700 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50"
                        >
                          Delete All
                        </button>
                      )}
                    </div>
                  </div>
                  <div className="max-h-[600px] overflow-y-auto">
                    <table className="w-full text-sm">
                      <thead className="sticky top-0 bg-gray-50 dark:bg-slate-800 z-10">
                        <tr className="border-b border-gray-200 dark:border-slate-700">
                          {dbType === 'postgres' && (
                            <th className="px-2 py-2 w-8">
                              <input
                                type="checkbox"
                                checked={tableData.rows.length > 0 && selectedRows.size === tableData.rows.length}
                                onChange={e => {
                                  if (e.target.checked) {
                                    setSelectedRows(new Set(tableData.rows.map((_, i) => i)))
                                  } else {
                                    setSelectedRows(new Set())
                                  }
                                }}
                                className="rounded border-gray-300 dark:border-slate-600"
                              />
                            </th>
                          )}
                          <th className="px-2 py-2 text-left text-xs font-medium text-gray-400 dark:text-slate-500 w-10">#</th>
                          {tableData.columns.map((col, i) => (
                            <th key={i} className="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-slate-400 uppercase whitespace-nowrap">{col}</th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {tableData.rows.map((row, ri) => (
                          <tr
                            key={ri}
                            className={`border-b border-gray-100 dark:border-slate-700/50 ${selectedRows.has(ri) ? 'bg-red-50/50 dark:bg-red-900/10' : 'hover:bg-gray-50 dark:hover:bg-slate-700/30'}`}
                          >
                            {dbType === 'postgres' && (
                              <td className="px-2 py-1.5 w-8">
                                <input
                                  type="checkbox"
                                  checked={selectedRows.has(ri)}
                                  onChange={e => {
                                    const next = new Set(selectedRows)
                                    if (e.target.checked) next.add(ri)
                                    else next.delete(ri)
                                    setSelectedRows(next)
                                  }}
                                  className="rounded border-gray-300 dark:border-slate-600"
                                />
                              </td>
                            )}
                            <td className="px-2 py-1.5 text-xs text-gray-400 dark:text-slate-500 w-10">{browseOffset + ri + 1}</td>
                            {row.map((val, ci) => {
                              const isEditing = editCell?.row === ri && editCell?.col === ci
                              const isFirstCol = ci === 0
                              return (
                                <td
                                  key={ci}
                                  className="px-3 py-1.5 whitespace-nowrap max-w-[300px] truncate cursor-default"
                                  onClick={() => {
                                    if (dbType === 'postgres' && !isEditing) {
                                      setEditCell({ row: ri, col: ci, value: formatCellValue(val) })
                                    }
                                  }}
                                >
                                  {isEditing ? (
                                    <input
                                      autoFocus
                                      value={editCell.value}
                                      onChange={e => setEditCell({ ...editCell, value: e.target.value })}
                                      onKeyDown={e => {
                                        if (e.key === 'Enter') saveCell()
                                        if (e.key === 'Escape') setEditCell(null)
                                      }}
                                      disabled={editSaving}
                                      className="w-full px-1 py-0.5 text-sm rounded border border-blue-500 dark:border-cyan-500 bg-white dark:bg-slate-900 text-gray-900 dark:text-cyan-400 focus:outline-none"
                                    />
                                  ) : val === null || val === undefined ? (
                                    <span className="text-gray-300 dark:text-slate-600 italic">NULL</span>
                                  ) : isFirstCol ? (
                                    <span className="text-blue-600 dark:text-blue-400 font-medium">{formatCellValue(val)}</span>
                                  ) : (
                                    <span className="text-gray-700 dark:text-slate-300">{formatCellValue(val)}</span>
                                  )}
                                </td>
                              )
                            })}
                          </tr>
                        ))}
                        {tableData.rows.length === 0 && (
                          <tr>
                            <td colSpan={tableData.columns.length + (dbType === 'postgres' ? 2 : 1)} className="px-4 py-8 text-center text-gray-400 dark:text-slate-500">
                              No rows
                            </td>
                          </tr>
                        )}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      )}

      {/* Query View */}
      {browseView === 'query' && (
        <div className="space-y-4">
          {/* Quick table buttons */}
          {tableList && tableList.rows && (
            <div className="flex flex-wrap gap-1.5">
              {tableList.rows.map((row, i) => (
                <button
                  key={i}
                  onClick={() => setQuery(prev => {
                    const tbl = String(row[0])
                    const quoted = dbType === 'postgres' ? `"${tbl}"` : tbl
                    return prev ? `${prev}\nSELECT * FROM ${quoted} LIMIT 50` : `SELECT * FROM ${quoted} LIMIT 50`
                  })}
                  className="px-2 py-1 text-xs rounded border border-gray-200 dark:border-slate-600 text-gray-600 dark:text-slate-400 hover:border-blue-400 dark:hover:border-blue-500 hover:text-blue-600 dark:hover:text-blue-400 transition-colors"
                >
                  {String(row[0])}
                </button>
              ))}
            </div>
          )}

          {/* SQL input */}
          <div className="space-y-2">
            <textarea
              value={query}
              onChange={e => setQuery(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                  e.preventDefault()
                  executeQuery()
                }
              }}
              placeholder={`Enter SQL query... (Ctrl+Enter to execute)`}
              className="w-full h-32 rounded-lg border border-gray-300 dark:border-slate-600 bg-gray-50 dark:bg-black px-4 py-3 font-mono text-sm text-gray-900 dark:text-cyan-400 placeholder-gray-400 dark:placeholder-slate-600 focus:border-blue-500 dark:focus:border-cyan-500 focus:outline-none resize-y"
            />
            <div className="flex items-center gap-2">
              <button
                onClick={executeQuery}
                disabled={loading || !query.trim()}
                className="px-4 py-1.5 text-sm font-medium rounded-lg bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50 transition-colors"
              >
                {loading ? 'Executing...' : 'Execute'}
              </button>
              <button
                onClick={() => { setQuery(''); setResult(null) }}
                className="px-4 py-1.5 text-sm font-medium rounded-lg border border-gray-300 dark:border-slate-600 text-gray-700 dark:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-700 transition-colors"
              >
                Clear
              </button>
            </div>
          </div>

          {/* Results */}
          {result && (
            <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 overflow-hidden">
              {result.error ? (
                <div className="px-4 py-3 text-sm text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-900/20">
                  {result.error}
                </div>
              ) : (
                <>
                  <div className="px-4 py-2 border-b border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-800/50 flex items-center justify-between">
                    <span className="text-xs text-gray-500 dark:text-slate-400">
                      {result.rows.length} row{result.rows.length !== 1 ? 's' : ''} returned
                      {result.affected_rows !== undefined && result.affected_rows > 0 && ` / ${result.affected_rows} affected`}
                    </span>
                  </div>
                  {result.columns.length > 0 && (
                    <div className="max-h-[600px] overflow-y-auto">
                      <table className="w-full text-sm">
                        <thead className="sticky top-0 bg-gray-50 dark:bg-slate-800 z-10">
                          <tr className="border-b border-gray-200 dark:border-slate-700">
                            {result.columns.map((col, i) => (
                              <th key={i} className="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-slate-400 uppercase whitespace-nowrap">{col}</th>
                            ))}
                          </tr>
                        </thead>
                        <tbody>
                          {result.rows.map((row, ri) => (
                            <tr key={ri} className="border-b border-gray-100 dark:border-slate-700/50 hover:bg-gray-50 dark:hover:bg-slate-700/30">
                              {row.map((val, ci) => (
                                <td key={ci} className="px-3 py-1.5 whitespace-nowrap max-w-[300px] truncate">
                                  {val === null || val === undefined ? (
                                    <span className="text-gray-300 dark:text-slate-600 italic">NULL</span>
                                  ) : (
                                    <span className="text-gray-700 dark:text-slate-300">{formatCellValue(val)}</span>
                                  )}
                                </td>
                              ))}
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
