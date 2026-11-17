ALTER TABLE label_templates
    DROP CONSTRAINT IF EXISTS chk_label_templates_output_format,
    DROP COLUMN IF EXISTS output_format;
