package teamsauth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func decodeAuthClaims(token string) (authClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return authClaims{}, fmt.Errorf("invalid JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return authClaims{}, fmt.Errorf("decode JWT claims: %w", err)
	}
	var claims authClaims
	if err = json.Unmarshal(payload, &claims); err != nil {
		return authClaims{}, fmt.Errorf("parse JWT claims: %w", err)
	}
	return claims, nil
}

func CheckTokens() error {
	configDir, err := authConfigDir()
	if err != nil {
		return err
	}
	for _, kind := range []authTokenKind{authTeams, authSkype, authChatSvcAgg} {
		path := filepath.Join(configDir, "token-"+string(kind)+".jwt")
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("%s token unavailable; run teamsctl auth: %w", kind, readErr)
		}
		if validateErr := validateAuthToken(kind, strings.TrimSpace(string(data)), time.Now()); validateErr != nil {
			return validateErr
		}
	}
	return nil
}

func validateAuthToken(kind authTokenKind, token string, now time.Time) error {
	claims, decodeErr := decodeAuthClaims(token)
	if decodeErr != nil {
		return fmt.Errorf("%s token invalid; run teamsctl auth: %w", kind, decodeErr)
	}
	if claims.ExpiresAt == 0 || now.Unix() >= claims.ExpiresAt {
		return fmt.Errorf("%s token expired; run teamsctl auth", kind)
	}
	return nil
}

func saveAuthToken(configDir string, kind authTokenKind, token string) error {
	path := filepath.Join(configDir, "token-"+string(kind)+".jwt")
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return fmt.Errorf("save %s token: %w", kind, err)
	}
	return os.Chmod(path, 0o600)
}

func authConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".config", "fossteams"), nil
}
