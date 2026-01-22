package hackernews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"src/internal/categories"
	"src/internal/sessions"
	"src/internal/utils"
	"src/internal/web"
)

//--------------------------------------------------------------------------------------|

const (
	HNBaseURL     = "https://hacker-news.firebaseio.com/v0"
	TopStoriesURL = HNBaseURL + "/topstories.json"
	ItemURL       = HNBaseURL + "/item/%d.json"
	MaxStories    = 30
)

//--------------------------------------------------------------------------------------|

type HNItem struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Text  string `json:"text"`
	Score int    `json:"score"`
	By    string `json:"by"`
	Time  int64  `json:"time"`
	Type  string `json:"type"`
	Kids  []int  `json:"kids"`
}

//--------------------------------------------------------------------------------------|

type DBRepo struct {
	db *sql.DB
}

func NewDBRepo(db *sql.DB) *DBRepo {
	return &DBRepo{db: db}
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) SyncHackerNews(ctx context.Context, defaultUserID int) (int, error) {
	storyIDs, err := fetchTopStories()
	if err != nil {
		return 0, fmt.Errorf("failed to fetch top stories: %w", err)
	}

	imported := 0
	for i, storyID := range storyIDs {
		if i >= MaxStories {
			break
		}

		item, err := fetchItem(storyID)
		if err != nil {
			log.Printf("Failed to fetch item %d: %v", storyID, err)
			continue
		}

		if item.Type != "story" || item.Title == "" {
			continue
		}

		exists, err := r.postExists(ctx, storyID)
		if err != nil {
			log.Printf("Error checking if post exists: %v", err)
			continue
		}
		if exists {
			continue
		}

		title := item.Title
		if len(title) > 200 {
			title = title[:200]
		}

		body := item.Text
		if body == "" {
			body = fmt.Sprintf("Original story: %s", item.URL)
		}
		if len(body) > 3000 {
			body = body[:3000]
		}

		createdAt := time.Unix(item.Time, 0).UTC()

		categoryIDs, err := r.ensureCategories(ctx, item)
		if err != nil {
			log.Printf("Error ensuring categories: %v", err)
		}

		postID, err := r.createPostFromHN(ctx, defaultUserID, title, body, item.URL, storyID, createdAt, categoryIDs)
		if err != nil {
			log.Printf("Failed to create post from HN item %d: %v", storyID, err)
			continue
		}

		log.Printf("Imported HN story %d as post %d: %s", storyID, postID, title)
		imported++
	}

	return imported, nil
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) postExists(ctx context.Context, hnID int) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM posts WHERE hacker_news_id = ?)`, hnID).Scan(&exists)
	return exists, err
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) createPostFromHN(ctx context.Context, userID int, title, body, url string, hnID int, createdAt time.Time, categoryIDs []int) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("could not begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	result, err := tx.ExecContext(ctx,
		`INSERT INTO posts (user_id, title, body, url, hacker_news_id, created_at) 
         VALUES (?, ?, ?, ?, ?, ?)`,
		userID, title, body, url, hnID, createdAt)
	if err != nil {
		return 0, fmt.Errorf("failed to insert post: %w", err)
	}

	postID64, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID: %w", err)
	}
	postID := int(postID64)

	for _, catID := range categoryIDs {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO post_categories (post_id, category_id) VALUES (?, ?)`,
			postID, catID)
		if err != nil {
			return 0, fmt.Errorf("failed to link category %d: %w", catID, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return postID, nil
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) ensureCategories(ctx context.Context, item *HNItem) ([]int, error) {
	categoriesRepo := categories.NewDBRepo(r.db)

	categoryNames := []string{"Hacker News"}
	if item.URL != "" {
		categoryNames = append(categoryNames, "External Link")
	} else {
		categoryNames = append(categoryNames, "Ask HN")
	}

	var categoryIDs []int
	for _, catName := range categoryNames {
		catID, err := r.getOrCreateCategory(ctx, categoriesRepo, catName)
		if err != nil {
			log.Printf("Error getting/creating category %s: %v", catName, err)
			continue
		}
		categoryIDs = append(categoryIDs, catID)
	}

	return categoryIDs, nil
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) getOrCreateCategory(ctx context.Context, repo *categories.DBRepo, name string) (int, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `SELECT id FROM categories WHERE name = ?`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	cat, err := repo.CreateCategory(ctx, name)
	if err != nil {
		return 0, err
	}
	return cat.ID, nil
}

//--------------------------------------------------------------------------------------|

func fetchTopStories() ([]int, error) {
	resp, err := http.Get(TopStoriesURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var storyIDs []int
	if err := json.Unmarshal(body, &storyIDs); err != nil {
		return nil, err
	}

	return storyIDs, nil
}

//--------------------------------------------------------------------------------------|

func fetchItem(id int) (*HNItem, error) {
	url := fmt.Sprintf(ItemURL, id)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var item HNItem
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, err
	}

	return &item, nil
}

//--------------------------------------------------------------------------------------|

func SyncHandler(db *sql.DB, ts *web.TemplateStore, sm *sessions.SessionManager) http.HandlerFunc {
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

		var defaultUserID int
		err := db.QueryRowContext(r.Context(), `SELECT id FROM users LIMIT 1`).Scan(&defaultUserID)
		if err == sql.ErrNoRows {
			http.Redirect(w, r, "/posts?error="+http.StatusText(http.StatusBadRequest)+":+Please+register+first", http.StatusSeeOther)
			return
		}
		if err != nil {
			web.InternalServerError(w, r, ts, err, 0)
			return
		}

		repo := NewDBRepo(db)
		imported, err := repo.SyncHackerNews(r.Context(), defaultUserID)
		if err != nil {
			log.Printf("Sync error: %v", err)
			http.Redirect(w, r, "/posts?error=Sync+failed:+"+err.Error(), http.StatusSeeOther)
			return
		}

		msg := fmt.Sprintf("Successfully imported %d stories from Hacker News", imported)
		http.Redirect(w, r, "/posts?sync="+msg, http.StatusSeeOther)
	}
}
