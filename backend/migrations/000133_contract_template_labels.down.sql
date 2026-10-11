-- TEC-526 rollback: put the fixed tr/en captions back into the system
-- contract templates.
UPDATE document_templates
SET html = replace(replace(replace(replace(html,
        '<th>{{label_contract_no}}</th>', '<th>Sözleşme No</th>'),
        '<th>{{label_date}}</th>', '<th>Tarih</th>'),
        '<th>{{label_party}}</th>', '<th>Taraf</th>'),
        '<th>{{label_signed_at}}</th>', '<th>İmza tarihi</th>')
WHERE kind = 'contract' AND brand_id IS NULL AND language = 'tr';

UPDATE document_templates
SET html = replace(replace(replace(replace(html,
        '<th>{{label_contract_no}}</th>', '<th>Contract No</th>'),
        '<th>{{label_date}}</th>', '<th>Date</th>'),
        '<th>{{label_party}}</th>', '<th>Party</th>'),
        '<th>{{label_signed_at}}</th>', '<th>Signed at</th>')
WHERE kind = 'contract' AND brand_id IS NULL AND language = 'en';

UPDATE document_templates
SET variables = COALESCE((
        SELECT jsonb_agg(DISTINCT lower(m[1]))
        FROM regexp_matches(html, '\{\{\s*\.?([a-zA-Z0-9_]+)\s*\}\}', 'g') AS m
    ), '[]'::jsonb),
    content_hash = encode(sha256(convert_to(html, 'UTF8')), 'hex')
WHERE kind = 'contract' AND brand_id IS NULL AND language IN ('tr', 'en');
