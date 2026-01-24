package posts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"src/internal/categories"
	"src/internal/errmsg"
	"src/internal/models"
	"src/internal/sessions"
	"src/internal/utils"
	"src/internal/web"
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

func GetPostsHandler(db *sql.DB, ts *web.TemplateStore, sm *sessions.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID := utils.GetUserID(r.Context(), r, sm)
		limit, offset := web.GetLimitOffset(r)
		categoryID, _ := strconv.Atoi(r.URL.Query().Get("category"))
		filter := r.URL.Query().Get("filter")

		repo := NewDBRepo(db)
		posts, err := repo.GetPosts(r.Context(), userID, categoryID, filter, limit, offset)
		if err != nil {
			web.InternalServerError(w, r, ts, err, userID)
			return
		}

		categoriesRepo := categories.NewDBRepo(db)
		categories, err := categoriesRepo.GetCategories(r.Context())
		if err != nil {
			web.InternalServerError(w, r, ts, err, userID)
			return
		}

		syncMsg := r.URL.Query().Get("sync")
		errorMsg := r.URL.Query().Get("error")

		ts.RenderTemplate(w, "posts.html", map[string]any{
			"Posts":          posts,
			"Categories":     categories,
			"UserID":         userID,
			"FilterCategory": categoryID,
			"FilterType":     filter,
			"SyncMessage":    syncMsg,
			"Error":          errorMsg,
		})
	}
}

//--------------------------------------------------------------------------------------|

func GetPostHandler(db *sql.DB, ts *web.TemplateStore, sm *sessions.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 3 {
			http.Error(w, "Invalid post ID", http.StatusBadRequest)
			return
		}

		postID, err := strconv.Atoi(parts[2])
		if err != nil {
			http.Error(w, "Invalid post ID", http.StatusBadRequest)
			return
		}

		userID := utils.GetUserID(r.Context(), r, sm)
		repo := NewDBRepo(db)
		post, err := repo.GetPost(r.Context(), postID, userID)

		if errors.Is(err, sql.ErrNoRows) {
			web.NotFound(w, r, ts, userID)
			return
		}
		if err != nil {
			web.InternalServerError(w, r, ts, err, userID)
			return
		}

		rows, err := db.QueryContext(r.Context(),
			`SELECT c.id, c.user_id, c.post_id, u.username, c.body, c.created_at, c.parent_id, c.depth,
                    COALESCE(SUM(CASE WHEN l.value = 1 THEN 1 ELSE 0 END), 0) AS likes,
                    COALESCE(SUM(CASE WHEN l.value = -1 THEN 1 ELSE 0 END), 0) AS dislikes,
                    COALESCE(SUM(CASE WHEN l.user_id = ? THEN l.value ELSE 0 END), 0) AS user_like
             FROM comments c
             JOIN users u ON c.user_id = u.id
             LEFT JOIN likes l ON l.target_id = c.id AND l.target_type = 'comment'
             WHERE c.post_id = ?
             GROUP BY c.id
             ORDER BY c.created_at ASC`, userID, postID)
		if err != nil {
			web.InternalServerError(w, r, ts, err, userID)
			return
		}
		defer rows.Close()

		var comments []models.Comment
		for rows.Next() {
			var c models.Comment
			var parentID sql.NullInt64
			if err := rows.Scan(&c.ID, &c.UserID, &c.PostID, &c.Username, &c.Body, &c.CreatedAt, &parentID, &c.Depth, &c.Likes, &c.Dislikes, &c.UserLike); err != nil {
				web.InternalServerError(w, r, ts, err, userID)
				return
			}

			c.ViewerID = userID
			c.ParentID = parentID
			comments = append(comments, c)
		}

		organizedComments := OrganizeComments(comments, MaxCommentDepth)
		ts.RenderTemplate(w, "post.html", map[string]any{
			"Post":     post,
			"Comments": organizedComments,
			"UserID":   userID,
		})
	}
}

//--------------------------------------------------------------------------------------|

