package categories

import (
	"context"
	"database/sql"
	"fmt"
	"src/internal/errmsg"
	"src/internal/models"
	"strings"
)

//--------------------------------------------------------------------------------------|

type DBRepo struct {
	db *sql.DB
}

func NewDBRepo(db *sql.DB) *DBRepo {
	return &DBRepo{db: db}
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) GetCategories(ctx context.Context) ([]models.Category, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name FROM categories ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []models.Category
	for rows.Next() {
		var c models.Category
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, nil
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) CreateCategory(ctx context.Context, name string) (*models.Category, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO categories (name) VALUES (?)`, name)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, errmsg.ErrUniqueConstraint
		}

		return nil, fmt.Errorf("create category failed: %w", err)
	}
	id, _ := result.LastInsertId()
	return &models.Category{ID: int(id), Name: name}, nil
}
