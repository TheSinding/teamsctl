package teamsauth

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

func TestValidateAuthTokenExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	token := fmt.Sprintf("x.%s.x", base64.RawURLEncoding.EncodeToString([]byte(`{"exp":1001}`)))
	if err := validateAuthToken(authTeams, token, now); err != nil {
		t.Fatal(err)
	}
	if err := validateAuthToken(authTeams, token, now.Add(time.Second)); err == nil {
		t.Fatal("expected expired token error")
	}
}

func TestAuthConfigDirRespectsXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")
	dir, err := authConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/tmp/xdg-config", "teamsctl")
	if dir != want {
		t.Fatalf("expected %s, got %s", want, dir)
	}
}

func TestAuthConfigDirIgnoresRelativeXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "relative/path")
	dir, err := authConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != "teamsctl" || !filepath.IsAbs(dir) {
		t.Fatalf("expected fallback to home config dir, got %s", dir)
	}
	if filepath.Dir(dir) == "relative" {
		t.Fatalf("relative XDG_CONFIG_HOME should be ignored, got %s", dir)
	}
}

func TestSaveAndLoadAuthTokenUsesKeyring(t *testing.T) {
	keyring.MockInit()
	configDir := t.TempDir()

	if err := saveAuthToken(configDir, authTeams, "token-value"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(authTokenFilePath(configDir, authTeams)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no on-disk token file when keyring succeeds, stat err: %v", err)
	}

	got, err := loadAuthToken(configDir, authTeams)
	if err != nil {
		t.Fatal(err)
	}
	if got != "token-value" {
		t.Fatalf("expected token-value, got %s", got)
	}
}

func TestSaveAuthTokenFallsBackToFileWhenKeyringUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("keyring backend unavailable"))
	configDir := t.TempDir()

	if err := saveAuthToken(configDir, authSkype, "file-token"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(authTokenFilePath(configDir, authSkype))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "file-token" {
		t.Fatalf("expected file-token, got %s", string(data))
	}

	got, err := loadAuthToken(configDir, authSkype)
	if err != nil {
		t.Fatal(err)
	}
	if got != "file-token" {
		t.Fatalf("expected file-token, got %s", got)
	}
}

func TestLoadAuthTokenFallsBackToFileWhenKeyringMisses(t *testing.T) {
	keyring.MockInit()
	configDir := t.TempDir()

	if err := saveAuthTokenFile(configDir, authChatSvcAgg, "legacy-token"); err != nil {
		t.Fatal(err)
	}

	got, err := loadAuthToken(configDir, authChatSvcAgg)
	if err != nil {
		t.Fatal(err)
	}
	if got != "legacy-token" {
		t.Fatalf("expected legacy-token, got %s", got)
	}
}

func TestLoadAuthTokenReturnsErrorWhenMissingEverywhere(t *testing.T) {
	keyring.MockInit()
	configDir := t.TempDir()

	if _, err := loadAuthToken(configDir, authTeams); err == nil {
		t.Fatal("expected error when token is not in keyring or on disk")
	}
}

func TestCheckTokensUsesKeyring(t *testing.T) {
	keyring.MockInit()
	parentDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", parentDir)
	teamsctlDir := filepath.Join(parentDir, "teamsctl")
	if err := os.MkdirAll(teamsctlDir, 0o700); err != nil {
		t.Fatal(err)
	}

	validToken := fmt.Sprintf("x.%s.x", base64.RawURLEncoding.EncodeToString([]byte(`{"exp":9999999999}`)))
	for _, kind := range []authTokenKind{authTeams, authSkype, authChatSvcAgg} {
		if err := saveAuthToken(teamsctlDir, kind, validToken); err != nil {
			t.Fatal(err)
		}
	}

	if err := CheckTokens(); err != nil {
		t.Fatal(err)
	}
}