func CreatePostHandler(db *sql.DB, ts *web.TemplateStore, sm *sessions.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := utils.GetUserID(r.Context(), r, sm)
		if userID == 0 {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		repo := NewDBRepo(db)

		categoriesRepo := categories.NewDBRepo(db)
		categories, err := categoriesRepo.GetCategories(r.Context())
		if err != nil {
			web.InternalServerError(w, r, ts, err, userID)
			return
		}

		renderForm := func(w http.ResponseWriter, errorMsg, title, body string, categoryIDs []int, status int) {
			selectedCategories := make(map[int]bool)
			for _, id := range categoryIDs {
				selectedCategories[id] = true
			}

			if status > 0 {
				w.WriteHeader(status)
			}
			ts.RenderTemplate(w, "create_post.html", map[string]any{
				"Error":              errorMsg,
				"Categories":         categories,
				"UserID":             userID,
				"Title":              title,
				"Body":               body,
				"SelectedCategories": selectedCategories,
			})
		}

		if r.Method == http.MethodGet {
			renderForm(w, "", "", "", nil, 0)
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

		title := r.FormValue("title")
		body := r.FormValue("body")
		categoryIDs := parseCategoryIDs(r.Form["category_ids"])

		if len(categoryIDs) == 0 {
			renderForm(w, "Please select at least one category", title, body, categoryIDs, http.StatusBadRequest)
			return
		}

		if err := errmsg.ValidatePostTitle(title); err != nil {
			renderForm(w, err.Error(), title, body, categoryIDs, http.StatusBadRequest)
			return
		}
		if err := errmsg.ValidatePostBody(body); err != nil {
			renderForm(w, err.Error(), title, body, categoryIDs, http.StatusBadRequest)
			return
		}

		post, err := repo.CreatePost(r.Context(), userID, title, body, categoryIDs)
		if err != nil {
			web.InternalServerError(w, r, ts, err, userID)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("/posts/%d", post.ID), http.StatusSeeOther)
	}
}

//--------------------------------------------------------------------------------------|

func DeletePostHandler(db *sql.DB, ts *web.TemplateStore, sm *sessions.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := utils.GetUserID(r.Context(), r, sm)
		if userID == 0 {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
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

		postID, err := strconv.Atoi(r.FormValue("post_id"))
		if err != nil {
			http.Error(w, "Invalid post ID", http.StatusBadRequest)
			return
		}

		repo := NewDBRepo(db)
		if err := repo.DeletePost(r.Context(), postID, userID); err != nil {
			if errors.Is(err, errmsg.ErrPostNotFound) {
				web.NotFound(w, r, ts, userID)
				return
			}
			web.InternalServerError(w, r, ts, err, userID)
			return
		}
		http.Redirect(w, r, "/posts", http.StatusSeeOther)
	}
}

//--------------------------------------------------------------------------------------|

func LikeHandler(db *sql.DB, ts *web.TemplateStore, sm *sessions.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := utils.GetUserID(r.Context(), r, sm)
		if userID == 0 {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
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

		targetID, err := strconv.Atoi(r.FormValue("target_id"))
		if err != nil {
			http.Error(w, "Invalid target ID", http.StatusBadRequest)
			return
		}
		targetType := r.FormValue("target_type")
		value, err := strconv.Atoi(r.FormValue("value"))

		if err != nil || (value != 1 && value != -1 && value != 0) {
			http.Error(w, "Invalid value", http.StatusBadRequest)
			return
		}

		if targetType != "post" && targetType != "comment" {
			http.Error(w, "Invalid target type", http.StatusBadRequest)
			return
		}

		var authorID int
		var query string
		switch targetType {
		case "post":
			query = `SELECT user_id FROM posts WHERE id = ?`
		case "comment":
			query = `SELECT user_id FROM comments WHERE id = ?`
		}
		err = db.QueryRowContext(r.Context(), query, targetID).Scan(&authorID)
		if err == sql.ErrNoRows {
			web.NotFound(w, r, ts, userID)
			return
		}
		if err != nil {
			web.InternalServerError(w, r, ts, err, userID)
			return
		}

		var execErr error
		if value == 0 {
			_, execErr = db.ExecContext(r.Context(),
				`DELETE FROM likes 
                 WHERE user_id = ? AND target_id = ? AND target_type = ?`,
				userID, targetID, targetType)
		} else {
			_, execErr = db.ExecContext(r.Context(),
				`INSERT INTO likes (user_id, target_id, target_type, value)
                 VALUES (?, ?, ?, ?)
                 ON CONFLICT(user_id, target_id, target_type)
                 DO UPDATE SET value = ?`,
				userID, targetID, targetType, value, value)
		}

		if execErr != nil {
			web.InternalServerError(w, r, ts, execErr, userID)
			return
		}

		var redirectURL string
		if targetType == "post" {
			redirectURL = fmt.Sprintf("/posts/%d", targetID)
		} else {
			var postID int
			err := db.QueryRowContext(r.Context(), `SELECT post_id FROM comments WHERE id = ?`, targetID).Scan(&postID)
			if err != nil {
				redirectURL = "/posts"
			} else {
				redirectURL = fmt.Sprintf("/posts/%d", postID)
			}
		}

		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
	}
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) GetPosts(ctx context.Context, userID, categoryID int, filter string, limit, offset int) ([]models.Post, error) {
	query := `
        SELECT p.id, p.user_id, COALESCE(p.author, u.username) AS author, p.title, p.body, p.url, p.hacker_news_id, p.created_at,
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
		if err := rows.Scan(&p.ID, &p.UserID, &p.Username, &p.Title, &p.Body, &p.URL, &p.HackerNewsID, &p.CreatedAt, &p.Likes, &p.Dislikes, &p.UserLike); err != nil {
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
		`SELECT p.id, p.user_id, COALESCE(p.author, u.username) AS author, p.title, p.body, p.url, p.hacker_news_id, p.created_at,
                COALESCE(SUM(CASE WHEN l.value = 1 THEN 1 ELSE 0 END), 0) AS likes,
                COALESCE(SUM(CASE WHEN l.value = -1 THEN 1 ELSE 0 END), 0) AS dislikes,
                COALESCE(SUM(CASE WHEN l.user_id = ? THEN l.value ELSE 0 END), 0) AS user_like
         FROM posts p
         JOIN users u ON p.user_id = u.id
         LEFT JOIN likes l ON l.target_id = p.id AND l.target_type = 'post'
         WHERE p.id = ?
         GROUP BY p.id`, userID, postID).Scan(
		&p.ID, &p.UserID, &p.Username, &p.Title, &p.Body, &p.URL, &p.HackerNewsID, &p.CreatedAt, &p.Likes, &p.Dislikes, &p.UserLike)
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
