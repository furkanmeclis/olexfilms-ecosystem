-- TEC-88 (F0-12): PDF document templates (HTML from the Lexical editor) and
-- rendered PDF documents. Single PDF engine: Gotenberg Chromium.
--
-- document_templates: one row per version of (kind, brand, language). Brand
-- NULL is the platform default; a brand row overrides it. Exactly one active
-- (published) version and at most one open draft per (kind, brand, language).
-- Platform configuration managed by the center admin, so no organization_id.

CREATE TABLE document_templates (
    id            BIGSERIAL PRIMARY KEY,
    uuid          UUID         NOT NULL DEFAULT gen_random_uuid(),
    kind          VARCHAR(32)  NOT NULL,
    brand_id      BIGINT       NULL REFERENCES brands (id) ON DELETE CASCADE,
    language      VARCHAR(8)   NOT NULL,
    name          VARCHAR(150) NOT NULL,
    version       INT          NOT NULL DEFAULT 1,
    is_active     BOOLEAN      NOT NULL DEFAULT FALSE,
    lexical_json  JSONB        NULL,
    html          TEXT         NOT NULL,
    variables     JSONB        NOT NULL DEFAULT '[]'::jsonb,
    content_hash  CHAR(64)     NOT NULL,
    created_by    BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    published_at  TIMESTAMPTZ  NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_document_templates_uuid UNIQUE (uuid),
    CONSTRAINT chk_document_templates_kind CHECK (
        kind IN ('service', 'measurement', 'contract', 'order_slip', 'invoice_view', 'warranty')
    ),
    CONSTRAINT chk_document_templates_version CHECK (version > 0),
    CONSTRAINT chk_document_templates_active_published CHECK (NOT is_active OR published_at IS NOT NULL)
);

CREATE UNIQUE INDEX uq_document_templates_version
    ON document_templates (kind, COALESCE(brand_id, 0), language, version);
CREATE UNIQUE INDEX uq_document_templates_active
    ON document_templates (kind, COALESCE(brand_id, 0), language) WHERE is_active;
CREATE UNIQUE INDEX uq_document_templates_draft
    ON document_templates (kind, COALESCE(brand_id, 0), language) WHERE published_at IS NULL;

CREATE TRIGGER trg_document_templates_set_updated_at
    BEFORE UPDATE ON document_templates
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- document_renders: one rendered PDF per cache key. The key hashes the
-- template content + version, the source version, locale, brand and
-- organization, so publishing a template (new hash) or changing the source
-- (new source_version) yields a new key and the old PDF is no longer served.
CREATE TABLE document_renders (
    id                BIGSERIAL PRIMARY KEY,
    uuid              UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id   BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    brand_id          BIGINT       NOT NULL REFERENCES brands (id),
    kind              VARCHAR(32)  NOT NULL,
    template_id       BIGINT       NOT NULL REFERENCES document_templates (id),
    template_version  INT          NOT NULL,
    source_type       VARCHAR(64)  NOT NULL,
    source_id         VARCHAR(64)  NOT NULL,
    source_version    VARCHAR(128) NOT NULL,
    locale            VARCHAR(8)   NOT NULL,
    cache_key         CHAR(64)     NOT NULL,
    status            VARCHAR(16)  NOT NULL DEFAULT 'pending',
    attempts          INT          NOT NULL DEFAULT 0,
    storage_key       TEXT         NULL,
    sha256            CHAR(64)     NULL,
    size_bytes        BIGINT       NULL,
    error             TEXT         NULL,
    requested_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    rendered_at       TIMESTAMPTZ  NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_document_renders_uuid UNIQUE (uuid),
    CONSTRAINT uq_document_renders_cache_key UNIQUE (cache_key),
    CONSTRAINT chk_document_renders_status CHECK (status IN ('pending', 'processing', 'ready', 'failed'))
);

CREATE INDEX idx_document_renders_org_created ON document_renders (organization_id, created_at DESC);
CREATE INDEX idx_document_renders_source ON document_renders (source_type, source_id);

CREATE TRIGGER trg_document_renders_set_updated_at
    BEFORE UPDATE ON document_renders
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Permissions: center admin edits templates. Download rights come from the
-- source module (organization scope); these two only gate the editor.
INSERT INTO permissions (name, slug) VALUES
    ('Read document templates', 'platform.documents.templates.read'),
    ('Write document templates', 'platform.documents.templates.write')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('platform.documents.templates.read', 'platform.documents.templates.write')
WHERE r.slug = 'super_admin'
ON CONFLICT DO NOTHING;

