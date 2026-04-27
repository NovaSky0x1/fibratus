package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/fleetauth"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// DBAdminHandler provides raw database access for root users.
type DBAdminHandler struct {
	pgDB     *sql.DB
	chDBFunc func() *sql.DB
}

// NewDBAdminHandler creates a new database admin handler. chDBFunc is a getter
// that returns the active ClickHouse connection (or nil when no profile is
// active) — using a getter rather than a fixed *sql.DB lets profile hot-swaps
// route queries to the new connection without rebuilding the handler.
func NewDBAdminHandler(pgDB *sql.DB, chDBFunc func() *sql.DB) *DBAdminHandler {
	if chDBFunc == nil {
		chDBFunc = func() *sql.DB { return nil }
	}
	return &DBAdminHandler{pgDB: pgDB, chDBFunc: chDBFunc}
}

// QueryPG handles POST /api/v1/admin/db/postgres — executes a raw PostgreSQL query.
func (h *DBAdminHandler) QueryPG(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}

	log.Warnf("fleet: DB admin query (postgres) by root: %s", truncate(req.Query, 200))

	result, err := executeQuery(h.pgDB, req.Query)
	if err != nil {
		writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
			"error":   err.Error(),
			"columns": []string{},
			"rows":    [][]interface{}{},
		}})
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: result})
}

// QueryCH handles POST /api/v1/admin/db/clickhouse — executes a raw ClickHouse query.
func (h *DBAdminHandler) QueryCH(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	chDB := h.chDBFunc()
	if chDB == nil {
		writeError(w, http.StatusNotFound, "ClickHouse not configured")
		return
	}

	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}

	log.Warnf("fleet: DB admin query (clickhouse) by root: %s", truncate(req.Query, 200))

	result, err := executeQuery(chDB, req.Query)
	if err != nil {
		writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
			"error":   err.Error(),
			"columns": []string{},
			"rows":    [][]interface{}{},
		}})
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: result})
}

// TablesPG handles GET /api/v1/admin/db/postgres/tables — lists PostgreSQL tables.
func (h *DBAdminHandler) TablesPG(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	result, err := executeQuery(h.pgDB,
		`SELECT table_name, pg_size_pretty(pg_total_relation_size(quote_ident(table_name))) as size,
			(SELECT count(*) FROM information_schema.columns c WHERE c.table_name = t.table_name AND c.table_schema = 'public') as columns
		 FROM information_schema.tables t
		 WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		 ORDER BY table_name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: result})
}

// TablesCH handles GET /api/v1/admin/db/clickhouse/tables — lists ClickHouse tables.
func (h *DBAdminHandler) TablesCH(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	chDB := h.chDBFunc()
	if chDB == nil {
		writeError(w, http.StatusNotFound, "ClickHouse not configured")
		return
	}

	result, err := executeQuery(chDB,
		`SELECT name as table_name,
			formatReadableSize(total_bytes) as size,
			total_rows as row_count,
			engine
		 FROM system.tables
		 WHERE database = 'fibratus'
		 ORDER BY name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: result})
}

// executeQuery runs a SQL query and returns columns + rows.
func executeQuery(db *sql.DB, query string) (map[string]interface{}, error) {
	query = strings.TrimSpace(query)

	// For SELECT queries, return columns + rows
	upperQuery := strings.ToUpper(query)
	if strings.HasPrefix(upperQuery, "SELECT") || strings.HasPrefix(upperQuery, "SHOW") ||
		strings.HasPrefix(upperQuery, "DESCRIBE") || strings.HasPrefix(upperQuery, "EXPLAIN") {

		rows, err := db.Query(query)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			return nil, err
		}

		var resultRows [][]interface{}
		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
				continue
			}
			// Convert to JSON-safe types
			row := make([]interface{}, len(columns))
			for i, v := range values {
				switch val := v.(type) {
				case []byte:
					row[i] = string(val)
				default:
					row[i] = val
				}
			}
			resultRows = append(resultRows, row)
		}

		return map[string]interface{}{
			"columns":       columns,
			"rows":          resultRows,
			"affected_rows": len(resultRows),
		}, nil
	}

	// For non-SELECT (INSERT, UPDATE, DELETE, ALTER, etc.)
	result, err := db.Exec(query)
	if err != nil {
		return nil, err
	}
	affected, _ := result.RowsAffected()
	return map[string]interface{}{
		"columns":       []string{"result"},
		"rows":          [][]interface{}{{"Query executed successfully"}},
		"affected_rows": affected,
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
