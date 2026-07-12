package postgres

import (
	"strconv"
	"sync"
	"testing"

	"github.com/lollinoo/theia/internal/domain"
)

func TestSettingsRepoUpdateSerializesConcurrentMutations(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingsRepo(db)
	if err := repo.Set(domain.SettingPollingInterval, "60"); err != nil {
		t.Fatalf("seed setting: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := repo.Update(domain.SettingPollingInterval, func(current string) (string, error) {
				value, err := strconv.Atoi(current)
				if err != nil {
					return "", err
				}
				return strconv.Itoa(value + 1), nil
			})
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Update returned error: %v", err)
		}
	}

	value, err := repo.Get(domain.SettingPollingInterval)
	if err != nil {
		t.Fatalf("get updated setting: %v", err)
	}
	if value != "62" {
		t.Fatalf("updated value = %q, want %q", value, "62")
	}
}
