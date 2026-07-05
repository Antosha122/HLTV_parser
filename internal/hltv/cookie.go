package hltv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"

	"psr/internal/config"
)

func CookieFilePath(cfg config.Config) string {
	if cfg.CookiePath != "" {
		return cfg.CookiePath
	}
	return "data/hltv.cookie"
}

func LoadStoredCookie(cfg config.Config) string {
	data, err := os.ReadFile(CookieFilePath(cfg))
	if err != nil {
		return ""
	}
	return NormalizeCookie(strings.TrimSpace(string(data)))
}

func SaveStoredCookie(cfg config.Config, cookie string) error {
	path := CookieFilePath(cfg)
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(cookie), 0o600)
}

// AcquireCookie opens Chrome, visits HLTV and returns session cookies automatically.
func AcquireCookie(ctx context.Context, cfg config.Config) (string, error) {
	l, _, err := newChromeLauncher(cfg, cfg.Headless)
	if err != nil {
		return "", err
	}

	url, err := l.Launch()
	if err != nil {
		return "", fmt.Errorf("launch chrome: %w", err)
	}

	browser := rod.New().ControlURL(url)
	if err := browser.Connect(); err != nil {
		return "", fmt.Errorf("connect chrome: %w", err)
	}
	defer browser.Close()

	page, err := stealth.Page(browser)
	if err != nil {
		return "", fmt.Errorf("stealth page: %w", err)
	}
	defer page.Close()

	page = page.Timeout(cfg.Timeout).Context(ctx)

	target := strings.TrimRight(cfg.BaseURL, "/")
	if err := page.Navigate(target); err != nil {
		return "", fmt.Errorf("navigate hltv: %w", err)
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		_ = page.WaitLoad()
		time.Sleep(2 * time.Second)

		html, err := page.HTML()
		if err == nil && !looksBlocked(html) {
			cookie, err := cookieHeader(page)
			if err == nil && cookie != "" {
				return cookie, nil
			}
		}

		cookies, err := page.Cookies([]string{target, target + "/"})
		if err == nil {
			if hdr := formatCookies(cookies); hdr != "" && hasCloudflareClearance(cookies) {
				return hdr, nil
			}
		}

		time.Sleep(2 * time.Second)
	}

	return "", fmt.Errorf("не удалось пройти защиту HLTV автоматически — подождите и нажмите Обновить снова")
}

func cookieHeader(page *rod.Page) (string, error) {
	cookies, err := page.Cookies([]string{})
	if err != nil {
		return "", err
	}
	return formatCookies(cookies), nil
}

func formatCookies(cookies []*proto.NetworkCookie) string {
	if len(cookies) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if c.Name == "" {
			continue
		}
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

func hasCloudflareClearance(cookies []*proto.NetworkCookie) bool {
	for _, c := range cookies {
		if c.Name == "cf_clearance" {
			return true
		}
	}
	return false
}

func injectCookieString(page *rod.Page, cookieStr, baseURL string) error {
	cookieStr = NormalizeCookie(cookieStr)
	if cookieStr == "" {
		return nil
	}

	home := strings.TrimRight(baseURL, "/")
	_ = page.Navigate(home + "/")
	_ = page.WaitLoad()

	var params []*proto.NetworkCookieParam
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		idx := strings.Index(part, "=")
		if idx <= 0 {
			continue
		}
		name := part[:idx]
		value := part[idx+1:]
		params = append(params, &proto.NetworkCookieParam{
			Name:     name,
			Value:    value,
			Domain:   ".hltv.org",
			Path:     "/",
			Secure:   true,
			HTTPOnly: name == "cf_clearance" || name == "__cf_bm",
			URL:      home,
		})
	}
	if len(params) == 0 {
		return nil
	}
	return page.SetCookies(params)
}

func injectStoredCookies(page *rod.Page, cfg config.Config, baseURL string) error {
	return injectCookieString(page, LoadStoredCookie(cfg), baseURL)
}
