package orgs

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Organization struct {
	ID        uuid.UUID
	Name      string
	Slug      string
	Status    string // active, suspended, archived
	Config    []byte // JSONB
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Team struct {
	ID          uuid.UUID
	OrgID       uuid.UUID
	Name        string
	Slug        string
	Description string
	CreatedAt   time.Time
}

type OrgMember struct {
	UserID    uuid.UUID
	Email     string
	Name      string
	Role      string // admin, member
	CreatedAt time.Time
}

type TeamMember struct {
	UserID    uuid.UUID
	Email     string
	Name      string
	Role      string
	CreatedAt time.Time
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// ---------------------------------------------------------------------------
// Organization CRUD
// ---------------------------------------------------------------------------

func (s *Store) CreateOrg(ctx context.Context, name, slug string) (*Organization, error) {
	o := &Organization{ID: uuid.New(), Name: name, Slug: slug, Status: "active", Config: []byte("{}")}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO organizations (id, name, slug, status, config)
		 VALUES ($1, $2, $3, $4, $5::jsonb)
		 RETURNING created_at, updated_at`,
		o.ID, o.Name, o.Slug, o.Status, o.Config,
	).Scan(&o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("orgs.CreateOrg: %w", err)
	}
	return o, nil
}

func (s *Store) GetOrg(ctx context.Context, id uuid.UUID) (*Organization, error) {
	o := &Organization{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, status, config, created_at, updated_at
		 FROM organizations WHERE id = $1`, id,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.Status, &o.Config, &o.CreatedAt, &o.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("orgs.GetOrg: %w", err)
	}
	return o, nil
}

func (s *Store) GetOrgBySlug(ctx context.Context, slug string) (*Organization, error) {
	o := &Organization{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, status, config, created_at, updated_at
		 FROM organizations WHERE slug = $1`, slug,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.Status, &o.Config, &o.CreatedAt, &o.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("orgs.GetOrgBySlug: %w", err)
	}
	return o, nil
}

func (s *Store) ListOrgs(ctx context.Context, limit, offset int) ([]*Organization, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM organizations`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("orgs.ListOrgs count: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, slug, status, config, created_at, updated_at
		 FROM organizations ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("orgs.ListOrgs query: %w", err)
	}
	defer rows.Close()

	var orgs []*Organization
	for rows.Next() {
		o := &Organization{}
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.Status, &o.Config, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("orgs.ListOrgs scan: %w", err)
		}
		orgs = append(orgs, o)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("orgs.ListOrgs rows: %w", err)
	}
	return orgs, total, nil
}

func (s *Store) UpdateOrg(ctx context.Context, id uuid.UUID, name, slug, status *string) error {
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
	if slug != nil {
		idx++
		sets = append(sets, fmt.Sprintf("slug = $%d", idx))
		args = append(args, *slug)
	}
	if status != nil {
		idx++
		sets = append(sets, fmt.Sprintf("status = $%d", idx))
		args = append(args, *status)
	}

	if len(sets) == 0 {
		return nil
	}

	sets = append(sets, "updated_at = now()")
	idx++
	args = append(args, id)

	q := fmt.Sprintf("UPDATE organizations SET %s WHERE id = $%d", strings.Join(sets, ", "), idx)
	_, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("orgs.UpdateOrg: %w", err)
	}
	return nil
}

func (s *Store) DeleteOrg(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM organizations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("orgs.DeleteOrg: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Org Membership
// ---------------------------------------------------------------------------

func (s *Store) AddOrgMember(ctx context.Context, orgID, userID uuid.UUID, role string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO org_memberships (id, org_id, user_id, role)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, org_id) DO UPDATE SET role = EXCLUDED.role`,
		uuid.New(), orgID, userID, role,
	)
	if err != nil {
		return fmt.Errorf("orgs.AddOrgMember: %w", err)
	}
	return nil
}

func (s *Store) RemoveOrgMember(ctx context.Context, orgID, userID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM org_memberships WHERE org_id = $1 AND user_id = $2`,
		orgID, userID,
	)
	if err != nil {
		return fmt.Errorf("orgs.RemoveOrgMember: %w", err)
	}
	return nil
}

