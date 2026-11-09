-- TEC-1061 (RV-5.7): a product the center keeps out of dealer / distributor
-- orders (plexi sets, samples). Closed products are hidden from the catalog
-- list of non-center organizations and rejected in their own orders; the
-- center may still add them to an order it opens for a dealer (TEC-1024).
ALTER TABLE products
    ADD COLUMN dealer_orderable BOOLEAN NOT NULL DEFAULT true;
