package comments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"src/internal/errmsg"
	"src/internal/models"
	"src/internal/posts"
	"src/internal/sessions"
	"src/internal/utils"
	"src/internal/web"
)

//--------------------------------------------------------------------------------------|

type DBRepo struct {
	db *sql.DB
}

func NewDBRepo(db *sql.DB) *DBRepo {
	return &DBRepo{db: db}
}

//--------------------------------------------------------------------------------------|

func CreateCommentHandler(db *sql.DB, ts *web.TemplateStore, sm *sessions.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: need to get rid of it since it's violating DRY
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
		body := r.FormValue("body")

		renderForm := func(w http.ResponseWriter, errorMsg, body string, postID, userID int) {
			repo := posts.NewDBRepo(db)
			post, err := repo.GetPost(r.Context(), postID, userID)
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
				if err := rows.Scan(&c.ID, &c.UserID, &c.PostID, &c.Username, &c.Body,
					&c.CreatedAt, &parentID, &c.Depth, &c.Likes, &c.Dislikes, &c.UserLike); err != nil {
					web.InternalServerError(w, r, ts, err, userID)
					return
				}
				c.ParentID = parentID
				comments = append(comments, c)
			}

			organized := posts.OrganizeComments(comments, posts.MaxCommentDepth)
			ts.RenderTemplate(w, "post.html", map[string]any{
				"Post":     post,
				"Comments": organized,
				"UserID":   userID,
				"Error":    errorMsg,
				"Body":     body,
			})
		}

		if err := errmsg.ValidateCommentBody(body); err != nil {
			renderForm(w, err.Error(), body, postID, userID)
			return
		}

		var parentID sql.NullInt64
		if parentIDStr := r.FormValue("parent_id"); parentIDStr != "" {
			p, err := strconv.Atoi(parentIDStr)
			if err == nil {
				parentID = sql.NullInt64{Int64: int64(p), Valid: true}
			}
		}

		repo := NewDBRepo(db)
		if err := repo.CreateComment(r.Context(), userID, postID, body, parentID); err != nil {
			if errors.Is(err, errmsg.ErrCommentDepthExceeded) {
				http.Error(w, "Reply depth limit reached", http.StatusBadRequest)
				return
			}

			if errors.Is(err, errmsg.ErrParentCommentNotFound) {
				http.Error(w, "Parent comment not found", http.StatusBadRequest)
				return
			}

			if errors.Is(err, errmsg.ErrPostNotFound) {
				http.Error(w, "Post not found", http.StatusNotFound)
				return
			}

			web.InternalServerError(w, r, ts, err, userID)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/posts/%d", postID), http.StatusSeeOther)
	}
}

//--------------------------------------------------------------------------------------|

func DeleteCommentHandler(db *sql.DB, ts *web.TemplateStore, sm *sessions.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: need to get rid of it since it's violating DRY
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

		commentID, err := strconv.Atoi(r.FormValue("comment_id"))
		if err != nil {
			http.Error(w, "Invalid comment ID", http.StatusBadRequest)
			return
		}

		repo := NewDBRepo(db)
		postID, err := repo.DeleteComment(r.Context(), commentID, userID)

		if err != nil {
			if errors.Is(err, errmsg.ErrCommentNotFound) {
				web.NotFound(w, r, ts, userID)
				return
			}

			web.InternalServerError(w, r, ts, err, userID)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("/posts/%d", postID), http.StatusSeeOther)
	}
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) CreateComment(ctx context.Context, userID, postID int, body string, parentID sql.NullInt64) error {
	var depth int
	if parentID.Valid {
		err := r.db.QueryRowContext(ctx, `SELECT depth FROM comments WHERE id = ?`, parentID.Int64).Scan(&depth)

		if err == sql.ErrNoRows {
			return errmsg.ErrParentCommentNotFound
		}
		if err != nil {
			log.Printf("Error checking parent comment depth: %v", err)
			return fmt.Errorf("database query error: %w", err)
		}
		if depth >= 9 {
			return errmsg.ErrCommentDepthExceeded
		}
		depth++
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO comments (user_id, post_id, parent_id, body, depth)
		VALUES (?, ?, ?, ?, ?)`, userID, postID, parentID, body, depth)
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
			return errmsg.ErrPostNotFound
		}
		log.Printf("CreateComment error: %v", err)
	}
	return err
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) DeleteComment(ctx context.Context, commentID, userID int) (int, error) {
	var postID int
	var commentUserID int

	err := r.db.QueryRowContext(ctx,
		`SELECT post_id, user_id FROM comments WHERE id = ?`, commentID).Scan(&postID, &commentUserID)
	if err == sql.ErrNoRows {
		return 0, errmsg.ErrCommentNotFound
	}
	if err != nil {
		log.Printf("Error retrieving details for comment %d: %v", commentID, err)
		return 0, fmt.Errorf("database query error: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `DELETE FROM comments WHERE id = ?`, commentID)
	if err != nil {
		log.Printf("Error deleting comment %d: %v", commentID, err)
		return 0, fmt.Errorf("database exec error: %w", err)
	}

	return postID, nil
}
