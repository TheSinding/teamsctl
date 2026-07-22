package teamsauth

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func Run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("auth", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	email := flags.String("email", os.Getenv("TEAMS_EMAIL"), "Microsoft login email")
	chrome := flags.String("chrome", os.Getenv("CHROME_PATH"), "Chrome executable")
	timeout := flags.Duration("timeout", 5*time.Minute, "timeout for each token")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: teams-cli auth [flags]")
	}
	if *timeout <= 0 {
		return fmt.Errorf("timeout must be greater than zero")
	}

	options := OptionsFromEnvironment(*email)
	options.ChromePath = strings.TrimSpace(*chrome)
	options.Timeout = *timeout
	return Authenticate(context.Background(), stdout, options)
}

func OptionsFromEnvironment(email string) Options {
	options := Options{
		Email:      strings.TrimSpace(email),
		Password:   os.Getenv("TEAMS_PASSWORD"),
		OTP:        os.Getenv("TEAMS_OTP"),
		ChromePath: strings.TrimSpace(os.Getenv("CHROME_PATH")),
		Timeout:    5 * time.Minute,
	}
	_ = os.Unsetenv("TEAMS_PASSWORD")
	_ = os.Unsetenv("TEAMS_OTP")
	return options
}
