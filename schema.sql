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
    recommended_package VARCHAR(50) NOT NULL,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Ensure existing databases get the new column
ALTER TABLE social_audits ADD COLUMN IF NOT EXISTS recommended_package VARCHAR(50) NOT NULL DEFAULT 'Vox Value';

-- Index for querying by URL if needed
CREATE INDEX idx_social_audits_target_url ON social_audits(target_url);

-- Users table for authentication
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash TEXT,
    google_id VARCHAR(255) UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unified Audit Records Table (Persistent Audit History)
CREATE TABLE IF NOT EXISTS audit_records (
    id VARCHAR(255) PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    audit_type VARCHAR(50) NOT NULL,
    target_domain TEXT NOT NULL,
    brand_name TEXT NOT NULL,
    social_account TEXT DEFAULT '',
    overall_score INT NOT NULL DEFAULT 0,
    seo_score INT,
    social_score INT,
    status VARCHAR(50) NOT NULL DEFAULT 'completed',
    evidence_coverage INT NOT NULL DEFAULT 92,
    full_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_records_user_id ON audit_records(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_records_created_at ON audit_records(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_records_type ON audit_records(audit_type);

