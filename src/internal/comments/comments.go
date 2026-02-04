package comments

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"src/internal/errmsg"
)

//--------------------------------------------------------------------------------------|

type DBRepo struct {
	db *sql.DB
}

func NewDBRepo(db *sql.DB) *DBRepo {
	return &DBRepo{db: db}
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) CreateComment(ctx context.Context, userID, postID int, body string, parentID sql.NullInt64) (int, error) {
	var depth int
	if parentID.Valid {
		err := r.db.QueryRowContext(ctx, `SELECT depth FROM comments WHERE id = ?`, parentID.Int64).Scan(&depth)

		if err == sql.ErrNoRows {
			return 0, errmsg.ErrParentCommentNotFound
		}
		if err != nil {
			log.Printf("Error checking parent comment depth: %v", err)
			return 0, fmt.Errorf("database query error: %w", err)
		}
		if depth >= 9 {
			return 0, errmsg.ErrCommentDepthExceeded
		}
		depth++
	}
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO comments (user_id, post_id, parent_id, body, depth)
		VALUES (?, ?, ?, ?, ?)`, userID, postID, parentID, body, depth)
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
			return 0, errmsg.ErrPostNotFound
		}
		log.Printf("CreateComment error: %v", err)
	}
	if err != nil {
		return 0, err
	}
	id, _ := result.LastInsertId()
	return int(id), nil
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
