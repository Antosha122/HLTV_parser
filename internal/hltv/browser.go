package hltv

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/stealth"

	"psr/internal/config"
)

type BrowserFetcher struct {
	cfg    config.Config
	browser *rod.Browser
	mu     sync.Mutex
}

func NewBrowserFetcher(cfg config.Config) (*BrowserFetcher, error) {
	l, _, err := newChromeLauncher(cfg, cfg.Headless)
	if err != nil {
		return nil, err
	}
	url, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("launch browser: %w", err)
	}

	browser := rod.New().ControlURL(url).MustConnect()
	return &BrowserFetcher{cfg: cfg, browser: browser}, nil
}

func (b *BrowserFetcher) Fetch(ctx context.Context, url string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	page, err := stealth.Page(b.browser)
	if err != nil {
		return "", fmt.Errorf("create stealth page: %w", err)
	}
	defer page.Close()

	page = page.Timeout(b.cfg.Timeout)
	page = page.Context(ctx)

	if err := page.Navigate(url); err != nil {
		return "", fmt.Errorf("navigate: %w", err)
	}

	if err := page.WaitLoad(); err != nil {
		return "", fmt.Errorf("wait load: %w", err)
	}

	// Cloudflare challenge may need a short settle time.
	time.Sleep(2 * time.Second)

	html, err := page.HTML()
	if err != nil {
		return "", fmt.Errorf("read html: %w", err)
	}
	if looksBlocked(html) {
		return html, fmt.Errorf("page still blocked by cloudflare")
	}
	return html, nil
}

func (b *BrowserFetcher) Close() error {
	if b.browser != nil {
		return b.browser.Close()
	}
	return nil
}
