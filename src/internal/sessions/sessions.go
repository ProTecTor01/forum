package sessions

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"src/internal/errmsg"
	"src/internal/models"
	"time"
)

//--------------------------------------------------------------------------------------|

const DefaultSessionTTL = 24 * time.Hour

//--------------------------------------------------------------------------------------|

type DBSessionStorage struct {
	db *sql.DB
}

//--------------------------------------------------------------------------------------|

type SessionManager struct {
	storage *DBSessionStorage
	ttl     time.Duration
}

func NewSessionManager(db *sql.DB, ttl time.Duration) *SessionManager {
	if ttl == 0 {
		ttl = DefaultSessionTTL
	}
	return &SessionManager{
		storage: &DBSessionStorage{db: db},
		ttl:     ttl,
	}
}

//--------------------------------------------------------------------------------------|

func (sm *SessionManager) CreateSession(ctx context.Context, userID int) (*models.Session, error) {
	return sm.storage.CreateSession(ctx, userID, sm.ttl)
}

//--------------------------------------------------------------------------------------|

func (sm *SessionManager) GetSession(ctx context.Context, id string) (*models.Session, error) {
	return sm.storage.GetSession(ctx, id)
}

//--------------------------------------------------------------------------------------|

func (sm *SessionManager) DeleteSession(ctx context.Context, id string) error {
	return sm.storage.DeleteSession(ctx, id)
}

//--------------------------------------------------------------------------------------|

func (s *DBSessionStorage) CreateSession(ctx context.Context, userID int, ttl time.Duration) (*models.Session, error) {
	idBytes := make([]byte, 32)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, errors.New("failed to generate session ID")
	}
	id := base64.URLEncoding.EncodeToString(idBytes)

	now := time.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(ttl).Truncate(time.Second)

	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, created_at, expires_at) 
         VALUES (?, ?, ?, ?)`,
		id, userID, now, expiresAt)
	if err != nil {
		return nil, err
	}

	return &models.Session{
		ID:        id,
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}, nil
}

//--------------------------------------------------------------------------------------|

func (s *DBSessionStorage) GetSession(ctx context.Context, id string) (*models.Session, error) {
	var session models.Session
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, created_at, expires_at 
         FROM sessions WHERE id = ? AND expires_at > ?`,
		id, time.Now()).Scan(
		&session.ID, &session.UserID, &session.CreatedAt, &session.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errmsg.ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

//--------------------------------------------------------------------------------------|

func (s *DBSessionStorage) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}
