-- 1. Normalize legacy roles to the four standard roles
UPDATE users SET role = 'sales_executive' WHERE role = 'agent';
UPDATE users SET role = 'sales_manager' WHERE role = 'manager';

-- 2. Add manager_id to users table to support Manager -> Executive hierarchy
ALTER TABLE users ADD COLUMN IF NOT EXISTS manager_id UUID REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_users_manager_id ON users(manager_id);

-- 3. Create leader_delegations table for granular admin-delegated permissions
CREATE TABLE IF NOT EXISTS leader_delegations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission VARCHAR(100) NOT NULL,
    granted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_leader_permission UNIQUE (user_id, permission)
);
CREATE INDEX IF NOT EXISTS idx_leader_delegations_user_id ON leader_delegations(user_id);

-- 4. Add created_by and assigned_to columns to leads table
ALTER TABLE leads ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS assigned_to UUID REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_leads_created_by ON leads(created_by);
CREATE INDEX IF NOT EXISTS idx_leads_assigned_to ON leads(assigned_to);

-- 5. Safe backfill of existing leads based on owner string
-- Map 'KAM One' to user with name 'KAM One'
UPDATE leads 
SET assigned_to = u.id, 
    created_by = COALESCE(leads.created_by, u.id)
FROM users u 
WHERE leads.owner = u.name AND (leads.assigned_to IS NULL OR leads.created_by IS NULL);

-- Map owner by email if owner contains '@'
UPDATE leads 
SET assigned_to = u.id, 
    created_by = COALESCE(leads.created_by, u.id)
FROM users u 
WHERE LOWER(leads.owner) = LOWER(u.email) AND (leads.assigned_to IS NULL OR leads.created_by IS NULL);

-- Fallback for any remaining unmapped leads: assign to admin user
UPDATE leads
SET assigned_to = (SELECT id FROM users WHERE role = 'admin' LIMIT 1),
    created_by = (SELECT id FROM users WHERE role = 'admin' LIMIT 1)
WHERE assigned_to IS NULL OR created_by IS NULL;
