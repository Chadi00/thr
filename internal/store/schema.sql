CREATE TABLE IF NOT EXISTS schema_migrations (
  name TEXT PRIMARY KEY NOT NULL,
  applied_at INTEGER NOT NULL
);

INSERT OR IGNORE INTO schema_migrations(name, applied_at) VALUES
  ('001_init.sql', CAST(strftime('%s', 'now') AS INTEGER) * 1000),
  ('002_indexes.sql', CAST(strftime('%s', 'now') AS INTEGER) * 1000),
  ('003_drop_schema_version.sql', CAST(strftime('%s', 'now') AS INTEGER) * 1000),
  ('004_embedding_metadata.sql', CAST(strftime('%s', 'now') AS INTEGER) * 1000);

CREATE TABLE scopes (
  id TEXT PRIMARY KEY NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('user', 'repo')),
  label TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  CHECK ((id = 'user') = (kind = 'user'))
);
CREATE UNIQUE INDEX idx_scopes_single_user ON scopes(kind) WHERE kind = 'user';
INSERT INTO scopes(id, kind, label, created_at, updated_at)
VALUES ('user', 'user', 'user', CAST(strftime('%s', 'now') AS INTEGER) * 1000, CAST(strftime('%s', 'now') AS INTEGER) * 1000);
CREATE TRIGGER scopes_no_delete BEFORE DELETE ON scopes BEGIN
  SELECT RAISE(ABORT, 'scopes cannot be deleted');
END;
CREATE TRIGGER scopes_identity_immutable BEFORE UPDATE OF id, kind ON scopes BEGIN
  SELECT RAISE(ABORT, 'scope identity is immutable');
END;

CREATE TABLE repository_bindings (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  scope_id TEXT NOT NULL REFERENCES scopes(id) ON DELETE RESTRICT,
  common_dir TEXT NOT NULL,
  worktree_root TEXT NOT NULL DEFAULT '',
  remote_name TEXT NOT NULL DEFAULT '',
  remote_url TEXT NOT NULL DEFAULT '',
  canonical_remote TEXT NOT NULL DEFAULT '',
  normalization_version INTEGER NOT NULL DEFAULT 0,
  active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX idx_repository_bindings_active_common_dir
ON repository_bindings(common_dir) WHERE active = 1;

CREATE TABLE repository_remote_aliases (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  scope_id TEXT NOT NULL REFERENCES scopes(id) ON DELETE RESTRICT,
  binding_id INTEGER,
  canonical_remote TEXT NOT NULL,
  normalization_version INTEGER NOT NULL,
  active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
  intentionally_non_unique INTEGER NOT NULL DEFAULT 0 CHECK (intentionally_non_unique IN (0, 1)),
  observed_at INTEGER NOT NULL
);
CREATE INDEX idx_repository_remote_aliases_active
ON repository_remote_aliases(canonical_remote) WHERE active = 1;

CREATE TABLE repository_binding_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  binding_id INTEGER NOT NULL,
  scope_id TEXT NOT NULL,
  action TEXT NOT NULL,
  common_dir TEXT NOT NULL,
  canonical_remote TEXT NOT NULL DEFAULT '',
  observed_at INTEGER NOT NULL
);

CREATE TABLE scoped_memories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  text TEXT NOT NULL,
  scope_id TEXT NOT NULL REFERENCES scopes(id) ON DELETE RESTRICT,
  scope_assignment TEXT NOT NULL CHECK (scope_assignment IN ('automatic_context', 'explicit', 'legacy_default')),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  scope_updated_at INTEGER NOT NULL,
  revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0)
);
CREATE INDEX idx_scoped_memories_created ON scoped_memories(scope_id, created_at DESC);
CREATE INDEX idx_scoped_memories_updated ON scoped_memories(scope_id, updated_at DESC);

CREATE VIRTUAL TABLE scoped_memory_embeddings USING vec0(
  scope_id TEXT PARTITION KEY,
  embedding float[768]
);

CREATE TABLE scoped_memory_embedding_metadata (
  memory_id INTEGER PRIMARY KEY NOT NULL REFERENCES scoped_memories(id) ON DELETE CASCADE,
  model_id TEXT NOT NULL,
  model_revision TEXT NOT NULL,
  manifest_sha256 TEXT NOT NULL,
  dimension INTEGER NOT NULL,
  indexed_at INTEGER NOT NULL
);
CREATE INDEX idx_scoped_embedding_identity
ON scoped_memory_embedding_metadata(model_id, model_revision, manifest_sha256, dimension);

CREATE VIRTUAL TABLE scoped_memory_fts USING fts5(
  text,
  tokenize='porter unicode61',
  content='scoped_memories',
  content_rowid='id'
);

CREATE TRIGGER scoped_memories_ai AFTER INSERT ON scoped_memories BEGIN
  INSERT INTO scoped_memory_fts(rowid, text) VALUES (new.id, new.text);
END;
CREATE TRIGGER scoped_memories_ad AFTER DELETE ON scoped_memories BEGIN
  INSERT INTO scoped_memory_fts(scoped_memory_fts, rowid, text) VALUES('delete', old.id, old.text);
END;
CREATE TRIGGER scoped_memories_au AFTER UPDATE OF text ON scoped_memories BEGIN
  INSERT INTO scoped_memory_fts(scoped_memory_fts, rowid, text) VALUES('delete', old.id, old.text);
  INSERT INTO scoped_memory_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TABLE database_format (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  version INTEGER NOT NULL
);
INSERT INTO database_format(singleton, version) VALUES (1, 2);
