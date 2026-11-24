// ============================================================
// FILE: internal/utils/token.go
// ROLE: JWT generation and verification.
//
// 📚 WHAT IS A JWT?
//   JSON Web Token — a compact, URL-safe string that proves identity.
//   Structure: header.payload.signature  (Base64URL encoded, dot-separated)
//
//   HEADER:  {"alg":"HS256","typ":"JWT"}
//   PAYLOAD: {"userId":"abc","roles":["JOB_SEEKER"],"exp":1700000000}
//   SIGNATURE: HMAC-SHA256(header + "." + payload, secret)
//
//   Anyone can decode header + payload (they're just Base64).
//   Only the server (which knows the secret) can CREATE a valid signature.
//   The server verifies the signature on every request to ensure the token
//   wasn't tampered with.
//
// 📝 COMPARISON with Node.js:
//   jwt.sign({ userId, roles }, secret, { expiresIn: "15m" })  → GenerateAccessToken
//   jwt.verify(token, secret)                                   → VerifyToken
//
// 📝 HOW TOKENS ARE USED IN THIS APP:
//   1. POST /api/auth/login → server creates accessToken + refreshToken
//   2. Both are stored as HttpOnly cookies (invisible to JavaScript)
//   3. Browser sends cookies automatically on every request
//   4. RequireAuth middleware calls VerifyToken() to check the accessToken
//   5. If expired, POST /api/auth/refresh uses the refreshToken to get a new pair
// ============================================================
package utils

import (
	"time"

	// golang-jwt/jwt — the most widely used JWT library for Go.
	// Equivalent to the jsonwebtoken npm package.
	"github.com/golang-jwt/jwt/v5"
)

// TokenClaims is the payload embedded in every JWT issued by this server.
// It EXTENDS jwt.RegisteredClaims (which provides standard fields like
// ExpiresAt, IssuedAt, Issuer) with our custom fields.
//
// 📝 WIRE FORMAT (what actually goes into the JWT):
//   {
//     "userId": "clx1234...",
//     "roles": ["JOB_SEEKER"],
//     "exp": 1700000900,    ← ExpiresAt (Unix timestamp)
//     "iat": 1700000000     ← IssuedAt
//   }
//
// The json:"..." struct tags control the field names in the JSON payload.
type TokenClaims struct {
	UserID string   `json:"userId"` // matches the Node.js { userId } claim name
	Roles  []string `json:"roles"`  // e.g. ["JOB_SEEKER"] or ["JOB_SEEKER","RECRUITER"]
	jwt.RegisteredClaims             // embeds Issuer, Subject, ExpiresAt, IssuedAt, etc.
}

// GenerateAccessToken creates a short-lived signed JWT (default 15 minutes).
//
// 📝 STEP-BY-STEP:
//   1. Build a TokenClaims struct with the user's ID, roles, and expiry
//   2. Create a jwt.Token using HMAC-SHA256 signing (HS256)
//   3. Sign it with the secret → returns a compact "header.payload.sig" string
//
// 📝 EXAMPLE:
//   token, _ := utils.GenerateAccessToken("user-123", []string{"JOB_SEEKER"}, "my-secret", 15*time.Minute)
//   // token: "eyJhbGciOiJIUzI1NiJ9.eyJ1c2VySWQiOiJ1c2VyLTEyMyJ9.xxx"
func GenerateAccessToken(userID string, roles []string, secret string, expiry time.Duration) (string, error) {
	claims := TokenClaims{
		UserID: userID,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			// time.Now().Add(expiry) calculates the expiry timestamp.
			// jwt.NewNumericDate wraps it in the JWT-standard date format.
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	// jwt.NewWithClaims picks the signing algorithm and attaches the claims.
	// jwt.SigningMethodHS256 = HMAC-SHA256 — symmetric (same key for sign + verify).
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// token.SignedString(key) signs the header+payload with the key and
	// returns the final compact JWT string.
	// []byte(secret) converts the string secret to bytes for the HMAC function.
	return token.SignedString([]byte(secret))
}

// GenerateRefreshToken creates a longer-lived JWT (default 7 days).
// The refresh token is used ONLY to issue a new access token when the
// short-lived one expires. It should be stored in a more restricted cookie.
//
// The logic is identical to GenerateAccessToken; the only difference is the
// longer expiry. In a production app you might also store refresh tokens
// in the database to allow revocation (logout from all devices).
//
// 📝 EXAMPLE:
//   rt, _ := utils.GenerateRefreshToken(userID, roles, refreshSecret, 7*24*time.Hour)
func GenerateRefreshToken(userID string, roles []string, secret string, expiry time.Duration) (string, error) {
	claims := TokenClaims{
		UserID: userID,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// VerifyToken parses a JWT string and verifies its signature and expiry.
// Returns the decoded *TokenClaims on success, or an error on failure.
//
// 📝 WHAT CAN GO WRONG?
//   - jwt.ErrTokenExpired          → token past its ExpiresAt timestamp
//   - jwt.ErrSignatureInvalid      → token was tampered with or wrong secret
//   - jwt.ErrTokenMalformed        → not a valid JWT format
//
// 📝 EXAMPLE:
//   claims, err := utils.VerifyToken(cookieValue, cfg.AccessTokenSecret)
//   if err != nil {
//       // return 401 Unauthorized
//   }
//   // claims.UserID, claims.Roles are now available
func VerifyToken(tokenString, secret string) (*TokenClaims, error) {
	// jwt.ParseWithClaims parses the token string and populates the claims struct.
	// The key function (3rd argument) is called to provide the signing key.
	// It receives the parsed-but-unverified token so we can check the algorithm.
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(t *jwt.Token) (interface{}, error) {
		// Security check: reject tokens signed with an unexpected algorithm.
		// Without this check, an attacker could send a token signed with "none"
		// (no signature) and bypass verification.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		// Return the secret key so the library can verify the signature.
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err // jwt.ErrTokenExpired, ErrSignatureInvalid, etc.
	}

	// token.Claims is of type jwt.Claims (interface), so we need a type assertion
	// to get back our concrete *TokenClaims type.
	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}
