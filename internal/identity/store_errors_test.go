package identity

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/google/uuid"
)

var errDB = errors.New("db error")

func TestGetUserByIDQueryError(t *testing.T) {
	conn := &mockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return nil, errDB
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	_, err := s.GetUserByID(context.Background(), uuid.New())
	if err == nil {
		t.Error("expected error from query failure")
	}
}

func TestGetUserByEmailQueryError(t *testing.T) {
	conn := &mockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return nil, errDB
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	_, err := s.GetUserByEmail(context.Background(), "x@x.com")
	if err == nil {
		t.Error("expected error from query failure")
	}
}

func TestDeleteUserExecError(t *testing.T) {
	conn := &mockConn{
		execFunc: func(_ string, _ []driver.Value) (driver.Result, error) {
			return nil, errDB
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	err := s.DeleteUser(context.Background(), uuid.New())
	if err == nil {
		t.Error("expected error from exec failure")
	}
}

func TestUpdateLastLoginExecError(t *testing.T) {
	conn := &mockConn{
		execFunc: func(_ string, _ []driver.Value) (driver.Result, error) {
			return nil, errDB
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	err := s.UpdateLastLogin(context.Background(), uuid.New())
	if err == nil {
		t.Error("expected error from exec failure")
	}
}

func TestUpdateUserExecError(t *testing.T) {
	conn := &mockConn{
		execFunc: func(_ string, _ []driver.Value) (driver.Result, error) {
			return nil, errDB
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	name := "Alice"
	err := s.UpdateUser(context.Background(), uuid.New(), &name, nil, nil, nil)
	if err == nil {
		t.Error("expected error from exec failure")
	}
}

func TestUpdateUserAllFields(t *testing.T) {
	var execCalled bool
	conn := &mockConn{
		execFunc: func(_ string, _ []driver.Value) (driver.Result, error) {
			execCalled = true
			return driver.RowsAffected(1), nil
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	name := "Alice"
	email := "alice@example.com"
	status := "active"
	superAdmin := true
	err := s.UpdateUser(context.Background(), uuid.New(), &name, &email, &status, &superAdmin)
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if !execCalled {
		t.Error("expected exec to be called")
	}
}

func TestListUsersCountError(t *testing.T) {
	conn := &mockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return nil, errDB
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	_, _, err := s.ListUsers(context.Background(), "active", "", 10, 0)
	if err == nil {
		t.Error("expected error from count query failure")
	}
}

func TestListUsersWithFilters(t *testing.T) {
	callCount := 0
	conn := &mockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			callCount++
			if callCount == 1 {
				return &mockRows{
					cols: []string{"count"},
					data: [][]driver.Value{{int64(0)}},
				}, nil
			}
			return &mockRows{
				cols: []string{"id", "email", "name", "password_hash", "status", "is_super_admin", "last_login_at", "created_at", "updated_at"},
				data: nil,
			}, nil
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	users, total, err := s.ListUsers(context.Background(), "active", "alice", 10, 0)
	if err != nil {
		t.Fatalf("ListUsers with filters: %v", err)
	}
	if total != 0 {
		t.Errorf("total: got %d, want 0", total)
	}
	if len(users) != 0 {
		t.Errorf("users: got %d, want 0", len(users))
	}
}

func TestListUsersDataQueryError(t *testing.T) {
	callCount := 0
	conn := &mockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			callCount++
			if callCount == 1 {
				// COUNT succeeds
				return &mockRows{
					cols: []string{"count"},
					data: [][]driver.Value{{int64(1)}},
				}, nil
			}
			// data query fails
			return nil, errDB
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	_, _, err := s.ListUsers(context.Background(), "", "", 10, 0)
	if err == nil {
		t.Error("expected error from data query failure")
	}
}

func TestUpdatePasswordExecError(t *testing.T) {
	conn := &mockConn{
		execFunc: func(_ string, _ []driver.Value) (driver.Result, error) {
			return nil, errDB
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	err := s.UpdatePassword(context.Background(), uuid.New(), "newpass")
	if err == nil {
		t.Error("expected error from exec failure")
	}
}

func TestUpdatePasswordSuccess(t *testing.T) {
	var execCalled bool
	conn := &mockConn{
		execFunc: func(_ string, _ []driver.Value) (driver.Result, error) {
			execCalled = true
			return driver.RowsAffected(1), nil
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	err := s.UpdatePassword(context.Background(), uuid.New(), "newpass123")
	if err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	if !execCalled {
		t.Error("expected exec to be called")
	}
}
