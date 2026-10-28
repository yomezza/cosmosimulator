CREATE TABLE IF NOT EXISTS player_ideas (
 id BIGSERIAL PRIMARY KEY,
 user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
 author_name TEXT NOT NULL,
 request_id TEXT NOT NULL CHECK(request_id ~ '^[a-f0-9]{32}$'),
 category TEXT NOT NULL CHECK(category IN('gameplay','interface','economy','other')),
 title TEXT NOT NULL CHECK(char_length(title) BETWEEN 3 AND 80),
 description TEXT NOT NULL CHECK(char_length(description) BETWEEN 10 AND 3000),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(user_id,request_id)
);
CREATE INDEX IF NOT EXISTS player_ideas_author_date ON player_ideas(user_id,created_at DESC,id DESC);
