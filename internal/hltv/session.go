package hltv

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"

	"psr/internal/config"
	"psr/internal/logx"
)

// Session reuses one Chrome instance — cf_clearance only works inside real Chrome.
type Session struct {
	cfg          config.Config
	mu           sync.Mutex
	browser      *rod.Browser
	attached     bool // connected to user's Chrome — do not Close() on cleanup
	http         *ChromeHTTPFetcher
	cookie       string
	ready        bool
	lastReq         time.Time
	lastVerified    time.Time
	rateLimitUntil  time.Time
}

func NewSession(cfg config.Config) *Session {
	s := &Session{cfg: cfg}
	if cfg.Cookie != "" {
		s.cookie = NormalizeCookie(cfg.Cookie)
	} else {
		s.cookie = LoadStoredCookie(cfg)
	}
	return s
}

func (s *Session) Ensure(ctx context.Context) error {
	if s.prefersHTTP() {
		return s.ensureHTTP(ctx)
	}

	s.mu.Lock()
	ready := s.ready
	hasBrowser := s.browser != nil
	s.mu.Unlock()

	if ready && hasBrowser {
		s.mu.Lock()
		recentlyVerified := time.Since(s.lastVerified) < 90*time.Second
		s.mu.Unlock()
		if recentlyVerified {
			return nil
		}
		logx.Info("session", "проверка существующей сессии HLTV...")
		if err := s.verifyBrowser(ctx); err == nil {
			s.mu.Lock()
			s.lastVerified = time.Now()
			s.mu.Unlock()
			logx.Info("session", "сессия активна")
			return nil
		}
		logx.Warn("session", "сессия устарела, переподключение...")
		s.mu.Lock()
		s.resetLocked()
		s.mu.Unlock()
	}

	s.mu.Lock()
	if s.cookie == "" {
		s.cookie = LoadStoredCookie(s.cfg)
	}
	s.mu.Unlock()

	logx.Info("session", "подключение через профиль Chrome PSR...")
	if err := s.ensureBrowser(ctx, false); err != nil {
		return err
	}
	if err := s.verifyBrowser(ctx); err == nil {
		s.mu.Lock()
		s.ready = true
		s.lastVerified = time.Now()
		s.mu.Unlock()
		logx.Info("session", "HLTV доступен")
		return nil
	}

	s.mu.Lock()
	wasAttached := s.attached
	s.resetLocked()
	s.mu.Unlock()

	if !s.cfg.AutoChrome {
		if wasAttached {
			logx.Warn("session", "не удалось загрузить /events — в Chrome PSR откройте Events вручную, затем снова Подключиться")
		} else {
			logx.Warn("session", "Chrome PSR не найден — сначала Открыть HLTV, не закрывайте окно")
		}
		return ErrCookieRequired
	}

	logx.Info("session", "авто-Chrome включён (HLTV_AUTO_CHROME=true)...")
	for attempt := 1; attempt <= 3; attempt++ {
		if err := ctx.Err(); err != nil {
			logx.Info("session", "прервано пользователем")
			return err
		}
		headless := s.cfg.Headless && attempt == 1
		if attempt > 1 {
			headless = false
		}
		logx.Info("session", "попытка подключения %d/3 (headless=%v)", attempt, headless)
		if err := s.bootstrap(ctx, headless); err != nil {
			logx.Warn("session", "bootstrap не удался: %v", err)
			s.mu.Lock()
			s.resetLocked()
			s.mu.Unlock()
			HumanPause(ctx, s.cfg, "повтор после ошибки bootstrap")
			continue
		}
		if err := s.verifyBrowser(ctx); err != nil {
			logx.Warn("session", "проверка /events не удалась: %v", err)
			s.mu.Lock()
			s.resetLocked()
			s.mu.Unlock()
			HumanPause(ctx, s.cfg, "повтор после ошибки verify")
			continue
		}
		s.mu.Lock()
		s.ready = true
		s.lastVerified = time.Now()
		s.mu.Unlock()
		_ = SaveStoredCookie(s.cfg, s.cookie)
		logx.Info("session", "подключение к HLTV успешно")
		return nil
	}
	logx.Error("session", "не удалось подключиться к HLTV после 3 попыток")
	return ErrHLTVUnavailable
}

