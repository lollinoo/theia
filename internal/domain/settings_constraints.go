package domain

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// SettingValueType identifies the scalar type enforced for bounded runtime settings.
type SettingValueType string

const (
	SettingValueTypeInteger SettingValueType = "integer"
	SettingValueTypeFloat   SettingValueType = "float"
)

// SettingConstraint is the single source of truth for bounded runtime setting values.
type SettingConstraint struct {
	Type     SettingValueType
	MinInt   int
	MaxInt   int
	MinFloat float64
	MaxFloat float64
	MinLabel string
	MaxLabel string
}

// SettingConstraints maps runtime setting keys to operationally safe ranges.
var SettingConstraints = map[string]SettingConstraint{
	SettingPollingInterval:               intConstraint(5, 3600),
	SettingSNMPWorkerPoolSize:            intConstraint(1, 128),
	SettingSNMPWorkerPoolPerformance:     intConstraint(1, 128),
	SettingSNMPWorkerPoolOperational:     intConstraint(1, 128),
	SettingSNMPWorkerPoolStatic:          intConstraint(1, 128),
	SettingSNMPTimeout:                   intConstraint(1, 120),
	SettingSNMPRetries:                   intConstraint(0, 10),
	SettingPollingEssentialWorkers:       intConstraint(1, 256),
	SettingPollingMaxWorkersPerSite:      intConstraint(1, 256),
	SettingPollingMaxWorkersPerSubnet:    intConstraint(1, 256),
	SettingPollingMaxWorkersPerDevice:    intConstraint(1, 32),
	SettingPollingMaxInflightPerProfile:  intConstraint(1, 256),
	SettingPollingEssentialTimeoutMillis: intConstraint(100, 30000),
	SettingPollingEssentialRetries:       intConstraint(0, 10),
	SettingPollingWebSocketCoalesceMS:    intConstraint(50, 5000),
	SettingPollingPersistenceBatchMS:     intConstraint(100, 10000),
	SettingPollingCapacitySafetyMargin:   floatConstraint(1.0, 5.0, "1.0", "5.0"),
	SettingInstanceBackupRetentionCount:  intConstraint(1, 365),
	SettingDeviceBackupRetentionCount:    intConstraint(1, 365),
	SettingBridgePort:                    intConstraint(1, 65535),
}

func intConstraint(min, max int) SettingConstraint {
	return SettingConstraint{
		Type:     SettingValueTypeInteger,
		MinInt:   min,
		MaxInt:   max,
		MinLabel: strconv.Itoa(min),
		MaxLabel: strconv.Itoa(max),
	}
}

func floatConstraint(min, max float64, minLabel, maxLabel string) SettingConstraint {
	return SettingConstraint{
		Type:     SettingValueTypeFloat,
		MinFloat: min,
		MaxFloat: max,
		MinLabel: minLabel,
		MaxLabel: maxLabel,
	}
}

// RangeString returns the human-readable inclusive range for validation errors.
func (c SettingConstraint) RangeString() string {
	return c.MinLabel + " and " + c.MaxLabel
}

// NormalizeConstrainedSetting validates and normalizes a setting value for API writes.
func NormalizeConstrainedSetting(key, value string) (string, error) {
	constraint, ok := SettingConstraints[key]
	if !ok {
		return value, nil
	}

	trimmed := strings.TrimSpace(value)
	switch constraint.Type {
	case SettingValueTypeInteger:
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return "", fmt.Errorf("%s must be a valid integer", key)
		}
		if parsed < constraint.MinInt || parsed > constraint.MaxInt {
			return "", fmt.Errorf("%s must be between %s", key, constraint.RangeString())
		}
		return strconv.Itoa(parsed), nil
	case SettingValueTypeFloat:
		parsed, err := strconv.ParseFloat(trimmed, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return "", fmt.Errorf("%s must be a valid float", key)
		}
		if parsed < constraint.MinFloat || parsed > constraint.MaxFloat {
			return "", fmt.Errorf("%s must be between %s", key, constraint.RangeString())
		}
		return trimmed, nil
	default:
		return value, nil
	}
}

// CoerceConstrainedInt clamps a persisted integer setting to its safe range.
// Malformed values fall back so legacy bad data cannot crash startup.
func CoerceConstrainedInt(key, value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	constraint, ok := SettingConstraints[key]
	if !ok || constraint.Type != SettingValueTypeInteger {
		return parsed
	}
	if parsed < constraint.MinInt {
		return constraint.MinInt
	}
	if parsed > constraint.MaxInt {
		return constraint.MaxInt
	}
	return parsed
}

// CoerceConstrainedFloat clamps a persisted float setting to its safe range.
// Malformed and non-finite values fall back so legacy bad data cannot crash startup.
func CoerceConstrainedFloat(key, value string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return fallback
	}
	constraint, ok := SettingConstraints[key]
	if !ok || constraint.Type != SettingValueTypeFloat {
		return parsed
	}
	if parsed < constraint.MinFloat {
		return constraint.MinFloat
	}
	if parsed > constraint.MaxFloat {
		return constraint.MaxFloat
	}
	return parsed
}
