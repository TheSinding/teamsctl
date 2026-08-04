package teamsctl

import "testing"

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

func TestConversationTargetTreatsNamesAndIDsDifferently(t *testing.T) {
	if looksLikeConversationID("Mikkel") {
		t.Fatal("name was treated as an ID")
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
