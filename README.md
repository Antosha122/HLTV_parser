# PSR — CS2 Match Predictor (HLTV)

Приложение на Go для прогнозирования исходов CS2-матчей на основе данных [HLTV](https://www.hltv.org).

Парсит турниры, команды, результаты матчей и статистику карт, сохраняет данные в SQLite и рассчитывает вероятности победы ансамблем моделей: **Elo**, **форма**, **head-to-head** и **симуляция veto**. Включает веб-интерфейс с live-логами (SSE), бэктест точности и автокалибровку весов.

---

## Содержание

- [Возможности](#возможности)
- [Технологии](#технологии)
- [Архитектура](#архитектура)
- [Быстрый старт](#быстрый-старт)
- [Сборка](#сборка)
- [Команды CLI](#команды-cli)
- [Веб-интерфейс](#веб-интерфейс)
- [HTTP API](#http-api)
- [Переменные окружения](#переменные-окружения)
- [Модель прогнозирования](#модель-прогнозирования)
- [Работа с Cloudflare](#работа-с-cloudflare)
- [Docker](#docker)
- [Тестирование и линтеры](#тестирование-и-линтеры)
- [Структура проекта](#структура-проекта)
- [Устранение неисправностей](#устранение-неисправностей)
- [Дорожная карта](#дорожная-карта)

---

## Возможности

- **Парсинг HLTV** через HTTP+cookie с обходом Cloudflare; резервный сценарий — автоматизация Chrome (CDP) через `go-rod`.
- **Хранилище SQLite** (чистый Go-драйвер `modernc.org/sqlite`, режим WAL): команды, игроки, турниры, матчи, veto, статистика карт.
- **Прогноз матча**: вероятности победы каждой команды, прогноз счёта серии (Bo1/Bo3), симуляция veto, пул карт, h2h и текущая форма.
- **Бэктест** точности модели (accuracy, log-loss, Brier score) с разбивкой по форматам.
- **Автокалибровка весов** ансамбля через grid search по сохранённой истории матчей.
- **Веб-интерфейс** (`/api/*`, встроенная статика `internal/api/static`) с live-логами через Server-Sent Events.
- **CLI** для синхронизации данных, прогнозов, бэктеста и управления БД.

---

## Технологии

| Компонент | Библиотека / подход |
|---|---|
| Язык | Go 1.25 |
| HTML-парсинг | [`PuerkitoBio/goquery`](https://github.com/PuerkitoBio/goquery) |
| Автоматизация Chrome | [`go-rod/rod`](https://github.com/go-rod/rod) + [`go-rod/stealth`](https://github.com/go-rod/stealth) |
| HTTP-клиент (TLS-fingerprint) | [`bogdanfinn/tls-client`](https://github.com/bogdanfinn/tls-client) |
| База данных | [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) (pure Go, без CGO) |
| Маршрутизация | `net/http.ServeMux` (Go 1.22+ pattern matching) |
| Логи | Кастомный `internal/logx` (stdout + in-memory history + SSE) |

---

## Запуск

```bash
go run ./cmd/psr
```

Команды CLI:

```bash
psr events [-status ongoing|upcoming|past]
psr sync teams -team1 <id> -team2 <id> [-months 3]
psr predict -team1 <id> -team2 <id> [-format bo3]
psr backtest
psr calibrate
```

## Переменные окружения

| Переменная | Описание | По умолчанию |
|---|---|---|
| `PSR_DB_PATH` | Путь к SQLite | `data/psr.db` |
| `PSR_API_ADDR` | Адрес HTTP-сервера | `:8080` |
| `PSR_WEIGHTS_PATH` | Файл весов модели | `data/weights.json` |
| `HLTV_COOKIE` | Cookie для обхода Cloudflare | — |
| `HLTV_USE_BROWSER` | Использовать Chrome как fallback | `false` |

## Тесты

```bash
go test ./...
go test -race -cover ./...
