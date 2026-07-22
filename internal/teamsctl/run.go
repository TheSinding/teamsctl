package teamsctl

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	teamsapi "github.com/fossteams/teams-api"
	"github.com/fossteams/teams-api/pkg/csa"
	"github.com/fossteams/teams-api/pkg/models"
	"thesinding/teamsctl/internal/teamsauth"
	"thesinding/teamsctl/internal/version"
)

type Conversation struct {
	Kind      string   `json:"kind"`
	ID        string   `json:"id"`
	IDs       []string `json:"ids"`
	Title     string   `json:"title"`
	TeamID    string   `json:"team_id,omitempty"`
	TeamTitle string   `json:"team_title,omitempty"`
	Unread    bool     `json:"unread"`
	OneOnOne  bool     `json:"one_on_one,omitempty"`
}

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Author         string    `json:"author"`
	Content        string    `json:"content"`
	ContentType    string    `json:"content_type"`
	MessageType    string    `json:"message_type"`
	CreatedAt      time.Time `json:"created_at"`
	Mentions       string    `json:"mentions,omitempty"`
}

type SendOptions struct {
	Format          string
	Mentions        []string
	MentionEntities []MentionEntity
}

type MentionEntity struct {
	Token       string `json:"token"`
	DisplayName string `json:"display_name"`
	MRI         string `json:"mri,omitempty"`
	ObjectID    string `json:"object_id,omitempty"`
}

type mentionWire struct {
	ID          int    `json:"id"`
	MentionType string `json:"mentionType"`
	MRI         string `json:"mri,omitempty"`
	DisplayName string `json:"displayName"`
	ObjectID    string `json:"objectId,omitempty"`
}

type mentionResolution struct {
	Query string
	Wire  mentionWire
}

type stringFlags []string

func (values *stringFlags) String() string { return strings.Join(*values, ",") }
func (values *stringFlags) Set(value string) error {
	*values = append(*values, value)
	return nil
}

type Service struct {
	client        *teamsapi.TeamsClient
	cacheMu       sync.Mutex
	cacheUntil    time.Time
	response      *csa.ConversationResponse
	me            *models.User
	cacheEnriched bool
}

func NewService() (*Service, error) {
	client, err := teamsapi.New()
	if err != nil {
		return nil, fmt.Errorf("initialize Teams client: %w", err)
	}
	return &Service{client: client}, nil
}

func Run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printUsage(stdout)
		return nil
	}
	switch args[0] {
	case "auth":
		return teamsauth.Run(args[1:], stdout)
	case "conversations":
		return runConversations(args[1:], stdout)
	case "messages":
		return runMessages(args[1:], stdout)
	case "send":
		return runSend(args[1:], stdin, stdout)
	case "mcp":
		return RunMCP(stdin, stdout)
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version.Value)
		return nil
	default:
		return fmt.Errorf("unknown command %q; run teamsctl help", args[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  teamsctl auth [flags]            Authenticate using Chrome
  teamsctl conversations           List chats and channels as JSON
  teamsctl messages [flags] ID     Get messages as JSON
  teamsctl send [flags] ID [MESSAGE...] Send a message; reads stdin when omitted
  teamsctl mcp                     Run the stdio MCP server
  teamsctl version                 Print the build version

Messages flags:
  -limit N                         Return the newest N messages (0 = all)
  -name TITLE                      Conversation title used by the Teams API

Send flags:
  -format text|html                Message format (default text)
  -mention NAME                    Resolve @NAME as a Teams mention; repeatable

Auth environment:
  TEAMS_EMAIL, TEAMS_PASSWORD, TEAMS_OTP, CHROME_PATH`)
}

func runConversations(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("conversations", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("conversations takes no arguments")
	}
	service, err := NewService()
	if err != nil {
		return err
	}
	conversations, err := service.Conversations()
	if err != nil {
		return err
	}
	return writeJSON(stdout, conversations)
}

func runMessages(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("messages", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	limit := flags.Int("limit", 50, "newest messages to return; 0 returns all")
	name := flags.String("name", "", "conversation title")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: teamsctl messages [flags] ID")
	}
	if *limit < 0 {
		return fmt.Errorf("limit must be at least 0")
	}
	service, err := NewService()
	if err != nil {
		return err
	}
	messages, err := service.Messages(splitIDs(flags.Arg(0)), *name, *limit)
	if err != nil {
		return err
	}
	return writeJSON(stdout, messages)
}

func runSend(args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("send", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	format := flags.String("format", "text", "message format: text or html")
	var mentions stringFlags
	flags.Var(&mentions, "mention", "person to mention; repeatable")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 1 {
		return fmt.Errorf("usage: teamsctl send [flags] ID [MESSAGE...]")
	}
	content, err := readMessage(flags.Args()[1:], stdin)
	if err != nil {
		return err
	}
	service, err := NewService()
	if err != nil {
		return err
	}
	ids := splitIDs(flags.Arg(0))
	if err = service.Send(ids, content, SendOptions{Format: *format, Mentions: mentions}); err != nil {
		return err
	}
	return writeJSON(stdout, map[string]interface{}{"sent": true, "conversation_ids": ids})
}

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

func (s *Service) Messages(ids []string, displayName string, limit int) ([]Message, error) {
	ids = normalizeIDs(ids)
	if len(ids) == 0 {
		return nil, fmt.Errorf("no conversation id available")
	}
	var messages []csa.ChatMessage
	var lastErr error
	for _, id := range ids {
		messages, lastErr = s.client.GetMessages(&csa.Channel{Id: id, DisplayName: displayName})
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("get messages: %w", lastErr)
	}
	sort.Sort(csa.SortMessageByTime(messages))
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	records := make([]Message, 0, len(messages))
	for _, message := range messages {
		records = append(records, Message{
			ID:             message.Id,
			ConversationID: message.ConversationId,
			Author:         message.ImDisplayName,
			Content:        message.Content,
			ContentType:    message.ContentType,
			MessageType:    message.MessageType,
			CreatedAt:      time.Time(message.OriginalArrivalTime),
			Mentions:       message.Properties.Mentions,
		})
	}
	return records, nil
}

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

func readMessage(args []string, stdin io.Reader) (string, error) {
	content := strings.TrimSpace(strings.Join(args, " "))
	if content == "" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read message from stdin: %w", err)
		}
		content = strings.TrimSpace(string(data))
	}
	if content == "" {
		return "", fmt.Errorf("message is empty")
	}
	return content, nil
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

func writeJSON(w io.Writer, value interface{}) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
