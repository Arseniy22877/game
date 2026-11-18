CREATE TABLE games (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  phase         TEXT    NOT NULL,              -- lobby | round_open | round_closed | finished
  round         INTEGER NOT NULL DEFAULT 0,
  active        INTEGER NOT NULL DEFAULT 1,
  scenario_id   TEXT    NOT NULL DEFAULT '',
  scenario_json TEXT,                          -- копия файла сценария на момент старта
  created_at    TEXT    NOT NULL,
  started_at    TEXT,
  finished_at   TEXT
);
CREATE UNIQUE INDEX games_one_active ON games(active) WHERE active = 1;

CREATE TABLE teams (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  game_id    INTEGER NOT NULL REFERENCES games(id),
  name       TEXT    NOT NULL,
  name_key   TEXT    NOT NULL,                 -- lower(trim(name)), считает Go
  members    TEXT    NOT NULL,                 -- JSON ["Аня","Петя"]
  token      TEXT    NOT NULL UNIQUE,
  ready      INTEGER NOT NULL DEFAULT 1,
  created_at TEXT    NOT NULL,
  UNIQUE (game_id, name_key)
);

CREATE TABLE decisions (                       -- последнее решение команды за раунд
  team_id    INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  round      INTEGER NOT NULL,
  decision   TEXT    NOT NULL,                 -- JSON api.Decision
  cost_money INTEGER NOT NULL,
  cost_qp    INTEGER NOT NULL,
  auto       INTEGER NOT NULL DEFAULT 0,       -- решение по умолчанию при force
  updated_at TEXT    NOT NULL,
  PRIMARY KEY (team_id, round)
);

CREATE TABLE round_results (                   -- строка появляется при закрытии раунда
  team_id            INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  round              INTEGER NOT NULL,
  predictions        TEXT    NOT NULL,         -- JSON []api.Prediction
  state_after_calc   TEXT    NOT NULL,         -- JSON api.TeamGameState
  fired_events       TEXT,                     -- JSON [4,7] в порядке применения; NULL, пока препод не выбрал
  state_after_events TEXT,                     -- JSON api.TeamGameState
  closed_at          TEXT    NOT NULL,
  events_at          TEXT,
  PRIMARY KEY (team_id, round)
);

CREATE TABLE grades (
  team_id      INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  skill      TEXT    NOT NULL,                 -- код навыка из api.Skills
  score      INTEGER NOT NULL,
  comment    TEXT    NOT NULL DEFAULT '',
  updated_at TEXT    NOT NULL,
  PRIMARY KEY (team_id, skill)
);
