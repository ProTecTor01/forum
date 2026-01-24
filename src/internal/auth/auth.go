package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"src/internal/errmsg"
	"src/internal/models"
	"src/internal/sessions"
	"src/internal/utils"
	"src/internal/web"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

//--------------------------------------------------------------------------------------|

type DBRepo struct {
	db *sql.DB
}

func NewDBRepo(db *sql.DB) *DBRepo {
	return &DBRepo{db: db}
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) Register(ctx context.Context, email, username, password string) (*models.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now().UTC().Truncate(time.Second)

	result, err := r.db.ExecContext(ctx,
		`INSERT INTO users (username, email, password_hash, created_at, role) 
         VALUES (LOWER(?), LOWER(?), ?, ?, 'user')`, username, email, hash, now)

	if err != nil {
		log.Printf("User registration error for email %s: %v", email, err)

		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, errmsg.ErrUserAlreadyExists
		}
		return nil, fmt.Errorf("database insert error: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve last insert ID: %w", err)
	}

	return &models.User{
		ID:           int(id),
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		CreatedAt:    now,
		Role:         "user",
	}, nil
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) Login(ctx context.Context, email, password string) (*models.User, error) {
	var user models.User

	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, email, password_hash, created_at, role 
         FROM users WHERE email = LOWER(?)`, email).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.Role)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, errmsg.ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("database query error: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return nil, errmsg.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("password comparison failed: %w", err)
	}

	return &user, nil
}

//--------------------------------------------------------------------------------------|

func RegisterHandler(db *sql.DB, ts *web.TemplateStore, sm *sessions.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := utils.GetUserID(r.Context(), r, sm)
		if userID > 0 {
			http.Redirect(w, r, "/posts", http.StatusSeeOther)
			return
		}

		if r.Method == http.MethodGet {
			ts.RenderTemplate(w, "register.html", nil)
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

		email := r.FormValue("email")
		username := r.FormValue("username")
		password := r.FormValue("password")

		renderForm := func(errorMsg string, status int) {
			if status > 0 {
				w.WriteHeader(status)
			}
			ts.RenderTemplate(w, "register.html", map[string]any{
				"Error":    errorMsg,
				"Email":    email,
				"Username": username,
			})
		}

		if err := errmsg.ValidateEmail(email); err != nil {
			renderForm(err.Error(), http.StatusBadRequest)
			return
		}
		if err := errmsg.ValidateUsername(username); err != nil {
			renderForm(err.Error(), http.StatusBadRequest)
			return
		}
		if err := errmsg.ValidatePassword(password); err != nil {
			renderForm(err.Error(), http.StatusBadRequest)
			return
		}

		repo := NewDBRepo(db)
		_, err := repo.Register(r.Context(), email, username, password)

		if err != nil {
			if errors.Is(err, errmsg.ErrUserAlreadyExists) {
				renderForm("Email or username is already taken", http.StatusBadRequest)
				return
			}

			log.Printf("Registration error: %v", err)
			web.InternalServerError(w, r, ts, err, 0)
			return
		}

		user, err := repo.Login(r.Context(), email, password)
		if err != nil {
			log.Printf("Auto-login failed after registration for %s: %v", email, err)
			web.InternalServerError(w, r, ts, err, 0)
			return
		}

		session, err := sm.CreateSession(r.Context(), user.ID)
		if err != nil {
			web.InternalServerError(w, r, ts, err, 0)
			return
		}

		setAuthCookie(w, session.ID, session.ExpiresAt)
		http.Redirect(w, r, "/posts", http.StatusSeeOther)
	}
}

//--------------------------------------------------------------------------------------|

func LoginHandler(db *sql.DB, ts *web.TemplateStore, sm *sessions.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := utils.GetUserID(r.Context(), r, sm)
		if userID > 0 {
			http.Redirect(w, r, "/posts", http.StatusSeeOther)
			return
		}

		if r.Method == http.MethodGet {
			ts.RenderTemplate(w, "login.html", nil)
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

		email := r.FormValue("email")
		password := r.FormValue("password")

		renderForm := func(errorMsg string, status int) {
			if status > 0 {
				w.WriteHeader(status)
			}
			ts.RenderTemplate(w, "login.html", map[string]any{
				"Error": errorMsg,
				"Email": email,
			})
		}

		if err := errmsg.ValidateEmail(email); err != nil {
			renderForm(err.Error(), http.StatusBadRequest)
			return
		}

		repo := NewDBRepo(db)
		user, err := repo.Login(r.Context(), email, password)

		if err != nil {
			if errors.Is(err, errmsg.ErrInvalidCredentials) {
				renderForm("Invalid credentials", http.StatusBadRequest)
				return
			}

			log.Printf("Login lookup/db error for email '%s': %v", email, err)
			web.InternalServerError(w, r, ts, err, 0)
			return
		}

		session, err := sm.CreateSession(r.Context(), user.ID)
		if err != nil {
			web.InternalServerError(w, r, ts, err, 0)
			return
		}

		setAuthCookie(w, session.ID, session.ExpiresAt)
		http.Redirect(w, r, "/posts", http.StatusSeeOther)
	}
}

//--------------------------------------------------------------------------------------|

func LogoutHandler(db *sql.DB, sm *sessions.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		cookie, err := r.Cookie("session_id")
		if err == nil {
			if err := sm.DeleteSession(r.Context(), cookie.Value); err != nil {
				log.Printf("Failed to delete session: %v", err)
			}
		}

		clearAuthCookie(w)
		http.Redirect(w, r, "/posts", http.StatusSeeOther)
	}
}

//--------------------------------------------------------------------------------------|

func setAuthCookie(w http.ResponseWriter, sessionID string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   utils.Getenv("FORCE_HTTPS", "false") == "true",
		SameSite: http.SameSiteStrictMode,
		Expires:  expiresAt,
	})
}

func clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   utils.Getenv("FORCE_HTTPS", "false") == "true",
		SameSite: http.SameSiteStrictMode,
	})
}
