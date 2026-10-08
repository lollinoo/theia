package polling

import (
	"github.com/lollinoo/theia/internal/domain"
	"testing"
)

type unusedPersistenceBatchSettings struct{ t *testing.T }

func (s unusedPersistenceBatchSettings) Get(key string) (string, error) {
	if key == domain.SettingPollingPersistenceBatchMS {
		s.t.Fatal("polling still reads a setting with no batching consumer")
	}
	return "", domain.ErrSettingNotFound
}

func TestPolicyDoesNotReadUnusedPersistenceBatch(t *testing.T) {
	PolicyFromSettings(unusedPersistenceBatchSettings{t: t}, 0, 0, 0)
}
