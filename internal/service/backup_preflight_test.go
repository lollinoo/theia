package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"golang.org/x/crypto/ssh"
)

func TestBackupPreflightUsesExecutionTarget(t *testing.T) {
	for _, flow := range []string{"manual", "bulk"} {
		for _, scenario := range []string{"backup", "management", "primary", "unreachable backup"} {
			t.Run(flow+"/"+scenario, func(t *testing.T) {
				port := listenOnRandomPort(t)
				device := &domain.Device{ID: uuid.New(), IP: "127.0.0.2", Vendor: "testvendor", Status: domain.DeviceStatusUp}
				switch scenario {
				case "backup":
					device.Addresses = []domain.DeviceAddress{
						{Address: "127.0.0.2", Role: domain.DeviceAddressRoleManagement},
						{Address: "127.0.0.1", Role: domain.DeviceAddressRoleBackup},
					}
				case "management":
					device.Addresses = []domain.DeviceAddress{{Address: "127.0.0.1", Role: domain.DeviceAddressRoleManagement}}
				case "primary":
					device.IP = "127.0.0.1"
				case "unreachable backup":
					device.IP = "127.0.0.1"
					device.Addresses = []domain.DeviceAddress{{Address: "127.0.0.2", Role: domain.DeviceAddressRoleBackup}}
				}
				jobs := newMockBackupJobRepo()
				profiles := newMockCredentialProfileRepo()
				if err := profiles.Create(&domain.CredentialProfile{ID: uuid.New(), Port: port, AuthMethod: domain.SSHAuthPassword}); err != nil {
					t.Fatal(err)
				}
				devices := newMockDeviceRepo()
				if err := devices.Create(device); err != nil {
					t.Fatal(err)
				}
				runs := newMockBulkBackupRunRepo()
				svc := NewBackupService(jobs, newMockBackupFileRepo(), profiles, devices, newMockBackupSettingsRepo(),
					buildTestVendorRegistry("testvendor", true), &mockSSHDialer{}, nil, t.TempDir(), ssh.InsecureIgnoreHostKey(), WithBulkBackupRunRepo(runs))
				unreachable := scenario == "unreachable backup"
				if flow == "manual" {
					job, err := svc.TriggerBackup(context.Background(), device.ID)
					if unreachable {
						if err == nil || !strings.Contains(err.Error(), "device unreachable") || job != nil {
							t.Fatalf("job=%v error=%v, want unreachable without a job", job, err)
						}
					} else {
						if err != nil || job == nil {
							t.Fatalf("reachable backup target rejected: job=%v error=%v", job, err)
						}
						// Wait for the mock executor to finish before test collaborators are released.
						deadline := time.Now().Add(5 * time.Second)
						for {
							stored, err := jobs.GetByID(job.ID)
							if err != nil {
								t.Fatal(err)
							}
							if stored.Status == domain.BackupStatusFailed {
								break
							}
							if time.Now().After(deadline) {
								t.Fatal("mock backup executor did not finish")
							}
							time.Sleep(time.Millisecond)
						}
					}
				} else {
					item := domain.BulkBackupRunItem{ID: uuid.New(), RunID: uuid.New(), DeviceID: device.ID, Status: domain.BulkBackupRunItemStatusChecking}
					if err := runs.CreateRun(&domain.BulkBackupRun{ID: item.RunID, Status: domain.BulkBackupRunStatusRunning}, []domain.BulkBackupRunItem{item}); err != nil {
						t.Fatal(err)
					}
					queued := svc.prepareBulkRunBatch([]domain.BulkBackupRunItem{item})
					if unreachable {
						items, _ := runs.ListRunItems(item.RunID)
						if len(queued) != 0 || items[0].Status != domain.BulkBackupRunItemStatusSkipped || items[0].Reason != "device unreachable" {
							t.Fatalf("queued=%v item=%+v, want unreachable skip", queued, items[0])
						}
					} else if len(queued) != 1 || domain.BackupAddress(queued[0].device) != "127.0.0.1" {
						t.Fatalf("reachable backup target not queued: %v", queued)
					}
				}
				wantJobs := 1
				if unreachable {
					wantJobs = 0
				}
				if got := mockBackupJobCount(jobs); got != wantJobs {
					t.Fatalf("job count=%d, want %d", got, wantJobs)
				}
			})
		}
	}
}
