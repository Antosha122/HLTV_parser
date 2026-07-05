package hltv

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	"psr/internal/config"
)

const chromeUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// ChromeHTTPFetcher uses a Chrome TLS fingerprint — required for cf_clearance cookies.
type ChromeHTTPFetcher struct {
	client tls_client.HttpClient
	cfg    config.Config
}

func NewChromeHTTPFetcher(cfg config.Config) (*ChromeHTTPFetcher, error) {
	sec := int(cfg.Timeout / time.Second)
	if sec < 10 {
		sec = 90
	}
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(sec),
		tls_client.WithClientProfile(profiles.Chrome_131),
	}
	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("tls client: %w", err)
	}
	return &ChromeHTTPFetcher{client: client, cfg: cfg}, nil
}

func (h *ChromeHTTPFetcher) Fetch(ctx context.Context, url string) (string, error) {
	return h.fetchWithCookie(ctx, url, h.cfg.Cookie)
}

func (h *ChromeHTTPFetcher) fetchWithCookie(ctx context.Context, url string, cookie string) (string, error) {
	cookie = EssentialCookie(cookie)
	if cookie == "" {
		return "", ErrCookieRequired
	}

	req, err := fhttp.NewRequestWithContext(ctx, fhttp.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header = fhttp.Header{
		"User-Agent":                {chromeUserAgent},
		"Accept":                    {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"},
		"Accept-Language":           {"en-US,en;q=0.9"},
		"Accept-Encoding":           {"gzip, deflate, br"},
		"Cache-Control":             {"no-cache"},
		"Upgrade-Insecure-Requests": {"1"},
		"Sec-Fetch-Dest":            {"document"},
		"Sec-Fetch-Mode":            {"navigate"},
		"Sec-Fetch-Site":            {"none"},
		"Sec-Fetch-User":            {"?1"},
		"Sec-Ch-Ua":                 {`"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`},
		"Sec-Ch-Ua-Mobile":          {"?0"},
		"Sec-Ch-Ua-Platform":        {`"Windows"`},
		"Cookie":                    {cookie},
		fhttp.HeaderOrderKey: {
			"user-agent",
			"accept",
			"accept-language",
			"accept-encoding",
			"cache-control",
			"upgrade-insecure-requests",
			"sec-fetch-dest",
			"sec-fetch-mode",
			"sec-fetch-site",
			"sec-fetch-user",
			"sec-ch-ua",
			"sec-ch-ua-mobile",
			"sec-ch-ua-platform",
			"cookie",
		},
	}

	base := strings.TrimRight(h.cfg.BaseURL, "/")
	if strings.HasPrefix(url, base) {
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Referer", refererForHLTV(base, url))
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode == fhttp.StatusForbidden || resp.StatusCode == fhttp.StatusTooManyRequests {
		return string(data), fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return string(data), fmt.Errorf("http %d", resp.StatusCode)
	}

	return string(data), nil
}

func (h *ChromeHTTPFetcher) Close() error {
	return nil
}

func refererForHLTV(base, requestURL string) string {
	u, err := url.Parse(requestURL)
	if err != nil {
		return base + "/events"
	}
	q := u.Query()
	if teamID := q.Get("team"); teamID != "" {
		return fmt.Sprintf("%s/team/%s/_", base, teamID)
	}
	if q.Has("event") {
		return base + "/events"
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 4 && parts[0] == "stats" && parts[1] == "teams" && parts[2] == "maps" {
		return fmt.Sprintf("%s/team/%s/_", base, parts[3])
	}
	if len(parts) >= 2 && parts[0] == "team" {
		return base + "/events"
	}
	return base + "/events"
}
