package worker

import (
	"strconv"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

// GetPollingInterval reads the polling interval from the settings repository.
// Returns 60 seconds as the default if the setting is missing or invalid.
func GetPollingInterval(settingsRepo domain.SettingsRepository) time.Duration {
	val, err := settingsRepo.Get(domain.SettingPollingInterval)
	if err != nil {
		return 60 * time.Second
	}
	seconds, err := strconv.Atoi(val)
	if err != nil || seconds <= 0 {
		return 60 * time.Second
	}
	return time.Duration(seconds) * time.Second
}
