package postgres

// This file defines bool value persistence behavior, ordering guarantees, and not-found conventions.

import (
	"fmt"
	"strconv"
	"strings"
)

func normalizeBoolValue(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case int:
		return v != 0, nil
	case int64:
		return v != 0, nil
	case int32:
		return v != 0, nil
	case []byte:
		return normalizeBoolValue(string(v))
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(v))
		switch trimmed {
		case "true", "t", "yes", "y", "on", "1":
			return true, nil
		case "false", "f", "no", "n", "off", "0", "":
			return false, nil
		default:
			parsed, err := strconv.ParseBool(trimmed)
			if err == nil {
				return parsed, nil
			}
			return false, fmt.Errorf("unsupported bool value %q", v)
		}
	case nil:
		return false, nil
	default:
		return false, fmt.Errorf("unsupported bool value type %T", value)
	}
}
