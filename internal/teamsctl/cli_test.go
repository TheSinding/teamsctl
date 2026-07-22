package teamsctl

import (
	"strings"
	"testing"
)

func TestReadMessage(t *testing.T) {
	message, err := readMessage([]string{"hello", "world"}, strings.NewReader("ignored"))
	if err != nil || message != "hello world" {
		t.Fatalf("readMessage() = %q, %v", message, err)
	}
	message, err = readMessage(nil, strings.NewReader("from stdin\n"))
	if err != nil || message != "from stdin" {
		t.Fatalf("readMessage(stdin) = %q, %v", message, err)
	}
}
