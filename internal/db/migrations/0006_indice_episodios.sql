-- Toda listagem de arquivos faz LEFT JOIN episodes ON e.media_file_id = f.id,
-- e essa coluna não tinha índice. O SQLite não fazia o pior caso (varredura
-- por linha): ele construía um AUTOMATIC COVERING INDEX temporário a cada
-- execução — visível no EXPLAIN QUERY PLAN — e jogava fora no fim da query.
--
-- Ou seja, a página de uma série pagava para reindexar os episódios toda vez
-- que era aberta. Medido numa série de 1000 episódios: 2,4 ms por abertura só
-- montando esse índice descartável.
CREATE INDEX idx_episodes_file ON episodes (media_file_id);

-- Ganho de tabela: media_file_id é REFERENCES media_files ON DELETE CASCADE.
-- Sem índice, cada arquivo removido num scan obrigava o SQLite a varrer a
-- tabela de episódios inteira para achar as filhas.

ANALYZE;
