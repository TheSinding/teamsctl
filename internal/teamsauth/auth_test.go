package teamsauth

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAuthLoginURL(t *testing.T) {
	raw, state, err := authLoginURL(authSkype, "tenant", "user@example.com")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("resource") != authSkypeResource || parsed.Query().Get("login_hint") != "user@example.com" {
		t.Fatalf("unexpected auth URL: %s", raw)
	}
	if !strings.HasSuffix(state, "|"+authSkypeResource) {
		t.Fatalf("unexpected state: %s", state)
	}
}

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

func TestParseAuthCallback(t *testing.T) {
	t.Run("teams ignores stale state", func(t *testing.T) {
		_, matched, err := parseAuthCallback(authRedirectURI+"#id_token=token&state=stale", authTeams, "expected")
		if err != nil || matched {
			t.Fatalf("parseAuthCallback() matched=%v err=%v", matched, err)
		}
	})
	t.Run("resource permits missing state", func(t *testing.T) {
		token, matched, err := parseAuthCallback(authRedirectURI+"#access_token=token", authSkype, "expected")
		if err != nil || !matched || token != "token" {
			t.Fatalf("parseAuthCallback() = %q, %v, %v", token, matched, err)
		}
	})
	t.Run("empty callback is ignored", func(t *testing.T) {
		_, matched, err := parseAuthCallback(authRedirectURI+"#", authSkype, "expected")
		if err != nil || matched {
			t.Fatalf("parseAuthCallback() matched=%v err=%v", matched, err)
		}
	})
}
