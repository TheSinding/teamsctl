package teamsctl

import (
	"testing"

	"github.com/TheSinding/teamsctl/third_party/teams-api/pkg/csa"
	"github.com/TheSinding/teamsctl/third_party/teams-api/pkg/models"
)

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
