package teamsctl

import (
	"encoding/json"
	"time"
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

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}
