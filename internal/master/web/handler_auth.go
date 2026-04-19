package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"craftstack/internal/master/store"
)

// ─────────────────────────────────────────────────────────────
// in-memory session store
// ─────────────────────────────────────────────────────────────

const (
	sessionCookieName = "craftstack_session"
	sessionMaxAge     = 24 * time.Hour
)

type session struct {
	UserID    int
	Username  string
	Role      string
	CreatedAt time.Time
}

type sessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*session
}

func newSessionStore() *sessionStore {
	return &sessionStore{
		sessions: make(map[string]*session),
	}
}

func (ss *sessionStore) Create(userID int, username, role string) string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)

	ss.mu.Lock()
	ss.sessions[token] = &session{
		UserID:    userID,
		Username:  username,
		Role:      role,
		CreatedAt: time.Now(),
	}
	ss.mu.Unlock()
	return token
}

func (ss *sessionStore) Get(token string) *session {
	ss.mu.RLock()
	s, ok := ss.sessions[token]
	ss.mu.RUnlock()
	if !ok {
		return nil
	}
	if time.Since(s.CreatedAt) > sessionMaxAge {
		ss.Delete(token)
		return nil
	}
	return s
}

func (ss *sessionStore) Delete(token string) {
	ss.mu.Lock()
	delete(ss.sessions, token)
	ss.mu.Unlock()
}

// DeleteUserSessions removes all sessions for a specific user.
func (ss *sessionStore) DeleteUserSessions(userID int) {
	ss.mu.Lock()
	for k, s := range ss.sessions {
		if s.UserID == userID {
			delete(ss.sessions, k)
		}
	}
	ss.mu.Unlock()
}

// ─────────────────────────────────────────────────────────────
// auth middleware
// ─────────────────────────────────────────────────────────────

// authMiddleware checks if the user is logged in and approved.
// Sets "user_id", "username", "user_role" in echo.Context.
func (s *Server) authMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		cookie, err := c.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			return c.Redirect(http.StatusFound, "/login")
		}

		sess := s.sessions.Get(cookie.Value)
		if sess == nil {
			// session only expired — cookie delete
			clearSessionCookie(c)
			return c.Redirect(http.StatusFound, "/login")
		}

		c.Set("user_id", sess.UserID)
		c.Set("username", sess.Username)
		c.Set("user_role", sess.Role)
		return next(c)
	}
}

// adminOnly middleware restricts access to admin role only.
func (s *Server) adminOnly(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		role, _ := c.Get("user_role").(string)
		if role != store.RoleAdmin {
			return c.JSON(http.StatusForbidden, map[string]string{
				"status": "error", "message": "admin permission is required",
			})
		}
		return next(c)
	}
}

// adminOrEditor middleware allows admin and editor roles.
func (s *Server) adminOrEditor(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		role, _ := c.Get("user_role").(string)
		if role != store.RoleAdmin && role != store.RoleEditor {
			return c.JSON(http.StatusForbidden, map[string]string{
				"status": "error", "message": "edit permission is required",
			})
		}
		return next(c)
	}
}

// getCurrentUser returns the user info from context.
func getCurrentUser(c echo.Context) (int, string, string) {
	userID, _ := c.Get("user_id").(int)
	username, _ := c.Get("username").(string)
	role, _ := c.Get("user_role").(string)
	return userID, username, role
}

// ─────────────────────────────────────────────────────────────
// login / logout / sign up handler
// ─────────────────────────────────────────────────────────────

func (s *Server) handleLoginPage(c echo.Context) error {
	// already login if present dashboard load
	cookie, err := c.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		if sess := s.sessions.Get(cookie.Value); sess != nil {
			return c.Redirect(http.StatusFound, "/")
		}
	}
	msg := c.QueryParam("msg")
	return c.HTML(http.StatusOK, buildLoginPageHTML(msg))
}

func (s *Server) handleRegisterPage(c echo.Context) error {
	msg := c.QueryParam("msg")
	return c.HTML(http.StatusOK, buildRegisterPageHTML(msg))
}

func (s *Server) apiLogin(c echo.Context) error {
	username := c.FormValue("username")
	password := c.FormValue("password")

	if username == "" || password == "" {
		return c.Redirect(http.StatusFound, "/login?msg=ID and+password+please enter")
	}

	user, err := s.db.GetUserByUsername(username)
	if err != nil {
		return c.Redirect(http.StatusFound, "/login?msg=ID+or+password+not valid+")
	}

	if !store.CheckPassword(user.PasswordHash, password) {
		return c.Redirect(http.StatusFound, "/login?msg=ID+or+password+not valid+")
	}

	if !user.Approved {
		return c.Redirect(http.StatusFound, "/login?msg=admin+signup+approve+pending+")
	}

	token := s.sessions.Create(user.ID, user.Username, user.Role)
	setSessionCookie(c, token)
	return c.Redirect(http.StatusFound, "/")
}

