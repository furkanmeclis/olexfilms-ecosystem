ALTER TABLE app_settings ALTER COLUMN primary_color SET DEFAULT '#0F172A';
UPDATE app_settings SET primary_color = '#0F172A' WHERE upper(primary_color) = '#003D25';
