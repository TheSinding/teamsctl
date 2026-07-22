package teamsctl

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	teamsapi "github.com/fossteams/teams-api"
	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/fossteams/teams-api/pkg/models"
)

func (s *Service) Conversations() ([]Conversation, error) {
	return s.conversationRecords(true)
}

func (s *Service) FindConversations(query, kind string, limit int) ([]Conversation, error) {
	if kind != "" && kind != "chat" && kind != "channel" {
		return nil, fmt.Errorf("kind must be chat or channel")
	}
	if limit < 0 {
		return nil, fmt.Errorf("limit must be at least 0")
	}
	records, err := s.conversationRecords(query == "")
	if err != nil {
		return nil, err
	}
	matches := filterConversations(records, query, kind, limit)
	if query != "" && len(matches) == 0 {
		records, err = s.conversationRecords(true)
		if err != nil {
			return nil, err
		}
		matches = filterConversations(records, query, kind, limit)
	}
	return matches, nil
}

func (s *Service) conversationRecords(enrich bool) ([]Conversation, error) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.response == nil || time.Now().After(s.cacheUntil) {
		me, err := s.client.GetMe()
		if err != nil {
			return nil, fmt.Errorf("get current user: %w", err)
		}
		response, err := s.client.GetConversations()
		if err != nil {
			return nil, fmt.Errorf("get conversations: %w", err)
		}
		s.me = me
		s.response = response
		s.cacheUntil = time.Now().Add(5 * time.Minute)
		s.cacheEnriched = false
	}
	if enrich && !s.cacheEnriched {
		enrichMembers(s.client, s.response)
		s.cacheEnriched = true
	}
	return conversationRecords(s.response, s.me), nil
}

func conversationRecords(response *csa.ConversationResponse, me *models.User) []Conversation {
	if response == nil {
		return []Conversation{}
	}
	records := []Conversation{}
	for _, team := range response.Teams {
		for _, channel := range team.Channels {
			records = append(records, Conversation{
				Kind: "channel", ID: channel.Id, IDs: []string{channel.Id}, Title: channel.DisplayName,
				TeamID: team.Id, TeamTitle: team.DisplayName, Unread: !channel.IsMessageRead,
			})
		}
	}
	for _, chat := range ensurePrivateNotesChat(response.Chats, response.PrivateFeeds) {
		ids := candidateIDs(chat, response.PrivateFeeds)
		if len(ids) == 0 {
			continue
		}
		records = append(records, Conversation{Kind: "chat", ID: ids[0], IDs: ids, Title: chatDisplayName(chat, me), Unread: !chat.IsRead, OneOnOne: chat.IsOneOnOne})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Kind != records[j].Kind {
			return records[i].Kind < records[j].Kind
		}
		return strings.ToLower(records[i].Title) < strings.ToLower(records[j].Title)
	})
	return records
}

