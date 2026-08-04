# MCP SDK and Recipient Resolution Design

## Goal

Replace the hand-written MCP JSON-RPC server with the official Go MCP SDK while
preserving the existing Teams service layer. Make tool behavior clear and safe
when an agent supplies a human name instead of a conversation ID.

## Architecture

`teamsctl mcp` will construct an SDK `mcp.Server`, register typed tools, and
run it through `mcp.StdioTransport`. The SDK owns initialization, protocol
negotiation, JSON-RPC framing, input schema generation, and tool dispatch.

The existing `Service`, Teams authentication, conversation lookup, message
fetching, and send logic remain application code. Tool handlers create the
service lazily after token validation and call those existing methods.

The manual request/response models, `RunMCP` JSON decoder/encoder loop,
`mcpTools`, and `callTool` dispatch are removed.

## Tool Contract

Typed SDK registrations expose names, descriptions, required fields, enums,
defaults, and JSON schemas through `tools/list`.

`list_conversations`, `get_latest_message`, and `get_messages` retain their
current behavior.

Tools accept a recipient phrase from the user request instead of requiring a
conversation ID. Resolution follows intent:

- A single person (`Mike`) resolves to the best matching one-to-one chat.
- A multi-person phrase (`Mike and Charlie`) resolves to an existing group chat
  containing those names.
- A group, channel, or thread title (`ASM group chat`, `ASM channel`) resolves
  to the matching existing conversation.

When a requested multi-person group chat does not exist, `send_message`
resolves every named recipient to a one-to-one chat, sends the message to each,
and reports that it used the individual-message fallback. It never creates a
new group chat. Read operations return a clear no-group-chat error instead of
falling back to multiple conversations.

## Authentication and Errors

Authentication is checked before initialization and every tool operation.
Authentication and application errors are returned as SDK tool errors. The
server process continues to use stdio and emits protocol messages only on
stdout.

## Tests

Replace manual JSON-RPC handshake/schema tests with SDK integration tests over
an in-memory transport where practical. Preserve coverage for tool discovery,
authentication failure, dispatch, and errors. Add tests for send-target
resolution: single-person, multi-person, group-title, and channel-title
matching; missing-group send fallback; and no-group read errors.
