package teamsctl

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPListsTypedTools(t *testing.T) {
	originalCheck := checkMCPAuth
	checkMCPAuth = func() error { return nil }
	defer func() { checkMCPAuth = originalCheck }()

	session, closeSession := connectMCP(t)
	defer closeSession()
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 4 {
		t.Fatalf("tools count = %d", len(result.Tools))
	}
	tools := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		tools[tool.Name] = tool
	}
	for _, name := range []string{"list_conversations", "get_latest_message", "get_messages", "send_message"} {
		if tools[name] == nil || tools[name].InputSchema == nil {
			t.Errorf("tool %q was not described with an input schema", name)
		}
	}
	if !strings.Contains(tools["send_message"].Description, "send individually") {
		t.Fatalf("send_message description = %q", tools["send_message"].Description)
	}
}

func TestMCPInitializeRejectsExpiredAuth(t *testing.T) {
	originalCheck := checkMCPAuth
	checkMCPAuth = func() error { return errors.New("teams token expired; run teamsctl auth") }
	defer func() { checkMCPAuth = originalCheck }()

	session, closeSession := connectMCP(t)
	defer closeSession()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_latest_message",
		Arguments: map[string]any{"query": "Mikkel"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "teams token expired") {
		t.Fatalf("CallTool() = %#v", result)
	}
}

func connectMCP(t *testing.T) (*mcp.ClientSession, func()) {
	t.Helper()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := newMCPServer().Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "teamsctl-test"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	return clientSession, func() {
		_ = clientSession.Close()
		_ = serverSession.Wait()
	}
}
