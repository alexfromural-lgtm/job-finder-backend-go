// ============================================================
// FILE: internal/middleware/security.go
// ROLE: HTTP security headers middleware (equivalent to helmet.js).
//
// 📚 WHAT ARE SECURITY HEADERS?
//   HTTP response headers that instruct browsers to enforce security policies.
//   They defend against common web vulnerabilities without changing any
//   application logic.
//
// 📝 COMPARISON with Node.js:
//   app.use(helmet())   →   r.Use(appMiddleware.SecureHeaders)
//
// 📝 HEADER EXPLANATIONS:
//
//   X-Frame-Options: DENY
//     Prevents the page from being embedded in an <iframe>.
//     Defends against clickjacking — attacker overlays an invisible iframe
//     on their own page and tricks the user into clicking buttons on your page.
//     Modern replacement: Content-Security-Policy: frame-ancestors 'none'
//
//   X-Content-Type-Options: nosniff
//     Tells the browser NOT to guess the content type ("MIME sniffing").
//     Without this, Chrome might execute a .jpg file as JavaScript if the
//     server returns the wrong Content-Type header.
//
//   Referrer-Policy: strict-origin-when-cross-origin
//     Controls how much of the URL is sent in the Referer header.
//     "strict-origin-when-cross-origin": sends full URL for same-origin,
//     only the origin (no path/query) for cross-origin requests.
//     Prevents leaking sensitive URL parameters to third-party sites.
//
//   Permissions-Policy: camera=(), microphone=(), geolocation=()
//     Disables browser APIs this app doesn't need.
//     Even if our app is compromised, injected JS cannot access the camera.
//
//   X-XSS-Protection: 1; mode=block
//     Tells older browsers (IE11, Chrome <78) to enable their built-in
//     XSS filter. Deprecated in modern browsers (they rely on CSP instead),
//     but still useful for legacy user-agents.
//
//   Strict-Transport-Security (HSTS): max-age=63072000; includeSubDomains; preload
//     PRODUCTION ONLY. Tells the browser to ALWAYS use HTTPS for 2 years.
//     Once set, the browser will refuse HTTP even if the user types http://.
//     This prevents SSL stripping attacks.
//     ⚠️  Never set in development — it would break http://localhost.
// ============================================================
package middleware

import (
	"net/http"
)

// SecureHeaders adds security HTTP response headers to every request.
// Apply it with r.Use(SecureHeaders) in main.go — it runs before every handler.
//
// 📝 MIDDLEWARE SIGNATURE:
//   func(next http.Handler) http.Handler
//   This is the standard Go middleware signature. It wraps `next`:
//     1. Runs pre-handler logic (set headers)
//     2. Calls next.ServeHTTP(w, r) to pass control downstream
//     3. Could run post-handler logic after (not needed here)
func SecureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// w.Header().Set(key, value) adds/overwrites a response header.
		// These headers are sent to the browser with every response.

		// Clickjacking protection
		w.Header().Set("X-Frame-Options", "DENY")

		// MIME-type sniffing protection
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Referrer info control
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Disable camera / mic / location APIs
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// Legacy XSS filter (kept for compatibility with older browsers)
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Pass control to the next middleware or handler.
		// Without this call, the request would be swallowed here (no response).
		next.ServeHTTP(w, r)
	})
}

// SecureHeadersProduction adds HSTS on top of SecureHeaders.
// Called ONLY when GO_ENV=production (see main.go):
//
//   if cfg.IsProduction() {
//       r.Use(appMiddleware.SecureHeadersProduction)
//   }
//
// ⚠️  CAUTION: Once a browser receives HSTS, it will refuse HTTP for max-age
// seconds (2 years). Do not enable this in development or on HTTP-only hosts.
//
// 📝 HSTS FIELD EXPLANATION:
//   max-age=63072000  → 2 years (63072000 seconds)
//   includeSubDomains → also applies to api.example.com, www.example.com, etc.
//   preload           → eligible to be hard-coded in browsers' HSTS preload lists
func SecureHeadersProduction(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(
			"Strict-Transport-Security",
			"max-age=63072000; includeSubDomains; preload",
		)
		next.ServeHTTP(w, r)
	})
}
