-- Esquema inicial do NAS.

CREATE TABLE users (
    id                   INTEGER PRIMARY KEY,
    username             TEXT    NOT NULL UNIQUE,
    password_hash        TEXT    NOT NULL,
    must_change_password INTEGER NOT NULL DEFAULT 0,
    created_at           INTEGER NOT NULL
);

CREATE TABLE sessions (
    token      TEXT    PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    user_agent TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE INDEX idx_sessions_user ON sessions (user_id);
CREATE INDEX idx_sessions_expires ON sessions (expires_at);

CREATE TABLE libraries (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL,
    path       TEXT    NOT NULL UNIQUE,
    kind       TEXT    NOT NULL CHECK (kind IN ('movie', 'tv', 'music', 'photo')),
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    scanned_at INTEGER
);

-- Um "título" é a unidade que aparece na interface: um filme, uma série,
-- um álbum ou um conjunto de fotos. Vários arquivos podem apontar para ele.
CREATE TABLE titles (
    id         INTEGER PRIMARY KEY,
    library_id INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    kind       TEXT    NOT NULL CHECK (kind IN ('movie', 'tv', 'album', 'photos')),
    name       TEXT    NOT NULL,
    sort_name  TEXT    NOT NULL,
    year       INTEGER,
    overview   TEXT    NOT NULL DEFAULT '',
    rating     REAL,
    genres     TEXT    NOT NULL DEFAULT '',
    artist     TEXT    NOT NULL DEFAULT '',
    tmdb_id    INTEGER,
    poster     TEXT    NOT NULL DEFAULT '',
    backdrop   TEXT    NOT NULL DEFAULT '',
    -- pending: ainda não consultado | matched: casou no TMDB
    -- unmatched: sem match confiável | manual: corrigido à mão
    meta_state TEXT    NOT NULL DEFAULT 'pending',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX idx_titles_library ON titles (library_id);
CREATE UNIQUE INDEX idx_titles_identity
    ON titles (library_id, kind, sort_name, IFNULL(year, 0));

CREATE TABLE media_files (
    id         INTEGER PRIMARY KEY,
    library_id INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    title_id   INTEGER REFERENCES titles (id) ON DELETE SET NULL,
    path       TEXT    NOT NULL UNIQUE,
    rel_path   TEXT    NOT NULL,
    ext        TEXT    NOT NULL,
    size       INTEGER NOT NULL,
    mtime      INTEGER NOT NULL,
    media_type TEXT    NOT NULL CHECK (media_type IN ('video', 'audio', 'photo')),
    duration   REAL,
    width      INTEGER,
    height     INTEGER,
    vcodec     TEXT    NOT NULL DEFAULT '',
    acodec     TEXT    NOT NULL DEFAULT '',
    track      INTEGER,
    probed_at  INTEGER,
    thumb      TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

CREATE INDEX idx_files_title ON media_files (title_id);
CREATE INDEX idx_files_library ON media_files (library_id);

CREATE TABLE episodes (
    id            INTEGER PRIMARY KEY,
    title_id      INTEGER NOT NULL REFERENCES titles (id) ON DELETE CASCADE,
    media_file_id INTEGER REFERENCES media_files (id) ON DELETE CASCADE,
    season        INTEGER NOT NULL,
    episode       INTEGER NOT NULL,
    name          TEXT    NOT NULL DEFAULT '',
    overview      TEXT    NOT NULL DEFAULT '',
    still         TEXT    NOT NULL DEFAULT '',
    air_date      TEXT    NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX idx_episodes_identity ON episodes (title_id, season, episode);

CREATE TABLE progress (
    user_id       INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    media_file_id INTEGER NOT NULL REFERENCES media_files (id) ON DELETE CASCADE,
    position_sec  REAL    NOT NULL DEFAULT 0,
    duration_sec  REAL    NOT NULL DEFAULT 0,
    finished      INTEGER NOT NULL DEFAULT 0,
    updated_at    INTEGER NOT NULL,
    PRIMARY KEY (user_id, media_file_id)
);

CREATE INDEX idx_progress_recent ON progress (user_id, updated_at DESC);

CREATE TABLE favorites (
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title_id   INTEGER NOT NULL REFERENCES titles (id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, title_id)
);
