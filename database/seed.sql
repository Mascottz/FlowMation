INSERT INTO workspaces (id, name, slug)
VALUES ('00000000-0000-0000-0000-000000000001', 'FlowMation Demo Workspace', 'flowmation-demo')
ON CONFLICT (id) DO NOTHING;

INSERT INTO users (id, workspace_id, full_name, email, role)
VALUES ('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 'Alex Morgan', 'alex@flowmation.local', 'owner')
ON CONFLICT (id) DO NOTHING;

INSERT INTO workflows (id, workspace_id, name, description, status, version, created_by)
VALUES ('00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'New lead enrichment', 'Enrich new leads and notify sales', 'active', 1, '00000000-0000-0000-0000-000000000002')
ON CONFLICT (id) DO NOTHING;

INSERT INTO workflow_steps (workflow_id, step_key, step_type, position, configuration)
VALUES
('00000000-0000-0000-0000-000000000003', 'webhook', 'webhook', 1, '{"method":"POST","path":"/hooks/leads"}'),
('00000000-0000-0000-0000-000000000003', 'enrich', 'action', 2, '{"provider":"hubspot","action":"enrich_contact"}'),
('00000000-0000-0000-0000-000000000003', 'qualified', 'condition', 3, '{"field":"score","operator":"greater_than","value":70}')
ON CONFLICT (workflow_id, step_key) DO NOTHING;
