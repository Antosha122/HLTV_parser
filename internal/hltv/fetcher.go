package hltv

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"psr/internal/config"
)

type Fetcher interface {
	Fetch(ctx context.Context, url string) (string, error)
	Close() error
}

func newFetcher(cfg config.Config) (Fetcher, error) {
	httpFetcher := &HTTPFetcher{
		client: &http.Client{Timeout: cfg.Timeout},
		cfg:    cfg,
	}

	if !cfg.UseBrowser {
		return httpFetcher, nil
	}

	browser, err := NewBrowserFetcher(cfg)
	if err != nil {
		return nil, fmt.Errorf("browser fetcher: %w", err)
	}
	return &FallbackFetcher{primary: httpFetcher, fallback: browser}, nil
}

type FallbackFetcher struct {
	primary  Fetcher
	fallback Fetcher
}

func (f *FallbackFetcher) Fetch(ctx context.Context, url string) (string, error) {
	body, err := f.primary.Fetch(ctx, url)
	if err == nil && !looksBlocked(body) {
		return body, nil
	}
	return f.fallback.Fetch(ctx, url)
}

func (f *FallbackFetcher) Close() error {
	var errs []string
	if err := f.primary.Close(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := f.fallback.Close(); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("close fallback fetcher: %s", strings.Join(errs, "; "))
	}
	return nil
}

func looksBlocked(body string) bool {
	lower := strings.ToLower(body)
	if len(body) < 500 {
		return true
	}
	return strings.Contains(lower, "just a moment") ||
		strings.Contains(lower, "cf-browser-verification") ||
		strings.Contains(lower, "attention required") ||
		strings.Contains(lower, "challenges.cloudflare.com") ||
		strings.Contains(lower, "_cf_chl_opt")
}

func looksLikeHLTV(body string) bool {
	lower := strings.ToLower(body)
	if looksBlocked(body) {
		return false
	}
	return strings.Contains(lower, "hltv.org") && (strings.Contains(lower, "counter-strike") ||
		strings.Contains(lower, "navbar") ||
		strings.Contains(lower, "/matches/") ||
		strings.Contains(lower, "eventname"))
}

type HTTPFetcher struct {
	client *http.Client
	cfg    config.Config
}

func (h *HTTPFetcher) Fetch(ctx context.Context, url string) (string, error) {
	return h.fetchWithCookie(ctx, url, h.cfg.Cookie)
}

func (h *HTTPFetcher) fetchWithCookie(ctx context.Context, url string, cookie string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	if strings.HasPrefix(url, h.cfg.BaseURL) {
		req.Header.Set("Referer", strings.TrimRight(h.cfg.BaseURL, "/")+"/events")
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
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

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return string(data), fmt.Errorf("%w: http %d", ErrBlocked, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return string(data), fmt.Errorf("%w: http %d", ErrBlocked, resp.StatusCode)
	}

	return string(data), nil
}

func (h *HTTPFetcher) Close() error {
	return nil
}
