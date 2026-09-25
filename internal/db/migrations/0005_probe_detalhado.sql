-- Detalhes técnicos que decidem se um arquivo toca no navegador.
--
-- vcodec = 'h264' não basta: H.264 High 10 (pix_fmt yuv420p10le) e 4:2:2 são
-- h264 legítimos que navegador nenhum decodifica. Sem pix_fmt e profile, o
-- pipeline mandaria esses arquivos para direct play e entregaria tela preta.
ALTER TABLE media_files ADD COLUMN pix_fmt TEXT NOT NULL DEFAULT '';
ALTER TABLE media_files ADD COLUMN vprofile TEXT NOT NULL DEFAULT '';
ALTER TABLE media_files ADD COLUMN channels INTEGER NOT NULL DEFAULT 0;
ALTER TABLE media_files ADD COLUMN vbitrate INTEGER NOT NULL DEFAULT 0;
