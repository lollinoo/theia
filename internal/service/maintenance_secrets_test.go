package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/instance"
)

func TestExternalPasswordRotationStopsBeforeAnyMaintenance(t *testing.T) {
	for _, metadata := range []string{`{"bundled_postgres":false}`, `{"external_postgres":true}`, `{}`} {
		t.Run(metadata, func(t *testing.T) {
			state, err := instance.Generate(time.Now())
			if err != nil {
				t.Fatal(err)
			}
			state.DeploymentMetadata = []byte(metadata)
			state.DBDSN = "postgres://theia:fixture@external-postgres:5432/theia"
			store := instance.Store{Path: filepath.Join(t.TempDir(), "control", "state.json")}
			if err := store.Create(state); err != nil {
				t.Fatal(err)
			}
			m := &Maintenance{StatePath: store.Path, DBDSN: state.DBDSN}
			if err := m.RotateOperationalSecrets(context.Background()); err == nil {
				t.Fatal("external password rotation accepted")
			}
			if op, err := m.Status(); err != nil || op != nil {
				t.Fatal("external rotation started a maintenance operation")
			}
			after, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if after.DatabasePassword != state.DatabasePassword || after.SessionSecret != state.SessionSecret {
				t.Fatal("external rejection changed instance secrets")
			}
		})
	}
}
