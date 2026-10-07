package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lollinoo/theia/internal/domain"
)

func TestBackupReadHandlersRejectFileRepositoryFailures(t *testing.T) {
	for _, operation := range []string{"list", "single", "latest", "bulk download"} {
		t.Run(operation, func(t *testing.T) {
			root := t.TempDir()
			handler, jobs, files, devices, _ := setupBackupHandlerForBulkLimitTests(t, root)
			deviceID := seedBulkDownloadBackupFile(t, root, jobs, files, devices, "running.cfg", []byte("configuration"))
			job, err := jobs.GetLatestByDeviceID(deviceID)
			if err != nil {
				t.Fatal(err)
			}
			files.getByJobErr = errors.New("injected database failure")
			var handle http.HandlerFunc
			var request *http.Request
			switch operation {
			case "list":
				handle = handler.HandleListBackups
				request = httptest.NewRequest(http.MethodGet, "/api/v1/devices/"+deviceID.String()+"/backups", nil)
			case "single":
				handle = handler.HandleGetBackupJob
				request = httptest.NewRequest(http.MethodGet, "/api/v1/backup-jobs/"+job.ID.String(), nil)
			case "latest":
				handle = handler.HandleGetLatestBackup
				request = httptest.NewRequest(http.MethodGet, "/api/v1/devices/"+deviceID.String()+"/backups/latest", nil)
			case "bulk download":
				handle = handler.HandleBulkDownload
				request = httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(fmt.Sprintf(`{"device_ids":["%s"]}`, deviceID)))
			}
			recorder := httptest.NewRecorder()
			handle(recorder, request)
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status=%d body=%s; want 500", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Header().Get("Content-Type"), "application/zip") || recorder.Header().Get("Content-Disposition") != "" {
				t.Fatal("failed selection started a ZIP response")
			}
			if job.Status != domain.BackupStatusSuccess {
				t.Fatal("read failure mutated job status")
			}
		})
	}
}
