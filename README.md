# PSR — CS2 Match Predictor (HLTV)

Приложение на Go для прогнозирования исходов CS2-матчей на основе данных HLTV.
Парсит турниры, команды, результаты матчей и статистику карт, сохраняет данные в SQLite и рассчитывает вероятности победы с помощью ансамбля моделей: Elo, форма, head-to-head и veto-симуляция.

## Возможности

- Парсинг HLTV через HTTP+cookie с обходом Cloudflare (fallback на Chrome/CDP).
- Хранение данных в SQLite (команды, игроки, матчи, veto, статистика карт).
- Прогноз матча: вероятности победы, счёт серии, veto, пул карт, h2h и форма.
- Бэктест точности модели + автокалибровка весов (grid search).
- Веб-UI (`/api/*`, встроенная статика) с live-логами (SSE).

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