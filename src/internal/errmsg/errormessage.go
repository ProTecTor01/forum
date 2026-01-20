package errmsg

import (
	"errors"
	"regexp"
	"strings"
)

//--------------------------------------------------------------------------------------|

var (
	ErrSessionNotFound       = errors.New("session not found")
	ErrPostNotFound          = errors.New("post not found")
	ErrCommentNotFound       = errors.New("comment not found")
	ErrParentCommentNotFound = errors.New("parent comment not found")
	ErrCommentDepthExceeded  = errors.New("comment depth limit exceeded")
	ErrUniqueConstraint      = errors.New("unique constraint violation")
	ErrUnauthorized          = errors.New("unauthorized")
)

var (
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrInvalidEmail        = errors.New("email must be a valid format and less than 255 characters")
	ErrUserAlreadyExists   = errors.New("email or username already exists")
	ErrUsernameLength      = errors.New("username must be between 3 and 16 characters")
	ErrUsernameFormat      = errors.New("username can only contain letters, numbers, hyphens, and underscores")
	ErrPasswordLength      = errors.New("password must be between 8 and 128 characters")
	ErrPasswordComplexity  = errors.New("password must contain at least one uppercase, one lowercase, one number, and one symbol")
	ErrInvalidCategoryName = errors.New("category name must be 1-50 characters")
	ErrInvalidPostTitle    = errors.New("post title must be 1-200 characters")
	ErrInvalidPostBody     = errors.New("post body must be 1-3000 characters")
	ErrInvalidCommentBody  = errors.New("comment body must be 1-1250 characters")
)

//--------------------------------------------------------------------------------------|

var (
	reEmail    = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,63}$`)
	reUsername = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

var (
	rePasswordHasLower  = regexp.MustCompile(`[a-z]`)
	rePasswordHasUpper  = regexp.MustCompile(`[A-Z]`)
	rePasswordHasDigit  = regexp.MustCompile(`\d`)
	rePasswordHasSymbol = regexp.MustCompile(`[^a-zA-Z\d\s]`)
)

//--------------------------------------------------------------------------------------|

func ValidateEmail(email string) error {
	trimmedEmail := strings.TrimSpace(email)
	if len([]rune(trimmedEmail)) > 254 {
		return ErrInvalidEmail
	}
	if !reEmail.MatchString(trimmedEmail) {
		return ErrInvalidEmail
	}
	return nil
}

//--------------------------------------------------------------------------------------|

func ValidateUsername(username string) error {
	trimmedUsername := strings.TrimSpace(username)
	if len([]rune(trimmedUsername)) < 3 || len([]rune(trimmedUsername)) > 16 {
		return ErrUsernameLength
	}
	if !reUsername.MatchString(trimmedUsername) {
		return ErrUsernameFormat
	}
	return nil
}

//--------------------------------------------------------------------------------------|

func ValidatePassword(password string) error {
	const minLen = 8
	const maxLen = 128

	passwordRunes := []rune(password)

	if len(passwordRunes) < minLen ||
		len(passwordRunes) > maxLen {
		return ErrPasswordLength
	}
	if !rePasswordHasLower.MatchString(password) {
		return ErrPasswordComplexity
	}
	if !rePasswordHasUpper.MatchString(password) {
		return ErrPasswordComplexity
	}
	if !rePasswordHasDigit.MatchString(password) {
		return ErrPasswordComplexity
	}
	if !rePasswordHasSymbol.MatchString(password) {
		return ErrPasswordComplexity
	}
	return nil
}

//--------------------------------------------------------------------------------------|

func ValidateCategoryName(name string) error {
	trimmedCatName := strings.TrimSpace(name)
	if len([]rune(trimmedCatName)) < 1 || len([]rune(trimmedCatName)) > 50 {
		return ErrInvalidCategoryName
	}
	return nil
}

//--------------------------------------------------------------------------------------|

func ValidatePostTitle(title string) error {
	trimmedTitle := strings.TrimSpace(title)
	if len([]rune(trimmedTitle)) < 1 || len([]rune(trimmedTitle)) > 200 {
		return ErrInvalidPostTitle
	}
	return nil
}

//--------------------------------------------------------------------------------------|

func ValidatePostBody(body string) error {
	trimmedBody := strings.TrimSpace(body)
	if len([]rune(trimmedBody)) < 1 || len([]rune(trimmedBody)) > 3000 {
		return ErrInvalidPostBody
	}
	return nil
}

//--------------------------------------------------------------------------------------|

func ValidateCommentBody(body string) error {
	trimmedBody := strings.TrimSpace(body)
	if len([]rune(trimmedBody)) < 1 || len([]rune(trimmedBody)) > 1250 {
		return ErrInvalidCommentBody
	}
	return nil
}
