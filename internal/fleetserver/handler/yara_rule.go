package handler

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// YARARuleHandler serves CRUD and validation for server-managed YARA rules.
//
// Validation strategy: the server does a lightweight syntax check (balanced
// braces, presence of at least one `rule NAME { ... }` declaration) so
// blatantly broken content fails fast. Real libyara compilation happens on
// the agent at scan time — any rule that passes the lightweight gate but
// fails libyara will surface the error inline in the scan result rather
// than block authoring.
type YARARuleHandler struct {
	rules store.YARARuleStore
}

func NewYARARuleHandler(rules store.YARARuleStore) *YARARuleHandler {
	return &YARARuleHandler{rules: rules}
}

// matches "rule <NAME>" with optional tags, colon, tags list, opening brace.
var yaraRuleDeclRe = regexp.MustCompile(`(?m)^\s*(?:private\s+|global\s+)*rule\s+[A-Za-z_][A-Za-z0-9_]*`)

// validateYARASyntax does a lightweight structural check.
// Returns ("valid", "") or ("invalid", reason).
func validateYARASyntax(content string) (status, errs string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "invalid", "rule content is empty"
	}
	if !yaraRuleDeclRe.MatchString(content) {
		return "invalid", "no `rule NAME { ... }` declaration found"
	}
	// Balanced braces outside strings/comments — approximated by
	// counting raw braces; strings would have to contain { or } to fool
	// this, which is rare in real rules.
	var depth int
	inLine := false
	inBlock := false
	inStr := false
	esc := false
	for i := 0; i < len(content); i++ {
		c := content[i]
		if inLine {
			if c == '\n' {
				inLine = false
			}
			continue
		}
		if inBlock {
			if c == '*' && i+1 < len(content) && content[i+1] == '/' {
				inBlock = false
				i++
			}
			continue
		}
		if inStr {
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '/':
			if i+1 < len(content) {
				switch content[i+1] {
				case '/':
					inLine = true
					i++
				case '*':
					inBlock = true
					i++
				}
			}
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return "invalid", "unbalanced braces: extra `}`"
			}
		}
	}
	if depth != 0 {
		return "invalid", "unbalanced braces: unclosed `{`"
	}
	return "valid", ""
}

// List handles GET /api/v1/orgs/{org_id}/yara-rules
func (h *YARARuleHandler) List(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	rules, err := h.rules.List(r.Context(), accountID)
	if err != nil {
		log.Errorf("yara-rules: list: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: rules})
}

type yaraRuleIn struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Enabled     *bool  `json:"enabled"`
}

// Create handles POST /api/v1/orgs/{org_id}/yara-rules
func (h *YARARuleHandler) Create(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	var in yaraRuleIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Content) == "" {
		writeError(w, http.StatusBadRequest, "name and content are required")
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	// Rules that fail the structural check are auto-disabled so they never
	// land in a command payload — prevents a syntax error in one rule from
	// failing the compile of the whole inline rule batch at scan time.
	// Dashboard surfaces the validation status so the user can fix + re-enable.
	status, errs := validateYARASyntax(in.Content)
	rule := &fleet.YARARule{
		ID:               GenerateID(),
		AccountID:        accountID,
		Name:             in.Name,
		Description:      in.Description,
		Content:          in.Content,
		Enabled:          enabled && status == "valid",
		ValidationStatus: status,
		ValidationErrors: errs,
	}
	if err := h.rules.Create(r.Context(), rule); err != nil {
		log.Errorf("yara-rules: create: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, fleet.Response{Data: rule})
}

// Get handles GET /api/v1/orgs/{org_id}/yara-rules/{id}
func (h *YARARuleHandler) Get(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	id := pathTail(r.URL.Path)
	rule, err := h.rules.Get(r.Context(), accountID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if rule == nil {
		writeError(w, http.StatusNotFound, "yara rule not found")
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: rule})
}

// Update handles PUT /api/v1/orgs/{org_id}/yara-rules/{id}
func (h *YARARuleHandler) Update(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	id := pathTail(r.URL.Path)
	existing, err := h.rules.Get(r.Context(), accountID, id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "yara rule not found")
		return
	}
	var in yaraRuleIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(in.Name) != "" {
		existing.Name = in.Name
	}
	existing.Description = in.Description
	contentChanged := false
	if strings.TrimSpace(in.Content) != "" && in.Content != existing.Content {
		existing.Content = in.Content
		existing.ValidationStatus, existing.ValidationErrors = validateYARASyntax(in.Content)
		contentChanged = true
	}
	if in.Enabled != nil {
		existing.Enabled = *in.Enabled && existing.ValidationStatus == "valid"
	} else if existing.ValidationStatus != "valid" {
		// Content edit flipped a previously-valid rule to invalid — force-disable
		// so the broken rule can't reach a scan payload.
		existing.Enabled = false
	}
	// Any dashboard edit on a github-sourced rule marks it user-modified so
	// subsequent GitHub syncs don't stomp the local change.
	if (contentChanged || in.Enabled != nil) && strings.HasPrefix(existing.Source, "github:") {
		existing.UserModified = true
	}
	if err := h.rules.Update(r.Context(), existing); err != nil {
		log.Errorf("yara-rules: update: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: existing})
}

// Delete handles DELETE /api/v1/orgs/{org_id}/yara-rules/{id}
func (h *YARARuleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	id := pathTail(r.URL.Path)
	if err := h.rules.Delete(r.Context(), accountID, id); err != nil {
		log.Errorf("yara-rules: delete: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Validate handles POST /api/v1/orgs/{org_id}/yara-rules/validate
// Dry-run check without persisting. Accepts {"content": "..."}.
func (h *YARARuleHandler) Validate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Content) == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	status, errs := validateYARASyntax(in.Content)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{
		"status": status, "errors": errs,
	}})
}

// pathTail returns the last non-empty segment of an URL path. Expects the
// route pattern /api/v1/orgs/{org}/yara-rules/{id} — strips the trailing
// /validate etc. if present.
func pathTail(p string) string {
	p = strings.TrimSuffix(p, "/")
	idx := strings.LastIndex(p, "/")
	if idx < 0 {
		return ""
	}
	return p[idx+1:]
}
