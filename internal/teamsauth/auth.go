package teamsauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	cdppage "github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const (
	authMicrosoftTenantID = "f8cdef31-a31e-4b4a-93e4-5f571e91255a"
	authTeamsAppID        = "5e3ce6c0-2b1f-4285-8d4b-75ee78787346"
	authSkypeResource     = "https://api.spaces.skype.com"
	authChatResource      = "https://chatsvcagg.teams.microsoft.com"
	authRedirectURI       = "https://teams.microsoft.com/go"
	authUserAgent         = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) MicrosoftTeams-Preview/1.4.00.7556 Chrome/80.0.3987.163 Electron/8.5.5 Safari/537.36"
)

type authTokenKind string

const (
	authTeams      authTokenKind = "teams"
	authSkype      authTokenKind = "skype"
	authChatSvcAgg authTokenKind = "chatsvcagg"
)

type Options struct {
	Email      string
	Password   string
	OTP        string
	ChromePath string
	Timeout    time.Duration
}

type authClaims struct {
	Audience  string `json:"aud"`
	TenantID  string `json:"tid"`
	ExpiresAt int64  `json:"exp"`
}

type authTenant struct {
	TenantID string `json:"tenantId"`
}

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

func Authenticate(parent context.Context, stdout io.Writer, options Options) error {
	executablePath := options.ChromePath
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	configDir, err := authConfigDir()
	if err != nil {
		return err
	}
	if executablePath == "" {
		executablePath, err = findChrome()
		if err != nil {
			return err
		}
	}
	if _, err = os.Stat(executablePath); err != nil {
		return fmt.Errorf("Chrome executable %q: %w", executablePath, err)
	}
	if err = os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("create auth config directory: %w", err)
	}

	allocatorOptions := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(executablePath),
		chromedp.UserDataDir(filepath.Join(configDir, "chrome-profile")),
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
	)
	allocatorContext, cancelAllocator := chromedp.NewExecAllocator(parent, allocatorOptions...)
	defer cancelAllocator()
	browserContext, cancelBrowser := chromedp.NewContext(allocatorContext)
	defer cancelBrowser()

	navigationEvents := make(chan string, 32)
	chromedp.ListenTarget(browserContext, func(event interface{}) {
		emit := func(candidate string) {
			if !strings.HasPrefix(candidate, authRedirectURI) {
				return
			}
			select {
			case navigationEvents <- candidate:
			default:
			}
		}
		switch navigated := event.(type) {
		case *cdppage.EventFrameNavigated:
			emit(navigated.Frame.URL)
		case *cdppage.EventNavigatedWithinDocument:
			emit(navigated.URL)
		case *cdppage.EventFrameRequestedNavigation:
			emit(navigated.URL)
		case *network.EventRequestWillBeSent:
			if navigated.Request != nil {
				emit(navigated.Request.URL)
			}
			if navigated.RedirectResponse != nil {
				for name, value := range navigated.RedirectResponse.Headers {
					if strings.EqualFold(name, "location") {
						if location, ok := value.(string); ok {
							emit(location)
						}
					}
				}
			}
		}
	})
	if err = chromedp.Run(browserContext,
		network.Enable(),
		emulation.SetUserAgentOverride(authUserAgent),
	); err != nil {
		return fmt.Errorf("start Chrome: %w", err)
	}

	stopAutofill := startAuthAutofill(browserContext, stdout, options)
	defer stopAutofill()

	tenantID := "common"
	teamsToken, err := captureAuthToken(browserContext, navigationEvents, authTeams, tenantID, options.Email, timeout, stdout)
	if err != nil {
		return err
	}
	teamsClaims, err := decodeAuthClaims(teamsToken)
	if err != nil {
		return err
	}
	if teamsClaims.Audience != authTeamsAppID {
		return fmt.Errorf("unexpected Teams token audience %q", teamsClaims.Audience)
	}
	if err = saveAuthToken(configDir, authTeams, teamsToken); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Saved teams token.")
	if teamsClaims.TenantID != "" && teamsClaims.TenantID != authMicrosoftTenantID {
		tenantID = teamsClaims.TenantID
	}

	skypeToken, err := captureAuthToken(browserContext, navigationEvents, authSkype, tenantID, options.Email, timeout, stdout)
	if err != nil {
		return err
	}
	skypeClaims, err := decodeAuthClaims(skypeToken)
	if err != nil {
		return err
	}
	if skypeClaims.TenantID == authMicrosoftTenantID {
		tenants, tenantErr := getAuthTenants(skypeToken)
		if tenantErr != nil {
			return tenantErr
		}
		if len(tenants) == 0 {
			return fmt.Errorf("account has no Teams tenants")
		}
		tenantID = tenants[0].TenantID
		skypeToken, err = captureAuthToken(browserContext, navigationEvents, authSkype, tenantID, options.Email, timeout, stdout)
		if err != nil {
			return err
		}
		skypeClaims, err = decodeAuthClaims(skypeToken)
		if err != nil {
			return err
		}
	}
	if skypeClaims.Audience != authSkypeResource {
		return fmt.Errorf("unexpected Skype token audience %q", skypeClaims.Audience)
	}
	if err = saveAuthToken(configDir, authSkype, skypeToken); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Saved skype token.")

	chatToken, err := captureAuthToken(browserContext, navigationEvents, authChatSvcAgg, tenantID, options.Email, timeout, stdout)
	if err != nil {
		return err
	}
	chatClaims, err := decodeAuthClaims(chatToken)
	if err != nil {
		return err
	}
	if chatClaims.Audience != authChatResource {
		return fmt.Errorf("unexpected ChatSvcAgg token audience %q", chatClaims.Audience)
	}
	if err = saveAuthToken(configDir, authChatSvcAgg, chatToken); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Saved chatsvcagg token.")
	fmt.Fprintln(stdout, "Authentication complete.")
	return nil
}

