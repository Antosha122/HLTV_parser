package storage

const schema = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS teams (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    hltv_rating REAL,
    world_rank  INTEGER,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS players (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    team_id    INTEGER NOT NULL REFERENCES teams(id),
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    status     TEXT,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS matches (
    id         INTEGER PRIMARY KEY,
    event_id   INTEGER REFERENCES events(id),
    event_name TEXT,
    team1_id   INTEGER NOT NULL REFERENCES teams(id),
    team2_id   INTEGER NOT NULL REFERENCES teams(id),
    format     TEXT,
    match_date TEXT,
    winner_id  INTEGER,
    stars      INTEGER,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS match_maps (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    match_id    INTEGER NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    map_name    TEXT NOT NULL,
    team1_score INTEGER,
    team2_score INTEGER,
    picked_by   INTEGER
);

CREATE TABLE IF NOT EXISTS vetoes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    match_id   INTEGER NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    veto_order INTEGER NOT NULL,
    action     TEXT NOT NULL,
    team_id    INTEGER,
    team_name  TEXT,
    map_name   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS team_map_stats (
    team_id      INTEGER NOT NULL REFERENCES teams(id),
    map_name     TEXT NOT NULL,
    wins         INTEGER NOT NULL DEFAULT 0,
    losses       INTEGER NOT NULL DEFAULT 0,
    pick_rate    REAL,
    ban_rate     REAL,
    period_start TEXT NOT NULL,
    period_end   TEXT NOT NULL,
    PRIMARY KEY (team_id, map_name, period_start)
);

CREATE TABLE IF NOT EXISTS event_teams (
    event_id   INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    team_id    INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (event_id, team_id)
);

CREATE TABLE IF NOT EXISTS sync_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL,
    entity_id   INTEGER NOT NULL,
    synced_at   TEXT NOT NULL,
    status      TEXT NOT NULL,
    message     TEXT
);
`
