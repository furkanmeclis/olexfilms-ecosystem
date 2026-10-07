-- TEC-353: the reviews module (TEC-120) is a standard module, not a paid
-- add-on (user decision 2026-10-07, docs/design.md §5.2). It is on by
-- default and free; it moves to the end of the standard block of the Go
-- catalog (features.Modules), so the add-ons before it shift one slot.
--
-- Organizations without their own reviews flag now resolve to the new
-- default (on). Explicit module_flags rows are kept: a module bundle
-- subscription stays on, and a deliberate "off" (system, admin, distributor)
-- stays off, as for any standard module.
UPDATE modules
SET level = 'standard', default_enabled = true, is_paid = false, sort_order = 240
WHERE key = 'reviews';

UPDATE modules m
SET sort_order = v.sort_order
FROM (VALUES
    ('ai_assistant', 250),
    ('whatsapp_gateway', 260),
    ('mcp', 270),
    ('dealer_showcase', 280),
    ('fleet', 290),
    ('certificates', 300),
    ('stock_forecast', 310),
    ('performance', 320),
    ('efficiency', 330),
    ('campaigns', 340),
    ('photo_standard', 350)
) AS v (key, sort_order)
WHERE m.key = v.key;
