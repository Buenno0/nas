-- Coleções: agrupamento feito à mão, que o TMDB não sabe fazer.
--
-- Até aqui a única curadoria possível era `favorites`, um booleano por título.
-- Dava para dizer "gosto disto", nunca "isto vai junto": uma trilogia, uma
-- maratona, o que assistir com alguém específico.
CREATE TABLE collections (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- Dois nomes iguais na mesma conta seriam duas listas indistinguíveis na tela.
-- NOCASE porque "Maratona" e "maratona" são a mesma intenção.
CREATE UNIQUE INDEX idx_collections_nome ON collections (user_id, name COLLATE NOCASE);

CREATE TABLE collection_items (
    collection_id INTEGER NOT NULL REFERENCES collections (id) ON DELETE CASCADE,
    title_id      INTEGER NOT NULL REFERENCES titles (id) ON DELETE CASCADE,
    -- Posição escolhida por quem montou. Uma trilogia fora de ordem não é uma
    -- trilogia, então ordem alfabética não serviria.
    position      INTEGER NOT NULL DEFAULT 0,
    added_at      INTEGER NOT NULL,
    PRIMARY KEY (collection_id, title_id)
);

CREATE INDEX idx_collection_items_ordem ON collection_items (collection_id, position);
