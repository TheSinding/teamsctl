package teamsauth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

// keyringService is the OS keyring service name under which tokens are stored.
const keyringService = "teamsctl"

type ClientTokens struct {
	Skype      string
	ChatSvcAgg string
}

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
		token, loadErr := loadAuthToken(configDir, kind)
		if loadErr != nil {
			return fmt.Errorf("%s token unavailable; run teamsctl auth: %w", kind, loadErr)
		}
		if validateErr := validateAuthToken(kind, strings.TrimSpace(token), time.Now()); validateErr != nil {
			return validateErr
		}
	}
	return nil
}

func LoadClientTokens() (ClientTokens, error) {
	configDir, err := authConfigDir()
	if err != nil {
		return ClientTokens{}, err
	}
	skype, err := loadAuthToken(configDir, authSkype)
	if err != nil {
		return ClientTokens{}, fmt.Errorf("%s token unavailable; run teamsctl auth: %w", authSkype, err)
	}
	chatSvcAgg, err := loadAuthToken(configDir, authChatSvcAgg)
	if err != nil {
		return ClientTokens{}, fmt.Errorf("%s token unavailable; run teamsctl auth: %w", authChatSvcAgg, err)
	}
	return ClientTokens{Skype: strings.TrimSpace(skype), ChatSvcAgg: strings.TrimSpace(chatSvcAgg)}, nil
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

// saveAuthToken stores a token in the OS keyring, preferring it over plaintext
// file storage. If the keyring backend is unavailable (e.g. no Secret Service
// running on a headless Linux host), it falls back to the file-based storage
// used previously. Any stale on-disk copy is removed once a token is
// successfully migrated to the keyring.
func saveAuthToken(configDir string, kind authTokenKind, token string) error {
	if err := keyring.Set(keyringService, string(kind), token); err == nil {
		if removeErr := removeAuthTokenFile(configDir, kind); removeErr != nil {
			return fmt.Errorf("remove stale %s token file: %w", kind, removeErr)
		}
		return nil
	}
	return saveAuthTokenFile(configDir, kind, token)
}

// loadAuthToken reads a token from the OS keyring, falling back to the
// file-based storage if the keyring backend is unavailable or has no entry
// for the requested token.
func loadAuthToken(configDir string, kind authTokenKind) (string, error) {
	token, err := keyring.Get(keyringService, string(kind))
	if err == nil {
		return token, nil
	}
	return loadAuthTokenFile(configDir, kind)
}

func authTokenFilePath(configDir string, kind authTokenKind) string {
	return filepath.Join(configDir, "token-"+string(kind)+".jwt")
}

func saveAuthTokenFile(configDir string, kind authTokenKind, token string) error {
	path := authTokenFilePath(configDir, kind)
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return fmt.Errorf("save %s token: %w", kind, err)
	}
	return os.Chmod(path, 0o600)
}

func loadAuthTokenFile(configDir string, kind authTokenKind) (string, error) {
	data, err := os.ReadFile(authTokenFilePath(configDir, kind))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func removeAuthTokenFile(configDir string, kind authTokenKind) error {
	err := os.Remove(authTokenFilePath(configDir, kind))
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func authConfigDir() (string, error) {
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(xdgConfigHome) {
		return filepath.Join(xdgConfigHome, "teamsctl"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".config", "teamsctl"), nil
}
