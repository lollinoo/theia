package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// Address writers take the owner lock before address locks and row locks. This
// also serializes address replacement with discovery changes to virtualness.
func lockDeviceAddressOwnerTx(ctx context.Context, tx *Tx, id uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(?)`, deviceImportAddressLockKey("theia:device-address-owner:"+id.String()))
	return err
}

func lockDeviceAddressValuesTx(ctx context.Context, tx *Tx, canonical []string) error {
	keys := make([]int64, 0, len(canonical))
	seen := make(map[int64]struct{}, len(canonical))
	for _, address := range canonical {
		key := deviceImportAddressLockKey(address)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	// Use the import lock keys and numeric order so all writers share one fence.
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(?)`, key); err != nil {
			return err
		}
	}
	return nil
}

func checkDeviceAddressWriteTx(ctx context.Context, tx *Tx, id uuid.UUID, deviceType domain.DeviceType, addresses []string, exclusiveAddresses bool) error {
	canonical := canonicalDeviceImportAddresses(addresses)
	if err := lockDeviceAddressValuesTx(ctx, tx, canonical); err != nil {
		return err
	}
	if len(canonical) == 0 || (deviceType == domain.DeviceTypeVirtual && !exclusiveAddresses) {
		return nil
	}
	args := make([]any, 0, len(canonical)+1)
	for _, address := range canonical {
		args = append(args, address)
	}
	args = append(args, exclusiveAddresses, id.String())
	var address, owner string
	err := tx.QueryRowContext(ctx, `SELECT da.address, d.id FROM device_addresses da
		JOIN devices d ON d.id=da.device_id
		WHERE da.normalized_address IN (`+strings.TrimSuffix(strings.Repeat("?,", len(canonical)), ",")+`)
		AND da.normalized_address <> '' AND (? OR d.device_type <> 'virtual') AND d.id <> ? LIMIT 1`, args...).Scan(&address, &owner)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: %s is already used by device %s", domain.ErrDeviceAddressConflict, address, owner)
}
