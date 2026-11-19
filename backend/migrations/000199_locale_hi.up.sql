-- TEC-1248: Hindi (hi, left to right) becomes the 15th UI locale
-- (platform/i18n.Supported). Every locale CHECK constraint and the dealer
-- showcase JSON validators accept 'hi'; the per-locale seed rows of
-- 000034/000110 (KVKK notice, legal texts), 000036/000054 (default
-- notification templates) and 000094/000121/000128 (quote, fleet report and
-- price list document templates) get their Hindi row, the same set 000195
-- gave fa. Existing rows are never touched (ON CONFLICT DO NOTHING); Hindi
-- keeps Latin digits.

-- 1. Locale CHECK constraints ------------------------------------------------
ALTER TABLE users DROP CONSTRAINT chk_users_locale;
ALTER TABLE users ADD CONSTRAINT chk_users_locale CHECK (
    locale IS NULL OR locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
);
ALTER TABLE organizations DROP CONSTRAINT chk_organizations_locale;
ALTER TABLE organizations ADD CONSTRAINT chk_organizations_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
);
ALTER TABLE notification_templates DROP CONSTRAINT notification_templates_language_chk;
ALTER TABLE notification_templates ADD CONSTRAINT notification_templates_language_chk CHECK (
    language IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
);
ALTER TABLE contract_template_locales DROP CONSTRAINT chk_contract_template_locales_locale;
ALTER TABLE contract_template_locales ADD CONSTRAINT chk_contract_template_locales_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
);
ALTER TABLE contract_instances DROP CONSTRAINT chk_contract_instances_locale;
ALTER TABLE contract_instances ADD CONSTRAINT chk_contract_instances_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
);
ALTER TABLE announcements DROP CONSTRAINT chk_announcements_default_locale;
ALTER TABLE announcements ADD CONSTRAINT chk_announcements_default_locale CHECK (
    default_locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
);
ALTER TABLE announcement_locales DROP CONSTRAINT chk_announcement_locales_locale;
ALTER TABLE announcement_locales ADD CONSTRAINT chk_announcement_locales_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
);
ALTER TABLE library_item_versions DROP CONSTRAINT chk_library_item_versions_locale;
ALTER TABLE library_item_versions ADD CONSTRAINT chk_library_item_versions_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
);
ALTER TABLE review_question_locales DROP CONSTRAINT chk_review_question_locales_locale;
ALTER TABLE review_question_locales ADD CONSTRAINT chk_review_question_locales_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
);
ALTER TABLE campaign_contents DROP CONSTRAINT chk_campaign_contents_locale;
ALTER TABLE campaign_contents ADD CONSTRAINT chk_campaign_contents_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
);
ALTER TABLE campaign_recipients DROP CONSTRAINT chk_campaign_recipients_locale;
ALTER TABLE campaign_recipients ADD CONSTRAINT chk_campaign_recipients_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
);
ALTER TABLE fleet_profiles DROP CONSTRAINT chk_fleet_profiles_report_locale;
ALTER TABLE fleet_profiles ADD CONSTRAINT chk_fleet_profiles_report_locale CHECK (
    report_locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
);
ALTER TABLE fleet_reports DROP CONSTRAINT chk_fleet_reports_locale;
ALTER TABLE fleet_reports ADD CONSTRAINT chk_fleet_reports_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
);

-- 2. Dealer showcase locale keyed JSON (000113) -----------------------------
CREATE OR REPLACE FUNCTION dealer_showcase_locale_texts_valid(j JSONB) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE AS $$
    SELECT jsonb_typeof(j) = 'object'
       AND NOT EXISTS (
           SELECT 1 FROM jsonb_each(j) e
           WHERE e.key NOT IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
              OR jsonb_typeof(e.value) <> 'string'
       )
$$;

CREATE OR REPLACE FUNCTION dealer_showcase_content_valid(j JSONB) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE AS $$
    SELECT jsonb_typeof(j) = 'object'
       AND NOT EXISTS (
           SELECT 1 FROM jsonb_each(j) e
           WHERE e.key NOT IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa', 'hi')
              OR jsonb_typeof(e.value) <> 'object'
              OR EXISTS (
                  SELECT 1 FROM jsonb_each(e.value) f
                  WHERE f.key NOT IN ('headline', 'about') OR jsonb_typeof(f.value) <> 'string'
              )
       )
