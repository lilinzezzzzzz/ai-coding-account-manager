CREATE TABLE accounts (
    provider_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    storage_id TEXT NOT NULL UNIQUE,
    label TEXT NOT NULL,
    email TEXT,
    plan_type TEXT,
    is_active INTEGER NOT NULL DEFAULT 0
        CHECK (is_active IN (0, 1)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_used_at INTEGER,
    PRIMARY KEY (provider_id, account_id)
);

CREATE UNIQUE INDEX accounts_one_active_per_provider
ON accounts(provider_id)
WHERE is_active = 1;

CREATE TABLE usage_snapshots (
    provider_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    status TEXT NOT NULL,
    used_percent REAL,
    resets_at INTEGER,
    snapshot_json TEXT,
    error_code TEXT,
    refreshed_at INTEGER NOT NULL,
    PRIMARY KEY (provider_id, account_id),
    FOREIGN KEY (provider_id, account_id)
        REFERENCES accounts(provider_id, account_id)
        ON DELETE CASCADE
);

CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL
);
