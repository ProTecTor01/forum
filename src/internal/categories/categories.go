package categories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"src/internal/errmsg"
	"src/internal/models"
	"src/internal/sessions"
	"src/internal/utils"
	"src/internal/web"
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

func GetCategoriesHandler(db *sql.DB, ts *web.TemplateStore, sm *sessions.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		repo := NewDBRepo(db)
		categories, err := repo.GetCategories(r.Context())
		if err != nil {
			web.InternalServerError(w, r, ts, err, utils.GetUserID(r.Context(), r, sm))
			return
		}
		ts.RenderTemplate(w, "categories.html", map[string]any{
			"Categories": categories,
			"UserID":     utils.GetUserID(r.Context(), r, sm),
		})
	}
}

//--------------------------------------------------------------------------------------|

func CreateCategoryHandler(db *sql.DB, ts *web.TemplateStore, sm *sessions.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: need to get rid of it since it's violating DRY
		userID := utils.GetUserID(r.Context(), r, sm)
		if userID == 0 {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		if r.Method == http.MethodGet {
			ts.RenderTemplate(w, "category_create.html", map[string]any{
				"UserID": userID,
			})
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		name := r.FormValue("name")
		if err := errmsg.ValidateCategoryName(name); err != nil {
			ts.RenderTemplate(w, "category_create.html", map[string]any{
				"Error":  err.Error(),
				"UserID": userID,
			})
			return
		}

		repo := NewDBRepo(db)
		_, err := repo.CreateCategory(r.Context(), name)
		if err != nil {
			if errors.Is(err, errmsg.ErrUniqueConstraint) {
				ts.RenderTemplate(w, "category_create.html", map[string]any{
					"Error":  "A category with this name already exists",
					"UserID": userID,
				})
			} else {
				web.InternalServerError(w, r, ts, err, userID)
			}
			return
		}
		http.Redirect(w, r, "/categories", http.StatusSeeOther)
	}
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