$$;

-- 3. KVKK notice and legal texts (000034, 000110) ---------------------------
INSERT INTO kvkk_notices (locale, version, body) VALUES
    ('hi', 1, 'KVKK: आपका फ़ोन नंबर केवल पहचान सत्यापन और सर्विस सूचनाओं के लिए तुर्की के व्यक्तिगत डेटा संरक्षण कानून संख्या 6698 (KVKK) के अंतर्गत संसाधित किया जाता है। विस्तृत सूचना: olexfilms.app/kvkk')
ON CONFLICT DO NOTHING;

INSERT INTO legal_texts (kind, locale, version, body) VALUES
    ('ai_guidelines', 'hi', 1, $md$## AI सहायक के दिशानिर्देश

पोर्टल का AI सहायक आपके प्रश्नों के उत्तर देने के लिए आपके सर्विस और वारंटी रिकॉर्ड का उपयोग करता है। उत्तर केवल जानकारी के लिए हैं; निश्चित विवरण के लिए अपने डीलर से संपर्क करें।

- सहायक के साथ स्वास्थ्य, पहचान या भुगतान संबंधी जानकारी साझा न करें।
- सर्विस की गुणवत्ता सुधारने के लिए बातचीत संग्रहीत की जाती है।
- आप किसी भी समय अपनी प्रोफ़ाइल से अपनी सहमति वापस ले सकते हैं।$md$),
    ('marketing_consent', 'hi', 1, $md$## व्यावसायिक इलेक्ट्रॉनिक संदेशों के लिए सहमति

मैं अभियानों, छूटों तथा नए उत्पादों और सर्विस की घोषणाओं को पुश सूचना, WhatsApp और ईमेल के माध्यम से प्राप्त करने के लिए सहमत हूँ।

- यह सहमति वैकल्पिक है; इसे न देने से आपकी सर्विस और वारंटी प्रक्रियाओं पर कोई प्रभाव नहीं पड़ता।
- सर्विस, अपॉइंटमेंट और वारंटी सूचनाएँ इस सहमति से स्वतंत्र रूप से भेजी जाती हैं।
- आप किसी भी समय अपनी प्रोफ़ाइल से अपनी सहमति वापस ले सकते हैं।$md$)
ON CONFLICT DO NOTHING;

