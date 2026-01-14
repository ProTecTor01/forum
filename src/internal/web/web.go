package web

import (
	"errors"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"src/internal/utils"
	"strconv"
)

//--------------------------------------------------------------------------------------|

const (
	defaultTemplateDir = "./assets/templates/"
	defaultStaticDir   = "./assets/static/"

	DefaultLimit = 10
	MaxLimit     = 100
)

//--------------------------------------------------------------------------------------|

type TemplateStore struct {
	templates map[string]*template.Template
}

//--------------------------------------------------------------------------------------|

func NewTemplateStore() (*TemplateStore, error) {
	dir := utils.Getenv("TEMPLATE_DIR", defaultTemplateDir)
	layout := filepath.Join(dir, "layout.html")

	tmpls := map[string][]string{
		"post.html":            {layout, filepath.Join(dir, "post.html"), filepath.Join(dir, "like_dislike.html"), filepath.Join(dir, "comments.html")},
		"posts.html":           {layout, filepath.Join(dir, "posts.html"), filepath.Join(dir, "like_dislike.html")},
		"login.html":           {layout, filepath.Join(dir, "login.html")},
		"register.html":        {layout, filepath.Join(dir, "register.html")},
		"categories.html":      {layout, filepath.Join(dir, "categories.html")},
		"create_post.html":     {layout, filepath.Join(dir, "create_post.html")},
		"category_create.html": {layout, filepath.Join(dir, "category_create.html")},
		"error.html":           {layout, filepath.Join(dir, "error.html")},
	}

	parsed := make(map[string]*template.Template)
	for name, files := range tmpls {
		t, err := template.ParseFiles(files...)
		if err != nil {
			return nil, err
		}
		parsed[name] = t
	}

	return &TemplateStore{templates: parsed}, nil
}

//--------------------------------------------------------------------------------------|

func (ts *TemplateStore) RenderTemplate(w http.ResponseWriter, name string, data any) error {
	t, ok := ts.templates[name]
	if !ok {
		log.Printf("[error] Template '%s' not found in store", name)
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return errors.New("template not found in store")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	err := t.ExecuteTemplate(w, "layout", data)
	if err != nil {
		log.Printf("[template error] executing template '%s': %v", name, err)
	}
	return err
}

//--------------------------------------------------------------------------------------|

func FileServer() http.Handler {
	return http.FileServer(http.Dir(utils.Getenv("STATIC_DIR", defaultStaticDir)))
}

//--------------------------------------------------------------------------------------|

func InternalServerError(w http.ResponseWriter, r *http.Request, ts *TemplateStore, err error, userID int) {
	log.Printf("[server error] %s %s: %v", r.Method, r.URL.Path, err)
	w.WriteHeader(http.StatusInternalServerError)
	ts.RenderTemplate(w, "error.html", map[string]any{
		"UserID":      userID,
		"Error":       "500 - Internal Server Error",
		"Description": "We're sorry, but our server encountered an **internal error**.\n We've logged the issue and our team is looking into it.",
	})
}

//--------------------------------------------------------------------------------------|

func NotFound(w http.ResponseWriter, r *http.Request, ts *TemplateStore, userID int) {
	log.Printf("[client error] %s %s: 404 Not Found", r.Method, r.URL.Path)
	w.WriteHeader(http.StatusNotFound)
	ts.RenderTemplate(w, "error.html", map[string]any{
		"UserID":      userID,
		"Error":       "404 - Not Found",
		"Description": "The page you are looking for does not exist.",
	})
}

//--------------------------------------------------------------------------------------|

func GetLimitOffset(r *http.Request) (int, int) {
	limitStr := r.URL.Query().Get("limit")
	limit, err := strconv.Atoi(limitStr)

	if err != nil || limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	offsetStr := r.URL.Query().Get("offset")
	offset, err := strconv.Atoi(offsetStr)

	if err != nil || offset < 0 {
		offset = 0
	}

	return limit, offset
}
