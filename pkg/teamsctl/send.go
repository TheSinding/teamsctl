package teamsctl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/fossteams/teams-api/pkg/models"
)

func (s *Service) Send(ids []string, content string, options SendOptions) error {
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return fmt.Errorf("no conversation id available")
	}
	formattedContent, err := formatMessageContent(content, options.Format)
	if err != nil {
		return err
	}
	properties := map[string]interface{}{}
	if len(options.Mentions) > 0 || len(options.MentionEntities) > 0 {
		resolved, resolveErr := s.resolveMentions(ids, options.Mentions, options.MentionEntities)
		if resolveErr != nil {
			return resolveErr
		}
		formattedContent, resolveErr = applyResolvedMentions(formattedContent, resolved)
		if resolveErr != nil {
			return resolveErr
		}
		wires := make([]mentionWire, 0, len(resolved))
		for _, mention := range resolved {
			wires = append(wires, mention.Wire)
		}
		encodedMentions, encodeErr := json.Marshal(wires)
		if encodeErr != nil {
			return fmt.Errorf("encode mentions: %w", encodeErr)
		}
		properties["mentions"] = string(encodedMentions)
	}
	payload := map[string]interface{}{
		"content":         formattedContent,
		"messagetype":     "RichText/Html",
		"contenttype":     "text",
		"clientmessageid": strconv.FormatInt(time.Now().UnixNano(), 10),
		"amsreferences":   []string{},
		"properties":      properties,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}
	var lastErr error
	for _, id := range ids {
		endpoint := csa.MessagesHost + "v1/users/ME/conversations/" + url.QueryEscape(id) + "/messages"
		request, requestErr := s.client.ChatSvc().AuthenticatedRequest(http.MethodPost, endpoint, bytes.NewReader(body))
		if requestErr != nil {
			lastErr = requestErr
			continue
		}
		request.Header.Set("Content-Type", "application/json")
		response, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			lastErr = requestErr
			continue
		}
		responseBody, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated {
			return nil
		}
		lastErr = fmt.Errorf("status=%d body=%s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("send failed")
	}
	return fmt.Errorf("send message: %w", lastErr)
}

func (s *Service) resolveMentions(ids, queries []string, entities []MentionEntity) ([]mentionResolution, error) {
	if _, err := s.conversationRecords(true); err != nil {
		return nil, err
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	targetCandidates := []csa.ChatMember{}
	allCandidates := []csa.ChatMember{}
	for _, chat := range s.response.Chats {
		allCandidates = append(allCandidates, chat.Members...)
		if idsOverlap(ids, candidateIDs(chat, s.response.PrivateFeeds)) {
			targetCandidates = append(targetCandidates, chat.Members...)
		}
	}
	resolved := make([]mentionResolution, 0, len(queries)+len(entities))
	for _, query := range queries {
		member, ok := findSelfMentionMember(query, s.me)
		if !ok {
			member, ok = findMentionMember(query, targetCandidates, s.me)
		}
		if !ok {
			member, ok = findMentionMember(query, allCandidates, s.me)
		}
		if !ok {
			var profileErr error
			member, ok, profileErr = s.findMentionProfile(query, targetCandidates)
			if profileErr != nil {
				return nil, fmt.Errorf("resolve mention %q: %w", query, profileErr)
			}
		}
		if !ok {
			var profileErr error
			member, ok, profileErr = s.findMentionProfile(query, allCandidates)
			if profileErr != nil {
				return nil, fmt.Errorf("resolve mention %q: %w", query, profileErr)
			}
		}
		if !ok {
			return nil, fmt.Errorf("unable to resolve mention %q from conversation members", query)
		}
		mri := strings.TrimSpace(member.Mri)
		if mri == "" && strings.TrimSpace(member.ObjectId) != "" {
			mri = "8:orgid:" + strings.TrimSpace(member.ObjectId)
		}
		if mri == "" {
			return nil, fmt.Errorf("mention %q has no Teams identity", query)
		}
		resolved = append(resolved, mentionResolution{
			Query: strings.TrimPrefix(strings.TrimSpace(query), "@"),
			Wire: mentionWire{
				ID: len(resolved), MentionType: "person", MRI: mri,
				DisplayName: strings.TrimSpace(member.FriendlyName), ObjectID: strings.TrimSpace(member.ObjectId),
			},
		})
	}
	for _, entity := range entities {
		token := strings.TrimPrefix(strings.TrimSpace(entity.Token), "@")
		if token == "" {
			token = strings.TrimSpace(entity.DisplayName)
		}
		mri := strings.TrimSpace(entity.MRI)
		objectID := strings.TrimSpace(entity.ObjectID)
		if mri == "" && objectID != "" {
			mri = "8:orgid:" + objectID
		}
		if token == "" || strings.TrimSpace(entity.DisplayName) == "" || mri == "" {
			return nil, fmt.Errorf("resolved mention requires token, display_name, and mri or object_id")
		}
		resolved = append(resolved, mentionResolution{
			Query: token,
			Wire: mentionWire{
				ID: len(resolved), MentionType: "person", MRI: mri,
				DisplayName: strings.TrimSpace(entity.DisplayName), ObjectID: objectID,
			},
		})
	}
	return resolved, nil
}

func findSelfMentionMember(query string, me *models.User) (csa.ChatMember, bool) {
	if me == nil || !matchesMentionQuery(query, me.DisplayName, me.Email, me.UserPrincipalName, me.Alias) {
		return csa.ChatMember{}, false
	}
	return csa.ChatMember{FriendlyName: me.DisplayName, Mri: me.Mri, ObjectId: me.ObjectId}, true
}

func (s *Service) findMentionProfile(query string, members []csa.ChatMember) (csa.ChatMember, bool, error) {
	mris := []string{}
	seen := map[string]bool{}
	for _, member := range members {
		mri := strings.TrimSpace(member.Mri)
		if mri == "" && strings.TrimSpace(member.ObjectId) != "" {
			mri = "8:orgid:" + strings.TrimSpace(member.ObjectId)
		}
		if mri != "" && !seen[mri] {
			seen[mri] = true
			mris = append(mris, mri)
		}
	}
	if len(mris) == 0 {
		return csa.ChatMember{}, false, nil
	}
	profiles, err := s.client.FetchShortProfile(mris)
	if err != nil {
		return csa.ChatMember{}, false, err
	}
	for _, profile := range profiles {
		if matchesMentionQuery(query, profile.DisplayName, profile.Email, profile.UserPrincipalName, profile.Alias) {
			return csa.ChatMember{
				FriendlyName: profile.DisplayName,
				Mri:          profile.Mri,
				ObjectId:     profile.ObjectId,
			}, true, nil
		}
	}
	return csa.ChatMember{}, false, nil
}

func matchesMentionQuery(query string, values ...string) bool {
	query = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(query), "@")))
	if query == "" {
		return false
	}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == query || strings.HasPrefix(value, query) || strings.Contains(value, query) {
			return true
		}
	}
	return false
}

