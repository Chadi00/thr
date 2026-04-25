CREATE TABLE IF NOT EXISTS memory_embedding_metadata (
  memory_id INTEGER PRIMARY KEY NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
  model_id TEXT NOT NULL,
  model_revision TEXT NOT NULL,
  manifest_sha256 TEXT NOT NULL,
  dimension INTEGER NOT NULL,
  indexed_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_embedding_metadata_identity
ON memory_embedding_metadata(model_id, model_revision, manifest_sha256, dimension);
