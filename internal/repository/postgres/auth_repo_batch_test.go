package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestAuthRepoListUsersBatchesAuthorizationQueries(t *testing.T) {
	t.Parallel()

	userIDs := []uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-000000000101"),
		uuid.MustParse("00000000-0000-0000-0000-000000000102"),
		uuid.MustParse("00000000-0000-0000-0000-000000000103"),
	}
	recorder := &authListQueryRecorder{userIDs: userIDs}
	db := sql.OpenDB(authListConnector{recorder: recorder})
	t.Cleanup(func() { db.Close() })

	users, err := NewAuthRepo(db).ListUsers(context.Background(), domain.UserListFilter{})
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}

	if got, want := len(recorder.queries), 3; got != want {
		t.Fatalf("ListUsers query count = %d, want %d; queries: %#v", got, want, recorder.queryTexts())
	}
	for _, query := range recorder.queries[1:] {
		if !strings.Contains(query.text, "ur.user_id IN ($1, $2, $3)") {
			t.Fatalf("authorization query does not batch all user IDs: %s", query.text)
		}
		if got, want := query.args, []driver.Value{userIDs[0].String(), userIDs[1].String(), userIDs[2].String()}; !reflect.DeepEqual(got, want) {
			t.Fatalf("authorization query args = %#v, want %#v", got, want)
		}
	}
	if !strings.Contains(recorder.queries[1].text, "ORDER BY ur.user_id ASC, r.name ASC") {
		t.Fatalf("role query does not preserve per-user role ordering: %s", recorder.queries[1].text)
	}
	if !strings.Contains(recorder.queries[2].text, "SELECT DISTINCT ur.user_id") ||
		!strings.Contains(recorder.queries[2].text, "ORDER BY ur.user_id ASC, p.key ASC") {
		t.Fatalf("permission query does not deduplicate and preserve per-user ordering: %s", recorder.queries[2].text)
	}

	if got, want := aggregateRoleIDs(users), [][]string{{domain.RoleAdmin, domain.RoleViewer}, nil, {domain.RoleViewer}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ListUsers role IDs = %#v, want %#v", got, want)
	}
	if got, want := aggregatePermissionKeys(users), [][]string{{domain.PermissionRolesRead, domain.PermissionUsersRead}, nil, {domain.PermissionUsersRead}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ListUsers permission keys = %#v, want %#v", got, want)
	}
}

func TestAuthRepoListUsersEmptyPageSkipsAuthorizationQueries(t *testing.T) {
	t.Parallel()

	recorder := &authListQueryRecorder{}
	db := sql.OpenDB(authListConnector{recorder: recorder})
	t.Cleanup(func() { db.Close() })

	users, err := NewAuthRepo(db).ListUsers(context.Background(), domain.UserListFilter{})
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if users == nil || len(users) != 0 {
		t.Fatalf("ListUsers empty page = %#v, want non-nil empty slice", users)
	}
	if got, want := len(recorder.queries), 1; got != want {
		t.Fatalf("ListUsers empty-page query count = %d, want %d", got, want)
	}
}

func aggregateRoleIDs(users []domain.UserWithRolesAndPermissions) [][]string {
	result := make([][]string, len(users))
	for i, user := range users {
		for _, role := range user.Roles {
			result[i] = append(result[i], role.ID)
		}
	}
	return result
}

func aggregatePermissionKeys(users []domain.UserWithRolesAndPermissions) [][]string {
	result := make([][]string, len(users))
	for i, user := range users {
		for _, permission := range user.Permissions {
			result[i] = append(result[i], permission.Key)
		}
	}
	return result
}

type recordedAuthListQuery struct {
	text string
	args []driver.Value
}

type authListQueryRecorder struct {
	userIDs []uuid.UUID
	queries []recordedAuthListQuery
}

func (r *authListQueryRecorder) queryTexts() []string {
	result := make([]string, len(r.queries))
	for i, query := range r.queries {
		result[i] = query.text
	}
	return result
}

type authListConnector struct {
	recorder *authListQueryRecorder
}

func (c authListConnector) Connect(context.Context) (driver.Conn, error) {
	return &authListConn{recorder: c.recorder}, nil
}

