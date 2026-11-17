-- TEC-1230: termal etiket.
--
-- label_templates.output_format: how a template is printed.
--   * sheet  — A4 grid of labels (the TEC-202 behaviour, default);
--   * single — one label per PDF page, the page is the label size (thermal
--              printers driven through the OS print dialog);
--   * zpl    — Zebra ZPL II text, one ^XA..^XZ block per label (203 dpi).
ALTER TABLE label_templates
    ADD COLUMN output_format VARCHAR(16) NOT NULL DEFAULT 'sheet',
    ADD CONSTRAINT chk_label_templates_output_format CHECK (output_format IN ('sheet', 'single', 'zpl'));
