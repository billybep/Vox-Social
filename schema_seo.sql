-- SEO Audit Schema Migration
-- Run this against your PostgreSQL database after the existing schema.sql

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS seo_audits (
    id               UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    domain           TEXT         NOT NULL,
    website_url      TEXT         NOT NULL,
    overall_score    INT          NOT NULL,
    overall_grade    VARCHAR(2)   NOT NULL,
    passed_count     INT          NOT NULL DEFAULT 0,
    warning_count    INT          NOT NULL DEFAULT 0,
    failed_count     INT          NOT NULL DEFAULT 0,
    ai_used          BOOLEAN      NOT NULL DEFAULT FALSE,

    -- Heavy nested data stored as JSONB for flexibility
    scores           JSONB        NOT NULL DEFAULT '{}',
    checks           JSONB        NOT NULL DEFAULT '[]',
    executive_summary JSONB       NOT NULL DEFAULT '[]',
    action_plan      JSONB        NOT NULL DEFAULT '[]',
    page_snapshot    JSONB        NOT NULL DEFAULT '{}',
    sitewide         JSONB        NOT NULL DEFAULT '{}',
    page_rows        JSONB        NOT NULL DEFAULT '[]',
    listings         JSONB        NOT NULL DEFAULT '[]',
    advanced         JSONB        NOT NULL DEFAULT '{}',
    request_meta     JSONB        NOT NULL DEFAULT '{}',

    generated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Index for domain history queries
CREATE INDEX IF NOT EXISTS idx_seo_audits_domain ON seo_audits(domain);
-- Index for recent audits queries
CREATE INDEX IF NOT EXISTS idx_seo_audits_generated_at ON seo_audits(generated_at DESC);
-- JSONB index for score queries
CREATE INDEX IF NOT EXISTS idx_seo_audits_overall_score ON seo_audits(overall_score);
