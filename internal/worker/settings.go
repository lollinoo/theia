package worker

import (
	"strconv"
	"strings"
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
	parsed, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil || parsed <= 0 {
		return 60 * time.Second
	}
	seconds := domain.CoerceConstrainedInt(domain.SettingPollingInterval, val, 60)
	return time.Duration(seconds) * time.Second
}
