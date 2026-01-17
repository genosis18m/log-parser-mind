-- PostgreSQL Schema for Log-Zero
-- Run this against your PostgreSQL instance

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Experiences table - stores learning from past fixes
CREATE TABLE IF NOT EXISTS experiences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_signature TEXT NOT NULL,
    issue_context TEXT,
    fix_applied TEXT NOT NULL,
    commands_executed TEXT[],
    success BOOLEAN NOT NULL DEFAULT false,
    resolution_time_seconds INTEGER,
    feedback_score REAL DEFAULT 0,
    times_referenced INTEGER DEFAULT 0,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_experiences_issue_signature ON experiences(issue_signature);
CREATE INDEX IF NOT EXISTS idx_experiences_success ON experiences(success);
CREATE INDEX IF NOT EXISTS idx_experiences_created_at ON experiences(created_at);
CREATE INDEX IF NOT EXISTS idx_experiences_feedback_score ON experiences(feedback_score);

-- Alerts table - stores detected issues
CREATE TABLE IF NOT EXISTS alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_id TEXT NOT NULL UNIQUE,
    severity TEXT NOT NULL CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    title TEXT NOT NULL,
    description TEXT,
    source TEXT,
    template_ids TEXT[],
    status TEXT DEFAULT 'open' CHECK (status IN ('open', 'acknowledged', 'in_progress', 'resolved', 'closed')),
    assigned_to TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    acknowledged_at TIMESTAMP WITH TIME ZONE,
    resolved_at TIMESTAMP WITH TIME ZONE,
    metadata JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);
CREATE INDEX IF NOT EXISTS idx_alerts_severity ON alerts(severity);
CREATE INDEX IF NOT EXISTS idx_alerts_created_at ON alerts(created_at);
CREATE INDEX IF NOT EXISTS idx_alerts_issue_id ON alerts(issue_id);

-- Fix history table - tracks all fix attempts
CREATE TABLE IF NOT EXISTS fix_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id UUID REFERENCES alerts(id) ON DELETE CASCADE,
    experience_id UUID REFERENCES experiences(id) ON DELETE SET NULL,
    proposal_id TEXT NOT NULL,
    commands_executed TEXT[],
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'success', 'failed', 'rolled_back')),
    output TEXT,
    error_message TEXT,
    executed_by TEXT,
    executed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    duration_ms INTEGER
);

CREATE INDEX IF NOT EXISTS idx_fix_history_alert_id ON fix_history(alert_id);
CREATE INDEX IF NOT EXISTS idx_fix_history_status ON fix_history(status);
CREATE INDEX IF NOT EXISTS idx_fix_history_executed_at ON fix_history(executed_at);

-- API keys table for authentication
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    permissions TEXT[] DEFAULT ARRAY['read'],
    rate_limit INTEGER DEFAULT 1000,
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_used_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_enabled ON api_keys(enabled);

-- Metrics snapshot table for analytics
CREATE TABLE IF NOT EXISTS metrics_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    metric_type TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    value REAL NOT NULL,
    metadata JSONB DEFAULT '{}',
    recorded_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_metrics_type_name ON metrics_snapshots(metric_type, metric_name);
CREATE INDEX IF NOT EXISTS idx_metrics_recorded_at ON metrics_snapshots(recorded_at);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger for experiences table
DROP TRIGGER IF EXISTS update_experiences_updated_at ON experiences;
CREATE TRIGGER update_experiences_updated_at
    BEFORE UPDATE ON experiences
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Insert sample experience for testing
INSERT INTO experiences (issue_signature, issue_context, fix_applied, commands_executed, success, resolution_time_seconds)
VALUES (
    'database_connection_refused',
    'Error: Connection refused to database at 192.168.1.1:5432',
    'Restart the database service and verify connectivity',
    ARRAY['systemctl restart postgresql', 'pg_isready -h 192.168.1.1'],
    true,
    120
) ON CONFLICT DO NOTHING;
