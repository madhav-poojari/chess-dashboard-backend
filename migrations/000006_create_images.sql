CREATE TABLE IF NOT EXISTS images (
    id                      SERIAL PRIMARY KEY,
    user_id                 VARCHAR(10) NOT NULL REFERENCES users(id),
    url_suffix              TEXT NOT NULL,
    filename                TEXT NOT NULL,
    title                   TEXT NOT NULL DEFAULT '',
    position_in_tournament  TEXT,
    is_private              BOOLEAN DEFAULT FALSE,
    created_at              TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_images_user_id ON images(user_id);
