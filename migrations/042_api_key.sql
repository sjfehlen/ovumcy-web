-- Per-account API key for non-browser clients (for example a Home Assistant
-- integration). One key per owner, with a fixed scope chosen when it is minted.
--
--   api_key_hash         -- hex HMAC-SHA256 of the full key under a key derived
--                           from SECRET_KEY. The key plaintext is NEVER stored.
--                           The key is 200 bits from crypto/rand, so the request
--                           path looks the row up by this value directly with a
--                           constant-time compare and no bcrypt work factor.
--                           Empty or NULL means the key is off.
--   api_key_scope        -- read or period. Enforced per route in the api layer.
--   api_key_created_at   -- when the current key was minted. Shown on the
--                           settings card so the owner can tell how old it is.
--   api_key_revealed_at  -- the shown-once consumption mark, the same
--                           compare-and-set the recovery code and calendar feed
--                           reveals use. Minting NULLs it and the reveal page
--                           claims it.
--
-- The unique index is PARTIAL so every key-off row can share the empty value.
--
-- The migration runner skips any ADD COLUMN whose column already exists.
--
-- NOTE: keep prose in this file free of semicolons -- the migration runner
-- splits statements on the semicolon character without stripping SQL comments.

ALTER TABLE users ADD COLUMN api_key_hash TEXT;
ALTER TABLE users ADD COLUMN api_key_scope TEXT;
ALTER TABLE users ADD COLUMN api_key_created_at DATETIME;
ALTER TABLE users ADD COLUMN api_key_revealed_at DATETIME;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_api_key_hash
    ON users (api_key_hash)
    WHERE api_key_hash <> '';
