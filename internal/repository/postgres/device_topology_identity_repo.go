package postgres

import (
	"fmt"
	"github.com/google/uuid"
	"strings"
)

// LookupDeviceIDsBySysNames resolves normalized discovery identities without
// loading credentials or relationships. Each query is bounded to 128 names.
func (r *DeviceRepo) LookupDeviceIDsBySysNames(names []string) (map[string]uuid.UUID, error) {
	keys := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		key := normalizeDeviceSysNameLookup(name)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	resolved := make(map[string]uuid.UUID, len(keys))
	for start := 0; start < len(keys); start += 128 {
		end := min(start+128, len(keys))
		args := make([]interface{}, 0, end-start)
		placeholders := make([]string, 0, end-start)
		for _, key := range keys[start:end] {
			args = append(args, key)
			placeholders = append(placeholders, "?")
		}
		rows, err := r.db.Query(`SELECT DISTINCT ON (sys_name_lookup) sys_name_lookup,id
			FROM devices WHERE sys_name_lookup IN (`+strings.Join(placeholders, ",")+`)
			ORDER BY sys_name_lookup,updated_at DESC,created_at DESC`, args...)
		if err != nil {
			return nil, fmt.Errorf("looking up topology identities: %w", err)
		}
		for rows.Next() {
			var key string
			var id uuid.UUID
			if err := rows.Scan(&key, &id); err != nil {
				rows.Close()
				return nil, err
			}
			resolved[key] = id
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	byName := make(map[string]uuid.UUID, len(names))
	for _, name := range names {
		if id, ok := resolved[normalizeDeviceSysNameLookup(name)]; ok {
			byName[name] = id
		}
	}
	return byName, nil
}
