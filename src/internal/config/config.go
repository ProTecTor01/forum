package config

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"src/internal/api"
	"src/internal/sessions"
	"src/internal/utils"
	"src/internal/ws"

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
	DB   *sql.DB
	Port int
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

	sessionManager := sessions.NewSessionManager(db, sessions.DefaultSessionTTL)
	hub := ws.NewHub()
	go hub.Run()

	apiHandler := api.NewAPI(db, sessionManager, hub)

	staticDir := utils.Getenv("STATIC_DIR", DefaultStaticDir)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	http.HandleFunc("/", apiHandler.Index)
	http.HandleFunc("/ws", apiHandler.WS)

	http.HandleFunc("/api/register", apiHandler.Register)
	http.HandleFunc("/api/login", apiHandler.Login)
	http.HandleFunc("/api/logout", apiHandler.Logout)
	http.HandleFunc("/api/me", apiHandler.Me)

	http.HandleFunc("/api/posts", apiHandler.Posts)
	http.HandleFunc("/api/posts/", apiHandler.PostByID)
	http.HandleFunc("/api/comments", apiHandler.Comments)
	http.HandleFunc("/api/categories", apiHandler.Categories)
	http.HandleFunc("/api/likes", apiHandler.Likes)

	http.HandleFunc("/api/users", apiHandler.Users)
	http.HandleFunc("/api/chats", apiHandler.ChatList)
	http.HandleFunc("/api/messages", apiHandler.Messages)

	cfg := &Config{
		DB:   db,
		Port: utils.GetIntEnv("PORT", defaultPort),
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

	applyMigration := func(path string) {
		migration, err := os.ReadFile(path)
		if err == nil {
			db.Exec(string(migration))
		}
	}
	applyMigration("./assets/database/migration_add_post_author.sql")
	applyMigration("./assets/database/migration_add_user_profile.sql")
	applyMigration("./assets/database/migration_add_messages.sql")

	return nil
}
