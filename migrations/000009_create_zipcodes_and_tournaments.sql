-- 1. zipcode_scrape_scopes: one row per (zipcode, distance), 3 rows per zipcode
CREATE TABLE IF NOT EXISTS zipcode_scrape_scopes (
    zipcode      VARCHAR(10),
    distance     INT,
    last_scraped TIMESTAMPTZ,
    PRIMARY KEY (zipcode, distance)
);

-- 2. tournaments: deduplicated tournament info, unique by url_path
CREATE TABLE IF NOT EXISTS tournaments (
    id          SERIAL PRIMARY KEY,
    title       TEXT DEFAULT '',
    url_path    TEXT UNIQUE DEFAULT '',
    city        TEXT DEFAULT '',
    state       TEXT DEFAULT '',
    dates       TEXT DEFAULT '',
    start_date  DATE,
    organizer   TEXT DEFAULT '',
    description TEXT DEFAULT '',
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- 3. tournaments_within_radius: junction table
CREATE TABLE IF NOT EXISTS tournaments_within_radius (
    zipcode       VARCHAR(10),
    distance      INT,
    tournament_id INT REFERENCES tournaments(id) ON DELETE CASCADE,
    PRIMARY KEY (zipcode, distance, tournament_id),
    FOREIGN KEY (zipcode, distance)
      REFERENCES zipcode_scrape_scopes(zipcode, distance) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_twr_tournament_id ON tournaments_within_radius(tournament_id);
CREATE INDEX IF NOT EXISTS idx_tournaments_start_date ON tournaments(start_date);
CREATE INDEX IF NOT EXISTS idx_tournaments_url_path ON tournaments(url_path);
