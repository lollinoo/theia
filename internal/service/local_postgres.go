package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func withIsolatedPostgres(ctx context.Context, verify func(string) error) error {
	if err := ensureSupportedPostgresCLITools(ctx, "initdb", "pg_ctl", "pg_restore", "psql"); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "theia-pg-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	run := func(ctx context.Context, name string, args ...string) error {
		cmd := exec.CommandContext(ctx, name, args...)
		if err := isolatedPostgresUser(cmd, dir); err != nil {
			return err
		}
		output, err := cmd.CombinedOutput()
		if err != nil {
			return externalCommandError(name, args, err, output)
		}
		return nil
	}
	cluster := filepath.Join(dir, "cluster")
	if err := run(ctx, "initdb", "-D", cluster, "-A", "trust", "-U", "theia_verify", "--no-locale", "--encoding=UTF8"); err != nil {
		return err
	}
	options := fmt.Sprintf("-c listen_addresses='' -c unix_socket_directories='%s' -c shared_buffers=16MB -c max_connections=10", dir)
	if err := run(ctx, "pg_ctl", "-D", cluster, "-l", filepath.Join(dir, "postgres.log"), "-o", options, "-w", "-t", "30", "start"); err != nil {
		return err
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = run(stopCtx, "pg_ctl", "-D", cluster, "-w", "-m", "immediate", "stop")
	}()
	dsn := fmt.Sprintf("host='%s' dbname=postgres user=theia_verify sslmode=disable", dir)
	return verify(dsn)
}
