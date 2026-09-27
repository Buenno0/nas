-- Quanto cada pessoa assistiu de cada arquivo em cada dia, para a
-- retrospectiva. `progress` guarda só a última posição; isto soma o tempo
-- de fato assistido (seeks e pulos não contam).
CREATE TABLE historico (
    user_id       INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    media_file_id INTEGER NOT NULL REFERENCES media_files (id) ON DELETE CASCADE,
    dia           TEXT    NOT NULL, -- AAAA-MM-DD, horário local do nó
    segundos      REAL    NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, media_file_id, dia)
);

-- Em que hora do dia se assiste, por ano (a retrospectiva mostra o horário
-- preferido).
CREATE TABLE historico_horas (
    user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    ano      INTEGER NOT NULL,
    hora     INTEGER NOT NULL CHECK (hora BETWEEN 0 AND 23),
    segundos REAL    NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, ano, hora)
);

-- A retrospectiva não começa vazia: o que já foi visto conta uma vez, no dia
-- da última atualização.
INSERT OR IGNORE INTO historico (user_id, media_file_id, dia, segundos)
SELECT user_id, media_file_id, date(updated_at, 'unixepoch', 'localtime'),
       CASE WHEN finished = 1 THEN duration_sec ELSE position_sec END
  FROM progress
 WHERE position_sec > 0 OR finished = 1;

-- Recomendações do TMDB já filtradas ao acervo, guardadas por uma semana:
-- a Home não chama o TMDB a cada abertura.
CREATE TABLE recomendacoes (
    title_id  INTEGER PRIMARY KEY REFERENCES titles (id) ON DELETE CASCADE,
    tmdb_ids  TEXT    NOT NULL DEFAULT '[]',
    em        INTEGER NOT NULL
);

-- Web Push: uma inscrição por aparelho. `tipos` diz quais avisos ele quer.
CREATE TABLE push_inscricoes (
    id       INTEGER PRIMARY KEY,
    user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    endpoint TEXT    NOT NULL UNIQUE,
    p256dh   TEXT    NOT NULL,
    auth     TEXT    NOT NULL,
    tipos    TEXT    NOT NULL DEFAULT 'envio,preparo,novidade',
    criado   INTEGER NOT NULL,
    falhas   INTEGER NOT NULL DEFAULT 0
);
