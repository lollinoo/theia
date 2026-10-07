package service

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	internalssh "github.com/lollinoo/theia/internal/ssh"
	"github.com/lollinoo/theia/internal/vendor"
	"golang.org/x/crypto/ssh"
)

func TestFullBackupPreservesCallerDeadlineAndCancellation(t *testing.T) {
	for _, scenario := range []string{"deadline after device-lock waiting", "cancellation during SSH handshake"} {
		t.Run(scenario, func(t *testing.T) {
			listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			jobs := newMockBackupJobRepo()
			device := &domain.Device{ID: uuid.New(), IP: "127.0.0.1"}
			job := &domain.BackupJob{ID: uuid.New(), DeviceID: device.ID, Status: domain.BackupStatusPending}
			if err := jobs.Create(job); err != nil {
				t.Fatal(err)
			}
			svc := NewBackupService(jobs, newMockBackupFileRepo(), nil, nil, nil, nil, &internalssh.DefaultDialer{}, nil, t.TempDir(), ssh.InsecureIgnoreHostKey())
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			deadline, _ := ctx.Deadline()
			release, err := svc.lockBackupDevice(context.Background(), device.ID)
			if err != nil {
				t.Fatal(err)
			}
			var releaseOnce sync.Once
			unlock := func() { releaseOnce.Do(release) }
			defer unlock()
			done := make(chan struct{})
			profile := &domain.CredentialProfile{Port: listener.Addr().(*net.TCPAddr).Port, AuthMethod: domain.SSHAuthPassword}
			go func() {
				defer close(done)
				svc.runFullBackupContext(ctx, device, profile, vendor.BackupConfig{}, job.ID)
			}()
			// Spend some of the total budget waiting behind another device backup.
			waitUntil := time.Now().Add(500 * time.Millisecond)
			for {
				svc.deviceLocksMu.Lock()
				waiting := svc.deviceLocks[device.ID].references > 1
				svc.deviceLocksMu.Unlock()
				if waiting {
					break
				}
				if time.Now().After(waitUntil) {
					t.Fatal("backup did not start waiting on the device lock")
				}
				time.Sleep(time.Millisecond)
			}
			time.Sleep(400 * time.Millisecond)
			unlock()
			if err := listener.SetDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			peer, err := listener.AcceptTCP()
			if err != nil {
				t.Fatal(err)
			}
			// The peer accepts TCP but sends no SSH banner, keeping negotiation blocked.
			t.Cleanup(func() {
				cancel()
				_ = peer.Close()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Error("backup executor leaked after transport cleanup")
				}
			})
			if scenario == "cancellation during SSH handshake" {
				cancel()
			}
			waitBudget := time.Until(deadline) + 250*time.Millisecond
			if scenario == "cancellation during SSH handshake" {
				waitBudget = 500 * time.Millisecond
			}
			select {
			case <-done:
			case <-time.After(waitBudget):
				t.Fatal("SSH negotiation ignored the caller's total deadline or cancellation")
			}
			if scenario == "deadline after device-lock waiting" && time.Now().Before(deadline) {
				t.Fatal("silent SSH peer failed before the caller deadline")
			}
			stored, err := jobs.GetByID(job.ID)
			// The socket deadline and context timer may finish in either order;
			// transport error wording is not part of the executor's deadline contract.
			if err != nil || stored.Status != domain.BackupStatusFailed || stored.ErrorMessage == "" {
				t.Fatalf("job=%+v caller error=%v repository error=%v", stored, ctx.Err(), err)
			}
		})
	}
}
