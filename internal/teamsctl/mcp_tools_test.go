package teamsctl

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestMCPListsTools(t *testing.T) {
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var output bytes.Buffer
	if err := RunMCP(input, &output); err != nil {
		t.Fatal(err)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	listed := response["result"].(map[string]interface{})["tools"].([]interface{})
	if len(listed) != 4 {
		t.Fatalf("tools count = %d", len(listed))
	}
}

func TestMCPToolSchemas(t *testing.T) {
	tools := mcpTools()
	if len(tools) != 4 {
		t.Fatalf("tools count = %d", len(tools))
	}

	wantNames := []string{"list_conversations", "get_latest_message", "get_messages", "send_message"}
	for i, tool := range tools {
		if tool["name"] != wantNames[i] {
			t.Errorf("tool %d name = %v, want %q", i, tool["name"], wantNames[i])
		}
		schema, ok := tool["inputSchema"].(map[string]interface{})
		if !ok {
			t.Fatalf("tool %q inputSchema has type %T", wantNames[i], tool["inputSchema"])
		}
		if schema["type"] != "object" || schema["additionalProperties"] != false {
			t.Errorf("tool %q has open or non-object schema: %#v", wantNames[i], schema)
		}
	}
}
