CREATE TABLE IF NOT EXISTS memories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  text TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS memory_embeddings USING vec0(
  embedding float[768]
);

CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
  text,
  tokenize='porter unicode61',
  content='memories',
  content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
  INSERT INTO memory_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
  INSERT INTO memory_fts(memory_fts, rowid, text) VALUES('delete', old.id, old.text);
END;

CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
  INSERT INTO memory_fts(memory_fts, rowid, text) VALUES('delete', old.id, old.text);
  INSERT INTO memory_fts(rowid, text) VALUES (new.id, new.text);
END;
