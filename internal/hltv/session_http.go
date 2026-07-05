package hltv

import (
	"context"
	"strings"
	"time"

	"psr/internal/logx"
)

const httpRetryAttempts = 2
const rateLimitCooldown = 25 * time.Second

func (s *Session) prefersHTTP() bool {
	return s.cfg.ChromeDebugPort <= 0
}

func (s *Session) ensureHTTP(ctx context.Context) error {
	s.mu.Lock()
	if s.cookie == "" {
		s.cookie = LoadStoredCookie(s.cfg)
	}
	cookie := s.cookie
	ready := s.ready
	recentlyVerified := time.Since(s.lastVerified) < 90*time.Second
	s.mu.Unlock()

	if cookie == "" {
		logx.Warn("session", "нет cookie — откройте HLTV в Chrome PSR, скопируйте cookie и нажмите «Сохранить cookie»")
		return ErrCookieRequired
	}

	if ready && recentlyVerified {
		return nil
	}

	logx.Info("session", "проверка cookie через HTTP...")
	if err := s.verifyHTTP(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	s.ready = true
	s.lastVerified = time.Now()
	s.mu.Unlock()
	logx.Info("session", "cookie работает, HLTV доступен по HTTP")
	return nil
}

func (s *Session) verifyHTTP(ctx context.Context) error {
	eventsURL := strings.TrimRight(s.cfg.BaseURL, "/") + "/events"
	html, err := s.fetchHTTPDirect(ctx, eventsURL)
	if err != nil {
		logx.Warn("session", "HTTP /events: %v", err)
		return ErrHLTVUnavailable
	}
	events, err := ParseEvents(html)
	if err != nil {
		return ErrHLTVUnavailable
	}
	logx.Info("session", "турниры на странице: %d", len(events))
	return nil
}

func (s *Session) waitRateLimitCooldown(ctx context.Context, fullURL string) {
	if !isRateLimitedPath(fullURL) {
		return
	}
	s.mu.Lock()
	until := s.rateLimitUntil
	s.mu.Unlock()
	if wait := time.Until(until); wait > 0 {
		logx.Warn("hltv", "пауза %s после 403 перед %s", wait.Round(time.Second), shortURL(fullURL))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (s *Session) noteHTTPRateLimit(fullURL string, err error) {
	if err == nil || !isRetryableHTTP(err) || !isRateLimitedPath(fullURL) {
		return
	}
	s.mu.Lock()
	next := time.Now().Add(rateLimitCooldown)
	if next.After(s.rateLimitUntil) {
		s.rateLimitUntil = next
	}
	s.mu.Unlock()
}

func shortURL(fullURL string) string {
	if i := strings.Index(fullURL, "hltv.org"); i >= 0 {
		return fullURL[i+8:]
	}
	return fullURL
}

func (s *Session) fetchHTTPDirect(ctx context.Context, fullURL string) (string, error) {
	s.waitRateLimitCooldown(ctx, fullURL)

	attempts := httpRetryAttempts
	if isRateLimitedPath(fullURL) {
		attempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			backoff := 3 * time.Second
			logx.Warn("hltv", "повтор HTTP %d/%d через %s: %v", attempt+1, attempts, backoff, lastErr)
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", ctx.Err()
			case <-timer.C:
			}
		}

		body, err := s.fetchHTTPOnce(ctx, fullURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		s.noteHTTPRateLimit(fullURL, err)
		if !isRetryableHTTP(err) {
			return "", err
		}
	}
	return "", lastErr
}

func isRetryableHTTP(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "http 403") || strings.Contains(msg, "http 429")
}

func (s *Session) fetchHTTPOnce(ctx context.Context, fullURL string) (string, error) {
	s.mu.Lock()
	if s.cookie == "" {
		s.cookie = LoadStoredCookie(s.cfg)
	}
	cookie := s.cookie
	s.lastReq = time.Now()
	s.mu.Unlock()

	if cookie == "" {
		return "", ErrCookieRequired
	}

	if s.http == nil {
		client, err := NewChromeHTTPFetcher(s.cfg)
		if err != nil {
			return "", err
		}
		s.http = client
	}

	body, err := s.http.fetchWithCookie(ctx, fullURL, cookie)
	if err != nil {
		return "", err
	}
	blocked := looksBlocked(body)
	hltvOK := looksLikeHLTV(body)
	logx.Info("hltv", "HTTP ← %d байт, blocked=%v, hltv=%v", len(body), blocked, hltvOK)
	if blocked || !hltvOK {
		return "", ErrHLTVUnavailable
	}
	return body, nil
}

func (s *Session) fetchPageHTTP(ctx context.Context, url string) (string, error) {
	if isRateLimitedPath(url) {
		s.waitRateLimitCooldown(ctx, url)
	}
	if wait := s.waitDuration(); wait > 0 {
		logx.Info("hltv", "минимальная пауза %s перед запросом", wait.Round(time.Millisecond))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	// В HTTP-режиме достаточно минимального интервала между запросами (без второй паузы).
	if !s.prefersHTTP() {
		HumanPause(ctx, s.cfg, "перед HTTP запросом")
	}
	logx.Info("hltv", "HTTP → %s", url)
	return s.fetchHTTPDirect(ctx, url)
}
