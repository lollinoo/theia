package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// AreaRepo implements domain.AreaRepository using SQLite.
type AreaRepo struct {
	db *sql.DB
}

// NewAreaRepo creates a new SQLite-backed area repository.
func NewAreaRepo(db *sql.DB) *AreaRepo {
	return &AreaRepo{db: db}
}

// Create inserts a new area.
func (r *AreaRepo) Create(area *domain.Area) error {
	if area.ID == uuid.Nil {
		area.ID = uuid.New()
	}
	now := time.Now().UTC()
	area.CreatedAt = now
	area.UpdatedAt = now

	_, err := r.db.Exec(
		`INSERT INTO areas (id, name, description, color, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		area.ID.String(),
		area.Name,
		area.Description,
		area.Color,
		now,
		now,
	)
	return err
}

// GetByID returns an area by its UUID.
func (r *AreaRepo) GetByID(id uuid.UUID) (*domain.Area, error) {
	row := r.db.QueryRow(
		`SELECT id, name, description, color, created_at, updated_at
		 FROM areas WHERE id = ?`,
		id.String(),
	)
	return r.scanArea(row)
}

// GetAll returns all areas ordered by name.
func (r *AreaRepo) GetAll() ([]domain.Area, error) {
	rows, err := r.db.Query(
		`SELECT id, name, description, color, created_at, updated_at
		 FROM areas ORDER BY name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var areas []domain.Area
	for rows.Next() {
		a, err := r.scanAreaRow(rows)
		if err != nil {
			return nil, err
		}
		areas = append(areas, *a)
	}
	return areas, rows.Err()
}

// GetAllWithDeviceCount returns all areas with the count of assigned devices.
func (r *AreaRepo) GetAllWithDeviceCount() ([]domain.AreaWithCount, error) {
	rows, err := r.db.Query(
		`SELECT a.id, a.name, a.description, a.color, a.created_at, a.updated_at,
		        COUNT(d.id) as device_count
		 FROM areas a
		 LEFT JOIN devices d ON d.area_id = a.id
		 GROUP BY a.id
		 ORDER BY a.name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var areas []domain.AreaWithCount
	for rows.Next() {
		var ac domain.AreaWithCount
		var idStr string
		err := rows.Scan(
			&idStr, &ac.Name, &ac.Description, &ac.Color,
			&ac.CreatedAt, &ac.UpdatedAt, &ac.DeviceCount,
		)
		if err != nil {
			return nil, err
		}
		ac.ID = uuid.MustParse(idStr)
		areas = append(areas, ac)
	}
	return areas, rows.Err()
}

// Update overwrites an existing area.
func (r *AreaRepo) Update(area *domain.Area) error {
	area.UpdatedAt = time.Now().UTC()

	res, err := r.db.Exec(
		`UPDATE areas SET name=?, description=?, color=?, updated_at=? WHERE id=?`,
		area.Name,
		area.Description,
		area.Color,
		area.UpdatedAt,
		area.ID.String(),
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("area not found: %s", area.ID)
	}
	return nil
}

// Delete removes an area by its UUID.
func (r *AreaRepo) Delete(id uuid.UUID) error {
	res, err := r.db.Exec(`DELETE FROM areas WHERE id = ?`, id.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("area not found: %s", id)
	}
	return nil
}

// --- helpers ---

// scanArea scans a single area row from a *sql.Row.
func (r *AreaRepo) scanArea(row *sql.Row) (*domain.Area, error) {
	var a domain.Area
	var idStr string

	err := row.Scan(&idStr, &a.Name, &a.Description, &a.Color, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("area not found")
		}
		return nil, err
	}
	a.ID = uuid.MustParse(idStr)
	return &a, nil
}

// scanAreaRow scans a single area row from *sql.Rows.
func (r *AreaRepo) scanAreaRow(rows *sql.Rows) (*domain.Area, error) {
	var a domain.Area
	var idStr string

	err := rows.Scan(&idStr, &a.Name, &a.Description, &a.Color, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	a.ID = uuid.MustParse(idStr)
	return &a, nil
}
