package utils

import (
	"context"
	"net/http"
	"os"
	"src/internal/sessions"
	"strconv"
)

//--------------------------------------------------------------------------------------|

func Getenv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

//--------------------------------------------------------------------------------------|

func GetIntEnv(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

//--------------------------------------------------------------------------------------|

func GetUserID(ctx context.Context, r *http.Request, sm *sessions.SessionManager) int {
	cookie, err := r.Cookie("session_id")
	if err != nil {
		return 0
	}

	session, err := sm.GetSession(ctx, cookie.Value)
	if err != nil {
		return 0
	}
	return session.UserID
}
