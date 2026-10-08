-- TEC-506 (F5-09b): `price_list` document kind and its platform default
-- templates in the 13 UI languages (recommended retail price list PDF that
-- is published to the document center after a recommended price
-- publication). Same seed shape as 000121 (variables and content_hash
-- derived from the HTML). prices_table is an html block the pricing loader
-- fills in the list language.

ALTER TABLE document_templates DROP CONSTRAINT chk_document_templates_kind;
ALTER TABLE document_templates ADD CONSTRAINT chk_document_templates_kind CHECK (
    kind IN ('service', 'measurement', 'contract', 'order_slip', 'invoice_view', 'warranty', 'quote', 'fleet_report', 'price_list')
);

WITH seed (kind, language, name, html) AS (
    VALUES
    ('price_list', 'tr', 'Tavsiye Satış Fiyat Listesi', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Tavsiye Satış Fiyat Listesi</h1>
<table class="doc-meta"><tr><th>Ülke</th><td>{{country_name}}</td><th>Para birimi</th><td>{{currency}}</td></tr><tr><th>Geçerlilik tarihi</th><td>{{effective_date}}</td><th>Belge tarihi</th><td>{{document_date}}</td></tr></table>
<h2>Fiyatlar</h2>{{prices_table}}
<p class="doc-note">Bu liste son kullanıcıya tavsiye edilen satış fiyatlarını içerir.</p>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('price_list', 'en', 'Recommended Retail Price List', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Recommended Retail Price List</h1>
<table class="doc-meta"><tr><th>Country</th><td>{{country_name}}</td><th>Currency</th><td>{{currency}}</td></tr><tr><th>Valid as of</th><td>{{effective_date}}</td><th>Document date</th><td>{{document_date}}</td></tr></table>
<h2>Prices</h2>{{prices_table}}
<p class="doc-note">This list contains the recommended end-customer retail prices.</p>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('price_list', 'bg', 'Ценова листа с препоръчителни цени', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Ценова листа с препоръчителни цени</h1>
<table class="doc-meta"><tr><th>Държава</th><td>{{country_name}}</td><th>Валута</th><td>{{currency}}</td></tr><tr><th>Валидна към</th><td>{{effective_date}}</td><th>Дата на документа</th><td>{{document_date}}</td></tr></table>
<h2>Цени</h2>{{prices_table}}
<p class="doc-note">Тази листа съдържа препоръчителните цени за краен клиент.</p>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('price_list', 'de', 'Preisliste der unverbindlichen Preisempfehlungen', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Preisliste der unverbindlichen Preisempfehlungen</h1>
<table class="doc-meta"><tr><th>Land</th><td>{{country_name}}</td><th>Währung</th><td>{{currency}}</td></tr><tr><th>Gültig ab</th><td>{{effective_date}}</td><th>Belegdatum</th><td>{{document_date}}</td></tr></table>
<h2>Preise</h2>{{prices_table}}
<p class="doc-note">Diese Liste enthält die unverbindlichen Verkaufspreisempfehlungen für Endkunden.</p>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('price_list', 'el', 'Τιμοκατάλογος προτεινόμενων τιμών λιανικής', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Τιμοκατάλογος προτεινόμενων τιμών λιανικής</h1>
<table class="doc-meta"><tr><th>Χώρα</th><td>{{country_name}}</td><th>Νόμισμα</th><td>{{currency}}</td></tr><tr><th>Ισχύει από</th><td>{{effective_date}}</td><th>Ημερομηνία εγγράφου</th><td>{{document_date}}</td></tr></table>
<h2>Τιμές</h2>{{prices_table}}
<p class="doc-note">Ο κατάλογος περιέχει τις προτεινόμενες τιμές λιανικής για τον τελικό πελάτη.</p>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('price_list', 'uk', 'Прайс-лист рекомендованих роздрібних цін', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Прайс-лист рекомендованих роздрібних цін</h1>
<table class="doc-meta"><tr><th>Країна</th><td>{{country_name}}</td><th>Валюта</th><td>{{currency}}</td></tr><tr><th>Діє з</th><td>{{effective_date}}</td><th>Дата документа</th><td>{{document_date}}</td></tr></table>
<h2>Ціни</h2>{{prices_table}}
<p class="doc-note">Цей список містить рекомендовані роздрібні ціни для кінцевого покупця.</p>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('price_list', 'ru', 'Прайс-лист рекомендованных розничных цен', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Прайс-лист рекомендованных розничных цен</h1>
<table class="doc-meta"><tr><th>Страна</th><td>{{country_name}}</td><th>Валюта</th><td>{{currency}}</td></tr><tr><th>Действует с</th><td>{{effective_date}}</td><th>Дата документа</th><td>{{document_date}}</td></tr></table>
<h2>Цены</h2>{{prices_table}}
<p class="doc-note">Этот список содержит рекомендованные розничные цены для конечного покупателя.</p>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('price_list', 'fr', 'Liste des prix de vente conseillés', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Liste des prix de vente conseillés</h1>
<table class="doc-meta"><tr><th>Pays</th><td>{{country_name}}</td><th>Devise</th><td>{{currency}}</td></tr><tr><th>Valable à partir du</th><td>{{effective_date}}</td><th>Date du document</th><td>{{document_date}}</td></tr></table>
<h2>Prix</h2>{{prices_table}}
<p class="doc-note">Cette liste contient les prix de vente conseillés au client final.</p>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('price_list', 'es', 'Lista de precios de venta recomendados', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Lista de precios de venta recomendados</h1>
<table class="doc-meta"><tr><th>País</th><td>{{country_name}}</td><th>Moneda</th><td>{{currency}}</td></tr><tr><th>Válida desde</th><td>{{effective_date}}</td><th>Fecha del documento</th><td>{{document_date}}</td></tr></table>
<h2>Precios</h2>{{prices_table}}
<p class="doc-note">Esta lista contiene los precios de venta recomendados al cliente final.</p>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('price_list', 'it', 'Listino prezzi di vendita consigliati', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Listino prezzi di vendita consigliati</h1>
<table class="doc-meta"><tr><th>Paese</th><td>{{country_name}}</td><th>Valuta</th><td>{{currency}}</td></tr><tr><th>Valido dal</th><td>{{effective_date}}</td><th>Data del documento</th><td>{{document_date}}</td></tr></table>
<h2>Prezzi</h2>{{prices_table}}
<p class="doc-note">Questo listino contiene i prezzi di vendita consigliati al cliente finale.</p>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('price_list', 'zh_CN', '建议零售价目表', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">建议零售价目表</h1>
<table class="doc-meta"><tr><th>国家</th><td>{{country_name}}</td><th>货币</th><td>{{currency}}</td></tr><tr><th>生效日期</th><td>{{effective_date}}</td><th>文件日期</th><td>{{document_date}}</td></tr></table>
<h2>价格</h2>{{prices_table}}
<p class="doc-note">本价目表列出建议的终端客户零售价格。</p>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('price_list', 'az', 'Tövsiyə olunan satış qiymətləri siyahısı', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Tövsiyə olunan satış qiymətləri siyahısı</h1>
<table class="doc-meta"><tr><th>Ölkə</th><td>{{country_name}}</td><th>Valyuta</th><td>{{currency}}</td></tr><tr><th>Qüvvədədir</th><td>{{effective_date}}</td><th>Sənəd tarixi</th><td>{{document_date}}</td></tr></table>
<h2>Qiymətlər</h2>{{prices_table}}
<p class="doc-note">Bu siyahı son istifadəçi üçün tövsiyə olunan satış qiymətlərini əhatə edir.</p>
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('price_list', 'ar', 'قائمة أسعار البيع الموصى بها', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">قائمة أسعار البيع الموصى بها</h1>
<table class="doc-meta"><tr><th>الدولة</th><td>{{country_name}}</td><th>العملة</th><td>{{currency}}</td></tr><tr><th>سارية اعتبارًا من</th><td>{{effective_date}}</td><th>تاريخ المستند</th><td>{{document_date}}</td></tr></table>
<h2>الأسعار</h2>{{prices_table}}
<p class="doc-note">تتضمن هذه القائمة أسعار البيع الموصى بها للعميل النهائي.</p>
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
