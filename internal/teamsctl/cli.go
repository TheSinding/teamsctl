package teamsctl

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"thesinding/teamsctl/internal/version"
	"thesinding/teamsctl/pkg/teamsauth"
	tctl "thesinding/teamsctl/pkg/teamsctl"
)

type stringFlags []string

func (values *stringFlags) String() string { return strings.Join(*values, ",") }
func (values *stringFlags) Set(value string) error {
	*values = append(*values, value)
	return nil
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
		_, _ = fmt.Fprintln(stdout, version.Value)
		return nil
	default:
		return fmt.Errorf("unknown command %q; run teamsctl help", args[0])
	}
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, `Usage:
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
	service, err := tctl.NewService()
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
	service, err := tctl.NewService()
	if err != nil {
		return err
	}
	messages, err := service.Messages(tctl.SplitIDs(flags.Arg(0)), *name, *limit)
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
	service, err := tctl.NewService()
	if err != nil {
		return err
	}
	ids := tctl.SplitIDs(flags.Arg(0))
	if err = service.Send(ids, content, tctl.SendOptions{Format: *format, Mentions: mentions}); err != nil {
		return err
	}
	return writeJSON(stdout, map[string]interface{}{"sent": true, "conversation_ids": ids})
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

func writeJSON(w io.Writer, value interface{}) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