-- 4. Default notification templates (000036, 000054; generic, global) -------
INSERT INTO notification_templates (code, role, channel, language, subject, body, format) VALUES
    ('notifications.test', 'generic', 'inapp', 'hi', 'परीक्षण सूचना', 'यह एक परीक्षण सूचना है।', 'text'),
    ('notifications.test', 'generic', 'email', 'hi', 'परीक्षण सूचना', 'यह एक परीक्षण ईमेल है।', 'markdown'),
    ('features.module_requested', 'generic', 'inapp', 'hi', 'मॉड्यूल अनुरोध',
     '{{organization_name}} ने {{module_key}} मॉड्यूल का अनुरोध किया। {{note}}', 'text'),
    ('features.module_requested', 'generic', 'email', 'hi', 'मॉड्यूल अनुरोध: {{module_key}}',
     '**{{organization_name}}** ने **{{module_key}}** मॉड्यूल का अनुरोध किया।

{{note}}', 'markdown'),
    ('auth.welcome', 'generic', 'inapp', 'hi',
     'App में आपका स्वागत है',
     'नमस्ते {{name}}, आपका खाता बना दिया गया है।', 'markdown'),
    ('auth.welcome', 'generic', 'email', 'hi',
     'App में आपका स्वागत है',
     'नमस्ते {{name}}, आपका App खाता सफलतापूर्वक बना दिया गया है।', 'markdown'),
    ('auth.password_reset', 'generic', 'email', 'hi',
     'पासवर्ड रीसेट कोड',
     'नमस्ते {{name}}, आपका पासवर्ड रीसेट कोड है: {{code}}। यह कोड {{expires_minutes}} मिनट तक मान्य है।', 'markdown'),
    ('auth.email_verification', 'generic', 'email', 'hi',
     'ईमेल सत्यापन कोड',
     'नमस्ते {{name}}, आपका सत्यापन कोड है: {{code}}। यह कोड {{expires_minutes}} मिनट तक मान्य है।', 'markdown'),
    ('auth.profile_updated', 'generic', 'inapp', 'hi',
     'प्रोफ़ाइल अपडेट की गई',
     'आपकी प्रोफ़ाइल जानकारी अपडेट कर दी गई है।', 'markdown')
ON CONFLICT DO NOTHING;

-- 5. Platform document templates (000094 quote, 000121 fleet report, 000128
-- price list): the en (LTR) HTML with Hindi texts; variables and
-- content_hash derived from the HTML like the original seeds.
WITH seed (kind, language, name, html) AS (
    VALUES
    ('quote', 'hi', 'कोटेशन', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">कोटेशन</h1>
<table class="doc-meta"><tr><th>कोटेशन संख्या</th><td>{{quote_number}}</td><th>तिथि</th><td>{{document_date}}</td></tr><tr><th>ग्राहक</th><td>{{customer_name}}</td><th>इस तिथि तक मान्य</th><td>{{valid_until}}</td></tr><tr><th>फ़ोन</th><td>{{customer_phone}}</td><th>स्थिति</th><td>{{status}}</td></tr></table>
{{items_table}}<table class="doc-totals"><tr><th>उप-योग</th><td>{{subtotal}}</td></tr><tr><th>छूट</th><td>{{discount_total}}</td></tr><tr><th>कर</th><td>{{tax_total}}</td></tr><tr><th>कुल</th><td>{{total_amount}}</td></tr></table><p class="doc-footer">{{footer_text}}</p>$html$),
    ('fleet_report', 'hi', 'फ़्लीट रिपोर्ट', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">फ़्लीट रिपोर्ट · {{period_label}}</h1>
<table class="doc-meta"><tr><th>फ़्लीट</th><td>{{fleet_legal_name}}</td><th>कर संख्या</th><td>{{fleet_tax_number}}</td></tr><tr><th>अवधि</th><td>{{period_start}} – {{period_end}}</td><th>रिपोर्ट तिथि</th><td>{{document_date}}</td></tr></table>
<h2>सारांश</h2><table class="doc-totals"><tr><th>वाहनों की संख्या</th><td>{{vehicle_count}}</td></tr><tr><th>सर्विस की संख्या</th><td>{{service_count}}</td></tr><tr><th>डीलरों की संख्या</th><td>{{dealer_count}}</td></tr><tr><th>सक्रिय वारंटी</th><td>{{warranty_active_count}}</td></tr><tr><th>समाप्त वारंटी</th><td>{{warranty_expired_count}}</td></tr><tr><th>इस अवधि में शुरू हुई वारंटी</th><td>{{warranty_started_count}}</td></tr></table>
<h2>डीलर के अनुसार सर्विस</h2>{{dealer_services_table}}
<h2>वाहन के अनुसार सर्विस</h2>{{vehicle_services_table}}
<h2>पार्ट्स का वितरण</h2>{{parts_table}}
<h2>उपयोग किए गए उत्पाद</h2>{{products_table}}
<h2>जल्द समाप्त होने वाली वारंटी</h2>{{upcoming_expirations_table}}
<h2>सर्विस राशियाँ और भुगतान</h2>{{accounts_table}}
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('price_list', 'hi', 'अनुशंसित खुदरा मूल्य सूची', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">अनुशंसित खुदरा मूल्य सूची</h1>
<table class="doc-meta"><tr><th>देश</th><td>{{country_name}}</td><th>मुद्रा</th><td>{{currency}}</td></tr><tr><th>प्रभावी तिथि</th><td>{{effective_date}}</td><th>दस्तावेज़ तिथि</th><td>{{document_date}}</td></tr></table>
<h2>मूल्य</h2>{{prices_table}}
<p class="doc-note">इस सूची में अंतिम ग्राहक के लिए अनुशंसित खुदरा मूल्य शामिल हैं।</p>
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
FROM seed s
ON CONFLICT DO NOTHING;
