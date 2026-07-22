package teamsctl

import (
	"encoding/json"
	"fmt"
)

func mcpTools() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        "list_conversations",
			"description": "Find Microsoft Teams chats and channels by title. Use query and kind instead of listing everything when looking for a person.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]string{"type": "string", "description": "Case-insensitive title or team-name substring."},
					"kind":  map[string]interface{}{"type": "string", "enum": []string{"chat", "channel"}},
					"limit": map[string]interface{}{"type": "integer", "minimum": 0, "default": 50},
				},
				"additionalProperties": false,
			},
		},
		{
			"name":        "get_latest_message",
			"description": "Find the best matching one-to-one chat by person or title and return its latest message in one call.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]string{"type": "string", "description": "Person name or chat-title substring."},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
			},
		},
		{
			"name":        "get_messages",
			"description": "Get recent messages from a chat or channel.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"conversation_id": map[string]string{"type": "string", "description": "Conversation ID, or comma-separated candidate IDs."},
					"limit":           map[string]interface{}{"type": "integer", "minimum": 0, "default": 50},
					"name":            map[string]string{"type": "string", "description": "Optional conversation title."},
				},
				"required":             []string{"conversation_id"},
				"additionalProperties": false,
			},
		},
		{
			"name":        "send_message",
			"description": "Send a message to a Microsoft Teams chat or channel. Use format=html for any structured or complex message. IMPORTANT: HTML such as <strong>@Name</strong> is only styled text and never creates a Teams mention. For every intended real mention, the message MUST contain @Name and the matching name MUST be included in mentions. Unlisted @ text remains plain text.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"conversation_id": map[string]string{"type": "string", "description": "Conversation ID, or comma-separated candidate IDs."},
					"message":         map[string]string{"type": "string"},
					"format": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"text", "html"},
						"default":     "text",
						"description": "Choose html for formatted or multi-part messages; choose text only for simple unformatted text.",
					},
					"mentions": map[string]interface{}{
						"type":        "array",
						"items":       map[string]string{"type": "string"},
						"description": "REQUIRED for real Teams mentions. List every intended mentioned person. Each value must have a matching @Name token in message, for example mentions=[\"Mikkel\"] with message=\"Hi @Mikkel\". Without this field, @Name remains plain text even in HTML.",
					},
					"mention_entities": map[string]interface{}{
						"type":        "array",
						"description": "Advanced pre-resolved mentions supplied by the consumer. Prefer mentions for automatic resolution.",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"token":        map[string]string{"type": "string", "description": "Matching @token in message; defaults to display_name."},
								"display_name": map[string]string{"type": "string"},
								"mri":          map[string]string{"type": "string"},
								"object_id":    map[string]string{"type": "string"},
							},
							"required":             []string{"display_name"},
							"additionalProperties": false,
						},
					},
				},
				"required":             []string{"conversation_id", "message"},
				"additionalProperties": false,
			},
		},
	}
}

func callTool(service *Service, rawParams json.RawMessage) (toolResult, error) {
	var request struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(rawParams, &request); err != nil {
		return toolResult{}, fmt.Errorf("invalid tool request: %w", err)
	}
	var value interface{}
	switch request.Name {
	case "list_conversations":
		var args struct {
			Query string `json:"query"`
			Kind  string `json:"kind"`
			Limit *int   `json:"limit"`
		}
		if err := json.Unmarshal(request.Arguments, &args); err != nil {
			return toolResult{}, fmt.Errorf("invalid list_conversations arguments: %w", err)
		}
		limit := 50
		if args.Limit != nil {
			limit = *args.Limit
		}
		conversations, err := service.FindConversations(args.Query, args.Kind, limit)
		if err != nil {
			return toolResult{}, err
		}
		value = conversations
	case "get_latest_message":
		var args struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(request.Arguments, &args); err != nil {
			return toolResult{}, fmt.Errorf("invalid get_latest_message arguments: %w", err)
		}
		if args.Query == "" {
			return toolResult{}, fmt.Errorf("query is required")
		}
		conversations, err := service.FindConversations(args.Query, "chat", 10)
		if err != nil {
			return toolResult{}, err
		}
		if len(conversations) == 0 {
			return toolResult{}, fmt.Errorf("no chat found matching %q", args.Query)
		}
		messages, err := service.Messages(conversations[0].IDs, conversations[0].Title, 1)
		if err != nil {
			return toolResult{}, err
		}
		var latest interface{}
		if len(messages) > 0 {
			latest = messages[0]
		}
		value = map[string]interface{}{"conversation": conversations[0], "message": latest}
	case "get_messages":
		var args struct {
			ConversationID string `json:"conversation_id"`
			Limit          *int   `json:"limit"`
			Name           string `json:"name"`
		}
		if err := json.Unmarshal(request.Arguments, &args); err != nil {
			return toolResult{}, fmt.Errorf("invalid get_messages arguments: %w", err)
		}
		if args.ConversationID == "" {
			return toolResult{}, fmt.Errorf("conversation_id is required")
		}
		limit := 50
		if args.Limit != nil {
			limit = *args.Limit
		}
		messages, err := service.Messages(splitIDs(args.ConversationID), args.Name, limit)
		if err != nil {
			return toolResult{}, err
		}
		value = messages
	case "send_message":
		var args struct {
			ConversationID  string          `json:"conversation_id"`
			Message         string          `json:"message"`
			Format          string          `json:"format"`
			Mentions        []string        `json:"mentions"`
			MentionEntities []MentionEntity `json:"mention_entities"`
		}
		if err := json.Unmarshal(request.Arguments, &args); err != nil {
			return toolResult{}, fmt.Errorf("invalid send_message arguments: %w", err)
		}
		if args.ConversationID == "" || args.Message == "" {
			return toolResult{}, fmt.Errorf("conversation_id and message are required")
		}
		if err := service.Send(splitIDs(args.ConversationID), args.Message, SendOptions{
			Format: args.Format, Mentions: args.Mentions, MentionEntities: args.MentionEntities,
		}); err != nil {
			return toolResult{}, err
		}
		value = map[string]bool{"sent": true}
	default:
		return toolResult{}, fmt.Errorf("unknown tool %q", request.Name)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return toolResult{}, err
	}
	return toolResult{Content: []toolContent{{Type: "text", Text: string(encoded)}}}, nil
}

func errorToolResult(err error) toolResult {
	return toolResult{Content: []toolContent{{Type: "text", Text: err.Error()}}, IsError: true}
}
