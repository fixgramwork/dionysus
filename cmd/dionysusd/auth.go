package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const jwtIssuer = "dionysus-pve"

type jwtClaims struct {
	Subject   string `json:"sub"`
	Issuer    string `json:"iss"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

func isPublicAPI(request *http.Request) bool {
	return request.Method == http.MethodPost && request.URL.Path == "/api2/json/access/ticket"
}

func loginTicket(cfg Config, body map[string]any) (map[string]any, error) {
	username := strings.TrimSpace(asString(body["username"]))
	password := asString(body["password"])
	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password are required")
	}

	user, ok, err := loginUser(cfg, username, password)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("invalid username or password")
	}

	token, expiresAt, err := issueJWT(cfg, user.Username, time.Now())
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"tokenType":      "Bearer",
		"token":          token,
		"username":       user.Username,
		"permissions":    user.Permissions,
		"canManageUsers": user.Username == rootAuthUsername,
		"expiresAt":      expiresAt,
	}, nil
}

func authorize(cfg Config, request *http.Request) error {
	if !fileExists(cfg.TokenFile) {
		return nil
	}
	token, ok := bearerToken(request)
	if !ok {
		return fmt.Errorf("missing bearer token")
	}
	if _, err := validateJWT(cfg, token, time.Now()); err != nil {
		return err
	}
	return nil
}

func bearerToken(request *http.Request) (string, bool) {
	header := request.Header.Get("Authorization")
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		token, ok = strings.CutPrefix(header, "bearer ")
	}
	return strings.TrimSpace(token), ok && strings.TrimSpace(token) != ""
}

func issueJWT(cfg Config, username string, now time.Time) (string, int64, error) {
	secret, err := jwtSecret(cfg)
	if err != nil {
		return "", 0, err
	}
	ttl := cfg.JWTTTLSeconds
	if ttl < 60 {
		ttl = defaultJWTTTLSeconds
	}
	claims := jwtClaims{
		Subject:   username,
		Issuer:    jwtIssuer,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Duration(ttl) * time.Second).Unix(),
	}
	headerJSON, _ := json.Marshal(map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	})
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", 0, fmt.Errorf("jwt claims cannot be encoded")
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := encodedHeader + "." + encodedClaims
	signature := signJWT([]byte(signingInput), secret)

	return signingInput + "." + signature, claims.ExpiresAt, nil
}

func validateJWT(cfg Config, token string, now time.Time) (jwtClaims, error) {
	var claims jwtClaims
	secret, err := jwtSecret(cfg)
	if err != nil {
		return claims, err
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, fmt.Errorf("invalid jwt token")
	}
	signingInput := parts[0] + "." + parts[1]
	expectedSignature := signJWT([]byte(signingInput), secret)
	if subtle.ConstantTimeCompare([]byte(expectedSignature), []byte(parts[2])) != 1 {
		return claims, fmt.Errorf("invalid jwt signature")
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, fmt.Errorf("invalid jwt header")
	}
	var header map[string]string
	if err := json.Unmarshal(headerJSON, &header); err != nil || header["alg"] != "HS256" {
		return claims, fmt.Errorf("unsupported jwt algorithm")
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, fmt.Errorf("invalid jwt claims")
	}
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return claims, fmt.Errorf("invalid jwt claims")
	}
	if claims.Issuer != jwtIssuer || claims.Subject == "" {
		return claims, fmt.Errorf("invalid jwt claims")
	}
	if now.Unix() >= claims.ExpiresAt {
		return claims, fmt.Errorf("jwt token expired")
	}
	if !authUserExists(cfg, claims.Subject) {
		return claims, fmt.Errorf("jwt subject is not allowed")
	}
	return claims, nil
}

func currentSession(cfg Config, request *http.Request) (map[string]any, error) {
	token, ok := bearerToken(request)
	if !ok {
		return nil, fmt.Errorf("missing bearer token")
	}
	claims, err := validateJWT(cfg, token, time.Now())
	if err != nil {
		return nil, err
	}
	user, ok, err := authUserByUsername(cfg, claims.Subject)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("jwt subject is not allowed")
	}
	return map[string]any{
		"username":       claims.Subject,
		"permissions":    user.Permissions,
		"canManageUsers": user.Username == rootAuthUsername,
		"expiresAt":      claims.ExpiresAt,
	}, nil
}

func signJWT(input []byte, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(input)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func jwtSecret(cfg Config) ([]byte, error) {
	raw, err := os.ReadFile(cfg.TokenFile)
	if err != nil {
		return nil, fmt.Errorf("jwt signing key is unreadable")
	}
	secret := strings.TrimSpace(string(raw))
	if secret == "" {
		return nil, fmt.Errorf("jwt signing key is empty")
	}
	return []byte(secret), nil
}

func authUsername(cfg Config) string {
	return firstNonEmpty(readTrim(cfg.AuthUserFile), cfg.AuthUsername, defaultAuthUsername)
}

func authPassword(cfg Config) (string, bool) {
	password := firstNonEmpty(cfg.AuthPassword, readTrim(cfg.AuthPasswordFile))
	return password, password != ""
}

func hasLoginCredentials(cfg Config) bool {
	authUsersMu.Lock()
	defer authUsersMu.Unlock()

	users, err := readAuthUsersUnlocked(cfg)
	return err == nil && len(users) > 0
}

func verifyPassword(expected string, provided string) bool {
	expected = strings.TrimSpace(expected)
	if strings.HasPrefix(expected, "sha256:") {
		sum := sha256.Sum256([]byte(provided))
		return constantTimeEqual(strings.TrimPrefix(expected, "sha256:"), hex.EncodeToString(sum[:]))
	}
	return constantTimeEqual(expected, provided)
}

func constantTimeEqual(left string, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
