package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

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
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO yara_rules
		 (id, account_id, name, description, content, enabled, validation_status, validation_errors, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		r.ID, r.AccountID, r.Name, r.Description, r.Content, r.Enabled,
		r.ValidationStatus, r.ValidationErrors, r.CreatedAt, r.UpdatedAt)
	return err
}

func (s *YARARuleStore) Get(ctx context.Context, accountID, id string) (*fleet.YARARule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, account_id, name, description, content, enabled,
		        validation_status, validation_errors, created_at, updated_at
		 FROM yara_rules WHERE account_id=$1 AND id=$2`, accountID, id)
	return scanYARARule(row)
}

func (s *YARARuleStore) List(ctx context.Context, accountID string) ([]*fleet.YARARule, error) {
	return s.queryRules(ctx,
		`SELECT id, account_id, name, description, content, enabled,
		        validation_status, validation_errors, created_at, updated_at
		 FROM yara_rules WHERE account_id=$1 ORDER BY name`, accountID)
}

func (s *YARARuleStore) ListEnabled(ctx context.Context, accountID string) ([]*fleet.YARARule, error) {
	return s.queryRules(ctx,
		`SELECT id, account_id, name, description, content, enabled,
		        validation_status, validation_errors, created_at, updated_at
		 FROM yara_rules
		 WHERE account_id=$1 AND enabled=true AND validation_status='valid'
		 ORDER BY name`, accountID)
}

func (s *YARARuleStore) Update(ctx context.Context, r *fleet.YARARule) error {
	r.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE yara_rules
		 SET name=$3, description=$4, content=$5, enabled=$6,
		     validation_status=$7, validation_errors=$8, updated_at=$9
		 WHERE account_id=$1 AND id=$2`,
		r.AccountID, r.ID, r.Name, r.Description, r.Content, r.Enabled,
		r.ValidationStatus, r.ValidationErrors, r.UpdatedAt)
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
	err := sc.Scan(&r.ID, &r.AccountID, &r.Name, &r.Description, &r.Content, &r.Enabled,
		&r.ValidationStatus, &r.ValidationErrors, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return r, nil
}
