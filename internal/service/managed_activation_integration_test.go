package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/repository/postgres"
	"github.com/lollinoo/theia/internal/security"
)

func TestManagedActivationIntegration(t *testing.T) {
	if os.Getenv("THEIA_MAINTENANCE_INTEGRATION") != "1" {
		t.Skip("run make maintenance-test for isolated PostgreSQL integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := withIsolatedPostgres(ctx, func(dsn string) error {
		s, _ := instance.Generate(time.Now())
		token, _ := s.BeginActivation(time.Now())
		store := instance.Store{Path: filepath.Join(t.TempDir(), "secrets.json")}
		if err := store.Create(s); err != nil {
			return err
		}
		db, err := postgres.OpenPrimaryDB(dsn)
		if err != nil {
			return err
		}
		defer db.Close()
		keys, _ := s.Keyring()
		if err := postgres.RunMigrations(db, keys); err != nil {
			return err
		}
		a := &ManagedActivation{Store: store, DB: db}
		if _, err := a.Recovery(ctx, "wrong-token"); !errors.Is(err, ErrActivationDenied) {
			return fmt.Errorf("invalid token accepted")
		}
		r, err := a.Recovery(ctx, token)
		if err != nil {
			return err
		}
		other, _ := age.GenerateX25519Identity()
		if err := store.Update(func(s *instance.State) error { s.RecoveryRecipient = r.Recipient; return nil }); err != nil {
			return err
		}
		preserved, err := a.Recovery(ctx, token)
		if err != nil {
			return err
		}
		if preserved.RecoveryFile != "" || preserved.Recipient != r.Recipient || preserved.Proof != r.Proof {
			return fmt.Errorf("imported recovery recipient was replaced during activation")
		}
		input := ActivateInstanceInput{Username: "FirstAdmin", Email: "admin@example.test", Password: "StrongPassword42!", RecoveryFile: other.String(), Proof: r.Proof}
		if err := a.Complete(ctx, token, input); err == nil {
			return fmt.Errorf("unrelated saved recovery file accepted")
		}
		input.RecoveryFile = r.RecoveryFile
		a.AfterCommit = func() error { return fmt.Errorf("injected state-save interruption") }
		if err := a.Complete(ctx, token, input); err == nil {
			return fmt.Errorf("injected failure ignored")
		}
		var count int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil || count != 1 {
			return fmt.Errorf("transaction did not create exactly one user")
		}
		a.AfterCommit = nil
		if err := a.Complete(ctx, token, input); err != nil {
			return err
		}
		persisted, err := store.Load()
		if err != nil {
			return err
		}
		if persisted.Activation != nil || persisted.RecoveryRecipient != r.Recipient {
			return fmt.Errorf("activation was not consumed or recovery recipient differs")
		}
		data, _ := os.ReadFile(store.Path)
		if strings.Contains(string(data), "AGE-SECRET-KEY-") {
			return fmt.Errorf("private recovery material persisted")
		}
		var hash, normalized string
		if err := db.QueryRowContext(ctx, "SELECT password_hash,username_normalized FROM users").Scan(&hash, &normalized); err != nil {
			return err
		}
		if normalized != "firstadmin" {
			return fmt.Errorf("username normalization differs")
		}
		ok, err := security.VerifyPassword(input.Password, hash)
		if err != nil || !ok {
			return fmt.Errorf("chosen password does not authenticate")
		}
		if _, err := db.ExecContext(ctx, "UPDATE users SET status='disabled'"); err != nil {
			return err
		}
		var renewed string
		if err := store.Update(func(s *instance.State) error { var err error; renewed, err = s.BeginActivation(time.Now()); return err }); err != nil {
			return err
		}
		if _, err := a.Recovery(ctx, renewed); !errors.Is(err, ErrActivationDenied) {
			return fmt.Errorf("disabled existing users were bootstrapped again")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
