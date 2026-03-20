package domain

import "time"

// VendorConfigRecord represents a vendor configuration stored in the database.
type VendorConfigRecord struct {
	Name        string
	DisplayName string
	ConfigJSON  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// VendorConfigRepository defines persistence operations for vendor configs.
type VendorConfigRepository interface {
	GetAll() ([]VendorConfigRecord, error)
	GetByName(name string) (*VendorConfigRecord, error)
	Upsert(record *VendorConfigRecord) error
}
