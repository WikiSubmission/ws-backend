-- Notifications: zikr reminder texts (rotating display strip + startup splash
-- in the mobile app, managed from the admin notifications page), FCM device
-- registry for the Capacitor Android app, and admin-scheduled notifications
-- dispatched by the in-process scheduler (notify package) over web push and FCM.

CREATE TABLE IF NOT EXISTS zikr_entries (
    id         INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    text       TEXT NOT NULL,
    lang       TEXT NOT NULL DEFAULT 'en',
    source     TEXT,        -- optional attribution, e.g. a verse reference
    active     BOOLEAN NOT NULL DEFAULT TRUE,
    weight     INT NOT NULL DEFAULT 0,  -- display order, ascending
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS zikr_entries_active_idx
    ON zikr_entries (active, lang, weight, id);

-- FCM device tokens from the mobile app. Registration is anonymous-friendly:
-- auth_id is set only when the caller presented a valid bearer token. The token
-- is the natural key, so re-registration is an upsert. updated_at doubles as a
-- last-seen marker for stale pruning.
CREATE TABLE IF NOT EXISTS fcm_devices (
    token       TEXT PRIMARY KEY,
    auth_id     TEXT,
    platform    TEXT NOT NULL CHECK (platform IN ('ios', 'android', 'web')),
    locale      TEXT,
    timezone    TEXT,
    categories  TEXT[] NOT NULL DEFAULT '{}',
    app_version TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS fcm_devices_categories_idx
    ON fcm_devices USING GIN (categories);
CREATE INDEX IF NOT EXISTS fcm_devices_updated_idx
    ON fcm_devices (updated_at);

-- Admin-scheduled notifications. Recurrence is fully determined by
-- starts_at + timezone + frequency (weekly = same weekday and wall-clock time
-- as starts_at; monthly = same day-of-month, clamped to shorter months), which
-- avoids a class of inconsistent day/time column combinations.
CREATE TABLE IF NOT EXISTS scheduled_notifications (
    id              INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title           TEXT NOT NULL,
    body            TEXT NOT NULL,
    url             TEXT,        -- deep link / web path opened on tap
    category        TEXT,        -- notification tag shown to clients
    destination     TEXT NOT NULL CHECK (destination IN ('all', 'web_push', 'fcm')),
    category_filter TEXT,        -- NULL = every subscriber/device; else only those opted into this category
    frequency       TEXT NOT NULL CHECK (frequency IN ('once', 'daily', 'weekly', 'biweekly', 'monthly')),
    starts_at       TIMESTAMPTZ NOT NULL,
    timezone        TEXT NOT NULL DEFAULT 'UTC',
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    next_run_at     TIMESTAMPTZ NOT NULL,
    last_run_at     TIMESTAMPTZ,
    last_status     TEXT,        -- 'dispatching' | 'ok' | 'partial' | 'error'
    last_error      TEXT,
    last_web_sent   INT,
    last_fcm_sent   INT,
    created_by      TEXT,        -- admin email from the auth context
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS scheduled_notifications_due_idx
    ON scheduled_notifications (next_run_at) WHERE enabled;
