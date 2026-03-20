-- Rename position_in_tournament to tags and convert to JSONB array
ALTER TABLE images DROP COLUMN IF EXISTS position_in_tournament;
ALTER TABLE images ADD COLUMN tags JSONB DEFAULT '[]';