func (s *Session) ensureBrowser(ctx context.Context, headless bool) error {
	s.mu.Lock()
	if s.browser != nil {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	// Prefer attaching to Chrome opened via «Открыть HLTV» — avoids Cloudflare re-challenge.
	if !headless {
		if err := s.tryAttachBrowser(); err == nil {
			return nil
		}
		if !s.cfg.AutoChrome {
			logx.Warn("session", "Chrome PSR не найден — сначала «Открыть HLTV», не закрывайте окно")
			return ErrChromeNotOpen
		}
	}

	if headless {
		logx.Info("session", "запуск Google Chrome (headless)...")
	} else {
		logx.Info("session", "запуск Google Chrome (авто-режим)...")
	}

	l, bin, err := newChromeLauncher(s.cfg, headless)
	if err != nil {
		return err
	}
	logx.Info("session", "chrome: %s", bin)

	url, err := l.Launch()
	if err != nil {
		return fmt.Errorf("launch chrome: %w", err)
	}

	browser := rod.New().ControlURL(url)
	if err := browser.Connect(); err != nil {
		return fmt.Errorf("connect chrome: %w", err)
	}

	s.mu.Lock()
	s.browser = browser
	s.attached = false
	s.mu.Unlock()
	_ = ctx
	return nil
}

func (s *Session) tryAttachBrowser() error {
	port := s.cfg.ChromeDebugPort
	if port <= 0 {
		port = 9222
	}
	logx.Info("session", "поиск открытого Chrome PSR (порт %d)...", port)

	u, err := launcher.ResolveURL(strconv.Itoa(port))
	if err != nil {
		return err
	}

	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		return err
	}

	s.mu.Lock()
	s.browser = browser
	s.attached = true
	s.mu.Unlock()
	logx.Info("session", "подключено к вашему окну Chrome (без нового запуска)")
	return nil
}

func (s *Session) bootstrap(ctx context.Context, headless bool) error {
	if err := s.ensureBrowser(ctx, headless); err != nil {
		return err
	}
	if !headless {
		logx.Info("session", "дождитесь загрузки HLTV в окне Chrome — не закрывайте его (до 3 мин)")
	}

	s.mu.Lock()
	browser := s.browser
	s.mu.Unlock()

	page, err := newPage(browser, headless)
	if err != nil {
		return err
	}
	defer page.Close()

	page = page.Timeout(s.cfg.Timeout).Context(ctx)
	home := strings.TrimRight(s.cfg.BaseURL, "/")

	if err := injectStoredCookies(page, s.cfg, home); err != nil {
		logx.Warn("session", "не удалось подставить cookie: %v", err)
	}

	logx.Info("session", "открываю главную: %s", home)
	HumanPause(ctx, s.cfg, "перед открытием HLTV")
	if err := page.Navigate(home); err != nil {
		return err
	}

	deadline := time.Now().Add(180 * time.Second)
	try := 0
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		try++
		_ = page.WaitLoad()
		if err := sleepCtx(ctx, 2*time.Second); err != nil {
			return err
		}

		cookies, _ := page.Cookies([]string{home, home + "/"})
		hasCF := hasCloudflareClearance(cookies)
		if hasCF {
			logx.Info("session", "получен cf_clearance cookie")
		}

		html, err := page.HTML()
		size := 0
		if html != "" {
			size = len(html)
		}
		blocked := err == nil && looksBlocked(html)
		hltvOK := err == nil && looksLikeHLTV(html)
		logx.Info("session", "ответ главной #%d: %d байт, blocked=%v, hltv=%v, cf=%v",
			try, size, blocked, hltvOK, hasCF)

		if err == nil && (hltvOK || (hasCF && !blocked)) {
			if c, err := cookieHeader(page); err == nil && c != "" {
				s.mu.Lock()
				s.cookie = c
				s.mu.Unlock()
				logx.Info("session", "cookie получен (%d символов)", len(c))
				return nil
			}
		}

		if hasCF && blocked {
			logx.Info("session", "cf_clearance есть, ждём редирект после проверки Cloudflare...")
		}

		HumanPause(ctx, s.cfg, "ожидание Cloudflare")
	}
	return ErrHLTVUnavailable
}

func (s *Session) verifyBrowser(ctx context.Context) error {
	eventsURL := strings.TrimRight(s.cfg.BaseURL, "/") + "/events"
	logx.Info("session", "проверка страницы турниров: %s", eventsURL)
	html, err := s.fetchPageBrowser(ctx, eventsURL)
	if err != nil {
		return err
	}
	events, err := ParseEvents(html)
	if err != nil {
		logx.Warn("session", "парсер турниров: ошибка (html %d байт)", len(html))
		return ErrHLTVUnavailable
	}
	logx.Info("session", "турниры на странице: %d", len(events))
	return nil
}

