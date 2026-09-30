INSERT INTO roles (name, slug, description, is_system) VALUES
    ('Platform Admin', 'super_admin', 'Platform-level operator with all permissions', true);

INSERT INTO permissions (name, slug) VALUES
    ('Auth session', 'auth.session'),
    ('Read notifications', 'notifications.read'),
    ('Manage notifications', 'notifications.manage'),
    ('Read platform notifications', 'platform.notifications.read'),
    ('Export platform notifications', 'platform.notifications.export'),
    ('Read platform users', 'platform.users.read'),
    ('Write platform users', 'platform.users.write'),
    ('Export platform users', 'platform.users.export'),
    ('Import platform users', 'platform.users.import'),
    ('Bulk disable platform users', 'platform.users.bulk.disable'),
    ('Bulk enable platform users', 'platform.users.bulk.enable'),
    ('Impersonate platform users', 'platform.users.impersonate'),
    ('Read platform roles', 'platform.roles.read'),
    ('Write platform roles', 'platform.roles.write'),
    ('Export platform roles', 'platform.roles.export'),
    ('Import platform roles', 'platform.roles.import'),
    ('Bulk delete platform roles', 'platform.roles.bulk.delete'),
    ('Read bulk jobs', 'platform.bulk.read'),
    ('Read platform settings', 'platform.settings.read'),
    ('Write platform settings', 'platform.settings.write'),
    ('Read activity log', 'platform.activity.read'),
    ('Read import jobs', 'platform.imports.read'),
    ('Read export jobs', 'platform.exports.read'),
    ('Read platform storage', 'platform.storage.read'),
    ('Write platform storage', 'platform.storage.write'),
    ('Read platform logs', 'platform.logs.read'),
    ('Write platform logs', 'platform.logs.write'),
    ('Read access settings', 'platform.access.read'),
    ('Write access settings', 'platform.access.write'),
    ('Read GitHub integration settings', 'platform.integrations.github.read'),
    ('Write GitHub integration settings', 'platform.integrations.github.write'),
    ('Read Google integration settings', 'platform.integrations.google.read'),
    ('Write Google integration settings', 'platform.integrations.google.write'),
    ('Read Facebook integration settings', 'platform.integrations.facebook.read'),
    ('Write Facebook integration settings', 'platform.integrations.facebook.write'),
    ('Read Apple integration settings', 'platform.integrations.apple.read'),
    ('Write Apple integration settings', 'platform.integrations.apple.write'),
    ('Read auth registration settings', 'platform.auth.settings.read'),
    ('Write auth registration settings', 'platform.auth.settings.write');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.slug = 'super_admin';
