package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"src/internal/errmsg"
	"src/internal/models"

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

func (r *DBRepo) Register(ctx context.Context, email, username, firstName, lastName string, age int, gender, password string) (*models.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now().UTC().Truncate(time.Second)

	result, err := r.db.ExecContext(ctx,
		`INSERT INTO users (username, email, first_name, last_name, age, gender, password_hash, created_at, role)
         VALUES (LOWER(?), LOWER(?), ?, ?, ?, ?, ?, ?, 'user')`,
		username, email, firstName, lastName, age, strings.ToLower(gender), hash, now)

	if err != nil {
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
		FirstName:    firstName,
		LastName:     lastName,
		Age:          age,
		Gender:       strings.ToLower(gender),
		PasswordHash: string(hash),
		CreatedAt:    now,
		Role:         "user",
	}, nil
}

//--------------------------------------------------------------------------------------|

func (r *DBRepo) Login(ctx context.Context, identifier, password string) (*models.User, error) {
	var user models.User

	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, email, first_name, last_name, age, gender, password_hash, created_at, role
         FROM users WHERE email = LOWER(?) OR username = LOWER(?)`, identifier, identifier).Scan(
		&user.ID, &user.Username, &user.Email, &user.FirstName, &user.LastName, &user.Age, &user.Gender, &user.PasswordHash, &user.CreatedAt, &user.Role)

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

func (r *DBRepo) GetUserByID(ctx context.Context, userID int) (*models.User, error) {
	var user models.User
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, email, first_name, last_name, age, gender, password_hash, created_at, role
         FROM users WHERE id = ?`, userID).Scan(
		&user.ID, &user.Username, &user.Email, &user.FirstName, &user.LastName, &user.Age, &user.Gender, &user.PasswordHash, &user.CreatedAt, &user.Role)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
