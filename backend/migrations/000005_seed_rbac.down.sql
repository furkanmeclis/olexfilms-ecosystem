DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM roles WHERE slug = 'super_admin');

DELETE FROM roles WHERE slug = 'super_admin';

DELETE FROM permissions
WHERE slug IN (
    'auth.session',
    'notifications.read',
    'notifications.manage',
    'platform.notifications.read',
    'platform.notifications.export',
    'platform.users.read',
    'platform.users.write',
    'platform.users.export',
    'platform.users.import',
    'platform.users.bulk.disable',
    'platform.users.bulk.enable',
    'platform.users.impersonate',
    'platform.roles.read',
    'platform.roles.write',
    'platform.roles.export',
    'platform.roles.import',
    'platform.roles.bulk.delete',
    'platform.bulk.read',
    'platform.settings.read',
    'platform.settings.write',
    'platform.activity.read',
    'platform.imports.read',
    'platform.exports.read',
    'platform.storage.read',
    'platform.storage.write',
    'platform.logs.read',
    'platform.logs.write',
    'platform.access.read',
    'platform.access.write',
    'platform.integrations.github.read',
    'platform.integrations.github.write',
    'platform.integrations.google.read',
    'platform.integrations.google.write',
    'platform.integrations.facebook.read',
    'platform.integrations.facebook.write',
    'platform.integrations.apple.read',
    'platform.integrations.apple.write',
    'platform.auth.settings.read',
    'platform.auth.settings.write'
);
