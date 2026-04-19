package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// role constant
const (
	RoleAdmin  = "admin"  // all manage permission
	RoleEditor = "editor" // instance control, file edit, etc. (create/delete)
	RoleViewer = "viewer" // read-only (dashboard, console view)
)

// ValidRoles maps valid role names.
var ValidRoles = map[string]bool{
	RoleAdmin:  true,
	RoleEditor: true,
	RoleViewer: true,
}

// User represents an authenticated user.
type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"` // JSON extract no
	Role         string    `json:"role"`
	Approved     bool      `json:"approved"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateUser inserts a new user.
func (d *DB) CreateUser(u *User) error {
	_, err := d.Exec(`
		INSERT INTO users (username, password_hash, role, approved)
		VALUES (?, ?, ?, ?)
	`, u.Username, u.PasswordHash, u.Role, u.Approved)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// GetUserByUsername retrieves a user by username.
func (d *DB) GetUserByUsername(username string) (*User, error) {
	u := &User{}
	var approved int
	err := d.QueryRow(`
		SELECT id, username, password_hash, role, approved, created_at, updated_at
		FROM users WHERE username = ?
	`, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &approved, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get user %s: %w", username, err)
	}
	u.Approved = approved != 0
	return u, nil
}

// GetUserByID retrieves a user by ID.
func (d *DB) GetUserByID(id int) (*User, error) {
	u := &User{}
	var approved int
	err := d.QueryRow(`
		SELECT id, username, password_hash, role, approved, created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &approved, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get user %d: %w", id, err)
	}
	u.Approved = approved != 0
	return u, nil
}

// ListUsers returns all users.
func (d *DB) ListUsers() ([]*User, error) {
	rows, err := d.Query(`
		SELECT id, username, password_hash, role, approved, created_at, updated_at
		FROM users ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		var approved int
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &approved, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		u.Approved = approved != 0
		users = append(users, u)
	}
	return users, rows.Err()
}

// UpdateUserPassword changes the password.
func (d *DB) UpdateUserPassword(id int, passwordHash string) error {
	_, err := d.Exec(`
		UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, passwordHash, id)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// UpdateUsername changes the username.
func (d *DB) UpdateUsername(id int, username string) error {
	_, err := d.Exec(`
		UPDATE users SET username = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, username, id)
	if err != nil {
		return fmt.Errorf("update username: %w", err)
	}
	return nil
}

// UpdateUserRole changes the role.
func (d *DB) UpdateUserRole(id int, role string) error {
	_, err := d.Exec(`
		UPDATE users SET role = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, role, id)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	return nil
}

// ApproveUser sets approved = 1.
func (d *DB) ApproveUser(id int) error {
	_, err := d.Exec(`
		UPDATE users SET approved = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("approve user: %w", err)
	}
	return nil
}

// RejectUser deletes a pending user.
func (d *DB) RejectUser(id int) error {
	_, err := d.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("reject user: %w", err)
	}
	return nil
}

// DeleteUser deletes a user by ID.
func (d *DB) DeleteUser(id int) error {
	_, err := d.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

// CountUsers returns the total number of users.
func (d *DB) CountUsers() (int, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

// EnsureAdminUser creates the default admin user if no users exist.
// Returns the generated password (empty string if admin already exists).
func (d *DB) EnsureAdminUser(log interface{ Info(string, ...any) }) string {
	count, err := d.CountUsers()
	if err != nil || count > 0 {
		return ""
	}

	// random password create (16 hex)
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		// fallback
		randomBytes = []byte("admin1234")
	}
	password := hex.EncodeToString(randomBytes)

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return ""
	}

	u := &User{
		Username:     "admin",
		PasswordHash: string(hash),
		Role:         RoleAdmin,
		Approved:     true,
	}
	if err := d.CreateUser(u); err != nil {
		return ""
	}

	log.Info("default admin account created",
		"username", "admin",
		"password", password,
	)

	return password
}

// HashPassword hashes a plaintext password using bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword compares a plaintext password with a hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