func (s *Server) apiLogout(c echo.Context) error {
	cookie, err := c.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		s.sessions.Delete(cookie.Value)
	}
	clearSessionCookie(c)
	return c.Redirect(http.StatusFound, "/login")
}

func (s *Server) apiRegister(c echo.Context) error {
	username := c.FormValue("username")
	password := c.FormValue("password")
	confirmPassword := c.FormValue("confirm_password")

	if username == "" || password == "" {
		return c.Redirect(http.StatusFound, "/register?msg=all+field+please enter")
	}

	if len(username) < 3 || len(username) > 32 {
		return c.Redirect(http.StatusFound, "/register?msg=ID+3~32+")
	}

	if len(password) < 4 {
		return c.Redirect(http.StatusFound, "/register?msg=password+4+must be at least+")
	}

	if password != confirmPassword {
		return c.Redirect(http.StatusFound, "/register?msg=password+mismatch+")
	}

	// duplicate check
	if _, err := s.db.GetUserByUsername(username); err == nil {
		return c.Redirect(http.StatusFound, "/register?msg=already+use+in progress+ID")
	}

	hash, err := store.HashPassword(password)
	if err != nil {
		return c.Redirect(http.StatusFound, "/register?msg=server+error+occurreddone")
	}

	user := &store.User{
		Username:     username,
		PasswordHash: hash,
		Role:         store.RoleViewer,
		Approved:     false, // admin approve needed
	}
	if err := s.db.CreateUser(user); err != nil {
		return c.Redirect(http.StatusFound, "/register?msg=sign up+failed")
	}

	s.log.Info("new user sign up new", "username", username)
	return c.Redirect(http.StatusFound, "/login?msg=sign up+completed.+admin+approve+waitplease.")
}

// ─────────────────────────────────────────────────────────────
// user management handler (admin only)
// ─────────────────────────────────────────────────────────────

func (s *Server) handleUsersPage(c echo.Context) error {
	users, err := s.db.ListUsers()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	_, username, role := getCurrentUser(c)
	data := map[string]interface{}{
		"Title":       "user management",
		"Users":       users,
		"CurrentUser": username,
		"CurrentRole": role,
	}
	return renderPage(c, "users", data)
}

func (s *Server) apiApproveUser(c echo.Context) error {
	idStr := c.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)

	if err := s.db.ApproveUser(id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status": "error", "message": fmt.Sprintf("approve failed: %v", err),
		})
	}
	// audit log
	approvedUser, _ := s.db.GetUserByID(id)
	approvedName := fmt.Sprintf("user#%d", id)
	if approvedUser != nil {
		approvedName = approvedUser.Username
	}
	s.audit(c, "approve", "user", fmt.Sprintf("%d", id), approvedName, "", "", "",
		fmt.Sprintf("approve user: %s", approvedName))
	s.log.Info("user sign up approve", "user_id", id)
	return c.JSON(http.StatusOK, map[string]string{"status": "success", "message": "approve"})
}

func (s *Server) apiRejectUser(c echo.Context) error {
	idStr := c.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)

	// delete before username query (audit log)
	rejectedUser, _ := s.db.GetUserByID(id)
	rejectedName := fmt.Sprintf("user#%d", id)
	if rejectedUser != nil {
		rejectedName = rejectedUser.Username
	}

	if err := s.db.RejectUser(id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status": "error", "message": fmt.Sprintf("reject failed: %v", err),
		})
	}
	s.sessions.DeleteUserSessions(id)
	// audit log
	s.audit(c, "reject", "user", fmt.Sprintf("%d", id), rejectedName, "", "", "",
		fmt.Sprintf("user reject/delete: %s", rejectedName))
	s.log.Info("user sign up reject/delete", "user_id", id)
	return c.JSON(http.StatusOK, map[string]string{"status": "success", "message": "deleted"})
}

func (s *Server) apiChangeRole(c echo.Context) error {
	idStr := c.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)

	var req struct {
		Role string `json:"role"`
	}
	if err := c.Bind(&req); err != nil || !store.ValidRoles[req.Role] {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"status": "error", "message": "valid role please select (admin, editor, viewer)",
		})
	}

	// change before role query (audit log)
	roleUser, _ := s.db.GetUserByID(id)
	oldRole := ""
	roleName := fmt.Sprintf("user#%d", id)
	if roleUser != nil {
		oldRole = roleUser.Role
		roleName = roleUser.Username
	}

	if err := s.db.UpdateUserRole(id, req.Role); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status": "error", "message": fmt.Sprintf("role change failed: %v", err),
		})
	}
	// existing session invalid (role apply  to relogin)
	s.sessions.DeleteUserSessions(id)
	// audit log
	s.audit(c, "role_change", "user", fmt.Sprintf("%d", id), roleName, "role", oldRole, req.Role,
		fmt.Sprintf("role change: %s %s → %s", roleName, oldRole, req.Role))
	s.log.Info("user role change", "user_id", id, "new_role", req.Role)
	return c.JSON(http.StatusOK, map[string]string{"status": "success", "message": "role change"})
}

