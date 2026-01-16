package config

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"src/internal/auth"
	"src/internal/categories"
	"src/internal/comments"
	"src/internal/hackernews"
	"src/internal/posts"
	"src/internal/sessions"
	"src/internal/utils"
	"src/internal/web"

	_ "github.com/mattn/go-sqlite3"
)

//--------------------------------------------------------------------------------------

const (
	defaultPort      = 8080
	defaultDBPath    = "./assets/database/forum.db"
	DefaultStaticDir = "./assets/static/"
)

//--------------------------------------------------------------------------------------

type Config struct {
	DB       *sql.DB
	Template *web.TemplateStore
	Port     int
}

//--------------------------------------------------------------------------------------

func Setup() (*Config, error) {
	dbPath := utils.Getenv("DB_PATH", defaultDBPath)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %v", err)
	}

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate db: %v", err)
	}

	// Auto-sync Hacker News on first run if database is empty
	if err := autoSyncIfEmpty(db); err != nil {
		log.Printf("Auto-sync warning: %v", err)
	}

	templateStore, err := web.NewTemplateStore()
	if err != nil {
		return nil, fmt.Errorf("template store: %v", err)
	}

	sessionManager := sessions.NewSessionManager(db, sessions.DefaultSessionTTL)

	http.Handle("/static/", http.StripPrefix("/static/", web.FileServer()))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/posts", http.StatusSeeOther)
	})
	http.HandleFunc("/register", auth.RegisterHandler(db, templateStore, sessionManager))
	http.HandleFunc("/login", auth.LoginHandler(db, templateStore, sessionManager))
	http.HandleFunc("/logout", auth.LogoutHandler(db, sessionManager))
	http.HandleFunc("/posts", posts.GetPostsHandler(db, templateStore, sessionManager))
	http.HandleFunc("/posts/", posts.GetPostHandler(db, templateStore, sessionManager))
	http.HandleFunc("/posts/create", posts.CreatePostHandler(db, templateStore, sessionManager))
	http.HandleFunc("/posts/delete", posts.DeletePostHandler(db, templateStore, sessionManager))
	http.HandleFunc("/comments/create", comments.CreateCommentHandler(db, templateStore, sessionManager))
	http.HandleFunc("/comments/delete", comments.DeleteCommentHandler(db, templateStore, sessionManager))
	http.HandleFunc("/categories", categories.GetCategoriesHandler(db, templateStore, sessionManager))
	http.HandleFunc("/categories/create", categories.CreateCategoryHandler(db, templateStore, sessionManager))
	http.HandleFunc("/likes", posts.LikeHandler(db, templateStore, sessionManager))
	http.HandleFunc("/sync/hackernews", hackernews.SyncHandler(db, templateStore, sessionManager))

	cfg := &Config{
		DB:       db,
		Template: templateStore,
		Port:     utils.GetIntEnv("PORT", defaultPort),
	}
	return cfg, nil
}

//--------------------------------------------------------------------------------------

func (c *Config) Close() {
	if err := c.DB.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}
}

//--------------------------------------------------------------------------------------

func migrate(db *sql.DB) error {
	schemaPath := os.Getenv("SCHEMA_PATH")
	if schemaPath == "" {
		schemaPath = "./assets/database/schema.sql"
	}
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}

	_, err = db.Exec(string(schema))
	if err != nil {
		return err
	}

	// Apply migration for existing databases
	migrationPath := "./assets/database/migration_add_hn_fields.sql"
	migration, err := os.ReadFile(migrationPath)
	if err == nil {
		// Try to apply migration, ignore errors if columns already exist
		db.Exec(string(migration))
	}

	return nil
}

//--------------------------------------------------------------------------------------

func autoSyncIfEmpty(db *sql.DB) error {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM posts`).Scan(&count)
	if err != nil {
		return err
	}

	// If no posts exist, try to sync
	if count == 0 {
		var userID int
		err := db.QueryRow(`SELECT id FROM users LIMIT 1`).Scan(&userID)
		if err == sql.ErrNoRows {
			// No users yet, skip auto-sync
			return nil
		}
		if err != nil {
			return err
		}

		log.Println("[auto-sync] Database is empty, syncing Hacker News...")
		repo := hackernews.NewDBRepo(db)
		imported, err := repo.SyncHackerNews(context.Background(), userID)
		if err != nil {
			return fmt.Errorf("auto-sync failed: %w", err)
		}
		log.Printf("[auto-sync] Successfully imported %d stories", imported)
	}

	return nil
}
