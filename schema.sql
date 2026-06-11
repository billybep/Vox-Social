CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS social_audits (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    target_url TEXT NOT NULL,
    overall_score INT NOT NULL,
    status VARCHAR(50) NOT NULL,
    profile_identity TEXT NOT NULL,
    growth_potential VARCHAR(50) NOT NULL,
    profile_readiness INT NOT NULL,
    key_strengths TEXT[] NOT NULL,
    opportunities TEXT[] NOT NULL,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for querying by URL if needed
CREATE INDEX idx_social_audits_target_url ON social_audits(target_url);
