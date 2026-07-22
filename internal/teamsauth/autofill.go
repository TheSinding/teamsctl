package teamsauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func startAuthAutofill(ctx context.Context, stdout io.Writer, options Options) func() {
	autofillContext, cancel := context.WithCancel(ctx)
	submitted := map[string]bool{}
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-autofillContext.Done():
				return
			case <-ticker.C:
				fillAuthLogin(autofillContext, submitted, options, stdout)
			}
		}
	}()
	return cancel
}

func fillAuthLogin(ctx context.Context, submitted map[string]bool, options Options, stdout io.Writer) {
	var location string
	if err := chromedp.Run(ctx, chromedp.Location(&location)); err != nil {
		return
	}
	parsed, err := url.Parse(location)
	if err != nil || (parsed.Hostname() != "login.live.com" && !strings.HasSuffix(parsed.Hostname(), ".microsoftonline.com")) {
		return
	}
	fields := []struct {
		name     string
		selector string
		value    string
	}{
		{"email", `input[name="loginfmt"], input[type="email"]`, options.Email},
		{"password", `input[name="passwd"], input[type="password"]`, options.Password},
		{"otp", `input[name="otc"], input[id="idTxtBx_SAOTCC_OTC"], input[autocomplete="one-time-code"]`, options.OTP},
	}
	for _, field := range fields {
		if field.value == "" || submitted[field.name] {
			continue
		}
		filled, fillErr := fillAuthField(ctx, field.selector, field.value)
		if fillErr != nil {
			if ctx.Err() == nil {
				fmt.Fprintf(stdout, "Login autofill paused: %v\n", fillErr)
			}
			return
		}
		if filled {
			submitted[field.name] = true
			return
		}
	}
}

func fillAuthField(ctx context.Context, selector, value string) (bool, error) {
	selectorJSON, _ := json.Marshal(selector)
	valueJSON, _ := json.Marshal(value)
	script := fmt.Sprintf(`(() => {
		const input = document.querySelector(%s);
		if (!input) return false;
		input.focus();
		const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set;
		setter.call(input, %s);
		input.dispatchEvent(new Event("input", { bubbles: true }));
		input.dispatchEvent(new Event("change", { bubbles: true }));
		if (input.form && input.form.requestSubmit) input.form.requestSubmit();
		else input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", code: "Enter", bubbles: true }));
		return true;
	})()`, selectorJSON, valueJSON)
	var filled bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &filled)); err != nil {
		return false, err
	}
	return filled, nil
}
