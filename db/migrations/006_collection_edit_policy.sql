-- Add edit policy to collections: 'owner_only' (default) or 'everyone'.
-- Only meaningful when is_public = true.
ALTER TABLE collections
  ADD COLUMN IF NOT EXISTS edit_policy TEXT NOT NULL DEFAULT 'owner_only'
    CHECK (edit_policy IN ('owner_only', 'everyone'));