// apiChangePassword allows a user to change their own password, or admin to change any.
func (s *Server) apiChangePassword(c echo.Context) error {
	idStr := c.Param("id")
	var targetID int
	fmt.Sscanf(idStr, "%d", &targetID)

	currentUserID, _, currentRole := getCurrentUser(c)

	// self or adminonly change available
	if currentUserID != targetID && currentRole != store.RoleAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{
			"status": "error", "message": "permission denied",
		})
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"status": "error", "message": "invalid request",
		})
	}

	if len(req.NewPassword) < 4 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"status": "error", "message": "new password 4 must be at least ",
		})
	}

	// self change when current password check (admin other user change when unnecessary)
	if currentUserID == targetID {
		user, err := s.db.GetUserByID(targetID)
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{
				"status": "error", "message": "user not found",
			})
		}
		if !store.CheckPassword(user.PasswordHash, req.CurrentPassword) {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"status": "error", "message": "current password not valid ",
			})
		}
	}

	hash, err := store.HashPassword(req.NewPassword)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status": "error", "message": "server error",
		})
	}

	if err := s.db.UpdateUserPassword(targetID, hash); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status": "error", "message": fmt.Sprintf("change password failed: %v", err),
		})
	}

	// invalidate existing session after password change (force relogin)
	s.sessions.DeleteUserSessions(targetID)
	// audit log (password value record no)
	pwUser, _ := s.db.GetUserByID(targetID)
	pwName := fmt.Sprintf("user#%d", targetID)
	if pwUser != nil {
		pwName = pwUser.Username
	}
	s.audit(c, "update", "user", fmt.Sprintf("%d", targetID), pwName, "password", "***", "***",
		fmt.Sprintf("change password: %s", pwName))
	s.log.Info("change password", "user_id", targetID)
	return c.JSON(http.StatusOK, map[string]string{"status": "success", "message": "password change"})
}

// apiChangeUsername allows a user to change their own username, or admin to change any.
func (s *Server) apiChangeUsername(c echo.Context) error {
	idStr := c.Param("id")
	var targetID int
	fmt.Sscanf(idStr, "%d", &targetID)

	currentUserID, _, currentRole := getCurrentUser(c)

	if currentUserID != targetID && currentRole != store.RoleAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{
			"status": "error", "message": "permission denied",
		})
	}

	var req struct {
		Username string `json:"username"`
	}
	if err := c.Bind(&req); err != nil || req.Username == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"status": "error", "message": "new ID please enter",
		})
	}

	if len(req.Username) < 3 || len(req.Username) > 32 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"status": "error", "message": "ID 3~32 ",
		})
	}

	// duplicate check
	if existing, err := s.db.GetUserByUsername(req.Username); err == nil && existing.ID != targetID {
		return c.JSON(http.StatusConflict, map[string]string{
			"status": "error", "message": "already use in progress ID",
		})
	}

	// change before user query (audit log)
	unUser, _ := s.db.GetUserByID(targetID)
	oldUsername := fmt.Sprintf("user#%d", targetID)
	if unUser != nil {
		oldUsername = unUser.Username
	}

	if err := s.db.UpdateUsername(targetID, req.Username); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status": "error", "message": fmt.Sprintf("ID change failed: %v", err),
		})
	}

	s.sessions.DeleteUserSessions(targetID)
	// audit log
	s.audit(c, "update", "user", fmt.Sprintf("%d", targetID), req.Username, "username", oldUsername, req.Username,
		fmt.Sprintf("ID change: %s → %s", oldUsername, req.Username))
	s.log.Info("ID change", "user_id", targetID, "new_username", req.Username)
	return c.JSON(http.StatusOK, map[string]string{"status": "success", "message": "ID change"})
}

// handleProfilePage shows the user's own profile for editing.
func (s *Server) handleProfilePage(c echo.Context) error {
	userID, username, role := getCurrentUser(c)
	user, err := s.db.GetUserByID(userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "user info query failed")
	}
	data := map[string]interface{}{
		"Title":       "my profile",
		"Profile":     user,
		"CurrentUser": username,
		"CurrentRole": role,
	}
	return renderPage(c, "profile", data)
}

// ─────────────────────────────────────────────────────────────
// cookie util
// ─────────────────────────────────────────────────────────────

func setSessionCookie(c echo.Context, token string) {
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionMaxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}