func (c authListConnector) Driver() driver.Driver {
	return authListDriver{}
}

type authListDriver struct{}

func (authListDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("auth list test driver requires its connector")
}

type authListConn struct {
	recorder *authListQueryRecorder
}

func (c *authListConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("auth list test driver does not prepare statements")
}

func (c *authListConn) Close() error { return nil }

func (c *authListConn) Begin() (driver.Tx, error) {
	return nil, errors.New("auth list test driver does not begin transactions")
}

func (c *authListConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	values := make([]driver.Value, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	c.recorder.queries = append(c.recorder.queries, recordedAuthListQuery{text: query, args: values})

	switch {
	case strings.Contains(query, "FROM users"):
		return authListUserRows(c.recorder.userIDs), nil
	case strings.Contains(query, "FROM roles r"):
		return authListRoleRows(query, values, c.recorder.userIDs), nil
	case strings.Contains(query, "FROM permissions p"):
		return authListPermissionRows(query, values, c.recorder.userIDs), nil
	default:
		return nil, errors.New("unexpected auth list test query: " + query)
	}
}

func authListUserRows(userIDs []uuid.UUID) driver.Rows {
	columns := []string{
		"id", "username", "username_normalized", "email", "email_normalized", "password_hash",
		"display_name", "status", "must_change_password", "created_at", "updated_at",
		"last_login_at", "password_changed_at", "failed_login_attempts", "locked_until", "created_by", "updated_by",
	}
	now := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	data := make([][]driver.Value, 0, len(userIDs))
	for i, userID := range userIDs {
		username := []string{"alice", "bob", "charlie"}[i]
		data = append(data, []driver.Value{
			userID.String(), username, username, username + "@example.test", username + "@example.test", "hash",
			strings.ToUpper(username), string(domain.UserStatusActive), false, now, now,
			nil, now, int64(0), nil, nil, nil,
		})
	}
	return &authListRows{columns: columns, data: data}
}

func authListRoleRows(query string, args []driver.Value, userIDs []uuid.UUID) driver.Rows {
	now := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	rolesByUser := map[string][][]driver.Value{
		userIDs[0].String(): {
			{domain.RoleAdmin, domain.RoleAdmin, "Admin", true, now, now},
			{domain.RoleViewer, domain.RoleViewer, "Viewer", true, now, now},
		},
		userIDs[2].String(): {{domain.RoleViewer, domain.RoleViewer, "Viewer", true, now, now}},
	}
	columns := []string{"id", "name", "description", "is_system_role", "created_at", "updated_at"}
	return authListGrantRows(query, args, columns, rolesByUser)
}

func authListPermissionRows(query string, args []driver.Value, userIDs []uuid.UUID) driver.Rows {
	permissionsByUser := map[string][][]driver.Value{
		userIDs[0].String(): {
			{domain.PermissionRolesRead, domain.PermissionRolesRead, "Read roles", "roles", "read"},
			{domain.PermissionUsersRead, domain.PermissionUsersRead, "Read users", "users", "read"},
		},
		userIDs[2].String(): {{domain.PermissionUsersRead, domain.PermissionUsersRead, "Read users", "users", "read"}},
	}
	columns := []string{"id", "key", "description", "resource", "action"}
	return authListGrantRows(query, args, columns, permissionsByUser)
}

func authListGrantRows(query string, args []driver.Value, columns []string, grantsByUser map[string][][]driver.Value) driver.Rows {
	if strings.Contains(query, "ur.user_id IN") {
		columns = append([]string{"user_id"}, columns...)
		var data [][]driver.Value
		for _, arg := range args {
			userID := arg.(string)
			for _, grant := range grantsByUser[userID] {
				data = append(data, append([]driver.Value{userID}, grant...))
			}
		}
		return &authListRows{columns: columns, data: data}
	}
	return &authListRows{columns: columns, data: grantsByUser[args[0].(string)]}
}

type authListRows struct {
	columns []string
	data    [][]driver.Value
	index   int
}

func (r *authListRows) Columns() []string { return r.columns }

func (r *authListRows) Close() error { return nil }

func (r *authListRows) Next(dest []driver.Value) error {
	if r.index >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.index])
	r.index++
	return nil
}
