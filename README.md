# PSR — CS2 Match Predictor (HLTV)


https://burkinanton.ru/

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

## Архитектура

```
┌──────────────────────────────────────────────────────────────┐
│                        cmd/psr (CLI)                         │
│   serve │ predict │ sync │ backtest │ calibrate │ events ... │
└──────┬────────────────────────────────────────────┬──────────┘
       │                                            │
       ▼                                            ▼
┌─────────────┐     ┌──────────────┐      ┌──────────────────┐
│  internal/  │     │  internal/   │      │   internal/api   │
│   predict   │     │    sync      │      │  (HTTP + static) │
│             │     │              │      │                  │
│ Elo · Form  │     │ events ·     │      │ /api/predict     │
│ H2H · Maps  │     │ matches ·    │      │ /api/sync        │
│ Veto · Bo3  │     │ teams ·      │      │ /api/logs (SSE)  │
│ Backtest    │     │ map stats    │      │ /api/teams       │
│ Calibrate   │     │              │      │                  │
└──────┬──────┘     └──────┬───────┘      └────────┬─────────┘
       │                   │                       │
       ▼                   ▼                       ▼
┌──────────────────────────────────────────────────────────────┐
│                     internal/storage                         │
│                     SQLite (WAL mode)                        │
│  teams · players · matches · events · vetoes · team_map_stats│
└──────────────────────────────────────────────────────────────┘
       ▲
       │
┌──────┴───────────────────────────────────────────────────────┐
│                     internal/hltv                            │
│   HTTP+cookie (tls-client) → Chrome/CDP (go-rod) fallback    │
│   Парсеры: events, results, teams, match, ranking, maps      │
└──────────────────────────────────────────────────────────────┘
```

**Поток данных:**

1. **Сбор** (`internal/hltv`): HTTP-запросы к HLTV с TLS-фингерпринтом и cookie `cf_clearance` для обхода Cloudflare. При отсутствии cookie — автоматизация реального Chrome через CDP.
2. **Синхронизация** (`internal/sync`): оркестрация загрузки — турниры → участники → матчи → история команд → статистика карт. Идемпотентные upsert-операции.
3. **Хранение** (`internal/storage`): SQLite с миграциями, WAL-режимом и `busy_timeout`.
4. **Прогноз** (`internal/predict`): ансамбль из 4 сигналов + veto-симуляция; кэш Elo-рейтингов для производительности.
5. **API/UI** (`internal/api`): HTTP-эндпоинты + встроенный веб-интерфейс с live-логами через SSE.

---

## Быстрый старт

### Требования

