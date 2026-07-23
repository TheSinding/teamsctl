package teamsauth

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"testing"
	"time"
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
