package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"src/internal/auth"
	"src/internal/categories"
	"src/internal/comments"
	"src/internal/errmsg"
	"src/internal/models"
	"src/internal/posts"
	"src/internal/sessions"
	"src/internal/utils"
	"src/internal/ws"

	"github.com/gorilla/websocket"
)

//--------------------------------------------------------------------------------------|

type API struct {
	db  *sql.DB
	sm  *sessions.SessionManager
	hub *ws.Hub
}

func NewAPI(db *sql.DB, sm *sessions.SessionManager, hub *ws.Hub) *API {
	return &API{db: db, sm: sm, hub: hub}
}

//--------------------------------------------------------------------------------------|

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

//--------------------------------------------------------------------------------------|

func (a *API) Index(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	staticDir := utils.Getenv("STATIC_DIR", "./assets/static/")
	path := filepath.Join(staticDir, "index.html")
	http.ServeFile(w, r, path)
}

//--------------------------------------------------------------------------------------|

func (a *API) WS(w http.ResponseWriter, r *http.Request) {
	userID := utils.GetUserID(r.Context(), r, a.sm)
	if userID == 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := ws.NewClient(userID, a.hub, conn)
	a.hub.Register(client)

	presence := map[string]any{
		"users": a.hub.OnlineUserIDs(),
	}
	client.Send(ws.EncodeEvent("presence", presence))
	a.hub.Broadcast(ws.Event{Type: "presence", Data: presence})

	go client.WritePump()
	client.ReadPump(func(msg []byte) {
		a.handleWSMessage(userID, msg)
	}, func() {
		presenceUpdate := map[string]any{"users": a.hub.OnlineUserIDs()}
		a.hub.Broadcast(ws.Event{Type: "presence", Data: presenceUpdate})
	})
}

//--------------------------------------------------------------------------------------|

