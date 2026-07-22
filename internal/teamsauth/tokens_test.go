package teamsauth

import (
	"encoding/base64"
	"fmt"
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
