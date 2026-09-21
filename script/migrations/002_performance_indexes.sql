-- Performance and Caching Optimization Indexes

-- 1. Expression indexes for case-insensitive filtering on owner and region in analytics queries
CREATE INDEX IF NOT EXISTS idx_leads_lower_owner ON leads (LOWER(owner));
CREATE INDEX IF NOT EXISTS idx_leads_lower_region ON leads (LOWER(region));

-- 2. Composite index for overdue tasks and activities calculations
CREATE INDEX IF NOT EXISTS idx_activities_overdue ON activities (completed, due_date, lead_id);

-- 3. Composite indexes for preloading commercial estimation nested collections
CREATE INDEX IF NOT EXISTS idx_commercial_resources_lead ON commercial_resources (lead_id, created_at);
CREATE INDEX IF NOT EXISTS idx_commercial_expenses_lead ON commercial_expenses (lead_id, created_at);
CREATE INDEX IF NOT EXISTS idx_sdlc_allocations_lead ON sdlc_allocations (lead_id, created_at);

-- 4. Composite heat-map index for owner and stage grouping
CREATE INDEX IF NOT EXISTS idx_leads_deleted_owner_stage ON leads (deleted_at, owner, stage);

