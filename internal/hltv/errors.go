package hltv

import (
	"errors"
)

var (
	ErrHLTVUnavailable = errors.New("hltv_unavailable")
	ErrCookieRequired  = errors.New("cookie_required")
	ErrChromeNotOpen   = errors.New("chrome_not_open")
	// ErrBlocked is returned when HLTV/Cloudflare actively blocks the request
	// (HTTP 403/429 or a known challenge page). Callers should slow down or
	// refresh the cookie.
	ErrBlocked = errors.New("hltv_blocked")
)

func UserError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrChromeNotOpen) || errors.Is(err, ErrCookieRequired) {
		return "1) Открыть HLTV → 2) пройти капчу → 3) открыть Events → 4) F12 → Сеть → F5 → скопировать cookie → Сохранить cookie → 5) Подключиться."
	}
	if errors.Is(err, ErrHLTVUnavailable) || errors.Is(err, ErrBlocked) {
		return "Cookie не принят (устарел или скопирован не с www.hltv.org). Откройте Events, сразу скопируйте cf_clearance и __cf_bm из Приложение → Cookies → https://www.hltv.org и сохраните."
	}
	return "Ошибка загрузки данных. Попробуйте ещё раз."
}
