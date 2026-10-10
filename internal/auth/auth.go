package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Role string

const (
	RoleAdmin      Role = "admin"
	RoleAccountant Role = "accountant"
	RoleViewer     Role = "viewer"
)

type User struct {
	ID           int
	Username     string
	PasswordHash string
	Role         Role
}

type Session struct {
	ID        int
	UserID    int
	TokenHash string
	ExpiresAt time.Time
}

// HashPassword hashes a plain text password using bcrypt.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

// CheckPasswordHash compares a bcrypt hashed password with its possible plaintext equivalent.
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateSessionToken generates a cryptographically random session token and its SHA256 hash.
func GenerateSessionToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token := base64.URLEncoding.EncodeToString(b)
	hash := HashSessionToken(token)
	return token, hash, nil
}

// HashSessionToken computes the SHA256 hash of a session token for secure database storage.
func HashSessionToken(token string) string {
	h := sha256.New()
	h.Write([]byte(token))
	return fmt.Sprintf("%x", h.Sum(nil))
}

var dummyBcryptHash = "$2a$14$F39/X95qE0b1Z0T0U/V6Y.1.2.3.4.5.6.7.8.9.0.1.2.3.4.5.6.7.8"

// Authenticate verifies the username and password against the database, returning the user if valid.
func Authenticate(db *sql.DB, username, password string) (*User, error) {
	var user User
	err := db.QueryRow("SELECT id, username, password_hash, role FROM users WHERE username = ?", username).
		Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role)
	if err != nil {
		if err == sql.ErrNoRows {
			// Perform a dummy hash comparison to prevent timing attacks that reveal if a user exists
			_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(password))
			return nil, fmt.Errorf("invalid credentials")
		}
		return nil, err
	}

	if !CheckPasswordHash(password, user.PasswordHash) {
		return nil, fmt.Errorf("invalid credentials")
	}

	return &user, nil
}

// CreateSession creates a new session in the database.
func CreateSession(db *sql.DB, userID int, duration time.Duration) (string, error) {
	token, hash, err := GenerateSessionToken()
	if err != nil {
		return "", err
	}

	expiresAt := time.Now().Add(duration)
	_, err = db.Exec("INSERT INTO sessions (user_id, token_hash, expires_at) VALUES (?, ?, ?)", userID, hash, expiresAt)
	if err != nil {
		return "", err
	}

	return token, nil
}

// ValidateSession checks if a session token is valid and not expired, returning the associated user.
func ValidateSession(db *sql.DB, token string) (*User, error) {
	hash := HashSessionToken(token)

	var user User
	var expiresAt time.Time
	err := db.QueryRow(`
		SELECT u.id, u.username, u.role, s.expires_at
		FROM sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.token_hash = ?`, hash).
		Scan(&user.ID, &user.Username, &user.Role, &expiresAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid session")
		}
		return nil, err
	}

	if time.Now().After(expiresAt) {
		// Session expired
		DeleteSession(db, token)
		return nil, fmt.Errorf("session expired")
	}

	return &user, nil
}

// DeleteSession removes a session from the database.
func DeleteSession(db *sql.DB, token string) error {
	hash := HashSessionToken(token)
	_, err := db.Exec("DELETE FROM sessions WHERE token_hash = ?", hash)
	return err
}

// GenerateCSRFToken generates a CSRF token.
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// VerifyCSRFToken securely compares two CSRF tokens in constant time.
func VerifyCSRFToken(token1, token2 string) bool {
	// If tokens are empty, they are invalid.
	if len(token1) == 0 || len(token1) != len(token2) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token1), []byte(token2)) == 1
}
