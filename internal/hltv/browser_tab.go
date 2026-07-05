package hltv

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"

	"psr/internal/logx"
)

func findHLTVPage(browser *rod.Browser) (*rod.Page, error) {
	pages, err := browser.Pages()
	if err != nil {
		return nil, err
	}

	var fallback *rod.Page
	for _, p := range pages {
		info, err := p.Info()
		if err != nil {
			continue
		}
		u := strings.ToLower(info.URL)
		if !strings.Contains(u, "hltv.org") {
			continue
		}
		if strings.HasPrefix(u, "chrome://") || strings.HasPrefix(u, "devtools://") {
			continue
		}
		if strings.Contains(u, "/events") {
			return p, nil
		}
		fallback = p
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, fmt.Errorf("нет открытой вкладки hltv.org — оставьте вкладку HLTV открытой")
}

func urlsMatch(current, target string) bool {
	cu, err1 := url.Parse(current)
	tu, err2 := url.Parse(target)
	if err1 != nil || err2 != nil {
		return strings.EqualFold(strings.TrimRight(current, "/"), strings.TrimRight(target, "/"))
	}
	if !strings.EqualFold(cu.Host, tu.Host) {
		return false
	}
	if cu.Path != tu.Path {
		return false
	}
	return cu.RawQuery == tu.RawQuery
}

// urlsMatchPathQuery matches by path and selected query keys (e.g. event id).
func urlsMatchPathQuery(current, target string, keys ...string) bool {
	cu, err1 := url.Parse(current)
	tu, err2 := url.Parse(target)
	if err1 != nil || err2 != nil {
		return urlsMatch(current, target)
	}
	if !strings.EqualFold(cu.Host, tu.Host) || cu.Path != tu.Path {
		return false
	}
	if len(keys) == 0 {
		return cu.RawQuery == tu.RawQuery
	}
	cq, tq := cu.Query(), tu.Query()
	for _, k := range keys {
		if cq.Get(k) != tq.Get(k) {
			return false
		}
	}
	return true
}

func waitForReadablePage(ctx context.Context, page *rod.Page, targetURL string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	lastLog := time.Time{}

	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		info, _ := page.Info()
		if info == nil || !urlsMatch(info.URL, targetURL) {
			sleepCtx(ctx, 2*time.Second)
			continue
		}

		html, err := page.HTML()
		if err != nil {
			sleepCtx(ctx, 2*time.Second)
			continue
		}

		blocked := looksBlocked(html)
		hltvOK := looksLikeHLTV(html)
		logx.Info("hltv", "ответ ← %d байт, blocked=%v, hltv=%v", len(html), blocked, hltvOK)

		if !blocked && hltvOK {
			return html, nil
		}

		if time.Since(lastLog) > 12*time.Second {
			if blocked {
				logx.Info("hltv", "капча Cloudflare — пройдите её в окне Chrome PSR, программа ждёт...")
			} else {
				logx.Info("hltv", "ждём загрузку страницы %s", targetURL)
			}
			lastLog = time.Now()
		}

		sleepCtx(ctx, 3*time.Second)
	}

	return "", fmt.Errorf("страница не загрузилась за %s — пройдите капчу в Chrome PSR и повторите", timeout.Round(time.Second))
}
