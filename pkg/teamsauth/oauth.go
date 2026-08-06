package teamsauth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

const (
	authMicrosoftTenantID = "f8cdef31-a31e-4b4a-93e4-5f571e91255a"
	authTeamsAppID        = "5e3ce6c0-2b1f-4285-8d4b-75ee78787346"
	authSkypeResource     = "https://api.spaces.skype.com"
	authChatResource      = "https://chatsvcagg.teams.microsoft.com"
	authRedirectURI       = "https://teams.microsoft.com/go"
)

const (
	authTeams      authTokenKind = "teams"
	authSkype      authTokenKind = "skype"
	authChatSvcAgg authTokenKind = "chatsvcagg"
)

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
	_, _ = fmt.Fprintf(stdout, "Authorizing %s with tenant %s...\n", kind, tenantID)
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
				_, _ = fmt.Fprintf(stdout, "Auth callback kind=%s state=%q keys=%s\n", kind, values.Get("state"), strings.Join(keys, ","))
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
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("get Teams tenants: status %d", response.StatusCode)
	}
	var tenants []authTenant
	if err = json.NewDecoder(response.Body).Decode(&tenants); err != nil {
		return nil, fmt.Errorf("decode Teams tenants: %w", err)
	}
	return tenants, nil
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