-- Seed: platform default templates for the six kinds, tr + en. Simple and
-- functional; the base stylesheet (fonts, tables, header) comes from the
-- renderer skeleton. {{key}} placeholders; *_table / company_logo / *_html
-- are server-built HTML blocks.
WITH seed (kind, language, name, html) AS (
    VALUES
    ('service', 'tr', 'Hizmet formu', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Hizmet Formu</h1>
<table class="doc-meta">
<tr><th>Belge No</th><td>{{document_number}}</td><th>Tarih</th><td>{{document_date}}</td></tr>
<tr><th>Müşteri</th><td>{{customer_name}}</td><th>Telefon</th><td>{{customer_phone}}</td></tr>
<tr><th>Plaka</th><td>{{plate}}</td><th>Araç</th><td>{{vehicle}}</td></tr>
<tr><th>Teknisyen</th><td>{{technician_name}}</td><th>Garanti No</th><td>{{warranty_code}}</td></tr>
</table>
<h2>Uygulanan hizmetler</h2>
{{items_table}}
<p class="doc-total">Toplam: {{total_amount}}</p>
<p>{{notes}}</p>
<div class="doc-sign"><div>Müşteri imzası</div><div>Yetkili imzası</div></div>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('service', 'en', 'Service form', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Service Form</h1>
<table class="doc-meta">
<tr><th>Document No</th><td>{{document_number}}</td><th>Date</th><td>{{document_date}}</td></tr>
<tr><th>Customer</th><td>{{customer_name}}</td><th>Phone</th><td>{{customer_phone}}</td></tr>
<tr><th>Plate</th><td>{{plate}}</td><th>Vehicle</th><td>{{vehicle}}</td></tr>
<tr><th>Technician</th><td>{{technician_name}}</td><th>Warranty No</th><td>{{warranty_code}}</td></tr>
</table>
<h2>Services performed</h2>
{{items_table}}
<p class="doc-total">Total: {{total_amount}}</p>
<p>{{notes}}</p>
<div class="doc-sign"><div>Customer signature</div><div>Authorized signature</div></div>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('measurement', 'tr', 'Ölçüm raporu', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}}</div></header>
<h1 class="doc-title">Ölçüm Raporu</h1>
<table class="doc-meta">
<tr><th>Belge No</th><td>{{document_number}}</td><th>Ölçüm tarihi</th><td>{{measured_at}}</td></tr>
<tr><th>Müşteri</th><td>{{customer_name}}</td><th>Plaka</th><td>{{plate}}</td></tr>
<tr><th>Araç</th><td>{{vehicle}}</td><th>Ölçen</th><td>{{technician_name}}</td></tr>
</table>
<h2>Ölçüm değerleri</h2>
{{measurements_table}}
<p>{{notes}}</p>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('measurement', 'en', 'Measurement report', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}}</div></header>
<h1 class="doc-title">Measurement Report</h1>
<table class="doc-meta">
<tr><th>Document No</th><td>{{document_number}}</td><th>Measured at</th><td>{{measured_at}}</td></tr>
<tr><th>Customer</th><td>{{customer_name}}</td><th>Plate</th><td>{{plate}}</td></tr>
<tr><th>Vehicle</th><td>{{vehicle}}</td><th>Measured by</th><td>{{technician_name}}</td></tr>
</table>
<h2>Readings</h2>
{{measurements_table}}
<p>{{notes}}</p>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('contract', 'tr', 'Sözleşme', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}</div></header>
<h1 class="doc-title">{{contract_title}}</h1>
<table class="doc-meta">
<tr><th>Sözleşme No</th><td>{{document_number}}</td><th>Tarih</th><td>{{document_date}}</td></tr>
<tr><th>Taraf</th><td>{{party_name}}</td><th>İmza tarihi</th><td>{{signed_at}}</td></tr>
</table>
{{contract_body_html}}
<div class="doc-sign"><div>{{party_name}}<br>{{signature_image}}</div><div>{{company_name}}</div></div>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('contract', 'en', 'Contract', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}</div></header>
<h1 class="doc-title">{{contract_title}}</h1>
<table class="doc-meta">
<tr><th>Contract No</th><td>{{document_number}}</td><th>Date</th><td>{{document_date}}</td></tr>
<tr><th>Party</th><td>{{party_name}}</td><th>Signed at</th><td>{{signed_at}}</td></tr>
</table>
{{contract_body_html}}
<div class="doc-sign"><div>{{party_name}}<br>{{signature_image}}</div><div>{{company_name}}</div></div>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('order_slip', 'tr', 'Sipariş fişi', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}}</div></header>
<h1 class="doc-title">Sipariş Fişi</h1>
<table class="doc-meta">
<tr><th>Sipariş No</th><td>{{order_number}}</td><th>Tarih</th><td>{{order_date}}</td></tr>
<tr><th>Alıcı</th><td>{{customer_name}}</td><th>Teslimat adresi</th><td>{{delivery_address}}</td></tr>
</table>
{{items_table}}
<table class="doc-totals">
<tr><th>Ara toplam</th><td>{{subtotal}}</td></tr>
<tr><th>Vergi</th><td>{{tax_total}}</td></tr>
<tr><th>Genel toplam</th><td>{{total_amount}}</td></tr>
</table>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('order_slip', 'en', 'Order slip', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}}</div></header>
<h1 class="doc-title">Order Slip</h1>
<table class="doc-meta">
<tr><th>Order No</th><td>{{order_number}}</td><th>Date</th><td>{{order_date}}</td></tr>
<tr><th>Buyer</th><td>{{customer_name}}</td><th>Delivery address</th><td>{{delivery_address}}</td></tr>
</table>
{{items_table}}
<table class="doc-totals">
<tr><th>Subtotal</th><td>{{subtotal}}</td></tr>
<tr><th>Tax</th><td>{{tax_total}}</td></tr>
<tr><th>Grand total</th><td>{{total_amount}}</td></tr>
</table>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('invoice_view', 'tr', 'Fatura görünümü', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Fatura</h1>
<p class="muted">Bu belge bilgi amaçlı fatura görünümüdür.</p>
<table class="doc-meta">
<tr><th>Fatura No</th><td>{{invoice_number}}</td><th>Fatura tarihi</th><td>{{invoice_date}}</td></tr>
<tr><th>Alıcı</th><td>{{customer_name}}</td><th>Vade</th><td>{{due_date}}</td></tr>
<tr><th>Fatura adresi</th><td colspan="3">{{billing_address}}</td></tr>
</table>
{{items_table}}
<table class="doc-totals">
<tr><th>Ara toplam</th><td>{{subtotal}}</td></tr>
<tr><th>Vergi</th><td>{{tax_total}}</td></tr>
<tr><th>Genel toplam</th><td>{{total_amount}}</td></tr>
</table>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('invoice_view', 'en', 'Invoice view', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Invoice</h1>
<p class="muted">This document is an informational invoice view.</p>
<table class="doc-meta">
<tr><th>Invoice No</th><td>{{invoice_number}}</td><th>Invoice date</th><td>{{invoice_date}}</td></tr>
<tr><th>Bill to</th><td>{{customer_name}}</td><th>Due date</th><td>{{due_date}}</td></tr>
<tr><th>Billing address</th><td colspan="3">{{billing_address}}</td></tr>
</table>
{{items_table}}
<table class="doc-totals">
<tr><th>Subtotal</th><td>{{subtotal}}</td></tr>
<tr><th>Tax</th><td>{{tax_total}}</td></tr>
<tr><th>Grand total</th><td>{{total_amount}}</td></tr>
</table>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('warranty', 'tr', 'Garanti belgesi', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}}</div></header>
<h1 class="doc-title">Garanti Belgesi</h1>
<table class="doc-meta">
<tr><th>Garanti No</th><td>{{warranty_code}}</td><th>Düzenlenme</th><td>{{issued_at}}</td></tr>
<tr><th>Müşteri</th><td>{{customer_name}}</td><th>Plaka</th><td>{{plate}}</td></tr>
<tr><th>Araç</th><td>{{vehicle}}</td><th>Ürün</th><td>{{product_name}}</td></tr>
<tr><th>Geçerlilik</th><td colspan="3">{{valid_until}}</td></tr>
</table>
<h2>Garanti kapsamı</h2>
<p>{{coverage_text}}</p>
<div class="doc-verify">{{qr_code}}<p>Doğrulama: {{verify_url}}</p></div>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('warranty', 'en', 'Warranty certificate', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}}</div></header>
<h1 class="doc-title">Warranty Certificate</h1>
<table class="doc-meta">
<tr><th>Warranty No</th><td>{{warranty_code}}</td><th>Issued at</th><td>{{issued_at}}</td></tr>
<tr><th>Customer</th><td>{{customer_name}}</td><th>Plate</th><td>{{plate}}</td></tr>
<tr><th>Vehicle</th><td>{{vehicle}}</td><th>Product</th><td>{{product_name}}</td></tr>
<tr><th>Valid until</th><td colspan="3">{{valid_until}}</td></tr>
</table>
<h2>Coverage</h2>
<p>{{coverage_text}}</p>
<div class="doc-verify">{{qr_code}}<p>Verify: {{verify_url}}</p></div>
<p class="doc-footer">{{footer_text}}</p>$html$)
)
INSERT INTO document_templates (kind, brand_id, language, name, version, is_active, html, variables, content_hash, published_at)
SELECT s.kind, NULL, s.language, s.name, 1, TRUE, s.html,
       COALESCE((
           SELECT jsonb_agg(DISTINCT lower(m[1]))
           FROM regexp_matches(s.html, '\{\{\s*\.?([a-zA-Z0-9_]+)\s*\}\}', 'g') AS m
       ), '[]'::jsonb),
       encode(sha256(convert_to(s.html, 'UTF8')), 'hex'),
       NOW()
FROM seed s;
