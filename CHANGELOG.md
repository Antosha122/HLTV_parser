# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `StatusTracker` and `CancelManager` types: instance-based progress and
  cancellation state for `sync.Service`, replacing package-level globals so
  multiple services can run in isolation (P0.2).
- `Service.Tracker()` and `Service.Cancel()` accessors expose the per-instance
  state to the API layer.
- Domain error sentinels: `predict.ErrInsufficientData`,
  `predict.ErrInvalidArgument`, `storage.ErrNotFound`, `storage.ErrConflict`,
  `hltv.ErrBlocked`.
- `internal/sync/refresh_test.go` covering `Refresh` happy path, source error,
  context cancellation, no-data guard, `CancelManager` lifecycle, and tracker
  isolation across services (sync coverage 10% → 33%).
- `LICENSE` (MIT).

### Changed
- `sync.Service` now owns its own `StatusTracker` and `CancelManager`; all
  package-level status/cancel calls inside the service were replaced by
  instance methods. Package-level wrappers remain for backward compatibility
  with `cmd/psr`.
- API handlers now read status/cancellation from the active `sync.Service`
  instance instead of global state.
- Ignored errors (`_ = s.db.LogSync(...)`, `_ = s.db.UpsertEventTeam(...)`,
  `_ = s.db.UpsertEvent(...)`) are now logged via best-effort helpers instead
  of being silently swallowed (P0.6).

## [0.1.0] - Initial audit baseline

- Layered `internal/` architecture with consumer-defined interfaces
  (`Source`, `Store`, `DataProvider`).
- SQLite storage with idempotent migrations, WAL mode, `busy_timeout`,
  indexed lookup columns, and an Elo ratings cache table.
- HLTV scraping with HTTP+cookie primary and Chrome/CDP fallback, rate-limit
  cooldown, and retries.
- Prediction ensemble (Elo / Form / H2H / Maps) with veto simulation, backtest
  (log-loss / Brier), and grid-search weight calibration.
- Web UI with SSE live logs, CLI, `.golangci.yml`, `Makefile`, `Dockerfile`,
  and GitHub Actions CI (lint / test + coverage / cross-build / docker).