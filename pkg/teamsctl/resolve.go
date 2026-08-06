package teamsctl

import (
	"fmt"
	"strings"
)

// ConversationTarget is a resolved destination for Send or Messages, produced
// by ResolveConversationTarget from a free-form recipient phrase.
type ConversationTarget struct {
	IDs                []string
	IndividualIDs      [][]string
	Name               string
	Recipients         []string
	FallbackToOneOnOne bool
}

// MissingGroupChatError indicates a multi-person recipient phrase (e.g. "Mike
// and Charlie") did not match an existing group chat. Callers may recover by
// calling ResolveIndividualTargets with Recipients and messaging each person
// individually.
type MissingGroupChatError struct{ Recipients []string }

func (e *MissingGroupChatError) Error() string {
	return fmt.Sprintf("no group chat found for %s", strings.Join(e.Recipients, " and "))
}

// ResolveConversationTarget turns a recipient phrase (a Teams conversation
// ID, a person's name, "Name and Name" for a group chat, "X group chat", or
// "X channel") into a ConversationTarget. If target names a group chat that
// does not exist, it returns a *MissingGroupChatError.
func (s *Service) ResolveConversationTarget(target string) (ConversationTarget, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return ConversationTarget{}, fmt.Errorf("recipient is required")
	}
	if looksLikeConversationID(target) {
		return ConversationTarget{IDs: SplitIDs(target), Recipients: []string{target}}, nil
	}
	if recipients := splitRecipientNames(target); len(recipients) > 1 {
		conversation, err := s.FindGroupConversation(recipients)
		if err != nil {
			return ConversationTarget{}, err
		}
		if len(conversation.IDs) == 0 {
			return ConversationTarget{}, &MissingGroupChatError{Recipients: recipients}
		}
		return ConversationTarget{IDs: conversation.IDs, Name: conversation.Title, Recipients: []string{conversation.Title}}, nil
	}
	if query, kind := namedConversationQuery(target); kind != "" {
		conversation, err := s.findNamedConversation(query, kind)
		if err != nil {
			return ConversationTarget{}, err
		}
		return ConversationTarget{IDs: conversation.IDs, Name: conversation.Title, Recipients: []string{conversation.Title}}, nil
	}
	conversation, err := s.FindOneOnOneConversation(target)
	if err != nil {
		return ConversationTarget{}, err
	}
	return ConversationTarget{IDs: conversation.IDs, Name: conversation.Title, Recipients: []string{conversation.Title}}, nil
}

// ResolveIndividualTargets resolves each recipient to their own one-on-one
// conversation, for use as a fallback when a requested group chat does not
// exist.
func (s *Service) ResolveIndividualTargets(recipients []string) (ConversationTarget, error) {
	individualIDs := make([][]string, 0, len(recipients))
	resolved := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		conversation, err := s.FindOneOnOneConversation(recipient)
		if err != nil {
			return ConversationTarget{}, err
		}
		individualIDs = append(individualIDs, conversation.IDs)
		resolved = append(resolved, conversation.Title)
	}
	return ConversationTarget{IndividualIDs: individualIDs, Recipients: resolved}, nil
}

// FindOneOnOneConversation finds a one-to-one chat by person name or title.
func (s *Service) FindOneOnOneConversation(query string) (Conversation, error) {
	matches, err := s.FindConversations(query, "chat", 0)
	if err != nil {
		return Conversation{}, err
	}
	for _, conversation := range matches {
		if conversation.OneOnOne {
			return conversation, nil
		}
	}
	return Conversation{}, fmt.Errorf("no one-to-one chat found matching %q", query)
}

// FindGroupConversation finds a group chat whose title contains every given
// recipient name. It returns a zero-value Conversation (no error) if none is
// found, so callers can fall back to individual one-on-one messages.
func (s *Service) FindGroupConversation(recipients []string) (Conversation, error) {
	conversations, err := s.Conversations()
	if err != nil {
		return Conversation{}, err
	}
	if conversation, ok := matchingGroupConversation(conversations, recipients); ok {
		return conversation, nil
	}
	return Conversation{}, nil
}

func (s *Service) findNamedConversation(query, kind string) (Conversation, error) {
	conversations, err := s.FindConversations(query, kind, 0)
	if err != nil {
		return Conversation{}, err
	}
	for _, conversation := range conversations {
		if kind != "chat" || !conversation.OneOnOne {
			return conversation, nil
		}
	}
	return Conversation{}, fmt.Errorf("no %s found matching %q", kind, query)
}

func matchingGroupConversation(conversations []Conversation, recipients []string) (Conversation, bool) {
	for _, conversation := range conversations {
		if conversation.Kind != "chat" || conversation.OneOnOne {
			continue
		}
		title := strings.ToLower(conversation.Title)
		matched := true
		for _, recipient := range recipients {
			if !strings.Contains(title, strings.ToLower(recipient)) {
				matched = false
				break
			}
		}
		if matched {
			return conversation, true
		}
	}
	return Conversation{}, false
}

func looksLikeConversationID(target string) bool {
	ids := SplitIDs(target)
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if !strings.HasPrefix(id, "19:") && !strings.HasPrefix(id, "48:") {
			return false
		}
	}
	return true
}

func splitRecipientNames(target string) []string {
	parts := strings.Split(strings.TrimSpace(target), " and ")
	if len(parts) < 2 {
		return nil
	}
	return normalizeIDs(parts)
}

func namedConversationQuery(target string) (string, string) {
	lower := strings.ToLower(strings.TrimSpace(target))
	for _, suffix := range []struct {
		value string
		kind  string
	}{{" group chat", "chat"}, {" chat", "chat"}, {" channel", "channel"}} {
		if strings.HasSuffix(lower, suffix.value) {
			return strings.TrimSpace(target[:len(target)-len(suffix.value)]), suffix.kind
		}
	}
	return "", ""
}
