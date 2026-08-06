package teamsctl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"thesinding/teamsctl/internal/version"
	"thesinding/teamsctl/pkg/teamsauth"
	tctl "thesinding/teamsctl/pkg/teamsctl"
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
	Recipient       string               `json:"recipient,omitempty" jsonschema:"Recipient phrase from the user, such as Mike, Mike and Charlie, ASM group chat, or ASM channel."`
	ConversationID  string               `json:"conversation_id,omitempty" jsonschema:"Deprecated: use recipient. A Teams conversation ID remains accepted."`
	Message         string               `json:"message" jsonschema:"Message content."`
	Format          string               `json:"format,omitempty" jsonschema:"Message format: text or html. Use html for structured or formatted messages."`
	Mentions        []string             `json:"mentions,omitempty" jsonschema:"People to mention. Each must match an @Name token in message."`
	MentionEntities []tctl.MentionEntity `json:"mention_entities,omitempty" jsonschema:"Pre-resolved Teams mentions. Prefer mentions for automatic resolution."`
}

type mcpApplication struct {
	serviceMu sync.Mutex
	service   *tctl.Service
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

func (app *mcpApplication) serviceForTool() (*tctl.Service, error) {
	if err := checkMCPAuth(); err != nil {
		return nil, err
	}
	app.serviceMu.Lock()
	defer app.serviceMu.Unlock()
	if app.service == nil {
		service, err := tctl.NewService()
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
	conversation, err := service.FindOneOnOneConversation(input.Query)
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
	target, err := service.ResolveConversationTarget(firstNonEmpty(input.Recipient, input.ConversationID))
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
	target, err := service.ResolveConversationTarget(firstNonEmpty(input.Recipient, input.ConversationID))
	if err != nil {
		var missingGroup *tctl.MissingGroupChatError
		if !errors.As(err, &missingGroup) {
			return nil, nil, err
		}
		target, err = service.ResolveIndividualTargets(missingGroup.Recipients)
		if err != nil {
			return nil, nil, err
		}
		target.FallbackToOneOnOne = true
	}
	options := tctl.SendOptions{Format: input.Format, Mentions: input.Mentions, MentionEntities: input.MentionEntities}
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

func limitOrDefault(limit *int) int {
	if limit == nil {
		return 50
	}
	return *limit
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
