package teamsctl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"thesinding/teamsctl/internal/teamsauth"
	"thesinding/teamsctl/internal/version"
)

var checkMCPAuth = teamsauth.CheckTokens

type listConversationsInput struct {
	Query string `json:"query,omitempty" jsonschema:"Case-insensitive title or team-name substring."`
	Kind  string `json:"kind,omitempty" jsonschema:"Conversation kind: chat or channel."`
	Limit *int   `json:"limit,omitempty" jsonschema:"Maximum number of conversations to return. Omit for 50; use zero for all."`
}

type latestMessageInput struct {
	Query string `json:"query" jsonschema:"Name of a person or one-to-one chat title. A bare name always means a one-to-one chat."`
}

type messagesInput struct {
	Recipient      string `json:"recipient,omitempty" jsonschema:"Recipient phrase from the user, such as Mike, Mike and Charlie, ASM group chat, or ASM channel."`
	ConversationID string `json:"conversation_id,omitempty" jsonschema:"Deprecated: use recipient. A Teams conversation ID remains accepted."`
	Limit          *int   `json:"limit,omitempty" jsonschema:"Maximum number of messages to return. Omit for 50; use zero for all."`
}

type sendMessageInput struct {
	Recipient       string          `json:"recipient,omitempty" jsonschema:"Recipient phrase from the user, such as Mike, Mike and Charlie, ASM group chat, or ASM channel."`
	ConversationID  string          `json:"conversation_id,omitempty" jsonschema:"Deprecated: use recipient. A Teams conversation ID remains accepted."`
	Message         string          `json:"message" jsonschema:"Message content."`
	Format          string          `json:"format,omitempty" jsonschema:"Message format: text or html. Use html for structured or formatted messages."`
	Mentions        []string        `json:"mentions,omitempty" jsonschema:"People to mention. Each must match an @Name token in message."`
	MentionEntities []MentionEntity `json:"mention_entities,omitempty" jsonschema:"Pre-resolved Teams mentions. Prefer mentions for automatic resolution."`
}

type mcpApplication struct {
	serviceMu sync.Mutex
	service   *Service
}

func RunMCP(stdin io.Reader, stdout io.Writer) error {
	return newMCPServer().Run(context.Background(), &mcp.IOTransport{
		Reader: io.NopCloser(stdin),
		Writer: nopWriteCloser{Writer: stdout},
	})
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func newMCPServer() *mcp.Server {
	app := &mcpApplication{}
	server := mcp.NewServer(&mcp.Implementation{Name: "teamsctl", Version: version.Value}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
	})
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			if method == "initialize" {
				if err := checkMCPAuth(); err != nil {
					return nil, err
				}
			}
			return next(ctx, method, request)
		}
	})
	mcp.AddTool(server, &mcp.Tool{Name: "list_conversations", Description: "Find Microsoft Teams chats and channels by title. Use query and kind instead of listing everything when looking for a person."}, app.listConversations)
	mcp.AddTool(server, &mcp.Tool{Name: "get_latest_message", Description: "Find a one-to-one chat by person or title and return its latest message. A bare name always means a one-to-one chat."}, app.latestMessage)
	mcp.AddTool(server, &mcp.Tool{Name: "get_messages", Description: "Get recent messages using a recipient phrase: Mike for one-to-one, Mike and Charlie for their group chat, ASM group chat, or ASM channel. Do not require a conversation ID."}, app.messages)
	mcp.AddTool(server, &mcp.Tool{Name: "send_message", Description: "Send a message using a recipient phrase: Mike for one-to-one, Mike and Charlie for their group chat, ASM group chat, or ASM channel. If a requested multi-person group does not exist, send individually and report the fallback. Use format=html for structured messages. A real Teams mention requires @Name in message and the matching name in mentions."}, app.sendMessage)
	return server
}

func (app *mcpApplication) serviceForTool() (*Service, error) {
	if err := checkMCPAuth(); err != nil {
		return nil, err
	}
	app.serviceMu.Lock()
	defer app.serviceMu.Unlock()
	if app.service == nil {
		service, err := NewService()
		if err != nil {
			return nil, err
		}
		app.service = service
	}
	return app.service, nil
}

func (app *mcpApplication) listConversations(_ context.Context, _ *mcp.CallToolRequest, input listConversationsInput) (*mcp.CallToolResult, any, error) {
	service, err := app.serviceForTool()
	if err != nil {
		return nil, nil, err
	}
	conversations, err := service.FindConversations(input.Query, input.Kind, limitOrDefault(input.Limit))
	return nil, conversations, err
}

