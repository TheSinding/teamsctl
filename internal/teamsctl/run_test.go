package teamsctl

import (
	"strings"
	"testing"

	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/fossteams/teams-api/pkg/models"
)

func TestReadMessage(t *testing.T) {
	message, err := readMessage([]string{"hello", "world"}, strings.NewReader("ignored"))
	if err != nil || message != "hello world" {
		t.Fatalf("readMessage() = %q, %v", message, err)
	}
	message, err = readMessage(nil, strings.NewReader("from stdin\n"))
	if err != nil || message != "from stdin" {
		t.Fatalf("readMessage(stdin) = %q, %v", message, err)
	}
}

func TestFormatMessageHTMLEscapesInput(t *testing.T) {
	got, err := formatMessageContent("<script>\nnext", "text")
	if err != nil {
		t.Fatal(err)
	}
	want := "<div><div>&lt;script&gt;<br/>next</div></div>"
	if got != want {
		t.Fatalf("formatMessageHTML() = %q", got)
	}
}

func TestFormatMessageHTMLPreservesMarkup(t *testing.T) {
	got, err := formatMessageContent("<strong>Hello</strong>", "html")
	if err != nil {
		t.Fatal(err)
	}
	if got != "<strong>Hello</strong>" {
		t.Fatalf("formatMessageContent() = %q", got)
	}
}

func TestFilterConversationsPrefersOneOnOne(t *testing.T) {
	records := []Conversation{
		{Kind: "chat", Title: "Mikkel Ljungberg, Rasmus Prip"},
		{Kind: "chat", Title: "Mikkel Ljungberg", OneOnOne: true},
		{Kind: "channel", Title: "Mikkel planning"},
	}
	matches := filterConversations(records, "mikkel", "chat", 1)
	if len(matches) != 1 || matches[0].Title != "Mikkel Ljungberg" {
		t.Fatalf("filterConversations() = %#v", matches)
	}
}

func TestApplyResolvedMentions(t *testing.T) {
	got, err := applyResolvedMentions("<div>Email a@b.com, hello @Mikkel and @channel</div>", []mentionResolution{{
		Query: "Mikkel",
		Wire:  mentionWire{ID: 0, DisplayName: "Mikkel Ljungberg"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := `<div>Email a@b.com, hello <at id="0">@Mikkel Ljungberg</at> and @channel</div>`
	if got != want {
		t.Fatalf("applyResolvedMentions() = %q", got)
	}
}

func TestFindSelfMentionMemberByEmail(t *testing.T) {
	me := &models.User{DisplayName: "Simon Sinding", Email: "simon@example.com", Mri: "8:orgid:me", ObjectId: "me"}
	member, ok := findSelfMentionMember("simon@example.com", me)
	if !ok || member.ObjectId != "me" {
		t.Fatalf("findSelfMentionMember() = %#v, %v", member, ok)
	}
}

func TestFindMentionMemberExcludesCurrentUser(t *testing.T) {
	members := []csa.ChatMember{
		{FriendlyName: "Simon", ObjectId: "me"},
		{FriendlyName: "Mikkel Ljungberg", ObjectId: "mikkel", Mri: "8:orgid:mikkel"},
	}
	member, ok := findMentionMember("Mikkel", members, &models.User{ObjectId: "me"})
	if !ok || member.ObjectId != "mikkel" {
		t.Fatalf("findMentionMember() = %#v, %v", member, ok)
	}
}
