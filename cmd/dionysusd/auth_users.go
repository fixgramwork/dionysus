package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type authUser struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

type authUserStore struct {
	Users []authUser `json:"users"`
}

var authUsersMu sync.Mutex

func loginUser(cfg Config, username string, password string) (authUser, bool, error) {
	authUsersMu.Lock()
	defer authUsersMu.Unlock()

	users, err := readAuthUsersUnlocked(cfg)
	if err != nil {
		return authUser{}, false, err
	}
	for _, user := range users {
		if constantTimeEqual(user.Username, username) && verifyPassword(user.PasswordHash, password) {
			return user, true, nil
		}
	}
	return authUser{}, false, nil
}

func authUserExists(cfg Config, username string) bool {
	authUsersMu.Lock()
	defer authUsersMu.Unlock()

	users, err := readAuthUsersUnlocked(cfg)
	if err != nil {
		return false
	}
	for _, user := range users {
		if user.Username == username {
			return true
		}
	}
	return false
}

func authUserViews(cfg Config) ([]any, error) {
	authUsersMu.Lock()
	defer authUsersMu.Unlock()

	users, err := readAuthUsersUnlocked(cfg)
	if err != nil {
		return nil, err
	}
	views := make([]any, 0, len(users))
	for _, user := range users {
		views = append(views, publicAuthUser(user))
	}
	return views, nil
}

func createAuthUser(cfg Config, body map[string]any) (map[string]any, error) {
	username := strings.TrimSpace(asString(body["username"]))
	password := asString(body["password"])
	if err := validateAuthUsername(username); err != nil {
		return nil, err
	}
	if err := validateAuthPassword(password); err != nil {
		return nil, err
	}

	authUsersMu.Lock()
	defer authUsersMu.Unlock()

	users, err := readAuthUsersUnlocked(cfg)
	if err != nil {
		return nil, err
	}
	for _, user := range users {
		if user.Username == username {
			return nil, fmt.Errorf("user already exists")
		}
	}

	now := time.Now().Unix()
	user := authUser{
		Username:     username,
		PasswordHash: hashPassword(password),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	users = append(users, user)
	if err := writeAuthUsersUnlocked(cfg, users); err != nil {
		return nil, err
	}
	return publicAuthUser(user), nil
}

func updateAuthUserPassword(cfg Config, body map[string]any) (map[string]any, error) {
	username := strings.TrimSpace(asString(body["username"]))
	password := asString(body["password"])
	if err := validateAuthUsername(username); err != nil {
		return nil, err
	}
	if err := validateAuthPassword(password); err != nil {
		return nil, err
	}

	authUsersMu.Lock()
	defer authUsersMu.Unlock()

	users, err := readAuthUsersUnlocked(cfg)
	if err != nil {
		return nil, err
	}
	for index, user := range users {
		if user.Username != username {
			continue
		}
		users[index].PasswordHash = hashPassword(password)
		users[index].UpdatedAt = time.Now().Unix()
		if err := writeAuthUsersUnlocked(cfg, users); err != nil {
			return nil, err
		}
		return publicAuthUser(users[index]), nil
	}
	return nil, fmt.Errorf("user not found")
}

func deleteAuthUser(cfg Config, currentUsername string, body map[string]any) (map[string]any, error) {
	username := strings.TrimSpace(asString(body["username"]))
	if err := validateAuthUsername(username); err != nil {
		return nil, err
	}
	if username == currentUsername {
		return nil, fmt.Errorf("cannot delete the signed-in user")
	}

	authUsersMu.Lock()
	defer authUsersMu.Unlock()

	users, err := readAuthUsersUnlocked(cfg)
	if err != nil {
		return nil, err
	}
	if len(users) <= 1 {
		return nil, fmt.Errorf("cannot delete the last user")
	}

	next := make([]authUser, 0, len(users)-1)
	deleted := false
	for _, user := range users {
		if user.Username == username {
			deleted = true
			continue
		}
		next = append(next, user)
	}
	if !deleted {
		return nil, fmt.Errorf("user not found")
	}
	if err := writeAuthUsersUnlocked(cfg, next); err != nil {
		return nil, err
	}
	return map[string]any{"username": username, "deleted": true}, nil
}

func readAuthUsersUnlocked(cfg Config) ([]authUser, error) {
	if fileExists(cfg.AuthUsersFile) {
		raw, err := os.ReadFile(cfg.AuthUsersFile)
		if err != nil {
			return nil, fmt.Errorf("users file is unreadable")
		}
		var store authUserStore
		if err := json.Unmarshal(raw, &store); err != nil {
			return nil, fmt.Errorf("users file is invalid")
		}
		return normalizeAuthUsers(store.Users), nil
	}

	username := authUsername(cfg)
	password, ok := authPassword(cfg)
	if !ok || username == "" {
		return []authUser{}, nil
	}
	now := time.Now().Unix()
	return []authUser{{
		Username:     username,
		PasswordHash: password,
		CreatedAt:    now,
		UpdatedAt:    now,
	}}, nil
}

func writeAuthUsersUnlocked(cfg Config, users []authUser) error {
	users = normalizeAuthUsers(users)
	store := authUserStore{Users: users}
	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("users file cannot be encoded")
	}
	raw = append(raw, '\n')

	if err := os.MkdirAll(filepath.Dir(cfg.AuthUsersFile), 0755); err != nil {
		return fmt.Errorf("users directory cannot be created")
	}
	tmp := fmt.Sprintf("%s.tmp.%d", cfg.AuthUsersFile, os.Getpid())
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return fmt.Errorf("users file cannot be written")
	}
	if err := os.Rename(tmp, cfg.AuthUsersFile); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("users file cannot be replaced")
	}
	_ = os.Chmod(cfg.AuthUsersFile, 0600)
	return nil
}

func normalizeAuthUsers(users []authUser) []authUser {
	seen := map[string]bool{}
	next := make([]authUser, 0, len(users))
	for _, user := range users {
		user.Username = strings.TrimSpace(user.Username)
		user.PasswordHash = strings.TrimSpace(user.PasswordHash)
		if user.Username == "" || user.PasswordHash == "" || seen[user.Username] {
			continue
		}
		seen[user.Username] = true
		next = append(next, user)
	}
	sort.Slice(next, func(left int, right int) bool {
		return next[left].Username < next[right].Username
	})
	return next
}

func publicAuthUser(user authUser) map[string]any {
	return map[string]any{
		"username":  user.Username,
		"createdAt": user.CreatedAt,
		"updatedAt": user.UpdatedAt,
	}
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validateAuthUsername(username string) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if len(username) > 64 {
		return fmt.Errorf("username is too long")
	}
	for _, ch := range username {
		if ch >= 'a' && ch <= 'z' {
			continue
		}
		if ch >= 'A' && ch <= 'Z' {
			continue
		}
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '.' || ch == '_' || ch == '-' {
			continue
		}
		return fmt.Errorf("username can only contain letters, numbers, '.', '_' and '-'")
	}
	return nil
}

func validateAuthPassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if len(password) > 256 {
		return fmt.Errorf("password is too long")
	}
	return nil
}