func (app *mcpApplication) latestMessage(_ context.Context, _ *mcp.CallToolRequest, input latestMessageInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, nil, fmt.Errorf("query is required")
	}
	service, err := app.serviceForTool()
	if err != nil {
		return nil, nil, err
	}
	conversation, err := service.findOneOnOneConversation(input.Query)
	if err != nil {
		return nil, nil, err
	}
	messages, err := service.Messages(conversation.IDs, conversation.Title, 1)
	if err != nil {
		return nil, nil, err
	}
	var latest any
	if len(messages) > 0 {
		latest = messages[0]
	}
	return nil, map[string]any{"conversation": conversation, "message": latest}, nil
}

func (app *mcpApplication) messages(_ context.Context, _ *mcp.CallToolRequest, input messagesInput) (*mcp.CallToolResult, any, error) {
	service, err := app.serviceForTool()
	if err != nil {
		return nil, nil, err
	}
	target, err := service.resolveConversationTarget(firstNonEmpty(input.Recipient, input.ConversationID))
	if err != nil {
		return nil, nil, err
	}
	messages, err := service.Messages(target.IDs, target.Name, limitOrDefault(input.Limit))
	return nil, messages, err
}

func (app *mcpApplication) sendMessage(_ context.Context, _ *mcp.CallToolRequest, input sendMessageInput) (*mcp.CallToolResult, any, error) {
	service, err := app.serviceForTool()
	if err != nil {
		return nil, nil, err
	}
	target, err := service.resolveConversationTarget(firstNonEmpty(input.Recipient, input.ConversationID))
	if err != nil {
		var missingGroup *missingGroupChatError
		if !errors.As(err, &missingGroup) {
			return nil, nil, err
		}
		target, err = service.resolveIndividualTargets(missingGroup.Recipients)
		if err != nil {
			return nil, nil, err
		}
		target.FallbackToOneOnOne = true
	}
	options := SendOptions{Format: input.Format, Mentions: input.Mentions, MentionEntities: input.MentionEntities}
	if target.FallbackToOneOnOne {
		for _, ids := range target.IndividualIDs {
			if err := service.Send(ids, input.Message, options); err != nil {
				return nil, nil, err
			}
		}
	} else if err := service.Send(target.IDs, input.Message, options); err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"sent": true, "sent_to": target.Recipients, "fallback_to_one_on_one": target.FallbackToOneOnOne}, nil
}

type conversationTarget struct {
	IDs                []string
	IndividualIDs      [][]string
	Name               string
	Recipients         []string
	FallbackToOneOnOne bool
}

type missingGroupChatError struct{ Recipients []string }

func (e *missingGroupChatError) Error() string {
	return fmt.Sprintf("no group chat found for %s", strings.Join(e.Recipients, " and "))
}

func (s *Service) resolveConversationTarget(target string) (conversationTarget, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return conversationTarget{}, fmt.Errorf("recipient is required")
	}
	if looksLikeConversationID(target) {
		return conversationTarget{IDs: splitIDs(target), Recipients: []string{target}}, nil
	}
	if recipients := splitRecipientNames(target); len(recipients) > 1 {
		conversation, err := s.findGroupConversation(recipients)
		if err != nil {
			return conversationTarget{}, err
		}
		if len(conversation.IDs) == 0 {
			return conversationTarget{}, &missingGroupChatError{Recipients: recipients}
		}
		return conversationTarget{IDs: conversation.IDs, Name: conversation.Title, Recipients: []string{conversation.Title}}, nil
	}
	if query, kind := namedConversationQuery(target); kind != "" {
		conversation, err := s.findNamedConversation(query, kind)
		if err != nil {
			return conversationTarget{}, err
		}
		return conversationTarget{IDs: conversation.IDs, Name: conversation.Title, Recipients: []string{conversation.Title}}, nil
	}
	conversation, err := s.findOneOnOneConversation(target)
	if err != nil {
		return conversationTarget{}, err
	}
	return conversationTarget{IDs: conversation.IDs, Name: conversation.Title, Recipients: []string{conversation.Title}}, nil
}

func (s *Service) resolveIndividualTargets(recipients []string) (conversationTarget, error) {
	individualIDs := make([][]string, 0, len(recipients))
	resolved := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		conversation, err := s.findOneOnOneConversation(recipient)
		if err != nil {
			return conversationTarget{}, err
		}
		individualIDs = append(individualIDs, conversation.IDs)
		resolved = append(resolved, conversation.Title)
	}
	return conversationTarget{IndividualIDs: individualIDs, Recipients: resolved}, nil
}

func (s *Service) findOneOnOneConversation(query string) (Conversation, error) {
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

func (s *Service) findGroupConversation(recipients []string) (Conversation, error) {
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
	ids := splitIDs(target)
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

func limitOrDefault(limit *int) int {
	if limit == nil {
		return 50
	}
	return *limit
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