func captureAuthToken(ctx context.Context, events <-chan string, kind authTokenKind, tenantID, email string, timeout time.Duration, stdout io.Writer) (string, error) {
	authorizationURL, expectedState, err := authLoginURL(kind, tenantID, email)
	if err != nil {
		return "", err
	}
	for {
		select {
		case <-events:
		default:
			goto drained
		}
	}

drained:
	fmt.Fprintf(stdout, "Authorizing %s with tenant %s...\n", kind, tenantID)
	navigationContext, cancelNavigation := context.WithCancel(ctx)
	defer cancelNavigation()
	navigationDone := make(chan error, 1)
	go func() {
		navigationDone <- chromedp.Run(navigationContext, chromedp.Navigate(authorizationURL))
	}()
	var navigationErr error
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
			if navigationErr != nil {
				return "", fmt.Errorf("open %s authorization: %w", kind, navigationErr)
			}
			return "", fmt.Errorf("%s authorization timed out", kind)
		case navigationResult := <-navigationDone:
			if navigationResult != nil && !strings.Contains(navigationResult.Error(), "net::ERR_ABORTED") {
				navigationErr = navigationResult
			}
			navigationDone = nil
		case navigatedURL := <-events:
			if os.Getenv("TEAMS_AUTH_DEBUG") != "" && strings.HasPrefix(navigatedURL, authRedirectURI+"#") {
				values, _ := url.ParseQuery(strings.TrimPrefix(navigatedURL, authRedirectURI+"#"))
				keys := make([]string, 0, len(values))
				for key := range values {
					keys = append(keys, key)
				}
				fmt.Fprintf(stdout, "Auth callback kind=%s state=%q keys=%s\n", kind, values.Get("state"), strings.Join(keys, ","))
			}
			token, matched, parseErr := parseAuthCallback(navigatedURL, kind, expectedState)
			if parseErr != nil {
				return "", parseErr
			}
			if matched {
				return token, nil
			}
		}
	}
}

func authLoginURL(kind authTokenKind, tenantID, email string) (string, string, error) {
	state, err := authUUID()
	if err != nil {
		return "", "", err
	}
	nonce, err := authUUID()
	if err != nil {
		return "", "", err
	}
	requestID, err := authUUID()
	if err != nil {
		return "", "", err
	}
	values := url.Values{
		"client_id":         {authTeamsAppID},
		"client-request-id": {requestID},
		"redirect_uri":      {authRedirectURI},
		"x-client-SKU":      {"Js"},
		"x-client-Ver":      {"1.0.9"},
		"nonce":             {nonce},
	}
	expectedState := state
	if kind == authTeams {
		values.Set("response_type", "id_token")
	} else {
		values.Set("response_type", "token")
		resource := authSkypeResource
		if kind == authChatSvcAgg {
			resource = authChatResource
		}
		expectedState += "|" + resource
		values.Set("resource", resource)
	}
	values.Set("state", expectedState)
	if strings.TrimSpace(email) != "" {
		values.Set("login_hint", strings.TrimSpace(email))
	}
	endpoint := &url.URL{
		Scheme:   "https",
		Host:     "login.microsoftonline.com",
		Path:     "/" + tenantID + "/oauth2/authorize",
		RawQuery: values.Encode(),
	}
	return endpoint.String(), expectedState, nil
}