- **Go 1.25+** ([скачать](https://go.dev/dl/))
- **Google Chrome** (для режима обхода Cloudflare через CDP)
- ОС: Windows / Linux / macOS

### Первый запуск

```bash
# 1. Клонировать репозиторий
git clone https://github.com/Antosha122/HLTV_parser.git
cd HLTV_parser

# 2. Запустить веб-интерфейс (сборка + автозапуск браузера)
go run ./cmd/psr
```

Программа:
1. Создаст базу данных `data/psr.db`
2. Запустит HTTP-сервер на `:8080`
3. Откроет Chrome с профилем PSR

**В Windows** используйте готовые скрипты:

```cmd
run.bat     :: Сборка + запуск веб-интерфейса (HTTP-режим, cookie)
sync.bat    :: Интерактивная синхронизация двух команд
stop.bat    :: Остановка всех процессов PSR
```

### Первичная загрузка данных (через веб-интерфейс)

1. Нажмите **«1. Открыть HLTV»** — откроется Chrome с профилем PSR.
2. Пройдите капчу Cloudflare, откройте страницу **Events** на HLTV.
3. Скопируйте cookie `cf_clearance` и `__cf_bm` (F12 → Application → Cookies).
4. Вставьте в форму, нажмите **«Сохранить cookie»**.
5. Нажмите **«2. Подключиться»** — проверка соединения с HLTV.
6. Нажмите **«3. Обновить турниры и команды»** — начнётся загрузка.

> **Альтернатива (CLI):** см. раздел [Команды CLI](#команды-cli).

---

## Сборка

### Локальная сборка

```bash
# Через go build
go build -trimpath -o bin/psr ./cmd/psr

# Через Makefile (требуется GNU Make)
make build
make run       # сборка + запуск
```

### Флаги сборки

```bash
go build -trimpath \
    -ldflags "-s -w -X main.version=$(git describe --tags --always)" \
    -o bin/psr ./cmd/psr
```

---

## Команды CLI

Запуск без аргументов эквивалентен `serve` с автозапуском браузера:

```bash
go run ./cmd/psr
```

### Турниры и команды

```bash
# Список турниров HLTV
psr events [-status ongoing|upcoming|past]

# Поиск команды по названию
psr teams -search "navi"
```

### Синхронизация данных

```bash
# Полная синхронизация команды (матчи + профиль + статистика карт)
psr sync team -team <id> [-months 3] [-db path]

# Один матч (полная страница)
psr sync match -match <id> [-db path]

# Две команды (для прогноза: матчи обеих + h2h)
psr sync teams -team1 <id> -team2 <id> [-months 3]
```

Флаг `-months` задаёт глубину истории (по умолчанию 3 месяца).

### Просмотр данных

```bash
# Карточка команды (рейтинг, игроки, последние матчи)
psr show team -id <id>

# Детали матча (счёт, карты, veto, h2h)
psr show match -id <id>

# Личные встречи двух команд
psr show h2h -team1 <id> -team2 <id>

# Статистика БД (количество записей в таблицах)
psr db stats
```

### Прогноз

```bash
# Прогноз матча (Bo1 / Bo3 / Bo5)
psr predict -team1 <id> -team2 <id> [-format bo3] [-db path]
```

Вывод включает:
- Вероятности победы каждой команды
- Прогноз счёта серии (`2-0`, `2-1`, `1-2`, `0-2`)
- Разбор по моделям (Elo / Форма / H2H / Карты / Итог)
- Симуляцию veto (баны/пики/decider)
- Вероятности по каждой карте активного пула
- Оценку уверенности (confidence) на основе объёма данных

### Бэктест и калибровка

```bash
# Бэктест: точность модели на сохранённых матчах
psr backtest [-samples 15] [-warmup 30]

# Автокалибровка весов ансамбля (grid search → data/weights.json)
psr calibrate [-weights data/weights.json] [-warmup 30]
```

### Сервер

```bash
psr serve [-addr :8080] [-db data/psr.db]
```

---

## Веб-интерфейс

Доступен по адресу `http://127.0.0.1:8080` после запуска.

### Вкладки

| Вкладка | Функциональность |
|---|---|
| **Данные** | Управление cookie HLTV, подключение, обновление турниров и команд, live-лог, очистка БД |
| **Прогноз** | Выбор турнира и команд, формат матча, результат с вероятностями, veto, картами |
| **Команда** | Профиль команды: игроки, статистика карт, турниры, последние матчи |

### Особенности

- **Live-логи** через Server-Sent Events (`/api/logs/stream`) — выводятся в реальном времени.
- **Индикатор прогресса** синхронизации (фаза + детали) обновляется автоматически.
- **Остановка** длительной синхронизации в один клик.

---

## HTTP API

### Основные эндпоинты

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/api/health` | Проверка работоспособности сервера |
| `GET` | `/api/teams?search=&limit=` | Список команд (с метаданными) |
| `GET` | `/api/teams/{id}/profile?sync=0\|1` | Профиль команды (`sync=1` — обновить с HLTV) |
| `POST` | `/api/teams/{id}/sync` | Запустить синхронизацию команды в фоне |
| `POST` | `/api/teams/sync-all` | Синхронизация всех команд |
| `GET` | `/api/events` | Список турниров |
| `GET` | `/api/events/{id}/teams` | Команды-участницы турнира |
| `POST` | `/api/refresh` | Полное обновление (турниры + рейтинг + история) |
| `POST` | `/api/refresh/events` | Обновить только список турниров |
| `POST` | `/api/sync/stop` | Остановить текущую синхронизацию |
| `GET` | `/api/sync/status` | Статус синхронизации + статистика БД |
| `POST` | `/api/predict` | Прогноз матча |
| `GET` | `/api/backtest?samples=&warmup=` | Бэктест модели |
| `GET` | `/api/weights` | Текущие веса ансамбля |
| `GET` | `/api/db/stats` | Количество записей по таблицам |
| `POST` | `/api/db/clear` | Очистка базы данных |
| `POST` | `/api/cookie` | Сохранить cookie HLTV |
| `GET` | `/api/cookie/status` | Статус cookie (наличие / валидность) |
| `POST` | `/api/hltv/open` | Открыть HLTV в Chrome с профилем PSR |
| `POST` | `/api/connect` | Подключиться к открытой вкладке Chrome |
| `GET` | `/api/logs` | История логов (JSON-массив) |
| `GET` | `/api/logs/stream` | Live-логи (SSE, `text/event-stream`) |

### Пример: запрос прогноза

```bash
curl -X POST http://127.0.0.1:8080/api/predict \
  -H "Content-Type: application/json" \
  -d '{"team1_id": 4608, "team2_id": 6667, "format": "bo3"}'
```

**Ответ** (сокращённо):

```json
{
  "team1": {"id": 4608, "name": "Natus Vincere"},
  "team2": {"id": 6667, "name": "FaZe"},
  "format": "bo3",
  "win_prob": {"team1": 58.3, "team2": 41.7},
  "series_scores": {"2-0": 31.2, "2-1": 27.1, "1-2": 24.8, "0-2": 16.9},
  "breakdown": {"elo": 60.0, "form": 55.0, "h2h": 52.0, "maps": 58.0, "final": 58.3},
  "confidence": "medium",
  "veto": {"steps": [...]},
  "maps": [{"map_name": "Mirage", "team1_win_pct": 62.0, "in_series": true, "role": "pick"}]
}
```

---

## Переменные окружения

| Переменная | Описание | По умолчанию |
|---|---|---|
| `PSR_DB_PATH` | Путь к файлу SQLite | `data/psr.db` |
| `PSR_API_ADDR` | Адрес HTTP-сервера | `:8080` |
| `PSR_WEIGHTS_PATH` | Файл весов модели (JSON) | `data/weights.json` |
| `PSR_COOKIE_PATH` | Файл хранения cookie | `data/hltv.cookie` |
| `PSR_CHROME_PROFILE` | Директория профиля Chrome | `data/chrome-profile` |
| `PSR_CHROME_DEBUG_PORT` | Порт CDP для подключения к Chrome | `0` (авто) |
| `PSR_ALLOWED_ORIGINS` | Разрешённые CORS-origins (CSV) | `*` |
| `HLTV_BASE_URL` | Базовый URL HLTV | `https://www.hltv.org` |
| `HLTV_COOKIE` | Cookie для обхода Cloudflare | — |
| `HLTV_USE_BROWSER` | Использовать Chrome как основной режим | `false` |
| `HLTV_AUTO_CHROME` | Автоматический запуск Chrome при необходимости | `false` |
| `HLTV_HEADLESS` | Запуск Chrome в headless-режиме | `false` |
| `HLTV_REQUEST_DELAY` | Минимальная пауза между запросами (секунды) | `2` (HTTP: `1.5`) |
| `HLTV_HUMAN_DELAY_MIN` | Мин. «человеческая» пауза (секунды) | `0` |
| `HLTV_HUMAN_DELAY_MAX` | Макс. «человеческая» пауза (секунды) | `1` |
| `HLTV_TIMEOUT` | Таймаут HTTP/CDP-запроса (секунды) | `90` |

> **Примечание:** `HLTV_REQUEST_DELAY` указывается в **секундах** (целое число).

---

## Модель прогнозирования

### Ансамбль из 4 сигналов

Финальная вероятность победы команды 1 — взвешенная сумма четырёх независимых оценок:

| Сигнал | Описание | Вес по умолчанию |
|---|---|---|
| **Elo** | Классический Elo по всей истории матчей (K-factor зависит от формата: Bo1=16, Bo3=24, Bo5=32). При недостатке данных — приор из мирового рейтинга HLTV. | `0.25` |
| **Форма** | Процент побед за последние 15 матчей с регрессией к среднему. | `0.20` |
| **H2H** | Личные встречи (до 50 матчей), байесовское сглаживание. | `0.15` |
| **Карты** | Симуляция veto + вероятности по картам на основе статистики win/loss с Laplace-сглаживанием. | `0.40` |

Веса нормализуются и хранятся в `data/weights.json`:

```json
{
  "elo": 0.25,
  "form": 0.20,
  "h2h": 0.15,
  "maps": 0.40
}
```

### Veto-симуляция (Bo3)

Порядок: `ban → ban → pick → pick → ban → ban → decider`

Для каждого шага карта выбирается по максимизации преимущества:
- **Ban:** банится карта, где преимущество соперника максимально.
- **Pick:** пикуется карта, где собственное преимущество максимально.

### Серия

- **Bo1:** вероятность берётся с первой карты серии.
- **Bo3:** вероятности счёта `2-0`, `2-1`, `1-2`, `0-2` рассчитываются аналитически по вероятностям трёх карт.
- **Bo5:** обрабатывается как Bo3 (в roadmap — полная реализация).

### Кэширование Elo

Рейтинги Elo кэшируются в таблице `elo_ratings`. При новых матчах кэш инвалидируется и пересчитывается. В бэктесте (снапшоты на дату) кэш не используется.

### Бэктест и метрики

| Метрика | Описание |
|---|---|
| **Accuracy** | Доля матчей, где прогноз (P > 50%) совпал с фактом |
| **Log-loss** | `-Σ[p·log(p) + (1-p)·log(1-p)]` — штраф за уверенность в неверном исходе |
| **Brier score** | Средний квадрат отклонения прогноза от факта |

Калибровка перебирает веса из сетки `{0.15, 0.25, 0.35, 0.45}⁴` и выбирает набор с минимальным log-loss.

---

## Работа с Cloudflare

HLTV защищён Cloudflare. PSR поддерживает два режима обхода:

### Режим 1: HTTP + cookie (рекомендуемый)

1. Откройте `https://www.hltv.org` в Chrome (профиль PSR).
2. Пройдите проверку Cloudflare.
3. Скопируйте `cf_clearance` и `__cf_bm` через DevTools (F12 → Application → Cookies).
4. Вставьте в веб-интерфейс или передайте через `HLTV_COOKIE`.

Запросы выполняются через `tls-client` с TLS-фингерпринтом браузера. Пауза между запросами: **4 секунды** (настраивается через `HLTV_REQUEST_DELAY`).

### Режим 2: Chrome / CDP (fallback)

Если cookie недоступен или невалиден, PSR подключается к Chrome через Chrome DevTools Protocol (`go-rod`):

- **Attached-режим:** чтение HTML из вкладки, открытой пользователем (без навигации).
- **Auto-Chrome** (`HLTV_AUTO_CHROME=true`): автоматический запуск Chrome с профилем PSR, до 3 попыток.

### Ограничения

- Cookie `cf_clearance` живёт ограниченное время — периодически обновляйте.
- При капче переключитесь в Chrome PSR и пройдите её вручную.
- Минимальная пауза между запросами в CDP-режиме: **12 секунд**.

---

## Docker

Сборка образа:

```bash
# Через Makefile
make docker

# Напрямую
docker build -t psr:latest .
```

Запуск:

```bash
docker run -d \
  -p 8080:8080 \
  -v psr-data:/app/data \
  -e HLTV_COOKIE="cf_clearance=...; __cf_bm=..." \
  --name psr \
  psr:latest
```

Особенности Dockerfile:
- **Мультистейдж** (golang:1.25-alpine → alpine:3.20)
- **Non-root пользователь** `psr`
- **Healthcheck** на `/api/health`
- **Volume** `/app/data` для SQLite, весов и cookie

---

## Тестирование и линтеры

### Тесты

```bash
# Все тесты
go test ./...

# С race-детектором и покрытием
go test -race -cover ./...

# HTML-отчёт покрытия
make cover
# → coverage.html
```

### Линтеры

```bash
# golangci-lint (см. .golangci.yml)
make lint

# go vet
make vet

# Форматирование
make fmt
```

Включённые линтеры: `errcheck`, `govet`, `staticcheck`, `ineffassign`, `gocritic`, `revive`, `gosec`, `misspell`, `unused`, `gofmt`, `goimports`.

### Тестовые данные

Фикстуры HTML находятся в `testdata/` — покрывают парсеры страниц турниров, матчей, результатов, статистики карт.

---

## Структура проекта

```
.
├── cmd/psr/              # Точка входа (CLI)
├── internal/
│   ├── api/              # HTTP-сервер, хендлеры, SSE-логи, встроенная статика
│   │   └── static/       # Веб-интерфейс (HTML/CSS/JS, embed.go)
│   ├── config/           # Загрузка конфигурации из ENV
│   ├── hltv/             # Клиент HLTV: HTTP+cookie, Chrome/CDP, парсеры
│   ├── logx/             # Логирование (stdout + history + SSE)
│   ├── models/           # Доменные типы (Team, Match, Prediction, Event)
│   ├── platform/         # Открытие Chrome на разных ОС
│   ├── predict/          # Движок прогнозов: Elo, Form, H2H, Maps, Veto, Backtest
│   ├── storage/          # SQLite: миграции, upsert, запросы, кэш Elo
│   └── sync/             # Оркестрация синхронизации данных с HLTV
├── testdata/             # HTML-фикстуры для тестов парсеров
├── data/                 # SQLite, веса, cookie, профиль Chrome (gitignore)
├── .golangci.yml         # Конфигурация линтеров
├── Dockerfile            # Мультистейдж-сборка
├── Makefile              # Команды: build, test, lint, cover, docker
├── run.bat / stop.bat    # Скрипты для Windows
├── sync.bat              # Интерактивная синхронизация (Windows)
└── SENIOR_ROADMAP.md     # Дорожная карта развития проекта
```

---

## Устранение неисправностей

### «403 Forbidden» / «Cloudflare blocked»

- Cookie `cf_clearance` истёк — скопируйте заново из Chrome.
- Увеличьте `HLTV_REQUEST_DELAY` до 4–5 секунд.
- Убедитесь, что копируете cookie с домена `www.hltv.org`, а не с CDN.

### «База данных занята»

- Закройте другие экземпляры PSR (`stop.bat` в Windows).
- Проверьте, что никто другой не открыл `data/psr.db` (например, в DB Browser).
- SQLite использует WAL-режим + `busy_timeout=10s` для конкурентного доступа.

### «Хром не найден»

- Убедитесь, что Google Chrome установлен в стандартной директории.
- Либо задайте путь через переменную окружения `CHROME_PATH`.

### Прогноз показывает «недостаточно данных»

- Запустите `psr sync teams -team1 <id> -team2 <id> -months 3`.
- Проверьте наличие данных: `psr db stats`.
- Минимум ~30 матчей в БД для бэктеста (`-warmup 30`).

### Chrome PSR не подключается

- Нажмите **«Открыть HLTV»** в веб-интерфейсе.
- Не закрывайте окно Chrome PSR до завершения синхронизации.
- Откройте страницу Events и пройдите капчу, если она появилась.

---

## Дорожная карта

Подробный план развития проекта до senior-уровня — в [`SENIOR_ROADMAP.md`](SENIOR_ROADMAP.md).

Ключевые направления:
- **Архитектура:** переход на интерфейсы (consumer-defined), устранение глобального состояния.
- **Модель ML v2:** логистическая регрессия / градиентный бустинг, калибровка вероятностей, BO5.
- **Инфраструктура:** CI/CD (GitHub Actions), метрики Prometheus, structured logging (`slog`).
- **Продукт:** кэш прогнозов, история прогнозов, API-документация (OpenAPI).

---

*Данные предоставлены [HLTV.org](https://www.hltv.org). Проект создан в образовательных целях.*
