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
