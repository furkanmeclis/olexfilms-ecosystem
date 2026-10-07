UPDATE modules m
SET sort_order = v.sort_order
FROM (VALUES
    ('ai_assistant', 240),
    ('whatsapp_gateway', 250),
    ('mcp', 260),
    ('dealer_showcase', 270),
    ('fleet', 280),
    ('certificates', 290),
    ('stock_forecast', 300),
    ('performance', 310),
    ('efficiency', 320),
    ('campaigns', 330),
    ('photo_standard', 340)
) AS v (key, sort_order)
WHERE m.key = v.key;

UPDATE modules
SET level = 'addon', default_enabled = false, is_paid = true, sort_order = 350
WHERE key = 'reviews';
