-- +goose Up
ALTER TABLE analysis_shares
    ADD COLUMN IF NOT EXISTS token_ciphertext TEXT;

UPDATE analysis_shares
SET revoked_at = now()
WHERE token_ciphertext IS NULL
  AND revoked_at IS NULL;

UPDATE analysis_shares
SET token_ciphertext = COALESCE(token_ciphertext, '')
WHERE token_ciphertext IS NULL;

ALTER TABLE analysis_shares
    ALTER COLUMN token_ciphertext SET NOT NULL;

-- +goose Down
ALTER TABLE analysis_shares
    DROP COLUMN IF EXISTS token_ciphertext;
