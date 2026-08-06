package teamsctl

import "testing"

func TestConversationTargetTreatsNamesAndIDsDifferently(t *testing.T) {
	if looksLikeConversationID("Mikkel") {
		t.Fatal("name was treated as an ID")
	}
	if looksLikeConversationID("mikkel@example.com") {
		t.Fatal("email was treated as an ID")
	}
	if !looksLikeConversationID("19:conversation-id@thread.v2") {
		t.Fatal("Teams conversation ID was treated as a name")
	}
}

func TestRecipientIntent(t *testing.T) {
	if got := splitRecipientNames("Mike and Charlie"); len(got) != 2 || got[0] != "Mike" || got[1] != "Charlie" {
		t.Fatalf("splitRecipientNames() = %#v", got)
	}
	if query, kind := namedConversationQuery("ASM group chat"); query != "ASM" || kind != "chat" {
		t.Fatalf("namedConversationQuery() = %q, %q", query, kind)
	}
	if query, kind := namedConversationQuery("ASM channel"); query != "ASM" || kind != "channel" {
		t.Fatalf("namedConversationQuery() = %q, %q", query, kind)
	}
}

func TestMatchingGroupConversationRequiresEveryRecipient(t *testing.T) {
	conversations := []Conversation{
		{Kind: "chat", Title: "Mike, Charlie"},
		{Kind: "chat", Title: "Mike", OneOnOne: true},
	}
	conversation, ok := matchingGroupConversation(conversations, []string{"Mike", "Charlie"})
	if !ok || conversation.Title != "Mike, Charlie" {
		t.Fatalf("matchingGroupConversation() = %#v, %v", conversation, ok)
	}
	if _, ok := matchingGroupConversation(conversations, []string{"Mike", "Pat"}); ok {
		t.Fatal("matched a group chat without every requested recipient")
	}
}