func (s *Session) Fetch(ctx context.Context, url string) (string, error) {
	if err := s.Ensure(ctx); err != nil {
		return "", err
	}
	return s.fetchPage(ctx, url)
}

func (s *Session) fetchPage(ctx context.Context, url string) (string, error) {
	if s.prefersHTTP() {
		if err := s.ensureHTTP(ctx); err != nil {
			return "", err
		}
		return s.fetchPageHTTP(ctx, url)
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
	if err := s.ensureBrowser(ctx, s.useHeadless()); err != nil {
		return "", err
	}
	return s.fetchPageBrowser(ctx, url)
}

func (s *Session) useHeadless() bool {
	return s.cfg.Headless && s.cfg.AutoChrome && s.cookie == ""
}

func (s *Session) fetchPageBrowser(ctx context.Context, url string) (string, error) {
	s.mu.Lock()
	if s.browser == nil {
		s.mu.Unlock()
		return "", ErrHLTVUnavailable
	}
	browser := s.browser
	attached := s.attached
	cookie := s.cookie
	s.lastReq = time.Now()
	s.mu.Unlock()

	logx.Info("hltv", "режим загрузки: attached=%v", attached)
	if attached {
		return s.fetchViaAttachedTab(ctx, url)
	}

	HumanPause(ctx, s.cfg, "перед переходом на страницу")

	page, err := newPage(browser, s.useHeadless())
	if err != nil {
		return "", err
	}
	defer page.Close()

	page = page.Timeout(s.cfg.Timeout).Context(ctx)
	base := strings.TrimRight(s.cfg.BaseURL, "/")
	if cookie != "" {
		if err := injectCookieString(page, cookie, base); err != nil {
			logx.Warn("session", "подстановка cookie: %v", err)
		}
	}

	logx.Info("hltv", "переход → %s", url)
	if err := page.Navigate(url); err != nil {
		logx.Error("hltv", "navigate ошибка: %v", err)
		return "", err
	}
	if err := page.WaitLoad(); err != nil {
		logx.Warn("hltv", "wait load: %v", err)
	}

	if err := sleepCtx(ctx, 2*time.Second); err != nil {
		return "", err
	}
	HumanPause(ctx, s.cfg, "чтение страницы")

	html, err := page.HTML()
	if err != nil {
		return "", err
	}

	return s.validateHTML(page, html, true)
}

func (s *Session) fetchViaAttachedTab(ctx context.Context, targetURL string) (string, error) {
	s.attachedHumanPause(ctx, "чтение через вашу вкладку HLTV")

	if html, err := s.attachedQuickRead(ctx, targetURL); err == nil {
		return html, nil
	}

	// Другая страница: отключаем CDP — иначе Cloudflare не даёт пройти капчу.
	logx.Info("hltv", ">>> Откройте вручную в Chrome PSR: %s", targetURL)
	logx.Info("hltv", "CDP отключён — кликните ссылку на сайте HLTV сами, пройдите капчу если появится")
	s.detachBrowser()

	return s.waitDetachedRead(ctx, targetURL, 5*time.Minute)
}

func (s *Session) attachedQuickRead(ctx context.Context, targetURL string) (string, error) {
	if err := s.ensureBrowser(ctx, false); err != nil {
		return "", err
	}

	s.mu.Lock()
	browser := s.browser
	s.mu.Unlock()
	if browser == nil {
		return "", ErrChromeNotOpen
	}

	page, err := findHLTVPage(browser)
	if err != nil {
		return "", err
	}
	page = page.Timeout(s.cfg.Timeout).Context(ctx)

	info, _ := page.Info()
	if info != nil {
		logx.Info("hltv", "вкладка: %s", info.URL)
	}
	if info == nil || !urlMatchesTarget(info.URL, targetURL) {
		return "", fmt.Errorf("wrong url")
	}

	logx.Info("hltv", "читаем текущую вкладку (без перехода)")
	html, err := page.HTML()
	if err != nil {
		return "", err
	}
	return s.validateHTML(page, html, false)
}

func urlMatchesTarget(current, target string) bool {
	if urlsMatch(current, target) {
		return true
	}
	// /results?event=8301&offset=100 matches /results?event=8301&offset=0
	cu, err1 := url.Parse(current)
	tu, err2 := url.Parse(target)
	if err1 == nil && err2 == nil && cu.Path == "/results" && tu.Path == "/results" {
		return cu.Query().Get("event") == tu.Query().Get("event") && cu.Query().Get("event") != ""
	}
	// /events/8301/slug matches /events/8301/other-slug
	if err1 == nil && err2 == nil && strings.HasPrefix(cu.Path, "/events/") && strings.HasPrefix(tu.Path, "/events/") {
		cp := strings.Split(strings.Trim(cu.Path, "/"), "/")
		tp := strings.Split(strings.Trim(tu.Path, "/"), "/")
		return len(cp) >= 2 && len(tp) >= 2 && cp[0] == "events" && tp[0] == "events" && cp[1] == tp[1]
	}
	// /stats/teams/maps/4608/bench matches same team id (slug may differ)
	if err1 == nil && err2 == nil && strings.HasPrefix(cu.Path, "/stats/teams/maps/") && strings.HasPrefix(tu.Path, "/stats/teams/maps/") {
		cp := strings.Split(strings.Trim(cu.Path, "/"), "/")
		tp := strings.Split(strings.Trim(tu.Path, "/"), "/")
		return len(cp) >= 4 && len(tp) >= 4 && cp[0] == "stats" && cp[1] == "teams" && cp[2] == "maps" && cp[3] == tp[3]
	}
	return false
}

func (s *Session) detachBrowser() {
	s.mu.Lock()
	s.browser = nil
	s.mu.Unlock()
}

func (s *Session) waitDetachedRead(ctx context.Context, targetURL string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	lastLog := time.Time{}

	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		if err := s.ensureBrowser(ctx, false); err != nil {
			sleepCtx(ctx, 3*time.Second)
			continue
		}

		s.mu.Lock()
		browser := s.browser
		s.mu.Unlock()

		page, err := findHLTVPage(browser)
		if err != nil {
			s.detachBrowser()
			sleepCtx(ctx, 3*time.Second)
			continue
		}
		page = page.Timeout(s.cfg.Timeout).Context(ctx)

		info, _ := page.Info()
		if info == nil || !urlMatchesTarget(info.URL, targetURL) {
			if time.Since(lastLog) > 15*time.Second {
				logx.Info("hltv", "жду когда откроете: %s", targetURL)
				lastLog = time.Now()
			}
			s.detachBrowser()
			sleepCtx(ctx, 5*time.Second)
			continue
		}

		html, err := page.HTML()
		s.detachBrowser()

		if err != nil {
			sleepCtx(ctx, 3*time.Second)
			continue
		}

		blocked := looksBlocked(html)
		hltvOK := looksLikeHLTV(html)
		logx.Info("hltv", "ответ ← %d байт, blocked=%v, hltv=%v", len(html), blocked, hltvOK)

		if !blocked && hltvOK {
			if c, err := cookieHeader(page); err == nil && c != "" {
				s.mu.Lock()
				s.cookie = c
				s.mu.Unlock()
				_ = SaveStoredCookie(s.cfg, c)
			}
			return html, nil
		}

		if time.Since(lastLog) > 12*time.Second {
			logx.Info("hltv", "капча — пройдите в Chrome PSR (программа отключена от вкладки)")
			lastLog = time.Now()
		}
		sleepCtx(ctx, 5*time.Second)
	}

	return "", ErrHLTVUnavailable
}

