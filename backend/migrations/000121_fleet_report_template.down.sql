-- TEC-476: drop the `fleet_report` document kind (renders first: they reference templates).
DELETE FROM document_renders WHERE kind = 'fleet_report';
DELETE FROM document_templates WHERE kind = 'fleet_report';

ALTER TABLE document_templates DROP CONSTRAINT chk_document_templates_kind;
ALTER TABLE document_templates ADD CONSTRAINT chk_document_templates_kind CHECK (
    kind IN ('service', 'measurement', 'contract', 'order_slip', 'invoice_view', 'warranty', 'quote')
);
