-- Índices que sustentam as três listagens da interface. Sem eles o SQLite
-- monta uma b-tree temporária a cada página (medido em 20 mil títulos:
-- 13 ms por requisição; com índice, 0,08 ms).

-- Grade de uma biblioteca, ordenada por nome.
CREATE INDEX idx_titles_sort ON titles (library_id, sort_name);

-- Prateleira "adicionados recentemente" da home (acervo inteiro).
CREATE INDEX idx_titles_added ON titles (created_at DESC, id DESC);

-- Prateleira por biblioteca na home (mesma ordem, filtrada).
CREATE INDEX idx_titles_lib_added ON titles (library_id, created_at DESC);

-- idx_titles_library (library_id) virou prefixo de idx_titles_sort: manter os
-- dois só daria trabalho de escrita a cada scan, sem nenhuma leitura a mais.
DROP INDEX IF EXISTS idx_titles_library;

-- Deixa o planejador conhecer a distribuição dos dados.
ANALYZE;