func (s *Session) IsUserChrome() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attached
}

func (s *Session) attachedHumanPause(ctx context.Context, action string) {
	cfg := s.cfg
	if cfg.HumanDelayMin < 8*time.Second {
		cfg.HumanDelayMin = 8 * time.Second
	}
	if cfg.HumanDelayMax < 15*time.Second {
		cfg.HumanDelayMax = 15 * time.Second
	}
	HumanPause(ctx, cfg, action)
}

func (s *Session) validateHTML(page *rod.Page, html string, strict bool) (string, error) {
	blocked := looksBlocked(html)
	hltvOK := looksLikeHLTV(html)
	logx.Info("hltv", "ответ ← %d байт, blocked=%v, hltv=%v", len(html), blocked, hltvOK)

	if blocked || !hltvOK {
		if strict {
			s.mu.Lock()
			s.ready = false
			s.mu.Unlock()
		}
		return "", ErrHLTVUnavailable
	}

	if page != nil {
		if c, err := cookieHeader(page); err == nil && c != "" {
			s.mu.Lock()
			s.cookie = c
			s.mu.Unlock()
			_ = SaveStoredCookie(s.cfg, c)
		}
	}

	return html, nil
}

func (s *Session) SetCookie(ctx context.Context, cookie string) error {
	cookie = EssentialCookie(NormalizeCookie(cookie))
	if cookie == "" || !strings.Contains(cookie, "cf_clearance=") {
		return ErrCookieRequired
	}

	logx.Info("session", "проверка cookie (cf_clearance + __cf_bm, %d символов)...", len(cookie))
	logx.Info("session", "cookie из Chrome PSR после прохождения капчи на hltv.org/events")

	s.mu.Lock()
	s.cookie = cookie
	s.ready = false
	s.lastVerified = time.Time{}
	_ = s.closeBrowserLocked()
	s.mu.Unlock()

	if err := SaveStoredCookie(s.cfg, cookie); err != nil {
		return err
	}

	if s.prefersHTTP() {
		if err := s.verifyHTTP(ctx); err != nil {
			logx.Warn("session", "cookie не прошёл HTTP проверку — скопируйте заново сразу после загрузки Events")
			return ErrHLTVUnavailable
		}
		s.mu.Lock()
		s.ready = true
		s.lastVerified = time.Now()
		s.mu.Unlock()
		logx.Info("session", "cookie принят, HLTV доступен по HTTP")
		return nil
	}

	if err := s.ensureBrowser(ctx, false); err != nil {
		return err
	}
	if err := s.verifyBrowser(ctx); err != nil {
		logx.Warn("session", "cookie не прошёл проверку — скопируйте из Chrome PSR")
		return ErrHLTVUnavailable
	}

	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()
	logx.Info("session", "cookie принят, HLTV доступен")
	return nil
}

