// Package middleware contains HTTP middleware for the chi router.
package middleware

import (
	"net/http"
)

// SecureHeaders adds security-related HTTP response headers to every request.
// Mirrors the Node.js helmet middleware used in src/index.ts.
//
// Headers applied:
//   - X-Frame-Options: DENY — prevents clickjacking
//   - X-Content-Type-Options: nosniff — prevents MIME sniffing
//   - Referrer-Policy: strict-origin-when-cross-origin
//   - Permissions-Policy: disables camera, microphone, geolocation
//   - Strict-Transport-Security (HSTS): only in production (set by proxy in dev)
func SecureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// X-XSS-Protection is deprecated in modern browsers but kept for older UA compat
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		next.ServeHTTP(w, r)
	})
}

// SecureHeadersProduction adds HSTS on top of SecureHeaders.
// Called only when GO_ENV=production so dev doesn't get pinned to HTTPS.
func SecureHeadersProduction(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// max-age=63072000 = 2 years; includeSubDomains + preload for production
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		next.ServeHTTP(w, r)
	})
}
