import { useState, useEffect, useCallback } from 'react'
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

interface DbOverview {
  totalTables: number
  totalSize: string
  extra: string
}

const browseLimit = 50
const HISTORY_KEY = 'db-query-history'
const MAX_HISTORY = 20

function downloadCSV(columns: string[], rows: unknown[][], filename: string) {
  const header = columns.join(',')
  const body = rows.map(row => row.map(cell => {
    const val = cell === null ? '' : String(cell)
    return val.includes(',') || val.includes('"') || val.includes('\n') ? `"${val.replace(/"/g, '""')}"` : val
  }).join(',')).join('\n')
  const blob = new Blob([header + '\n' + body], { type: 'text/csv' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url; a.download = filename; a.click()
  URL.revokeObjectURL(url)
}

function downloadJSON(columns: string[], rows: unknown[][], filename: string) {
  const data = rows.map(row => {
    const obj: Record<string, unknown> = {}
    columns.forEach((col, i) => { obj[col] = row[i] })
    return obj
  })
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url; a.download = filename; a.click()
  URL.revokeObjectURL(url)
}

function loadHistory(): string[] {
  try {
    const raw = localStorage.getItem(HISTORY_KEY)
    if (raw) return JSON.parse(raw)
  } catch { /* ignore */ }
  return []
}

function saveHistory(history: string[]) {
  try {
    localStorage.setItem(HISTORY_KEY, JSON.stringify(history.slice(0, MAX_HISTORY)))
  } catch { /* ignore */ }
}

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

  // New state
  const [sortColumn, setSortColumn] = useState<string | null>(null)
  const [sortDir, setSortDir] = useState<'ASC' | 'DESC'>('ASC')
  const [rowCount, setRowCount] = useState<number | null>(null)
  const [queryHistory, setQueryHistory] = useState<string[]>(loadHistory)
  const [showHistory, setShowHistory] = useState(false)
  const [queryTime, setQueryTime] = useState<number | null>(null)
  const [showInsert, setShowInsert] = useState(false)
  const [insertValues, setInsertValues] = useState<Record<string, string>>({})
  const [insertLoading, setInsertLoading] = useState(false)
  const [showOperations, setShowOperations] = useState(false)
  const [operationLoading, setOperationLoading] = useState<string | null>(null)
  const [dropConfirmName, setDropConfirmName] = useState('')
  const [dbOverview, setDbOverview] = useState<DbOverview | null>(null)
  const [overviewLoading, setOverviewLoading] = useState(false)

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

  const tables = dbType === 'postgres' ? pgTables : chTables

  // Load database overview when in tables view
  const fetchOverview = useCallback(async () => {
    setOverviewLoading(true)
    try {
      const fn = dbType === 'postgres' ? api.dbQueryPostgres : api.dbQueryClickhouse
      if (dbType === 'postgres') {
        const [sizeRes, connRes] = await Promise.all([
          fn(`SELECT pg_size_pretty(sum(pg_total_relation_size(quote_ident(schemaname) || '.' || quote_ident(tablename)))) AS total_size, count(*) AS table_count FROM pg_tables WHERE schemaname = 'public'`),
          fn(`SELECT count(*) AS active_connections FROM pg_stat_activity WHERE state = 'active'`),
        ])
        const tableCount = sizeRes.data?.rows?.[0]?.[1] ?? 0
        const totalSize = sizeRes.data?.rows?.[0]?.[0] ?? '0 bytes'
        const activeConns = connRes.data?.rows?.[0]?.[0] ?? 0
        setDbOverview({
          totalTables: Number(tableCount),
          totalSize: String(totalSize),
          extra: `${activeConns} active connections`,
        })
      } else {
        const res = await fn(`SELECT count() AS table_count, formatReadableSize(sum(total_bytes)) AS total_size, sum(total_rows) AS total_rows FROM system.tables WHERE database = 'fibratus'`)
        const row = res.data?.rows?.[0]
        setDbOverview({
          totalTables: Number(row?.[0] ?? 0),
          totalSize: String(row?.[1] ?? '0 B'),
          extra: `${Number(row?.[2] ?? 0).toLocaleString()} total rows`,
        })
      }
    } catch {
      setDbOverview(null)
    } finally {
      setOverviewLoading(false)
    }
  }, [dbType])

  useEffect(() => {
    if (browseView === 'tables') fetchOverview()
  }, [browseView, dbType, fetchOverview])

  function buildDataQuery(table: string, offset: number, sortCol: string | null, sortDirection: 'ASC' | 'DESC') {
    const quotedTable = dbType === 'postgres' ? `"${table}"` : table
    let q = `SELECT * FROM ${quotedTable}`
    if (sortCol) {
      const quotedCol = dbType === 'postgres' ? `"${sortCol}"` : sortCol
      q += ` ORDER BY ${quotedCol} ${sortDirection}`
    }
    q += ` LIMIT ${browseLimit} OFFSET ${offset}`
    return q
  }

  async function executeQuery() {
    if (!query.trim()) return
    setLoading(true)
    setResult(null)
    setQueryTime(null)
    const start = performance.now()
    try {
      const fn = dbType === 'postgres' ? api.dbQueryPostgres : api.dbQueryClickhouse
      const res = await fn(query)
      const elapsed = performance.now() - start
      setQueryTime(elapsed)
      if (res.data) {
        setResult(res.data)
      } else if (res.error) {
        setResult({ columns: [], rows: [], error: res.error.message })
      }
      // Save to history
      const trimmed = query.trim()
      const updated = [trimmed, ...queryHistory.filter(h => h !== trimmed)].slice(0, MAX_HISTORY)
      setQueryHistory(updated)
      saveHistory(updated)
    } catch (err) {
      setQueryTime(performance.now() - start)
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
    setSortColumn(null)
    setSortDir('ASC')
    setRowCount(null)
    setShowInsert(false)
    setShowOperations(false)
    setDropConfirmName('')
    setTableLoading(true)
    setTableColumns(null)
    setTableData(null)
    try {
      const fn = dbType === 'postgres' ? api.dbQueryPostgres : api.dbQueryClickhouse
      const schemaQuery = dbType === 'postgres'
        ? `SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_schema = 'public' AND table_name = '${tableName}' ORDER BY ordinal_position`
        : `SELECT name AS column_name, type AS data_type, default_expression AS column_default FROM system.columns WHERE database = 'fibratus' AND table = '${tableName}' ORDER BY position`
      const dataQuery = buildDataQuery(tableName, 0, null, 'ASC')
      const countQuery = dbType === 'postgres'
        ? `SELECT COUNT(*) FROM "${tableName}"`
        : `SELECT COUNT(*) FROM ${tableName}`
      const [schemaRes, dataRes, countRes] = await Promise.all([fn(schemaQuery), fn(dataQuery), fn(countQuery)])
      if (schemaRes.data) setTableColumns(schemaRes.data)
      if (dataRes.data) setTableData(dataRes.data)
      if (countRes.data?.rows?.[0]?.[0] !== undefined) setRowCount(Number(countRes.data.rows[0][0]))
    } catch {
      // silently handle
    } finally {
      setTableLoading(false)
    }
  }

  async function loadPage(offset: number, overrideSortCol?: string | null, overrideSortDir?: 'ASC' | 'DESC') {
    if (!activeTable) return
    setBrowseOffset(offset)
    setSelectedRows(new Set())
    setTableLoading(true)
    const sc = overrideSortCol !== undefined ? overrideSortCol : sortColumn
    const sd = overrideSortDir !== undefined ? overrideSortDir : sortDir
    try {
      const fn = dbType === 'postgres' ? api.dbQueryPostgres : api.dbQueryClickhouse
      const dataQuery = buildDataQuery(activeTable, offset, sc, sd)
      const res = await fn(dataQuery)
      if (res.data) setTableData(res.data)
    } catch {
      // silently handle
    } finally {
      setTableLoading(false)
    }
  }

  function handleColumnSort(col: string) {
    let newDir: 'ASC' | 'DESC' = 'ASC'
    if (sortColumn === col) {
      newDir = sortDir === 'ASC' ? 'DESC' : 'ASC'
    }
    setSortColumn(col)
    setSortDir(newDir)
    loadPage(0, col, newDir)
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
      await loadPage(browseOffset)
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
      // Refresh count
      const countRes = await api.dbQueryPostgres(`SELECT COUNT(*) FROM "${activeTable}"`)
      if (countRes.data?.rows?.[0]?.[0] !== undefined) setRowCount(Number(countRes.data.rows[0][0]))
    } catch {
      // silently handle
    } finally {
      setDeleting(false)
    }
  }

  async function insertRow() {
    if (!activeTable || !tableColumns || dbType !== 'postgres') return
    setInsertLoading(true)
    try {
      const cols = tableColumns.rows.map(r => String(r[0]))
      const filledCols = cols.filter(c => insertValues[c]?.trim())
      if (filledCols.length === 0) { setInsertLoading(false); return }
      const colList = filledCols.map(c => `"${c}"`).join(', ')
      const valList = filledCols.map(c => {
        const v = insertValues[c].trim()
        if (v.toUpperCase() === 'NULL') return 'NULL'
        if (v.toUpperCase() === 'DEFAULT') return 'DEFAULT'
        return `'${v.replace(/'/g, "''")}'`
      }).join(', ')
      await api.dbQueryPostgres(`INSERT INTO "${activeTable}" (${colList}) VALUES (${valList})`)
      setShowInsert(false)
      setInsertValues({})
      await loadPage(browseOffset)
      const countRes = await api.dbQueryPostgres(`SELECT COUNT(*) FROM "${activeTable}"`)
      if (countRes.data?.rows?.[0]?.[0] !== undefined) setRowCount(Number(countRes.data.rows[0][0]))
    } catch {
      // silently handle
    } finally {
      setInsertLoading(false)
    }
  }

  async function runTableOperation(op: string) {
    if (!activeTable) return
    setOperationLoading(op)
    try {
      const fn = dbType === 'postgres' ? api.dbQueryPostgres : api.dbQueryClickhouse
      const quotedTable = dbType === 'postgres' ? `"${activeTable}"` : activeTable
      let q = ''
      switch (op) {
        case 'vacuum': q = `VACUUM "${activeTable}"`; break
        case 'analyze': q = `ANALYZE "${activeTable}"`; break
        case 'reindex': q = `REINDEX TABLE "${activeTable}"`; break
        case 'optimize': q = `OPTIMIZE TABLE ${activeTable}`; break
        case 'truncate':
          if (!confirm(`TRUNCATE all rows from "${activeTable}"? This cannot be undone.`)) {
            setOperationLoading(null)
            return
          }
          q = dbType === 'postgres' ? `TRUNCATE ${quotedTable}` : `TRUNCATE TABLE ${quotedTable}`
          break
        case 'drop':
          if (dropConfirmName !== activeTable) {
            alert('Table name does not match. Type the exact table name to confirm DROP.')
            setOperationLoading(null)
            return
          }
          if (!confirm(`DROP TABLE "${activeTable}"? This will permanently destroy the table and all its data.`)) {
            setOperationLoading(null)
            return
          }
          q = `DROP TABLE ${quotedTable}`
          break
        default: setOperationLoading(null); return
      }
      await fn(q)
      if (op === 'drop') {
        setActiveTable(null)
        setBrowseView('tables')
        pgTables.refetch()
        chTables.refetch()
      } else {
        await loadPage(browseOffset)
        const countFn = dbType === 'postgres' ? api.dbQueryPostgres : api.dbQueryClickhouse
        const countQ = dbType === 'postgres'
          ? `SELECT COUNT(*) FROM "${activeTable}"`
          : `SELECT COUNT(*) FROM ${activeTable}`
        const countRes = await countFn(countQ)
        if (countRes.data?.rows?.[0]?.[0] !== undefined) setRowCount(Number(countRes.data.rows[0][0]))
      }
    } catch {
      // silently handle
    } finally {
      setOperationLoading(null)
      setDropConfirmName('')
    }
  }

  function formatCellValue(val: unknown): string {
    if (val === null || val === undefined) return 'NULL'
    if (typeof val === 'object') return JSON.stringify(val)
    return String(val)
  }

  const tableList = tables.data?.data
  const sortedTableRows = tableList?.rows
    ? [...tableList.rows].sort((a, b) => String(a[0]).localeCompare(String(b[0])))
    : null

  return (
    <div className="space-y-4">
      {/* Server Info Bar */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4">
        <div className="grid grid-cols-3 gap-4">
          <div>
            <div className="text-xs text-gray-500 dark:text-slate-400 mb-1">Server URL</div>
            <div className="text-sm font-medium text-gray-900 dark:text-slate-100 truncate">{window.location.origin}</div>
          </div>
          <div>
            <div className="text-xs text-gray-500 dark:text-slate-400 mb-1">Version</div>
            <div className="text-sm font-medium text-gray-900 dark:text-slate-100">Fleet v1.0</div>
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
              {rowCount !== null && (
                <span className="ml-2 text-xs text-gray-400 dark:text-slate-500">({rowCount.toLocaleString()} rows)</span>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Database Overview Bar */}
      {browseView === 'tables' && dbOverview && !overviewLoading && (
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-3">
          <div className="flex items-center gap-6">
            <div className="flex items-center gap-2">
              <svg className="w-4 h-4 text-gray-400 dark:text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 7v10c0 2 8 2 8 2s8 0 8-2V7M4 7c0 2 8 2 8 2s8 0 8-2M4 7c0-2 8-2 8-2s8 0 8 2" />
              </svg>
              <span className="text-sm text-gray-700 dark:text-slate-300"><span className="font-semibold">{dbOverview.totalTables}</span> tables</span>
            </div>
            <div className="flex items-center gap-2">
              <svg className="w-4 h-4 text-gray-400 dark:text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 7v10c0 2 8 2 8 2s8 0 8-2V7" />
              </svg>
              <span className="text-sm text-gray-700 dark:text-slate-300"><span className="font-semibold">{dbOverview.totalSize}</span> total size</span>
            </div>
            <div className="flex items-center gap-2">
              <svg className="w-4 h-4 text-gray-400 dark:text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
              </svg>
              <span className="text-sm text-gray-700 dark:text-slate-300">{dbOverview.extra}</span>
            </div>
          </div>
        </div>
      )}

      {/* Tables List View */}
      {browseView === 'tables' && (
        <div>
          {tables.isLoading ? (
            <div className="text-center py-12 text-gray-500 dark:text-slate-400">Loading tables...</div>
          ) : tableList && tableList.columns && sortedTableRows ? (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
              {sortedTableRows.map((row, i) => {
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
          {tableLoading && !tableData ? (
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
                    <div className="flex items-center gap-3">
                      <span className="text-sm font-medium text-gray-700 dark:text-slate-300">
                        Data {tableData.rows.length > 0 && `(${browseOffset + 1}-${browseOffset + tableData.rows.length}`}{rowCount !== null && ` of ${rowCount.toLocaleString()}`}{tableData.rows.length > 0 && ')'}
                      </span>
                      {sortColumn && (
                        <span className="text-xs text-gray-400 dark:text-slate-500">
                          sorted by {sortColumn} {sortDir}
                        </span>
                      )}
                    </div>
                    <div className="flex items-center gap-2">
                      {/* Export buttons */}
                      <button
                        onClick={() => downloadCSV(tableData.columns, tableData.rows, `${activeTable}.csv`)}
                        className="rounded border border-gray-300 dark:border-slate-600 px-2 py-1 text-xs text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700"
                        title="Export CSV"
                      >
                        CSV
                      </button>
                      <button
                        onClick={() => downloadJSON(tableData.columns, tableData.rows, `${activeTable}.json`)}
                        className="rounded border border-gray-300 dark:border-slate-600 px-2 py-1 text-xs text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700"
                        title="Export JSON"
                      >
                        JSON
                      </button>
                      <div className="w-px h-4 bg-gray-200 dark:bg-slate-600" />
                      {/* Insert Row (PG only) */}
                      {dbType === 'postgres' && tableColumns && (
                        <button
                          onClick={() => {
                            setShowInsert(!showInsert)
                            if (!showInsert) {
                              const defaults: Record<string, string> = {}
                              tableColumns.rows.forEach(r => {
                                const colName = String(r[0])
                                const colDefault = r[3]
                                defaults[colName] = colDefault ? String(colDefault) : ''
                              })
                              setInsertValues(defaults)
                            }
                          }}
                          className="rounded border border-green-300 dark:border-green-700 px-2 py-1 text-xs text-green-600 dark:text-green-400 hover:bg-green-50 dark:hover:bg-green-900/20"
                        >
                          + Insert Row
                        </button>
                      )}
                      <div className="w-px h-4 bg-gray-200 dark:bg-slate-600" />
                      {/* Pagination */}
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
                    </div>
                  </div>

                  {/* Insert Row Panel */}
                  {showInsert && dbType === 'postgres' && tableColumns && (
                    <div className="px-4 py-3 border-b border-gray-200 dark:border-slate-700 bg-green-50/50 dark:bg-green-900/10">
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-sm font-medium text-green-700 dark:text-green-400">Insert New Row</span>
                        <button onClick={() => setShowInsert(false)} className="text-xs text-gray-400 hover:text-gray-600 dark:hover:text-slate-300">Close</button>
                      </div>
                      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2">
                        {tableColumns.rows.map((r, i) => {
                          const colName = String(r[0])
                          const colType = String(r[1])
                          return (
                            <div key={i}>
                              <label className="block text-xs text-gray-500 dark:text-slate-400 mb-0.5">
                                {colName} <span className="text-gray-300 dark:text-slate-600">({colType})</span>
                              </label>
                              <input
                                value={insertValues[colName] || ''}
                                onChange={e => setInsertValues(prev => ({ ...prev, [colName]: e.target.value }))}
                                placeholder="NULL"
                                className="w-full px-2 py-1 text-xs rounded border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-gray-900 dark:text-slate-100 focus:border-green-500 dark:focus:border-green-500 focus:outline-none"
                              />
                            </div>
                          )
                        })}
                      </div>
                      <div className="mt-2 flex items-center gap-2">
                        <button
                          onClick={insertRow}
                          disabled={insertLoading}
                          className="px-3 py-1 text-xs font-medium rounded bg-green-600 text-white hover:bg-green-700 disabled:opacity-50"
                        >
                          {insertLoading ? 'Inserting...' : 'Insert'}
                        </button>
                        <span className="text-xs text-gray-400 dark:text-slate-500">Leave blank for NULL, type DEFAULT for column default</span>
                      </div>
                    </div>
                  )}

                  <div className="max-h-[600px] overflow-auto">
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
                            <th
                              key={i}
                              onClick={() => handleColumnSort(col)}
                              className="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-slate-400 uppercase whitespace-nowrap cursor-pointer hover:text-blue-600 dark:hover:text-blue-400 select-none"
                            >
                              <span className="flex items-center gap-1">
                                {col}
                                {sortColumn === col && (
                                  <span className="text-blue-500 dark:text-blue-400">
                                    {sortDir === 'ASC' ? '\u2191' : '\u2193'}
                                  </span>
                                )}
                              </span>
                            </th>
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

              {/* Table Operations Panel */}
              <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 overflow-hidden">
                <button
                  onClick={() => setShowOperations(!showOperations)}
                  className="w-full px-4 py-2.5 border-b border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-800/50 flex items-center justify-between"
                >
                  <span className="text-sm font-medium text-gray-700 dark:text-slate-300">Operations</span>
                  <svg className={`w-4 h-4 text-gray-400 dark:text-slate-500 transition-transform ${showOperations ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                  </svg>
                </button>
                {showOperations && (
                  <div className="px-4 py-3 space-y-3">
                    {/* Maintenance operations */}
                    <div>
                      <div className="text-xs text-gray-500 dark:text-slate-400 mb-2">Maintenance</div>
                      <div className="flex flex-wrap items-center gap-2">
                        {dbType === 'postgres' && (
                          <>
                            <button
                              onClick={() => runTableOperation('vacuum')}
                              disabled={operationLoading !== null}
                              className="rounded border border-gray-300 dark:border-slate-600 px-2 py-1 text-xs text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700 disabled:opacity-50"
                            >
                              {operationLoading === 'vacuum' ? 'Running...' : 'VACUUM'}
                            </button>
                            <button
                              onClick={() => runTableOperation('analyze')}
                              disabled={operationLoading !== null}
                              className="rounded border border-gray-300 dark:border-slate-600 px-2 py-1 text-xs text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700 disabled:opacity-50"
                            >
                              {operationLoading === 'analyze' ? 'Running...' : 'ANALYZE'}
                            </button>
                            <button
                              onClick={() => runTableOperation('reindex')}
                              disabled={operationLoading !== null}
                              className="rounded border border-gray-300 dark:border-slate-600 px-2 py-1 text-xs text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700 disabled:opacity-50"
                            >
                              {operationLoading === 'reindex' ? 'Running...' : 'REINDEX'}
                            </button>
                          </>
                        )}
                        {dbType === 'clickhouse' && (
                          <button
                            onClick={() => runTableOperation('optimize')}
                            disabled={operationLoading !== null}
                            className="rounded border border-gray-300 dark:border-slate-600 px-2 py-1 text-xs text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700 disabled:opacity-50"
                          >
                            {operationLoading === 'optimize' ? 'Running...' : 'OPTIMIZE TABLE'}
                          </button>
                        )}
                      </div>
                    </div>
                    {/* Danger zone */}
                    <div>
                      <div className="text-xs text-red-500 dark:text-red-400 mb-2">Danger Zone</div>
                      <div className="flex flex-wrap items-center gap-2">
                        <button
                          onClick={() => runTableOperation('truncate')}
                          disabled={operationLoading !== null}
                          className="rounded border border-red-300 dark:border-red-700 px-2 py-1 text-xs text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50"
                        >
                          {operationLoading === 'truncate' ? 'Running...' : 'TRUNCATE TABLE'}
                        </button>
                        <div className="flex items-center gap-1">
                          <input
                            value={dropConfirmName}
                            onChange={e => setDropConfirmName(e.target.value)}
                            placeholder={`Type "${activeTable}" to confirm`}
                            className="px-2 py-1 text-xs rounded border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-gray-900 dark:text-slate-100 focus:border-red-500 focus:ring-1 focus:ring-red-500 focus:outline-none w-48"
                          />
                          <button
                            onClick={() => runTableOperation('drop')}
                            disabled={operationLoading !== null || dropConfirmName !== activeTable}
                            className="rounded border border-red-300 dark:border-red-700 px-2 py-1 text-xs text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50"
                          >
                            {operationLoading === 'drop' ? 'Dropping...' : 'DROP TABLE'}
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                )}
              </div>
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
              {[...tableList.rows].sort((a, b) => String(a[0]).localeCompare(String(b[0]))).map((row, i) => (
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

          {/* Query History */}
          {queryHistory.length > 0 && (
            <div>
              <button
                onClick={() => setShowHistory(!showHistory)}
                className="flex items-center gap-1 text-xs text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300 mb-1"
              >
                <svg className={`w-3 h-3 transition-transform ${showHistory ? 'rotate-90' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                </svg>
                Query History ({queryHistory.length})
              </button>
              {showHistory && (
                <div className="rounded-lg border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-2 max-h-40 overflow-y-auto space-y-1">
                  {queryHistory.map((h, i) => (
                    <button
                      key={i}
                      onClick={() => setQuery(h)}
                      className="block w-full text-left text-xs font-mono text-gray-500 cursor-pointer hover:text-blue-600 truncate px-1 py-0.5 rounded hover:bg-gray-50 dark:hover:bg-slate-700"
                      title={h}
                    >
                      {h}
                    </button>
                  ))}
                </div>
              )}
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
                onClick={() => { setQuery(''); setResult(null); setQueryTime(null) }}
                className="px-4 py-1.5 text-sm font-medium rounded-lg border border-gray-300 dark:border-slate-600 text-gray-700 dark:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-700 transition-colors"
              >
                Clear
              </button>
              {queryTime !== null && (
                <span className="text-xs text-gray-400 dark:text-slate-500">
                  {result ? `${result.rows.length} row${result.rows.length !== 1 ? 's' : ''} returned in ${queryTime.toFixed(0)} ms` : `${queryTime.toFixed(0)} ms`}
                </span>
              )}
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
                      {queryTime !== null && ` in ${queryTime.toFixed(0)} ms`}
                    </span>
                    {result.columns.length > 0 && result.rows.length > 0 && (
                      <div className="flex items-center gap-2">
                        <button
                          onClick={() => downloadCSV(result.columns, result.rows, 'query-result.csv')}
                          className="rounded border border-gray-300 dark:border-slate-600 px-2 py-1 text-xs text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700"
                        >
                          CSV
                        </button>
                        <button
                          onClick={() => downloadJSON(result.columns, result.rows, 'query-result.json')}
                          className="rounded border border-gray-300 dark:border-slate-600 px-2 py-1 text-xs text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700"
                        >
                          JSON
                        </button>
                      </div>
                    )}
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
