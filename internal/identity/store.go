package identity

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

type User struct {
	ID           uuid.UUID
	Email        string
	Name         string
	PasswordHash string
	Status       string // active, suspended, deactivated
	IsSuperAdmin bool
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type OrgMembership struct {
	OrgID   uuid.UUID
	OrgName string
	OrgSlug string
	Role    string // admin, member
}

type TeamMembership struct {
	TeamID   uuid.UUID
	TeamName string
	OrgID    uuid.UUID
	Role     string
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) CreateUser(ctx context.Context, email, name, password string, isSuperAdmin bool) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("identity.CreateUser: %w", err)
	}

	u := &User{
		ID:           uuid.New(),
		Email:        email,
		Name:         name,
		PasswordHash: string(hash),
		Status:       "active",
		IsSuperAdmin: isSuperAdmin,
	}

	err = s.db.QueryRowContext(ctx,
		`INSERT INTO users (id, email, name, password_hash, status, is_super_admin)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING created_at, updated_at`,
		u.ID, u.Email, u.Name, u.PasswordHash, u.Status, u.IsSuperAdmin,
	).Scan(&u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("identity.CreateUser: %w", err)
	}
	return u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	u := &User{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, password_hash, status, is_super_admin, last_login_at, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Status, &u.IsSuperAdmin, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("identity.GetUserByID: %w", err)
	}
	return u, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	u := &User{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, password_hash, status, is_super_admin, last_login_at, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Status, &u.IsSuperAdmin, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("identity.GetUserByEmail: %w", err)
	}
	return u, nil
}

func (s *Store) ListUsers(ctx context.Context, orgID *uuid.UUID, status string, search string, limit, offset int) ([]*User, int, error) {
	var (
		where []string
		args  []any
		idx   int
	)

	from := "FROM users u"
	if orgID != nil {
		idx++
		from += " JOIN org_memberships om ON om.user_id = u.id"
		where = append(where, fmt.Sprintf("om.org_id = $%d", idx))
		args = append(args, *orgID)
	}
	if status != "" {
		idx++
		where = append(where, fmt.Sprintf("u.status = $%d", idx))
		args = append(args, status)
	}
	if search != "" {
		idx++
		pattern := "%" + search + "%"
		where = append(where, fmt.Sprintf("(u.name ILIKE $%d OR u.email ILIKE $%d)", idx, idx))
		args = append(args, pattern)
	}

	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	countQ := "SELECT COUNT(DISTINCT u.id) " + from + clause
	if err := s.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("identity.ListUsers count: %w", err)
	}

	idx++
	limitIdx := idx
	idx++
	offsetIdx := idx
	dataQ := fmt.Sprintf(
		`SELECT DISTINCT u.id, u.email, u.name, u.password_hash, u.status, u.is_super_admin, u.last_login_at, u.created_at, u.updated_at
		 %s%s ORDER BY u.created_at DESC LIMIT $%d OFFSET $%d`,
		from, clause, limitIdx, offsetIdx,
	)
	dataArgs := append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, dataQ, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("identity.ListUsers query: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Status, &u.IsSuperAdmin, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("identity.ListUsers scan: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("identity.ListUsers rows: %w", err)
	}
	return users, total, nil
}

func (s *Store) UpdateUser(ctx context.Context, id uuid.UUID, name, email, status *string, isSuperAdmin *bool) error {
	var (
		sets []string
		args []any
		idx  int
	)

	if name != nil {
		idx++
		sets = append(sets, fmt.Sprintf("name = $%d", idx))
		args = append(args, *name)
	}
	if email != nil {
		idx++
		sets = append(sets, fmt.Sprintf("email = $%d", idx))
		args = append(args, *email)
	}
	if status != nil {
		idx++
		sets = append(sets, fmt.Sprintf("status = $%d", idx))
		args = append(args, *status)
	}
	if isSuperAdmin != nil {
		idx++
		sets = append(sets, fmt.Sprintf("is_super_admin = $%d", idx))
		args = append(args, *isSuperAdmin)
	}

	if len(sets) == 0 {
		return nil
	}

	idx++
	sets = append(sets, "updated_at = now()")
	args = append(args, id)

	q := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d", strings.Join(sets, ", "), idx)
	_, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("identity.UpdateUser: %w", err)
	}
	return nil
}

func (s *Store) UpdatePassword(ctx context.Context, id uuid.UUID, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("identity.UpdatePassword: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`,
		string(hash), id,
	)
	if err != nil {
		return fmt.Errorf("identity.UpdatePassword: %w", err)
	}
	return nil
}

func (s *Store) UpdateLastLogin(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET last_login_at = now(), updated_at = now() WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("identity.UpdateLastLogin: %w", err)
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("identity.DeleteUser: %w", err)
	}
	return nil
}

func (s *Store) Authenticate(ctx context.Context, email, password string) (*User, error) {
	u, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("identity.Authenticate: %w", err)
	}
	if u == nil {
		return nil, nil
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, nil
	}
	return u, nil
}

func (s *Store) GetOrgMemberships(ctx context.Context, userID uuid.UUID) ([]OrgMembership, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT om.org_id, o.name, o.slug, om.role
		 FROM org_memberships om
		 JOIN organizations o ON o.id = om.org_id
		 WHERE om.user_id = $1`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("identity.GetOrgMemberships: %w", err)
	}
	defer rows.Close()

	var memberships []OrgMembership
	for rows.Next() {
		var m OrgMembership
		if err := rows.Scan(&m.OrgID, &m.OrgName, &m.OrgSlug, &m.Role); err != nil {
			return nil, fmt.Errorf("identity.GetOrgMemberships scan: %w", err)
		}
		memberships = append(memberships, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("identity.GetOrgMemberships rows: %w", err)
	}
	return memberships, nil
}

func (s *Store) GetTeamMemberships(ctx context.Context, userID uuid.UUID, orgID *uuid.UUID) ([]TeamMembership, error) {
	q := `SELECT tm.team_id, t.name, t.org_id, tm.role
		  FROM team_memberships tm
		  JOIN teams t ON t.id = tm.team_id
		  WHERE tm.user_id = $1`
	args := []any{userID}

	if orgID != nil {
		q += " AND t.org_id = $2"
		args = append(args, *orgID)
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("identity.GetTeamMemberships: %w", err)
	}
	defer rows.Close()

	var memberships []TeamMembership
	for rows.Next() {
		var m TeamMembership
		if err := rows.Scan(&m.TeamID, &m.TeamName, &m.OrgID, &m.Role); err != nil {
			return nil, fmt.Errorf("identity.GetTeamMemberships scan: %w", err)
		}
		memberships = append(memberships, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("identity.GetTeamMemberships rows: %w", err)
	}
	return memberships, nil
}
