package posts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"src/internal/errmsg"
	"src/internal/models"
)

//--------------------------------------------------------------------------------------|

const MaxCommentDepth = 9

//--------------------------------------------------------------------------------------|

type DBRepo struct {
	db *sql.DB
}

func NewDBRepo(db *sql.DB) *DBRepo {
	return &DBRepo{db: db}
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) GetPosts(ctx context.Context, userID, categoryID int, filter string, limit, offset int) ([]models.Post, error) {
	query := `
        SELECT p.id, p.user_id, COALESCE(p.author, u.username) AS author, p.title, p.body, p.created_at,
               COALESCE(SUM(CASE WHEN l.value = 1 THEN 1 ELSE 0 END), 0) AS likes,
               COALESCE(SUM(CASE WHEN l.value = -1 THEN 1 ELSE 0 END), 0) AS dislikes,
               COALESCE(SUM(CASE WHEN l.user_id = ? THEN l.value ELSE 0 END), 0) AS user_like
        FROM posts p
        JOIN users u ON p.user_id = u.id
        LEFT JOIN likes l ON l.target_id = p.id AND l.target_type = 'post'`
	args := []any{userID}

	if categoryID > 0 {
		query += ` JOIN post_categories pc ON pc.post_id = p.id AND pc.category_id = ?`
		args = append(args, categoryID)
	}
	if filter == "created" && userID > 0 {
		query += ` WHERE p.user_id = ?`
		args = append(args, userID)
	} else if filter == "liked" && userID > 0 {
		query += ` JOIN likes l2 ON l2.target_id = p.id AND l2.target_type = 'post' AND l2.user_id = ? AND l2.value = 1`
		args = append(args, userID)
	}

	query += ` GROUP BY p.id ORDER BY p.created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []models.Post
	for rows.Next() {
		var p models.Post
		if err := rows.Scan(&p.ID, &p.UserID, &p.Username, &p.Title, &p.Body, &p.CreatedAt, &p.Likes, &p.Dislikes, &p.UserLike); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}

	if err := r.fetchCategoriesForPosts(ctx, posts); err != nil {
		return nil, err
	}

	return posts, nil
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) GetPost(ctx context.Context, postID, userID int) (*models.Post, error) {
	var p models.Post
	err := r.db.QueryRowContext(ctx,
		`SELECT p.id, p.user_id, COALESCE(p.author, u.username) AS author, p.title, p.body, p.created_at,
                COALESCE(SUM(CASE WHEN l.value = 1 THEN 1 ELSE 0 END), 0) AS likes,
                COALESCE(SUM(CASE WHEN l.value = -1 THEN 1 ELSE 0 END), 0) AS dislikes,
                COALESCE(SUM(CASE WHEN l.user_id = ? THEN l.value ELSE 0 END), 0) AS user_like
         FROM posts p
         JOIN users u ON p.user_id = u.id
         LEFT JOIN likes l ON l.target_id = p.id AND l.target_type = 'post'
         WHERE p.id = ?
         GROUP BY p.id`, userID, postID).Scan(
		&p.ID, &p.UserID, &p.Username, &p.Title, &p.Body, &p.CreatedAt, &p.Likes, &p.Dislikes, &p.UserLike)
	if err != nil {
		return nil, err
	}
	p.Categories, _ = r.getPostCategories(ctx, postID)
	return &p, nil
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) CreatePost(ctx context.Context, userID int, title, body string, categoryIDs []int) (*models.Post, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	result, err := tx.ExecContext(ctx, `INSERT INTO posts (user_id, title, body, created_at) VALUES (?, ?, ?, ?)`,
		userID, title, body, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to insert post: %w", err)
	}
	id, _ := result.LastInsertId()
	postID := int(id)

	for _, catID := range categoryIDs {
		_, err = tx.ExecContext(ctx, `INSERT INTO post_categories (post_id, category_id) VALUES (?, ?)`, postID, catID)
		if err != nil {
			return nil, fmt.Errorf("failed to link category %d: %w", catID, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &models.Post{ID: postID, UserID: userID, Title: title, Body: body}, nil
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) DeletePost(ctx context.Context, id, userID int) error {
	var postUserID int
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM posts WHERE id = ?`, id).Scan(&postUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return errmsg.ErrPostNotFound
	}
	if err != nil {
		return fmt.Errorf("database query error: %w", err)
	}

	if postUserID != userID {
		return errmsg.ErrPostNotFound
	}

	_, err = r.db.ExecContext(ctx, `DELETE FROM posts WHERE id = ?`, id)
	return err
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) getPostCategories(ctx context.Context, postID int) ([]models.Category, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT c.id, c.name FROM categories c JOIN post_categories pc ON pc.category_id = c.id WHERE pc.post_id = ?`, postID)
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

func (r *DBRepo) fetchCategoriesForPosts(ctx context.Context, posts []models.Post) error {
	if len(posts) == 0 {
		return nil
	}

	ids := make([]string, len(posts))
	postMap := make(map[int]*models.Post, len(posts))
	for i := range posts {
		ids[i] = strconv.Itoa(posts[i].ID)
		postMap[posts[i].ID] = &posts[i]
	}

	query := fmt.Sprintf(`
        SELECT pc.post_id, c.id, c.name
        FROM post_categories pc
        JOIN categories c ON pc.category_id = c.id
        WHERE pc.post_id IN (%s)
        ORDER BY pc.post_id, c.name ASC
    `, strings.Join(ids, ","))

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var postID int
		var category models.Category
		if err := rows.Scan(&postID, &category.ID, &category.Name); err != nil {
			return err
		}

		if post, ok := postMap[postID]; ok {
			post.Categories = append(post.Categories, category)
		}
	}
	return nil
}

//--------------------------------------------------------------------------------------|

func parseCategoryIDs(ids []string) []int {
	result := make([]int, 0, len(ids))
	for _, idStr := range ids {
		if id, err := strconv.Atoi(idStr); err == nil {
			result = append(result, id)
		}
	}
	return result
}

//--------------------------------------------------------------------------------------|

func OrganizeComments(comments []models.Comment, maxDepth int) []models.Comment {
	commentMap := make(map[int]*models.Comment)
	for i := range comments {
		commentMap[comments[i].ID] = &comments[i]
		comments[i].MaxDepth = maxDepth
	}

	var rootComments []*models.Comment
	for i := range comments {
		c := &comments[i]
		if !c.ParentID.Valid {
			rootComments = append(rootComments, c)
			continue
		}

		parentID := int(c.ParentID.Int64)
		parent, exists := commentMap[parentID]
		if !exists {
			rootComments = append(rootComments, c)
			continue
		}

		parent.Children = append(parent.Children, c)
	}

	out := make([]models.Comment, len(rootComments))
	for i, c := range rootComments {
		out[i] = *c
	}
	return out
}
