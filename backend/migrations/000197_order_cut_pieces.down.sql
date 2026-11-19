-- Restores the 000196 credit default and drops the per-piece columns.
CREATE OR REPLACE FUNCTION unit_credit_default(p_unit_id BIGINT) RETURNS INT
LANGUAGE sql STABLE AS $$
    SELECT CASE
        WHEN u.unit_kind <> 'serial' THEN NULL
        WHEN u.initial_meters IS NOT NULL THEN (
            SELECT oi.line_credits
            FROM order_item_units oiu
            JOIN order_items oi ON oi.id = oiu.order_item_id
            WHERE oiu.unit_id = u.id AND oi.line_credits IS NOT NULL
            ORDER BY oiu.assigned_at DESC, oiu.id DESC
            LIMIT 1
        )
        ELSE COALESCE((
            SELECT oi.plotter_credits
            FROM order_item_units oiu
            JOIN order_items oi ON oi.id = oiu.order_item_id
            WHERE oiu.unit_id = u.id AND oi.plotter_cut AND oi.plotter_credits IS NOT NULL
            ORDER BY oiu.assigned_at DESC, oiu.id DESC
            LIMIT 1
        ), p.usage_credits)
    END
    FROM units u
    JOIN products p ON p.id = u.product_id
    WHERE u.id = p_unit_id
$$;

ALTER TABLE order_item_units
    DROP CONSTRAINT IF EXISTS chk_order_item_units_credits,
    DROP COLUMN IF EXISTS piece_note,
    DROP COLUMN IF EXISTS credits;
