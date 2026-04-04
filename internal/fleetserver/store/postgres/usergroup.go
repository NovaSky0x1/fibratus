package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/rabbitstack/fibratus/pkg/fleet"
)

// UserGroupStore implements store.UserGroupStore backed by PostgreSQL.
type UserGroupStore struct {
	db *sql.DB
}

// NewUserGroupStore creates a new user group store.
func NewUserGroupStore(db *sql.DB) *UserGroupStore {
	return &UserGroupStore{db: db}
}

func (s *UserGroupStore) Create(ctx context.Context, group *fleet.UserGroup) error {
	perms, _ := json.Marshal(group.Permissions)
	orgRestrictions, _ := json.Marshal(group.OrgRestrictions)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_groups (id, account_id, name, description, permissions, org_restrictions, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		group.ID, group.AccountID, group.Name, group.Description,
		perms, orgRestrictions, group.CreatedAt, group.UpdatedAt,
	)
	return err
}

func (s *UserGroupStore) Get(ctx context.Context, id string) (*fleet.UserGroup, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, account_id, name, description, permissions, org_restrictions, created_at, updated_at
		 FROM user_groups WHERE id = $1`, id)

	g := &fleet.UserGroup{}
	var perms, orgRestrictions []byte
	err := row.Scan(&g.ID, &g.AccountID, &g.Name, &g.Description,
		&perms, &orgRestrictions, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	json.Unmarshal(perms, &g.Permissions)
	json.Unmarshal(orgRestrictions, &g.OrgRestrictions)
	return g, nil
}

func (s *UserGroupStore) List(ctx context.Context, accountID string) ([]*fleet.UserGroup, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT g.id, g.account_id, g.name, g.description, g.permissions, g.org_restrictions,
			g.created_at, g.updated_at,
			COALESCE((SELECT COUNT(*) FROM user_group_members m WHERE m.group_id = g.id), 0) as member_count
		 FROM user_groups g WHERE g.account_id = $1 ORDER BY g.name`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]*fleet.UserGroup, 0)
	for rows.Next() {
		g := &fleet.UserGroup{}
		var perms, orgRestrictions []byte
		var memberCount int
		if err := rows.Scan(&g.ID, &g.AccountID, &g.Name, &g.Description,
			&perms, &orgRestrictions, &g.CreatedAt, &g.UpdatedAt, &memberCount); err != nil {
			return nil, err
		}
		json.Unmarshal(perms, &g.Permissions)
		json.Unmarshal(orgRestrictions, &g.OrgRestrictions)
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *UserGroupStore) Update(ctx context.Context, group *fleet.UserGroup) error {
	perms, _ := json.Marshal(group.Permissions)
	orgRestrictions, _ := json.Marshal(group.OrgRestrictions)
	_, err := s.db.ExecContext(ctx,
		`UPDATE user_groups SET name = $2, description = $3, permissions = $4,
			org_restrictions = $5, updated_at = $6 WHERE id = $1`,
		group.ID, group.Name, group.Description, perms, orgRestrictions, time.Now().UTC(),
	)
	return err
}

func (s *UserGroupStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_groups WHERE id = $1`, id)
	return err
}

func (s *UserGroupStore) AddMember(ctx context.Context, userID, groupID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_group_members (user_id, group_id) VALUES ($1, $2)
		 ON CONFLICT (user_id, group_id) DO NOTHING`, userID, groupID)
	return err
}

func (s *UserGroupStore) RemoveMember(ctx context.Context, userID, groupID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM user_group_members WHERE user_id = $1 AND group_id = $2`, userID, groupID)
	return err
}

func (s *UserGroupStore) GetUserGroups(ctx context.Context, userID string) ([]fleet.UserGroupMembership, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT g.id, g.name FROM user_groups g
		 JOIN user_group_members m ON g.id = m.group_id
		 WHERE m.user_id = $1 ORDER BY g.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	memberships := make([]fleet.UserGroupMembership, 0)
	for rows.Next() {
		var m fleet.UserGroupMembership
		if err := rows.Scan(&m.GroupID, &m.GroupName); err != nil {
			return nil, err
		}
		memberships = append(memberships, m)
	}
	return memberships, rows.Err()
}
