-- TEC-526: the system contract templates (brand NULL, tr/en) print their
-- meta-table captions through placeholders. The renderer fills them in the
-- contract's own locale, so a contract in any of the 13 languages that falls
-- back to the tr/en template still gets localized captions.
UPDATE document_templates
SET html = replace(replace(replace(replace(html,
        '<th>Sözleşme No</th>', '<th>{{label_contract_no}}</th>'),
        '<th>Tarih</th>', '<th>{{label_date}}</th>'),
        '<th>Taraf</th>', '<th>{{label_party}}</th>'),
        '<th>İmza tarihi</th>', '<th>{{label_signed_at}}</th>')
WHERE kind = 'contract' AND brand_id IS NULL AND language = 'tr';

UPDATE document_templates
SET html = replace(replace(replace(replace(html,
        '<th>Contract No</th>', '<th>{{label_contract_no}}</th>'),
        '<th>Date</th>', '<th>{{label_date}}</th>'),
        '<th>Party</th>', '<th>{{label_party}}</th>'),
        '<th>Signed at</th>', '<th>{{label_signed_at}}</th>')
WHERE kind = 'contract' AND brand_id IS NULL AND language = 'en';

UPDATE document_templates
SET variables = COALESCE((
        SELECT jsonb_agg(DISTINCT lower(m[1]))
        FROM regexp_matches(html, '\{\{\s*\.?([a-zA-Z0-9_]+)\s*\}\}', 'g') AS m
    ), '[]'::jsonb),
    content_hash = encode(sha256(convert_to(html, 'UTF8')), 'hex')
WHERE kind = 'contract' AND brand_id IS NULL AND language IN ('tr', 'en');
