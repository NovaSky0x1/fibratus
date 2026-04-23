package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/rabbitstack/fibratus/pkg/fleet"
)

type YARARuleStore struct {
	db *sql.DB
}

func NewYARARuleStore(db *sql.DB) *YARARuleStore { return &YARARuleStore{db: db} }

func (s *YARARuleStore) Create(ctx context.Context, r *fleet.YARARule) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	r.UpdatedAt = time.Now().UTC()
	if r.Source == "" {
		r.Source = "manual"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO yara_rules
		 (id, account_id, name, description, content, enabled,
		  validation_status, validation_errors, source, user_modified, user_disabled,
		  created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		r.ID, r.AccountID, r.Name, r.Description, r.Content, r.Enabled,
		r.ValidationStatus, r.ValidationErrors, r.Source, r.UserModified, r.UserDisabled,
		r.CreatedAt, r.UpdatedAt)
	return err
}

// Upsert inserts or updates a rule keyed by (account_id, name). Returns
// "created" | "updated" | "skipped-user-modified". Used by GitHub sync so
// re-pulling a rule from the same repo overwrites the stored content, but
// never stomps a local edit made via the dashboard.
func (s *YARARuleStore) Upsert(ctx context.Context, r *fleet.YARARule) (string, error) {
	existing := &fleet.YARARule{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, account_id, name, description, content, enabled,
		        COALESCE(validation_status,'valid'), COALESCE(validation_errors,''),
		        COALESCE(source,'manual'), COALESCE(user_modified,false), COALESCE(user_disabled,false),
		        created_at, updated_at
		 FROM yara_rules WHERE account_id=$1 AND name=$2`,
		r.AccountID, r.Name,
	).Scan(&existing.ID, &existing.AccountID, &existing.Name, &existing.Description,
		&existing.Content, &existing.Enabled,
		&existing.ValidationStatus, &existing.ValidationErrors,
		&existing.Source, &existing.UserModified, &existing.UserDisabled,
		&existing.CreatedAt, &existing.UpdatedAt)
	if err == sql.ErrNoRows {
		if err := s.Create(ctx, r); err != nil {
			return "", err
		}
		return "created", nil
	}
	if err != nil {
		return "", err
	}
	if existing.UserModified {
		return "skipped-user-modified", nil
	}
	r.ID = existing.ID
	r.UpdatedAt = time.Now().UTC()
	_, err = s.db.ExecContext(ctx,
		`UPDATE yara_rules SET
		    description=$3, content=$4,
		    enabled=$5 AND NOT user_disabled,
		    validation_status=$6, validation_errors=$7,
		    source=$8, updated_at=$9
		 WHERE account_id=$1 AND name=$2`,
		r.AccountID, r.Name, r.Description, r.Content, r.Enabled,
		r.ValidationStatus, r.ValidationErrors, r.Source, r.UpdatedAt)
	if err != nil {
		return "", err
	}
	return "updated", nil
}

// DeleteBySourceExcept removes rules of a given source whose names are not
// in keepNames. Used by GitHub sync to prune rules that vanished from the
// upstream repo. Rules with user_modified=true are retained regardless so
// a local edit survives a repo-side delete.
func (s *YARARuleStore) DeleteBySourceExcept(ctx context.Context, accountID, source string, keepNames []string) (int, error) {
	var res sql.Result
	var err error
	if len(keepNames) == 0 {
		res, err = s.db.ExecContext(ctx,
			`DELETE FROM yara_rules
			 WHERE account_id=$1 AND source=$2 AND NOT user_modified`,
			accountID, source)
	} else {
		res, err = s.db.ExecContext(ctx,
			`DELETE FROM yara_rules
			 WHERE account_id=$1 AND source=$2 AND NOT user_modified AND name <> ALL($3)`,
			accountID, source, pq.Array(keepNames))
	}
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *YARARuleStore) Get(ctx context.Context, accountID, id string) (*fleet.YARARule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, account_id, name, description, content, enabled,
		        validation_status, validation_errors, created_at, updated_at,
		        COALESCE(source,'manual'), COALESCE(user_modified,false), COALESCE(user_disabled,false)
		 FROM yara_rules WHERE account_id=$1 AND id=$2`, accountID, id)
	return scanYARARule(row)
}

func (s *YARARuleStore) List(ctx context.Context, accountID string) ([]*fleet.YARARule, error) {
	return s.queryRules(ctx,
		`SELECT id, account_id, name, description, content, enabled,
		        validation_status, validation_errors, created_at, updated_at,
		        COALESCE(source,'manual'), COALESCE(user_modified,false), COALESCE(user_disabled,false)
		 FROM yara_rules WHERE account_id=$1 ORDER BY name`, accountID)
}

func (s *YARARuleStore) ListEnabled(ctx context.Context, accountID string) ([]*fleet.YARARule, error) {
	return s.queryRules(ctx,
		`SELECT id, account_id, name, description, content, enabled,
		        validation_status, validation_errors, created_at, updated_at,
		        COALESCE(source,'manual'), COALESCE(user_modified,false), COALESCE(user_disabled,false)
		 FROM yara_rules
		 WHERE account_id=$1 AND enabled=true AND validation_status='valid'
		 ORDER BY name`, accountID)
}

func (s *YARARuleStore) Update(ctx context.Context, r *fleet.YARARule) error {
	r.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE yara_rules
		 SET name=$3, description=$4, content=$5, enabled=$6,
		     validation_status=$7, validation_errors=$8,
		     user_modified=$9, user_disabled=$10, updated_at=$11
		 WHERE account_id=$1 AND id=$2`,
		r.AccountID, r.ID, r.Name, r.Description, r.Content, r.Enabled,
		r.ValidationStatus, r.ValidationErrors, r.UserModified, r.UserDisabled, r.UpdatedAt)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("yara rule not found")
	}
	return nil
}

func (s *YARARuleStore) Delete(ctx context.Context, accountID, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM yara_rules WHERE account_id=$1 AND id=$2`, accountID, id)
	return err
}

func (s *YARARuleStore) queryRules(ctx context.Context, q string, args ...any) ([]*fleet.YARARule, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*fleet.YARARule, 0)
	for rows.Next() {
		r, err := scanYARARule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanYARARule(sc rowScanner) (*fleet.YARARule, error) {
	r := &fleet.YARARule{}
	var source sql.NullString
	var userMod, userDis sql.NullBool
	err := sc.Scan(&r.ID, &r.AccountID, &r.Name, &r.Description, &r.Content, &r.Enabled,
		&r.ValidationStatus, &r.ValidationErrors, &r.CreatedAt, &r.UpdatedAt,
		&source, &userMod, &userDis)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if source.Valid {
		r.Source = source.String
	} else {
		r.Source = "manual"
	}
	r.UserModified = userMod.Bool
	r.UserDisabled = userDis.Bool
	return r, nil
}
