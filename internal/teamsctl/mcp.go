package teamsctl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"thesinding/teamsctl/internal/teamsauth"
	"thesinding/teamsctl/internal/version"
)

var checkMCPAuth = teamsauth.CheckTokens

func RunMCP(stdin io.Reader, stdout io.Writer) error {
	decoder := json.NewDecoder(bufio.NewReader(stdin))
	encoder := json.NewEncoder(stdout)
	var service *Service
	for {
		var request rpcRequest
		if err := decoder.Decode(&request); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("decode MCP request: %w", err)
		}
		if len(request.ID) == 0 {
			continue
		}
		response := rpcResponse{JSONRPC: "2.0", ID: request.ID}
		switch request.Method {
		case "initialize":
			if err := checkMCPAuth(); err != nil {
				response.Error = &rpcError{Code: -32001, Message: err.Error()}
				break
			}
			var params struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(request.Params, &params)
			if params.ProtocolVersion == "" {
				params.ProtocolVersion = "2024-11-05"
			}
			response.Result = map[string]interface{}{
				"protocolVersion": params.ProtocolVersion,
				"capabilities":    map[string]interface{}{"tools": map[string]bool{"listChanged": false}},
				"serverInfo":      map[string]string{"name": "teamsctl", "version": version.Value},
			}
		case "ping":
			response.Result = map[string]interface{}{}
		case "tools/list":
			response.Result = map[string]interface{}{"tools": mcpTools()}
		case "tools/call":
			if err := checkMCPAuth(); err != nil {
				response.Result = errorToolResult(err)
				break
			}
			if service == nil {
				var err error
				service, err = NewService()
				if err != nil {
					response.Result = errorToolResult(err)
					break
				}
			}
			result, err := callTool(service, request.Params)
			if err != nil {
				response.Result = errorToolResult(err)
			} else {
				response.Result = result
			}
		default:
			response.Error = &rpcError{Code: -32601, Message: "method not found"}
		}
		if err := encoder.Encode(response); err != nil {
			return fmt.Errorf("encode MCP response: %w", err)
		}
	}
}
