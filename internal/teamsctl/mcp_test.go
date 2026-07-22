package teamsctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestMCPHandshake(t *testing.T) {
	originalCheck := checkMCPAuth
	checkMCPAuth = func() error { return nil }
	defer func() { checkMCPAuth = originalCheck }()
	input := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n",
	)
	var output bytes.Buffer
	if err := RunMCP(input, &output); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	var initialize map[string]interface{}
	if err := decoder.Decode(&initialize); err != nil {
		t.Fatal(err)
	}
	result := initialize["result"].(map[string]interface{})
	if result["protocolVersion"] != "2025-03-26" {
		t.Fatalf("protocolVersion = %v", result["protocolVersion"])
	}
}

func TestMCPInitializeRejectsExpiredAuth(t *testing.T) {
	originalCheck := checkMCPAuth
	checkMCPAuth = func() error { return errors.New("teams token expired; run teamsctl auth") }
	defer func() { checkMCPAuth = originalCheck }()

	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	var output bytes.Buffer
	if err := RunMCP(input, &output); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || response.Error.Code != -32001 {
		t.Fatalf("unexpected response: %s", output.String())
	}
}
