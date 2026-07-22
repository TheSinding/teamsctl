package main

import (
	"fmt"
	"os"

	"thesinding/teamsctl/internal/teamsctl"
)

func main() {
	if err := teamsctl.Run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
