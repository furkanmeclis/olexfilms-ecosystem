-- TEC-772: the platform letterhead takes the legacy Olex green. Only the
-- untouched 000012 default is replaced; a color an admin chose stays.
UPDATE app_settings SET primary_color = '#003D25' WHERE upper(primary_color) = '#0F172A';
ALTER TABLE app_settings ALTER COLUMN primary_color SET DEFAULT '#003D25';
