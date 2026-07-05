package hltv

import (
	"errors"
	"strings"
)

var (
	ErrHLTVUnavailable = errors.New("hltv_unavailable")
	ErrCookieRequired  = errors.New("cookie_required")
	ErrChromeNotOpen   = errors.New("chrome_not_open")
)

func UserError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrChromeNotOpen) || errors.Is(err, ErrCookieRequired) {
		return "1) Открыть HLTV → 2) пройти капчу → 3) открыть Events → 4) F12 → Сеть → F5 → скопировать cookie → Сохранить cookie → 5) Подключиться."
	}
	if errors.Is(err, ErrHLTVUnavailable) {
		return "Cookie не принят (устарел или скопирован не с www.hltv.org). Откройте Events, сразу скопируйте cf_clearance и __cf_bm из Приложение → Cookies → https://www.hltv.org и сохраните."
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "403") ||
		strings.Contains(msg, "cloudflare") ||
		strings.Contains(msg, "blocked") ||
		strings.Contains(msg, "http 4") ||
		strings.Contains(msg, "http 5") {
		return "Не удалось получить данные с HLTV. Подождите и попробуйте снова."
	}
	return "Ошибка загрузки данных. Попробуйте ещё раз."
}
