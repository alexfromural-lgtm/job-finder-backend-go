// ============================================================
// FILE: internal/utils/cookie.go
// ROLE: Set and clear HttpOnly JWT cookies on the HTTP response.
//
// 📚 WHAT IS AN HTTPONLY COOKIE?
//   An HttpOnly cookie cannot be read or modified by JavaScript
//   (document.cookie), which prevents XSS attacks from stealing tokens.
//   The browser automatically attaches it to every request to the same origin.
//
// 📝 COMPARISON with Node.js (src/utils/auth.utils.ts):
//   res.cookie("accessToken", token, {
//     httpOnly: true, secure: isProd, sameSite: "lax",
//     maxAge: expiryMs
//   })
//   → SetAccessTokenCookie(w, token, expiry, isProduction)
//
// 📝 COOKIE SECURITY PROPERTIES:
//   HttpOnly: true  → JS can't read it (blocks XSS token theft)
//   Secure: true    → HTTPS only (set only in production)
//   SameSite: Lax   → sent on same-site requests + top-level GET navigations
//                     (blocks CSRF POST/PUT/DELETE from other origins)
//   MaxAge          → browser deletes cookie after this many seconds
//   Path: "/"       → cookie sent for all routes, not just /api/auth
// ============================================================
package utils

import (
	"net/http"
	"time"
)

// SetAccessTokenCookie writes the JWT access token as an HttpOnly cookie.
// Called after successful login or signup.
//
// 📝 EXAMPLE:
//   utils.SetAccessTokenCookie(w, "eyJ...", 15*time.Minute, false)
//   // Sets cookie: accessToken=eyJ...; HttpOnly; Path=/; MaxAge=900
func SetAccessTokenCookie(w http.ResponseWriter, token string, expiry time.Duration, isProduction bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "accessToken",         // cookie name the browser uses
		Value:    token,                 // the JWT string
		HttpOnly: true,                  // invisible to document.cookie in JS
		Secure:   isProduction,          // HTTPS only in production; allow HTTP in dev
		SameSite: http.SameSiteLaxMode,  // Lax: blocks cross-site POST, allows GET
		// MaxAge is in seconds. int(expiry.Seconds()) converts time.Duration → int.
		// e.g. 15 * time.Minute = 900 seconds
		MaxAge: int(expiry.Seconds()),
		Path:   "/",                     // send cookie on all routes
	})
}

// SetRefreshTokenCookie writes the JWT refresh token as an HttpOnly cookie.
// The refresh token has a longer life (default 7 days) than the access token.
//
// 📝 WHY A SEPARATE REFRESH TOKEN?
//   The access token is short-lived (15min) to limit damage if it leaks.
//   The refresh token is long-lived (7 days) so users don't have to log in
//   every 15 minutes. When the access token expires, the client calls
//   POST /api/auth/refresh, which issues a new access token.
//
// 📝 EXAMPLE:
//   utils.SetRefreshTokenCookie(w, "eyJ...", 7*24*time.Hour, true)
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
// Setting MaxAge to -1 tells the browser to delete the cookie immediately.
//
// 📝 HOW "DELETING" A COOKIE WORKS:
//   HTTP has no "delete cookie" command. Instead, we overwrite the cookie
//   with the same name but MaxAge=-1, which tells the browser to discard it.
//
// 📝 EXAMPLE:
//   // In the Logout handler:
//   utils.ClearAuthCookies(w)
//   writeJSON(w, 200, MessageResponse{Message: "Logged out"})
func ClearAuthCookies(w http.ResponseWriter) {
	// range over a string slice — iterates: name = "accessToken", then "refreshToken"
	for _, name := range []string{"accessToken", "refreshToken"} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",            // empty value
			HttpOnly: true,
			Secure:   false,         // intentionally permissive on clear: browser deletes regardless
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,            // -1 means "delete this cookie now"
			Path:     "/",
		})
	}
}
