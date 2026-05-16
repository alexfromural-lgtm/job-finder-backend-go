// Package utils provides shared helper functions for the application.
package utils

import (
	"net/http"
	"time"
)

// SetAccessTokenCookie writes the accessToken as an HttpOnly cookie.
// Mirrors Node.js setAccessTokenCookie() from src/utils/auth.utils.ts.
//
// Security properties:
//   - HttpOnly: true  — invisible to JavaScript, prevents XSS token theft
//   - Secure: true in production — HTTPS only
//   - SameSite: Lax  — allows top-level GET navigations (e.g. OAuth callbacks,
//     email magic-links) while still blocking cross-site POST/PUT/DELETE,
//     which covers the primary CSRF attack surface
func SetAccessTokenCookie(w http.ResponseWriter, token string, expiry time.Duration, isProduction bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "accessToken",
		Value:    token,
		HttpOnly: true,
		Secure:   isProduction,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(expiry.Seconds()),
		Path:     "/",
	})
}

// SetRefreshTokenCookie writes the refreshToken as an HttpOnly cookie.
// Mirrors Node.js setRefreshTokenCookie() from src/utils/auth.utils.ts.
//
// Security properties are identical to SetAccessTokenCookie; the only
// difference is the longer lifetime (7 days) and the cookie name.
func SetRefreshTokenCookie(w http.ResponseWriter, token string, expiry time.Duration, isProduction bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "refreshToken",
		Value:    token,
		HttpOnly: true,
		Secure:   isProduction,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(expiry.Seconds()),
		Path:     "/",
	})
}

// ClearAuthCookies expires both auth cookies, effectively logging the user out.
// Mirrors the Node.js logout controller's res.clearCookie() calls.
func ClearAuthCookies(w http.ResponseWriter) {
	for _, name := range []string{"accessToken", "refreshToken"} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			HttpOnly: true,
			Secure:   false, // intentionally permissive on clear — browser will delete either way
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
			Path:     "/",
		})
	}
}
