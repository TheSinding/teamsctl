package teamsctl

import (
	"testing"

	"github.com/TheSinding/teamsctl/third_party/teams-api/pkg/csa"
	"github.com/TheSinding/teamsctl/third_party/teams-api/pkg/models"
)

func TestIsSelfSender(t *testing.T) {
	me := &models.User{Mri: "8:orgid:me", ObjectId: "me-id"}
	if !isSelfSender("8:orgid:me", me) {
		t.Fatal("direct MRI match should be self")
	}
	if isSelfSender("8:orgid:someone-else", me) {
		t.Fatal("different MRI should not be self")
	}
	if isSelfSender("", me) {
		t.Fatal("empty from should not be self")
	}
	if isSelfSender("8:orgid:me", nil) {
		t.Fatal("nil me should not be self")
	}
}

func TestIsSelfSenderDerivesMRIFromObjectID(t *testing.T) {
	me := &models.User{ObjectId: "me-id"}
	if !isSelfSender("8:orgid:me-id", me) {
		t.Fatal("ObjectId-derived MRI should be self")
	}
}

func TestIsSelfSenderMatchesFullContactURL(t *testing.T) {
	me := &models.User{Mri: "8:orgid:me"}
	from := "https://emea.ng.msg.teams.microsoft.com/v1/users/ME/contacts/8:orgid:me"
	if !isSelfSender(from, me) {
		t.Fatal("a full contact URL ending in the MRI should be self")
	}
	if isSelfSender("https://emea.ng.msg.teams.microsoft.com/v1/users/ME/contacts/8:orgid:someone-else", me) {
		t.Fatal("a contact URL for a different MRI should not be self")
	}
}

func TestMessageRecordsBackfillsSelfAuthor(t *testing.T) {
	me := &models.User{DisplayName: "Simon Sinding", Mri: "8:orgid:me"}
	messages := []csa.ChatMessage{
		{Id: "1", From: "8:orgid:me", ImDisplayName: ""},
		{Id: "2", From: "8:orgid:other", ImDisplayName: "Niklas Johansson"},
		{Id: "3", From: "8:orgid:other", ImDisplayName: ""},
	}
	records := messageRecords(messages, me)
	if records[0].Author != "Simon Sinding" {
		t.Fatalf("self author not backfilled: %q", records[0].Author)
	}
	if records[1].Author != "Niklas Johansson" {
		t.Fatalf("other author changed: %q", records[1].Author)
	}
	if records[2].Author != "" {
		t.Fatalf("non-self empty author should stay empty: %q", records[2].Author)
	}
	if records[0].SenderID != "8:orgid:me" {
		t.Fatalf("sender id not set: %q", records[0].SenderID)
	}
}

func TestMessageRecordsNilIdentityLeavesAuthorUntouched(t *testing.T) {
	messages := []csa.ChatMessage{{Id: "1", From: "8:orgid:me", ImDisplayName: ""}}
	records := messageRecords(messages, nil)
	if records[0].Author != "" {
		t.Fatalf("author should stay empty without identity: %q", records[0].Author)
	}
}