func (s *Store) UpdateOrgMemberRole(ctx context.Context, orgID, userID uuid.UUID, role string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE org_memberships SET role = $1 WHERE org_id = $2 AND user_id = $3`,
		role, orgID, userID,
	)
	if err != nil {
		return fmt.Errorf("orgs.UpdateOrgMemberRole: %w", err)
	}
	return nil
}

func (s *Store) ListOrgMembers(ctx context.Context, orgID uuid.UUID) ([]OrgMember, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.id, u.email, u.name, om.role, om.created_at
		 FROM org_memberships om
		 JOIN users u ON u.id = om.user_id
		 WHERE om.org_id = $1
		 ORDER BY om.created_at`, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("orgs.ListOrgMembers: %w", err)
	}
	defer rows.Close()

	var members []OrgMember
	for rows.Next() {
		var m OrgMember
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("orgs.ListOrgMembers scan: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orgs.ListOrgMembers rows: %w", err)
	}
	return members, nil
}

func (s *Store) GetOrgMembership(ctx context.Context, orgID, userID uuid.UUID) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT role FROM org_memberships WHERE org_id = $1 AND user_id = $2`,
		orgID, userID,
	).Scan(&role)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("orgs.GetOrgMembership: %w", err)
	}
	return role, nil
}

// ---------------------------------------------------------------------------
// Team CRUD
// ---------------------------------------------------------------------------

func (s *Store) CreateTeam(ctx context.Context, orgID uuid.UUID, name, slug string) (*Team, error) {
	t := &Team{ID: uuid.New(), OrgID: orgID, Name: name, Slug: slug}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO teams (id, org_id, name, slug)
		 VALUES ($1, $2, $3, $4)
		 RETURNING description, created_at`,
		t.ID, t.OrgID, t.Name, t.Slug,
	).Scan(&t.Description, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("orgs.CreateTeam: %w", err)
	}
	return t, nil
}

func (s *Store) GetTeam(ctx context.Context, id uuid.UUID) (*Team, error) {
	t := &Team{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, slug, description, created_at
		 FROM teams WHERE id = $1`, id,
	).Scan(&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.Description, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("orgs.GetTeam: %w", err)
	}
	return t, nil
}

func (s *Store) ListTeams(ctx context.Context, orgID uuid.UUID) ([]*Team, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, name, slug, description, created_at
		 FROM teams WHERE org_id = $1 ORDER BY created_at`, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("orgs.ListTeams: %w", err)
	}
	defer rows.Close()

	var teams []*Team
	for rows.Next() {
		t := &Team{}
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.Description, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("orgs.ListTeams scan: %w", err)
		}
		teams = append(teams, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orgs.ListTeams rows: %w", err)
	}
	return teams, nil
}

func (s *Store) UpdateTeam(ctx context.Context, id uuid.UUID, name, slug, description *string) error {
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
	if slug != nil {
		idx++
		sets = append(sets, fmt.Sprintf("slug = $%d", idx))
		args = append(args, *slug)
	}
	if description != nil {
		idx++
		sets = append(sets, fmt.Sprintf("description = $%d", idx))
		args = append(args, *description)
	}

	if len(sets) == 0 {
		return nil
	}

	idx++
	args = append(args, id)

	q := fmt.Sprintf("UPDATE teams SET %s WHERE id = $%d", strings.Join(sets, ", "), idx)
	_, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("orgs.UpdateTeam: %w", err)
	}
	return nil
}

func (s *Store) DeleteTeam(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM teams WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("orgs.DeleteTeam: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Team Membership
// ---------------------------------------------------------------------------

func (s *Store) AddTeamMember(ctx context.Context, teamID, userID uuid.UUID, role string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO team_memberships (id, team_id, user_id, role)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (team_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		uuid.New(), teamID, userID, role,
	)
	if err != nil {
		return fmt.Errorf("orgs.AddTeamMember: %w", err)
	}
	return nil
}

func (s *Store) RemoveTeamMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM team_memberships WHERE team_id = $1 AND user_id = $2`,
		teamID, userID,
	)
	if err != nil {
		return fmt.Errorf("orgs.RemoveTeamMember: %w", err)
	}
	return nil
}

func (s *Store) ListTeamMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMember, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.id, u.email, u.name, tm.role, tm.created_at
		 FROM team_memberships tm
		 JOIN users u ON u.id = tm.user_id
		 WHERE tm.team_id = $1
		 ORDER BY tm.created_at`, teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("orgs.ListTeamMembers: %w", err)
	}
	defer rows.Close()

	var members []TeamMember
	for rows.Next() {
		var m TeamMember
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("orgs.ListTeamMembers scan: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orgs.ListTeamMembers rows: %w", err)
	}
	return members, nil
}