// Connect verifies HLTV access (HTTP+cookie or CDP tab read).
func (s *Session) Connect(ctx context.Context) error {
	if s.prefersHTTP() {
		return s.ensureHTTP(ctx)
	}

	logx.Info("session", "подключение через профиль Chrome PSR...")
	if err := s.ensureBrowser(ctx, false); err != nil {
		return err
	}

	s.mu.Lock()
	browser := s.browser
	s.mu.Unlock()
	if browser == nil {
		return ErrChromeNotOpen
	}

	page, err := findHLTVPage(browser)
	if err != nil {
		logx.Warn("session", "%v", err)
		return ErrChromeNotOpen
	}

	page = page.Timeout(s.cfg.Timeout).Context(ctx)
	info, _ := page.Info()
	if info != nil {
		logx.Info("session", "вкладка HLTV: %s", info.URL)
	}

	html, err := page.HTML()
	if err != nil {
		return err
	}
	if looksBlocked(html) || !looksLikeHLTV(html) {
		logx.Warn("session", "HLTV не готов — откройте Events в Chrome PSR, пройдите капчу")
		return ErrCookieRequired
	}

	if c, err := cookieHeader(page); err == nil && c != "" {
		s.mu.Lock()
		s.cookie = c
		s.mu.Unlock()
		_ = SaveStoredCookie(s.cfg, c)
	}

	s.mu.Lock()
	s.ready = true
	s.lastVerified = time.Now()
	s.mu.Unlock()
	logx.Info("session", "подключено — вкладка читается без переходов")
	return nil
}

func (s *Session) CookieStatus(ctx context.Context) (has bool, valid bool) {
	s.mu.Lock()
	cookie := s.cookie
	ready := s.ready
	s.mu.Unlock()

	if cookie == "" {
		cookie = LoadStoredCookie(s.cfg)
	}
	if cookie == "" {
		return false, false
	}
	if ready {
		return true, true
	}
	return true, strings.Contains(cookie, "cf_clearance=")
}

func (s *Session) Cookie() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cookie
}

func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	logx.Info("session", "закрытие Chrome")
	return s.closeBrowserLocked()
}

func (s *Session) resetLocked() {
	s.ready = false
	s.lastVerified = time.Time{}
	_ = s.closeBrowserLocked()
}

func (s *Session) waitDuration() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastReq.IsZero() {
		return 0
	}
	minDelay := s.cfg.RequestDelay
	if s.attached && minDelay < 12*time.Second {
		minDelay = 12 * time.Second
	}
	wait := minDelay - time.Since(s.lastReq)
	if wait < 0 {
		return 0
	}
	return wait
}

func (s *Session) closeBrowserLocked() error {
	if s.browser == nil {
		return nil
	}
	if s.attached {
		s.browser = nil
		return nil
	}
	err := s.browser.Close()
	s.browser = nil
	return err
}
