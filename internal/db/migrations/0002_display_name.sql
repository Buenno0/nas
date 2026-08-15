-- Nome vindo das tags do arquivo (título da faixa em MP3/FLAC, por exemplo).
-- Quando existe, vale mais do que o nome do arquivo na interface.
ALTER TABLE media_files ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
