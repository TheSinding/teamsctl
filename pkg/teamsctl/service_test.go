package teamsctl

import (
	"testing"

	teamsapi "github.com/TheSinding/teamsctl/third_party/teams-api"
	"github.com/zalando/go-keyring"
)

func TestNewServicePassesKeyringTokensToClient(t *testing.T) {
	keyring.MockInit()
	if err := keyring.Set("teamsctl", "skype", "skype-token"); err != nil {
		t.Fatal(err)
	}
	if err := keyring.Set("teamsctl", "chatsvcagg", "chat-token"); err != nil {
		t.Fatal(err)
	}

	original := newTeamsAPIClient
	defer func() { newTeamsAPIClient = original }()
	newTeamsAPIClient = func(skype, chat string) (*teamsapi.TeamsClient, error) {
		if skype != "skype-token" || chat != "chat-token" {
			t.Fatalf("unexpected tokens: skype=%q chat=%q", skype, chat)
		}
		return &teamsapi.TeamsClient{}, nil
	}

	if _, err := NewService(); err != nil {
		t.Fatal(err)
	}
}
