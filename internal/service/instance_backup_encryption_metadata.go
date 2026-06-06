package service

// This file defines instance backup encryption metadata backup and restore service behavior, including filesystem safety and cleanup expectations.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/lollinoo/theia/internal/domain"
)

func (s *InstanceBackupService) collectRequiredCredentialKeyIDs(ctx context.Context) ([]string, error) {
	if s == nil || s.db == nil || s.keyring == nil {
		return nil, nil
	}
	values, err := s.collectSensitiveCredentialValues(ctx)
	if err != nil {
		return nil, err
	}
	return requiredCredentialKeyIDsFromValues(values)
}

func (s *InstanceBackupService) collectSensitiveCredentialValues(ctx context.Context) ([]string, error) {
	var values []string
	deviceValues, err := collectSNMPCredentialJSONValues(ctx, s.db, `SELECT snmp_credentials_json FROM devices WHERE snmp_credentials_json != '' AND snmp_credentials_json != '{}'`)
	if err != nil {
		return nil, fmt.Errorf("collecting device credential key ids: %w", err)
	}
	values = append(values, deviceValues...)

	profileValues, err := collectSNMPCredentialJSONValues(ctx, s.db, `SELECT credentials_json FROM snmp_profiles WHERE credentials_json != '' AND credentials_json != '{}'`)
	if err != nil {
		return nil, fmt.Errorf("collecting SNMP profile credential key ids: %w", err)
	}
	values = append(values, profileValues...)

	credentialProfileValues, err := collectStringColumnValues(ctx, s.db, `SELECT encrypted_secret FROM credential_profiles WHERE encrypted_secret != ''`)
	if err != nil {
		return nil, fmt.Errorf("collecting credential profile key ids: %w", err)
	}
	values = append(values, credentialProfileValues...)
	return values, nil
}

func collectSNMPCredentialJSONValues(ctx context.Context, db *sql.DB, query string) ([]string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var creds domain.SNMPCredentials
		if err := json.Unmarshal([]byte(raw), &creds); err != nil {
			return nil, fmt.Errorf("parsing SNMP credential JSON: %w", err)
		}
		if creds.V2c != nil {
			values = append(values, creds.V2c.Community)
		}
		if creds.V3 != nil {
			values = append(values, creds.V3.AuthPassword, creds.V3.PrivPassword)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func collectStringColumnValues(ctx context.Context, db *sql.DB, query string) ([]string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}
