package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

const identityTestDriverName = "theia_instance_identity_test"

var (
	registerIdentityTestDriver sync.Once
	identityTestStoredValue    string
	identityTestQueryErr       error
	identityTestMu             sync.Mutex
)

type identityTestDriver struct{}

func (identityTestDriver) Open(name string) (driver.Conn, error) {
	return identityTestConn{}, nil
}

type identityTestConn struct{}

func (identityTestConn) Prepare(query string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

func (identityTestConn) Close() error {
	return nil
}

func (identityTestConn) Begin() (driver.Tx, error) {
	return nil, driver.ErrSkip
}

func (identityTestConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	identityTestMu.Lock()
	defer identityTestMu.Unlock()
	if identityTestQueryErr != nil {
		return nil, identityTestQueryErr
	}
	return &identitySingleStringRows{value: identityTestStoredValue}, nil
}

type identitySingleStringRows struct {
	value string
	done  bool
}

func (r *identitySingleStringRows) Columns() []string {
	return []string{"value"}
}

func (r *identitySingleStringRows) Close() error {
	return nil
}

func (r *identitySingleStringRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.value
	return nil
}

func openIdentityTestDB(t *testing.T) *sql.DB {
	t.Helper()
	registerIdentityTestDriver.Do(func() {
		sql.Register(identityTestDriverName, identityTestDriver{})
	})
	db, err := sql.Open(identityTestDriverName, "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestVerifyInstanceIdentityEmptyGuards(t *testing.T) {
	ctx := context.Background()

	// Empty instanceID must fail even with nil db without panicking.
	if err := VerifyInstanceIdentity(ctx, nil, ""); err == nil {
		t.Fatal("expected error for empty instanceID, got nil")
	} else if !strings.Contains(err.Error(), "database does not match the managed instance") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Whitespace-only instanceID must also fail.
	if err := VerifyInstanceIdentity(ctx, nil, "   \t\n "); err == nil {
		t.Fatal("expected error for whitespace instanceID, got nil")
	}
}

func TestVerifyInstanceIdentityMatchesAndMismatches(t *testing.T) {
	db := openIdentityTestDB(t)
	ctx := context.Background()

	identityTestMu.Lock()
	identityTestStoredValue = ""
	identityTestQueryErr = nil
	identityTestMu.Unlock()

	// Even if DB settings has empty value, empty instanceID must still be rejected.
	if err := VerifyInstanceIdentity(ctx, db, ""); err == nil {
		t.Fatal("expected error for empty instanceID even when stored setting is empty")
	}

	// Set stored value to instance-alpha
	identityTestMu.Lock()
	identityTestStoredValue = "instance-alpha"
	identityTestQueryErr = nil
	identityTestMu.Unlock()

	if err := VerifyInstanceIdentity(ctx, db, "instance-alpha"); err != nil {
		t.Fatalf("expected nil for matching instanceID, got %v", err)
	}

	if err := VerifyInstanceIdentity(ctx, db, "instance-beta"); err == nil {
		t.Fatal("expected error for mismatched instanceID, got nil")
	}

	// Simulated query failure
	identityTestMu.Lock()
	identityTestQueryErr = fmt.Errorf("connection failed")
	identityTestMu.Unlock()

	if err := VerifyInstanceIdentity(ctx, db, "instance-alpha"); err == nil {
		t.Fatal("expected error on query failure, got nil")
	}
}