func parseAuthCallback(rawURL string, kind authTokenKind, expectedState string) (string, bool, error) {
	prefix := authRedirectURI + "#"
	if !strings.HasPrefix(rawURL, prefix) {
		return "", false, nil
	}
	values, err := url.ParseQuery(strings.TrimPrefix(rawURL, prefix))
	if err != nil {
		return "", false, fmt.Errorf("parse %s callback: %w", kind, err)
	}
	actualState := values.Get("state")
	if (actualState != "" && actualState != expectedState) || (kind == authTeams && actualState == "") {
		return "", false, nil
	}
	if authErr := values.Get("error"); authErr != "" {
		return "", false, fmt.Errorf("%s: %s", authErr, values.Get("error_description"))
	}
	token := values.Get("id_token")
	if token == "" {
		token = values.Get("access_token")
	}
	if token == "" {
		return "", false, nil
	}
	return token, true, nil
}

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

func decodeAuthClaims(token string) (authClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return authClaims{}, fmt.Errorf("invalid JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return authClaims{}, fmt.Errorf("decode JWT claims: %w", err)
	}
	var claims authClaims
	if err = json.Unmarshal(payload, &claims); err != nil {
		return authClaims{}, fmt.Errorf("parse JWT claims: %w", err)
	}
	return claims, nil
}

func CheckTokens() error {
	configDir, err := authConfigDir()
	if err != nil {
		return err
	}
	for _, kind := range []authTokenKind{authTeams, authSkype, authChatSvcAgg} {
		path := filepath.Join(configDir, "token-"+string(kind)+".jwt")
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("%s token unavailable; run teamsctl auth: %w", kind, readErr)
		}
		if validateErr := validateAuthToken(kind, strings.TrimSpace(string(data)), time.Now()); validateErr != nil {
			return validateErr
		}
	}
	return nil
}

func validateAuthToken(kind authTokenKind, token string, now time.Time) error {
	claims, decodeErr := decodeAuthClaims(token)
	if decodeErr != nil {
		return fmt.Errorf("%s token invalid; run teamsctl auth: %w", kind, decodeErr)
	}
	if claims.ExpiresAt == 0 || now.Unix() >= claims.ExpiresAt {
		return fmt.Errorf("%s token expired; run teamsctl auth", kind)
	}
	return nil
}

func saveAuthToken(configDir string, kind authTokenKind, token string) error {
	path := filepath.Join(configDir, "token-"+string(kind)+".jwt")
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return fmt.Errorf("save %s token: %w", kind, err)
	}
	return os.Chmod(path, 0o600)
}

func getAuthTenants(token string) ([]authTenant, error) {
	request, err := http.NewRequest(http.MethodGet, "https://teams.microsoft.com/api/mt/emea/beta/users/tenants", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("get Teams tenants: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("get Teams tenants: status %d", response.StatusCode)
	}
	var tenants []authTenant
	if err = json.NewDecoder(response.Body).Decode(&tenants); err != nil {
		return nil, fmt.Errorf("decode Teams tenants: %w", err)
	}
	return tenants, nil
}

func authConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".config", "fossteams"), nil
}

func findChrome() (string, error) {
	candidates := []string{}
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")
	case "windows":
		for _, root := range []string{os.Getenv("PROGRAMFILES"), os.Getenv("PROGRAMFILES(X86)"), os.Getenv("LOCALAPPDATA")} {
			if root != "" {
				candidates = append(candidates, filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe"))
			}
		}
	default:
		for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
			if path, err := exec.LookPath(name); err == nil {
				return path, nil
			}
		}
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("Chrome not found; set CHROME_PATH or use -chrome")
}

func authUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate auth request id: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
