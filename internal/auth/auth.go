package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	CookieName    = "spectra_session"
	SessionMaxAge = 30 * 24 * time.Hour // 30 days
)

// AuthManager handles stateless HMAC session verification and middleware
type AuthManager struct {
	username string
	password string
	secret   []byte
}

// NewAuthManager initializes auth state
func NewAuthManager(username, password string) *AuthManager {
	if username == "" && password == "" {
		return &AuthManager{}
	}
	// Derive tamper-proof HMAC secret from credentials
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("spectra-secure-salt:%s:%s", username, password)))
	secret := h.Sum(nil)

	return &AuthManager{
		username: username,
		password: password,
		secret:   secret,
	}
}

// IsEnabled returns true if credentials are configured
func (a *AuthManager) IsEnabled() bool {
	return a != nil && a.username != "" && a.password != ""
}

// ValidateCredentials checks user and pass
func (a *AuthManager) ValidateCredentials(user, pass string) bool {
	if !a.IsEnabled() {
		return true
	}
	uMatch := hmac.Equal([]byte(user), []byte(a.username))
	pMatch := hmac.Equal([]byte(pass), []byte(a.password))
	return uMatch && pMatch
}

// GenerateSessionToken creates a signed timestamp token
func (a *AuthManager) GenerateSessionToken() string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(ts))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s.%s", ts, sig)
}

// VerifySessionToken verifies token validity and expiration
func (a *AuthManager) VerifySessionToken(token string) bool {
	if !a.IsEnabled() {
		return true
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	tsStr, sigHex := parts[0], parts[1]

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return false
	}

	// Check expiration (30 days)
	tokenTime := time.Unix(ts, 0)
	if time.Since(tokenTime) > SessionMaxAge || time.Until(tokenTime) > 10*time.Minute {
		return false
	}

	// Verify HMAC signature
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(tsStr))
	expectedSig := mac.Sum(nil)

	actualSig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	return hmac.Equal(expectedSig, actualSig)
}

// Middleware protects application routes
func (a *AuthManager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.IsEnabled() {
			next.ServeHTTP(w, r)
			return
		}

		path := r.URL.Path

		// Whitelisted unauthenticated endpoints
		if path == "/health" || path == "/login" || path == "/api/login" ||
			path == "/style.css" || path == "/login.html" {
			next.ServeHTTP(w, r)
			return
		}

		// 1. Check HTTP Basic Auth (for automated scripts or curl)
		if u, p, ok := r.BasicAuth(); ok {
			if a.ValidateCredentials(u, p) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// 2. Check Cookie Session
		if cookie, err := r.Cookie(CookieName); err == nil {
			if a.VerifySessionToken(cookie.Value) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// If unauthorized:
		// For API requests, return JSON 401
		if strings.HasPrefix(path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
			return
		}

		// For HTML page requests, redirect to /login
		http.Redirect(w, r, "/login", http.StatusFound)
	})
}
