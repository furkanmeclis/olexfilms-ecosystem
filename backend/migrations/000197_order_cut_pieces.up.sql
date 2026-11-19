-- TEC-1241 (kutu/metraj 4/7, kararlar TEC-1236 K5/K7/K11/K12): picking a
-- meter line cuts a piece from an open roll or from a closed box opened on
-- the spot. One piece covers the line by default; "parçalı gönder" (K11)
-- lets the warehouse ship more pieces, each with its share of the line's
-- credits and a note.
--
-- order_item_units.credits: the usage credits this piece carries when the
-- line's credits are split across pieces (NULL = the line's line_credits,
-- the single-piece default). piece_note: the warehouse's note of a
-- partial piece.
ALTER TABLE order_item_units
    ADD COLUMN credits INT NULL,
    ADD COLUMN piece_note TEXT NULL,
    ADD CONSTRAINT chk_order_item_units_credits CHECK (
        credits IS NULL OR (meters IS NOT NULL AND credits BETWEEN 0 AND 1000)
    );

-- A cut piece carries the credits written on its assignment, else its
-- meter line's line_credits (000196); pieces without meters are unchanged.
CREATE OR REPLACE FUNCTION unit_credit_default(p_unit_id BIGINT) RETURNS INT
LANGUAGE sql STABLE AS $$
    SELECT CASE
        WHEN u.unit_kind <> 'serial' THEN NULL
        WHEN u.initial_meters IS NOT NULL THEN (
            SELECT COALESCE(oiu.credits, oi.line_credits)
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