func (a *API) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload struct {
		Email     string `json:"email"`
		Username  string `json:"username"`
		Password  string `json:"password"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Age       int    `json:"age"`
		Gender    string `json:"gender"`
	}
	if err := readJSON(r, &payload); err != nil {
		badRequest(w, err.Error())
		return
	}

	if err := errmsg.ValidateEmail(payload.Email); err != nil {
		badRequest(w, err.Error())
		return
	}
	if err := errmsg.ValidateUsername(payload.Username); err != nil {
		badRequest(w, err.Error())
		return
	}
	if err := errmsg.ValidatePassword(payload.Password); err != nil {
		badRequest(w, err.Error())
		return
	}
	if err := errmsg.ValidateFirstName(payload.FirstName); err != nil {
		badRequest(w, err.Error())
		return
	}
	if err := errmsg.ValidateLastName(payload.LastName); err != nil {
		badRequest(w, err.Error())
		return
	}
	if err := errmsg.ValidateAge(payload.Age); err != nil {
		badRequest(w, err.Error())
		return
	}
	if err := errmsg.ValidateGender(payload.Gender); err != nil {
		badRequest(w, err.Error())
		return
	}

	repo := auth.NewDBRepo(a.db)
	user, err := repo.Register(r.Context(), payload.Email, payload.Username, payload.FirstName, payload.LastName, payload.Age, payload.Gender, payload.Password)
	if err != nil {
		if errors.Is(err, errmsg.ErrUserAlreadyExists) {
			badRequest(w, "Email or username is already taken")
			return
		}
		serverError(w, err)
		return
	}

	session, err := a.sm.CreateSession(r.Context(), user.ID)
	if err != nil {
		serverError(w, err)
		return
	}

	a.hub.ForceLogoutUser(user.ID)
	setAuthCookie(w, session.ID, session.ExpiresAt)
	writeJSON(w, http.StatusCreated, user)
}

//--------------------------------------------------------------------------------------|

func (a *API) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload struct {
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}
	if err := readJSON(r, &payload); err != nil {
		badRequest(w, err.Error())
		return
	}

	if strings.TrimSpace(payload.Identifier) == "" || strings.TrimSpace(payload.Password) == "" {
		badRequest(w, "Identifier and password are required")
		return
	}

	repo := auth.NewDBRepo(a.db)
	user, err := repo.Login(r.Context(), payload.Identifier, payload.Password)
	if err != nil {
		if errors.Is(err, errmsg.ErrInvalidCredentials) {
			badRequest(w, "Invalid credentials")
			return
		}
		serverError(w, err)
		return
	}

	session, err := a.sm.CreateSession(r.Context(), user.ID)
	if err != nil {
		serverError(w, err)
		return
	}

	a.hub.ForceLogoutUser(user.ID)
	setAuthCookie(w, session.ID, session.ExpiresAt)
	writeJSON(w, http.StatusOK, user)
}

//--------------------------------------------------------------------------------------|

func (a *API) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	cookie, err := r.Cookie("session_id")
	if err == nil {
		_ = a.sm.DeleteSession(r.Context(), cookie.Value)
	}
	clearAuthCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

//--------------------------------------------------------------------------------------|

func (a *API) Me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	userID := utils.GetUserID(r.Context(), r, a.sm)
	if userID == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"user": nil})
		return
	}

	repo := auth.NewDBRepo(a.db)
	user, err := repo.GetUserByID(r.Context(), userID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

//--------------------------------------------------------------------------------------|

func (a *API) Posts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		userID := utils.GetUserID(r.Context(), r, a.sm)
		limit, offset := getLimitOffset(r)
		categoryID, _ := strconv.Atoi(r.URL.Query().Get("category"))
		filter := r.URL.Query().Get("filter")

		repo := posts.NewDBRepo(a.db)
		items, err := repo.GetPosts(r.Context(), userID, categoryID, filter, limit, offset)
		if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		userID := utils.GetUserID(r.Context(), r, a.sm)
		if userID == 0 {
			unauthorized(w)
			return
		}

		var payload struct {
			Title       string `json:"title"`
			Body        string `json:"body"`
			CategoryIDs []int  `json:"category_ids"`
		}
		if err := readJSON(r, &payload); err != nil {
			badRequest(w, err.Error())
			return
		}

		if len(payload.CategoryIDs) == 0 {
			badRequest(w, "Please select at least one category")
			return
		}
		if err := errmsg.ValidatePostTitle(payload.Title); err != nil {
			badRequest(w, err.Error())
			return
		}
		if err := errmsg.ValidatePostBody(payload.Body); err != nil {
			badRequest(w, err.Error())
			return
		}

		repo := posts.NewDBRepo(a.db)
		created, err := repo.CreatePost(r.Context(), userID, payload.Title, payload.Body, payload.CategoryIDs)
		if err != nil {
			serverError(w, err)
			return
		}

		post, err := repo.GetPost(r.Context(), created.ID, userID)
		if err != nil {
			serverError(w, err)
			return
		}

		a.hub.Broadcast(ws.Event{Type: "post_created", Data: post})
		writeJSON(w, http.StatusCreated, post)
	default:
		methodNotAllowed(w)
	}
}

//--------------------------------------------------------------------------------------|

func (a *API) PostByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/posts/")
	postID, err := strconv.Atoi(idStr)
	if err != nil {
		badRequest(w, "Invalid post ID")
		return
	}

	userID := utils.GetUserID(r.Context(), r, a.sm)
	repo := posts.NewDBRepo(a.db)
	post, err := repo.GetPost(r.Context(), postID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(w)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}

	commentsList, err := a.getComments(r.Context(), postID, userID)
	if err != nil {
		serverError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"post":     post,
		"comments": commentsList,
	})
}

//--------------------------------------------------------------------------------------|

func (a *API) Comments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	userID := utils.GetUserID(r.Context(), r, a.sm)
	if userID == 0 {
		unauthorized(w)
		return
	}

	var payload struct {
		PostID   int    `json:"post_id"`
		Body     string `json:"body"`
		ParentID *int   `json:"parent_id"`
	}
	if err := readJSON(r, &payload); err != nil {
		badRequest(w, err.Error())
		return
	}
	if err := errmsg.ValidateCommentBody(payload.Body); err != nil {
		badRequest(w, err.Error())
		return
	}

	var parentID sql.NullInt64
	if payload.ParentID != nil {
		parentID = sql.NullInt64{Int64: int64(*payload.ParentID), Valid: true}
	}

	repo := comments.NewDBRepo(a.db)
	commentID, err := repo.CreateComment(r.Context(), userID, payload.PostID, payload.Body, parentID)
	if err != nil {
		if errors.Is(err, errmsg.ErrCommentDepthExceeded) {
			badRequest(w, "Reply depth limit reached")
			return
		}
		if errors.Is(err, errmsg.ErrParentCommentNotFound) {
			badRequest(w, "Parent comment not found")
			return
		}
		if errors.Is(err, errmsg.ErrPostNotFound) {
			notFound(w)
			return
		}
		serverError(w, err)
		return
	}

	comment, err := a.getCommentByID(r.Context(), commentID, userID)
	if err != nil {
		serverError(w, err)
		return
	}

	a.hub.Broadcast(ws.Event{Type: "comment_created", Data: comment})
	writeJSON(w, http.StatusCreated, comment)
}

//--------------------------------------------------------------------------------------|

func (a *API) Categories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		repo := categories.NewDBRepo(a.db)
		items, err := repo.GetCategories(r.Context())
		if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		userID := utils.GetUserID(r.Context(), r, a.sm)
		if userID == 0 {
			unauthorized(w)
			return
		}

		var payload struct {
			Name string `json:"name"`
		}
		if err := readJSON(r, &payload); err != nil {
			badRequest(w, err.Error())
			return
		}
		if err := errmsg.ValidateCategoryName(payload.Name); err != nil {
			badRequest(w, err.Error())
			return
		}

		repo := categories.NewDBRepo(a.db)
		cat, err := repo.CreateCategory(r.Context(), payload.Name)
		if err != nil {
			if errors.Is(err, errmsg.ErrUniqueConstraint) {
				badRequest(w, "A category with this name already exists")
				return
			}
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, cat)
	default:
		methodNotAllowed(w)
	}
}

//--------------------------------------------------------------------------------------|

func (a *API) Likes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	userID := utils.GetUserID(r.Context(), r, a.sm)
	if userID == 0 {
		unauthorized(w)
		return
	}

	var payload struct {
		TargetID   int    `json:"target_id"`
		TargetType string `json:"target_type"`
		Value      int    `json:"value"`
	}
	if err := readJSON(r, &payload); err != nil {
		badRequest(w, err.Error())
		return
	}

	if payload.Value != 1 && payload.Value != -1 && payload.Value != 0 {
		badRequest(w, "Invalid value")
		return
	}
	if payload.TargetType != "post" && payload.TargetType != "comment" {
		badRequest(w, "Invalid target type")
		return
	}

	var execErr error
	if payload.Value == 0 {
		_, execErr = a.db.ExecContext(r.Context(),
			`DELETE FROM likes WHERE user_id = ? AND target_id = ? AND target_type = ?`,
			userID, payload.TargetID, payload.TargetType)
	} else {
		_, execErr = a.db.ExecContext(r.Context(),
			`INSERT INTO likes (user_id, target_id, target_type, value)
             VALUES (?, ?, ?, ?)
             ON CONFLICT(user_id, target_id, target_type)
             DO UPDATE SET value = ?`,
			userID, payload.TargetID, payload.TargetType, payload.Value, payload.Value)
	}
	if execErr != nil {
		serverError(w, execErr)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

//--------------------------------------------------------------------------------------|

func (a *API) Users(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	rows, err := a.db.QueryContext(r.Context(), `SELECT id, username, first_name, last_name FROM users ORDER BY username ASC`)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()

	var users []map[string]any
	for rows.Next() {
		var id int
		var username, firstName, lastName string
		if err := rows.Scan(&id, &username, &firstName, &lastName); err != nil {
			serverError(w, err)
			return
		}
		users = append(users, map[string]any{
			"id":         id,
			"username":   username,
			"first_name": firstName,
			"last_name":  lastName,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": users})
}

//--------------------------------------------------------------------------------------|

func (a *API) ChatList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	userID := utils.GetUserID(r.Context(), r, a.sm)
	if userID == 0 {
		unauthorized(w)
		return
	}

	query := `
        SELECT u.id, u.username, u.first_name, u.last_name,
               MAX(m.created_at) as last_time
        FROM users u
        LEFT JOIN messages m
          ON (m.sender_id = ? AND m.receiver_id = u.id)
          OR (m.sender_id = u.id AND m.receiver_id = ?)
        WHERE u.id != ?
        GROUP BY u.id
        ORDER BY 
          CASE WHEN last_time IS NULL THEN 1 ELSE 0 END,
          last_time DESC,
          u.username ASC
    `

	rows, err := a.db.QueryContext(r.Context(), query, userID, userID, userID)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()

	onlineSet := make(map[int]bool)
	for _, id := range a.hub.OnlineUserIDs() {
		onlineSet[id] = true
	}

	var items []map[string]any
	for rows.Next() {
		var id int
		var username, firstName, lastName string
		var lastTime sql.NullString
		if err := rows.Scan(&id, &username, &firstName, &lastName, &lastTime); err != nil {
			serverError(w, err)
			return
		}
		lastTimeRFC3339 := ""
		if lastTime.Valid {
			lastTimeRFC3339 = normalizeDBTimeToRFC3339(lastTime.String)
		}

		items = append(items, map[string]any{
			"id":         id,
			"username":   username,
			"first_name": firstName,
			"last_name":  lastName,
			"last_time":  lastTimeRFC3339,
			"online":     onlineSet[id],
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

//--------------------------------------------------------------------------------------|

func (a *API) Messages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		userID := utils.GetUserID(r.Context(), r, a.sm)
		if userID == 0 {
			unauthorized(w)
			return
		}

		otherID, _ := strconv.Atoi(r.URL.Query().Get("user_id"))
		if otherID == 0 {
			badRequest(w, "user_id is required")
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 50 {
			limit = 10
		}
		beforeID, _ := strconv.Atoi(r.URL.Query().Get("before_id"))

		messages, err := a.getMessages(r.Context(), userID, otherID, beforeID, limit)
		if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": messages})
	case http.MethodPost:
		userID := utils.GetUserID(r.Context(), r, a.sm)
		if userID == 0 {
			unauthorized(w)
			return
		}
		var payload struct {
			ToUserID int    `json:"to_user_id"`
			Body     string `json:"body"`
		}
		if err := readJSON(r, &payload); err != nil {
			badRequest(w, err.Error())
			return
		}
		if payload.ToUserID == 0 {
			badRequest(w, "to_user_id is required")
			return
		}
		if err := errmsg.ValidateMessageBody(payload.Body); err != nil {
			badRequest(w, err.Error())
			return
		}
		msg, err := a.insertMessage(r.Context(), userID, payload.ToUserID, payload.Body)
		if err != nil {
			serverError(w, err)
			return
		}
		a.hub.SendToUser(payload.ToUserID, ws.Event{Type: "pm_message", Data: msg})
		a.hub.SendToUser(userID, ws.Event{Type: "pm_message", Data: msg})
		writeJSON(w, http.StatusCreated, msg)
	default:
		methodNotAllowed(w)
	}
}

//--------------------------------------------------------------------------------------|

func (a *API) handleWSMessage(userID int, raw []byte) {
	var envelope struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return
	}

	switch envelope.Type {
	case "send_message":
		var payload struct {
			ToUserID int    `json:"to_user_id"`
			Body     string `json:"body"`
		}
		if err := json.Unmarshal(envelope.Data, &payload); err != nil {
			return
		}
		if payload.ToUserID == 0 {
			return
		}
		if err := errmsg.ValidateMessageBody(payload.Body); err != nil {
			return
		}
		msg, err := a.insertMessage(context.Background(), userID, payload.ToUserID, payload.Body)
		if err != nil {
			return
		}
		a.hub.SendToUser(payload.ToUserID, ws.Event{Type: "pm_message", Data: msg})
		a.hub.SendToUser(userID, ws.Event{Type: "pm_message", Data: msg})
	case "typing":
		var payload struct {
			ToUserID int  `json:"to_user_id"`
			Typing   bool `json:"typing"`
		}
		if err := json.Unmarshal(envelope.Data, &payload); err != nil {
			return
		}
		if payload.ToUserID == 0 {
			return
		}
		a.hub.SendToUser(payload.ToUserID, ws.Event{Type: "typing", Data: map[string]any{"from_user_id": userID, "typing": payload.Typing}})
	}
}

//--------------------------------------------------------------------------------------|

func (a *API) getComments(ctx context.Context, postID, userID int) ([]models.Comment, error) {
	rows, err := a.db.QueryContext(ctx,
		`SELECT c.id, c.user_id, c.post_id, u.username, c.body, c.created_at, c.parent_id, c.depth,
                COALESCE(SUM(CASE WHEN l.value = 1 THEN 1 ELSE 0 END), 0) AS likes,
                COALESCE(SUM(CASE WHEN l.value = -1 THEN 1 ELSE 0 END), 0) AS dislikes,
                COALESCE(SUM(CASE WHEN l.user_id = ? THEN l.value ELSE 0 END), 0) AS user_like
         FROM comments c
         JOIN users u ON c.user_id = u.id
         LEFT JOIN likes l ON l.target_id = c.id AND l.target_type = 'comment'
         WHERE c.post_id = ?
         GROUP BY c.id
         ORDER BY c.created_at ASC`, userID, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commentsList []models.Comment
	for rows.Next() {
		var c models.Comment
		var parentID sql.NullInt64
		if err := rows.Scan(&c.ID, &c.UserID, &c.PostID, &c.Username, &c.Body, &c.CreatedAt, &parentID, &c.Depth, &c.Likes, &c.Dislikes, &c.UserLike); err != nil {
			return nil, err
		}
		c.ViewerID = userID
		c.ParentID = parentID
		commentsList = append(commentsList, c)
	}

	organized := posts.OrganizeComments(commentsList, posts.MaxCommentDepth)
	return organized, nil
}

//--------------------------------------------------------------------------------------|

func (a *API) getCommentByID(ctx context.Context, commentID, userID int) (*models.Comment, error) {
	row := a.db.QueryRowContext(ctx,
		`SELECT c.id, c.user_id, c.post_id, u.username, c.body, c.created_at, c.parent_id, c.depth,
                COALESCE(SUM(CASE WHEN l.value = 1 THEN 1 ELSE 0 END), 0) AS likes,
                COALESCE(SUM(CASE WHEN l.value = -1 THEN 1 ELSE 0 END), 0) AS dislikes,
                COALESCE(SUM(CASE WHEN l.user_id = ? THEN l.value ELSE 0 END), 0) AS user_like
         FROM comments c
         JOIN users u ON c.user_id = u.id
         LEFT JOIN likes l ON l.target_id = c.id AND l.target_type = 'comment'
         WHERE c.id = ?
         GROUP BY c.id`, userID, commentID)

	var c models.Comment
	var parentID sql.NullInt64
	if err := row.Scan(&c.ID, &c.UserID, &c.PostID, &c.Username, &c.Body, &c.CreatedAt, &parentID, &c.Depth, &c.Likes, &c.Dislikes, &c.UserLike); err != nil {
		return nil, err
	}
	c.ViewerID = userID
	c.ParentID = parentID
	return &c, nil
}

//--------------------------------------------------------------------------------------|

func (a *API) insertMessage(ctx context.Context, senderID, receiverID int, body string) (*models.Message, error) {
	now := time.Now().UTC().Truncate(time.Second)
	result, err := a.db.ExecContext(ctx, `INSERT INTO messages (sender_id, receiver_id, body, created_at) VALUES (?, ?, ?, ?)`,
		senderID, receiverID, body, now)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return &models.Message{ID: int(id), SenderID: senderID, ReceiverID: receiverID, Body: body, CreatedAt: now}, nil
}

//--------------------------------------------------------------------------------------|

func (a *API) getMessages(ctx context.Context, userID, otherID, beforeID, limit int) ([]models.Message, error) {
	query := `
        SELECT id, sender_id, receiver_id, body, created_at
        FROM messages
        WHERE ((sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?))
    `
	args := []any{userID, otherID, otherID, userID}
	if beforeID > 0 {
		query += " AND id < ?"
		args = append(args, beforeID)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []models.Message
	for rows.Next() {
		var m models.Message
		if err := rows.Scan(&m.ID, &m.SenderID, &m.ReceiverID, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

//--------------------------------------------------------------------------------------|

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			log.Printf("[api] encode error: %v", err)
		}
	}
}

func getLimitOffset(r *http.Request) (int, int) {
	limitStr := r.URL.Query().Get("limit")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	offsetStr := r.URL.Query().Get("offset")
	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}
	return limit, offset
}

func normalizeDBTimeToRFC3339(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}

	return raw
}

func methodNotAllowed(w http.ResponseWriter) {
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
}

func unauthorized(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
}

func notFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("[api] error: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
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