func findMentionMember(query string, members []csa.ChatMember, me *models.User) (csa.ChatMember, bool) {
	query = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(query), "@")))
	if query == "" {
		return csa.ChatMember{}, false
	}
	unique := map[string]bool{}
	candidates := make([]csa.ChatMember, 0, len(members))
	for _, member := range members {
		if isCurrentUser(member, me) || strings.TrimSpace(member.FriendlyName) == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(member.Mri + "|" + member.ObjectId + "|" + member.FriendlyName))
		if !unique[key] {
			unique[key] = true
			candidates = append(candidates, member)
		}
	}
	for _, match := range []func(string) bool{
		func(name string) bool { return name == query },
		func(name string) bool { return strings.HasPrefix(name, query) },
		func(name string) bool { return strings.Contains(name, query) },
	} {
		for _, member := range candidates {
			if match(strings.ToLower(strings.TrimSpace(member.FriendlyName))) {
				return member, true
			}
		}
	}
	return csa.ChatMember{}, false
}

func idsOverlap(left, right []string) bool {
	seen := map[string]bool{}
	for _, id := range left {
		seen[strings.ToLower(strings.TrimSpace(id))] = true
	}
	for _, id := range right {
		if seen[strings.ToLower(strings.TrimSpace(id))] {
			return true
		}
	}
	return false
}

func applyResolvedMentions(content string, mentions []mentionResolution) (string, error) {
	for _, mention := range mentions {
		token := "@" + mention.Query
		replacement := fmt.Sprintf(`<at id="%d">@%s</at>`, mention.Wire.ID, html.EscapeString(mention.Wire.DisplayName))
		var replaced bool
		content, replaced = replaceFirstFold(content, token, replacement)
		if !replaced {
			return "", fmt.Errorf("message must contain %q for requested mention", token)
		}
	}
	return content, nil
}

func replaceFirstFold(value, old, replacement string) (string, bool) {
	index := strings.Index(strings.ToLower(value), strings.ToLower(old))
	if index < 0 {
		return value, false
	}
	return value[:index] + replacement + value[index+len(old):], true
}

func formatMessageContent(content, format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "text":
		return "<div><div>" + strings.ReplaceAll(html.EscapeString(content), "\n", "<br/>") + "</div></div>", nil
	case "html":
		return content, nil
	default:
		return "", fmt.Errorf("format must be text or html")
	}
}
