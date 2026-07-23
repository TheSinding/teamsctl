package teamsauth

import (
	"context"
	"fmt"
	"io"
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

const authUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) MicrosoftTeams-Preview/1.4.00.7556 Chrome/80.0.3987.163 Electron/8.5.5 Safari/537.36"

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
	executablePath, err = resolveChromeExecutable(executablePath)
	if err != nil {
		return err
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

func resolveChromeExecutable(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("Chrome executable %q: %w", path, err)
	}
	if !info.IsDir() {
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			return "", fmt.Errorf("Chrome executable %q is not executable", path)
		}
		return path, nil
	}
	if !strings.EqualFold(filepath.Ext(path), ".app") {
		return "", fmt.Errorf("Chrome path %q is a directory; pass an executable or macOS app bundle", path)
	}

	binDir := filepath.Join(path, "Contents", "MacOS")
	appName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	preferred := filepath.Join(binDir, appName)
	if candidate, statErr := os.Stat(preferred); statErr == nil && !candidate.IsDir() && candidate.Mode().Perm()&0o111 != 0 {
		return preferred, nil
	}

	entries, err := os.ReadDir(binDir)
	if err != nil {
		return "", fmt.Errorf("find executable in Chrome app %q: %w", path, err)
	}
	var executable string
	for _, entry := range entries {
		candidate, statErr := entry.Info()
		if statErr != nil || candidate.IsDir() || candidate.Mode().Perm()&0o111 == 0 {
			continue
		}
		if executable != "" {
			return "", fmt.Errorf("Chrome app %q contains multiple executables; pass the executable in Contents/MacOS", path)
		}
		executable = filepath.Join(binDir, entry.Name())
	}
	if executable == "" {
		return "", fmt.Errorf("Chrome app %q contains no executable in Contents/MacOS", path)
	}
	return executable, nil
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
