CREATE TABLE domjudge_connection (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  base_url TEXT NOT NULL,
  user TEXT NOT NULL,
  password TEXT NOT NULL,
  contest_id TEXT NOT NULL
);

CREATE TABLE settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  timezone TEXT,
  loom_url TEXT NOT NULL DEFAULT 'https://loom.gehack.nl'
);

INSERT INTO settings (id) VALUES (1);

CREATE TABLE group_filters (
  group_id TEXT NOT NULL,
  filter TEXT NOT NULL CHECK (filter IN ('hidden', 'no_first_solve')),
  PRIMARY KEY (group_id, filter)
);
