package postgres

// This file defines canvas map repo persistence behavior, ordering guarantees, and not-found conventions.

import (
	"database/sql"
)

// CanvasMapRepo implements domain.CanvasMapRepository using PostgreSQL SQL.
type CanvasMapRepo struct {
	db *DB
}

// NewCanvasMapRepo creates a new PostgreSQL-backed canvas map repository.
func NewCanvasMapRepo(db *sql.DB) *CanvasMapRepo {
	return &CanvasMapRepo{db: wrapDB(db)}
}

// CanvasMapPositionRepo implements domain.CanvasMapPositionRepository using PostgreSQL SQL.
type CanvasMapPositionRepo struct {
	db *DB
}

// NewCanvasMapPositionRepo creates a new PostgreSQL-backed canvas map position repository.
func NewCanvasMapPositionRepo(db *sql.DB) *CanvasMapPositionRepo {
	return &CanvasMapPositionRepo{db: wrapDB(db)}
}