func filterConversations(records []Conversation, query, kind string, limit int) []Conversation {
	query = strings.ToLower(strings.TrimSpace(query))
	matches := make([]Conversation, 0)
	for _, record := range records {
		if kind != "" && record.Kind != kind {
			continue
		}
		haystack := strings.ToLower(strings.TrimSpace(record.Title + " " + record.TeamTitle))
		if query == "" || strings.Contains(haystack, query) {
			matches = append(matches, record)
		}
	}
	if query != "" {
		sort.SliceStable(matches, func(i, j int) bool {
			return conversationMatchScore(matches[i], query) < conversationMatchScore(matches[j], query)
		})
	}
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func conversationMatchScore(record Conversation, query string) int {
	title := strings.ToLower(strings.TrimSpace(record.Title))
	score := 20
	if title == query {
		score = 0
	} else if strings.HasPrefix(title, query) {
		score = 10
	}
	if record.OneOnOne {
		score -= 5
	}
	return score
}

func enrichMembers(client *teamsapi.TeamsClient, response *csa.ConversationResponse) {
	if response == nil {
		return
	}
	mris := []string{}
	seen := map[string]bool{}
	for _, chat := range response.Chats {
		for _, member := range chat.Members {
			if member.FriendlyName != "" {
				continue
			}
			mri := strings.TrimSpace(member.Mri)
			if mri == "" && member.ObjectId != "" {
				mri = "8:orgid:" + strings.TrimSpace(member.ObjectId)
			}
			if mri != "" && !seen[mri] {
				seen[mri] = true
				mris = append(mris, mri)
			}
		}
	}
	if len(mris) == 0 {
		return
	}
	profiles, err := client.FetchShortProfile(mris)
	if err != nil {
		return
	}
	byID := map[string]string{}
	for _, profile := range profiles {
		name := strings.TrimSpace(profile.DisplayName)
		if name != "" {
			if mri := strings.ToLower(strings.TrimSpace(profile.Mri)); mri != "" {
				byID[mri] = name
			}
			if objectID := strings.ToLower(strings.TrimSpace(profile.ObjectId)); objectID != "" {
				byID[objectID] = name
			}
		}
	}
	for i := range response.Chats {
		for j := range response.Chats[i].Members {
			member := &response.Chats[i].Members[j]
			if member.FriendlyName == "" {
				if mri := strings.ToLower(strings.TrimSpace(member.Mri)); mri != "" {
					member.FriendlyName = byID[mri]
				}
				if member.FriendlyName == "" && strings.TrimSpace(member.ObjectId) != "" {
					member.FriendlyName = byID[strings.ToLower(strings.TrimSpace(member.ObjectId))]
				}
			}
		}
	}
}

func candidateIDs(chat csa.Chat, feeds []csa.PrivateFeed) []string {
	ids := normalizeIDs([]string{chat.Id, chat.LastMessage.ContainerId})
	for _, feed := range feeds {
		id := extractConversationID(feed.TargetLink)
		if id == "" {
			continue
		}
		if chat.LastMessage.ContainerId != "" && feed.LastMessage.ContainerId != "" && chat.LastMessage.ContainerId != feed.LastMessage.ContainerId {
			continue
		}
		ids = normalizeIDs(append(ids, id))
	}
	return ids
}

func chatDisplayName(chat csa.Chat, me *models.User) string {
	if isPrivateNotesChat(chat) {
		return "Private Notes"
	}
	if title := strings.TrimSpace(chat.Title); title != "" {
		return title
	}
	names := []string{}
	seen := map[string]bool{}
	for _, member := range chat.Members {
		if isCurrentUser(member, me) {
			continue
		}
		name := strings.TrimSpace(member.FriendlyName)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	if len(names) > 0 {
		return strings.Join(names, ", ")
	}
	author := strings.TrimSpace(chat.LastMessage.ImDisplayName)
	if author != "" && !isSelfName(author, me) {
		return author
	}
	if chat.IsOneOnOne {
		return "Chat"
	}
	return "Private Chat"
}

func isCurrentUser(member csa.ChatMember, me *models.User) bool {
	return me != nil && ((me.ObjectId != "" && member.ObjectId == me.ObjectId) || (me.Mri != "" && member.Mri == me.Mri))
}

func isSelfName(name string, me *models.User) bool {
	if me == nil {
		return false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	for _, candidate := range []string{me.DisplayName, me.GivenName, strings.TrimSpace(me.GivenName + " " + me.Surname), me.Alias, me.Email, me.UserPrincipalName} {
		if candidate = strings.ToLower(strings.TrimSpace(candidate)); candidate != "" && candidate == name {
			return true
		}
	}
	return false
}

func ensurePrivateNotesChat(chats []csa.Chat, feeds []csa.PrivateFeed) []csa.Chat {
	for _, chat := range chats {
		if isPrivateNotesChat(chat) {
			return chats
		}
	}
	for _, feed := range feeds {
		id := extractConversationID(feed.TargetLink)
		if id == "" && strings.Contains(strings.ToLower(feed.Id), "48:notes") {
			id = feed.Id
		}
		if strings.Contains(strings.ToLower(id), "48:notes") {
			return append(chats, csa.Chat{Id: id, Title: "Private Notes", LastMessage: feed.LastMessage})
		}
	}
	return chats
}

func isPrivateNotesChat(chat csa.Chat) bool {
	return strings.Contains(strings.ToLower(chat.Id), "48:notes") || strings.EqualFold(strings.TrimSpace(chat.Title), "Private Notes")
}

func extractConversationID(target string) string {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i, part := range parts {
		if (part == "chat" || part == "conversations") && i+1 < len(parts) {
			return strings.TrimSpace(parts[i+1])
		}
	}
	return ""
}

func normalizeIDs(ids []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func splitIDs(value string) []string {
	return normalizeIDs(strings.Split(value, ","))
}
