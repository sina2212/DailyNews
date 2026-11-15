CREATE TABLE IF NOT EXISTS news_sources (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  feed_url TEXT NOT NULL UNIQUE,
  category TEXT NOT NULL,
  region TEXT NOT NULL DEFAULT 'world',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS news_articles (
  id BIGSERIAL PRIMARY KEY,
  source_id BIGINT REFERENCES news_sources(id) ON DELETE SET NULL,
  external_id TEXT,
  url TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  raw_text TEXT NOT NULL DEFAULT '',
  language TEXT NOT NULL DEFAULT 'unknown',
  category TEXT NOT NULL DEFAULT 'other',
  region TEXT NOT NULL DEFAULT 'world',
  published_at TIMESTAMPTZ,
  fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  content_hash TEXT NOT NULL,
  processed BOOLEAN NOT NULL DEFAULT FALSE,
  UNIQUE(source_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_articles_published ON news_articles(published_at DESC);
CREATE INDEX IF NOT EXISTS idx_articles_processed ON news_articles(processed, published_at DESC);
CREATE INDEX IF NOT EXISTS idx_articles_hash ON news_articles(content_hash);

CREATE TABLE IF NOT EXISTS news_summaries (
  id BIGSERIAL PRIMARY KEY,
  article_id BIGINT NOT NULL REFERENCES news_articles(id) ON DELETE CASCADE,
  title_fa TEXT NOT NULL,
  summary_fa TEXT NOT NULL,
  importance SMALLINT NOT NULL CHECK (importance BETWEEN 1 AND 10),
  category TEXT NOT NULL,
  key_points JSONB NOT NULL DEFAULT '[]'::jsonb,
  model TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(article_id)
);

CREATE TABLE IF NOT EXISTS daily_digests (
  id BIGSERIAL PRIMARY KEY,
  digest_date DATE NOT NULL UNIQUE,
  timezone TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft',
  body TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  sent_at TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS digest_items (
  digest_id BIGINT NOT NULL REFERENCES daily_digests(id) ON DELETE CASCADE,
  summary_id BIGINT NOT NULL REFERENCES news_summaries(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  PRIMARY KEY(digest_id, summary_id),
  UNIQUE(digest_id, position)
);
CREATE TABLE IF NOT EXISTS telegram_subscribers (
  id BIGSERIAL PRIMARY KEY,
  chat_id TEXT NOT NULL UNIQUE,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS delivery_logs (
  id BIGSERIAL PRIMARY KEY,
  digest_id BIGINT REFERENCES daily_digests(id) ON DELETE SET NULL,
  chat_id TEXT NOT NULL,
  status TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
