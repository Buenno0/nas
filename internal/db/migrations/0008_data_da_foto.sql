-- Data de captura das fotos.
--
-- Sem isto a galeria ordena por nome de arquivo, o que transforma um álbum em
-- uma pasta: IMG_4471 antes de IMG_4472 não conta história nenhuma.
--
-- Fica zero quando o arquivo não diz — e quem lê usa o mtime como reserva. Zero
-- é honesto: significa "o arquivo não sabe", e não uma data inventada.
ALTER TABLE media_files ADD COLUMN taken_at INTEGER NOT NULL DEFAULT 0;

-- A linha do tempo é sempre do mais recente para o mais antigo, e sempre
-- dentro de uma biblioteca.
CREATE INDEX idx_files_taken ON media_files (library_id, taken_at DESC);

-- Fotos já indexadas nunca tiveram a data lida. Elas não passam pelo ffprobe,
-- então não têm probed_at para zerar: a marca de "precisa reexaminar" é o
-- mtime impossível, que força o próximo scan a reprocessá-las.
UPDATE media_files SET mtime = -1 WHERE media_type = 'photo';

ANALYZE;
