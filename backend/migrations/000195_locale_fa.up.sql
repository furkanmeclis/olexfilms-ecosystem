-- TEC-1233: Persian (fa, right to left) becomes the 14th UI locale
-- (platform/i18n.Supported). Every locale CHECK constraint and the dealer
-- showcase JSON validators accept 'fa'; the per-locale seed rows of
-- 000034/000110 (KVKK notice, legal texts), 000036/000054 (default
-- notification templates) and 000094/000121/000128 (quote, fleet report and
-- price list document templates) get their Persian row. Existing rows are
-- never touched (ON CONFLICT DO NOTHING); Persian keeps Latin digits.

-- 1. Locale CHECK constraints ------------------------------------------------
ALTER TABLE users DROP CONSTRAINT chk_users_locale;
ALTER TABLE users ADD CONSTRAINT chk_users_locale CHECK (
    locale IS NULL OR locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE organizations DROP CONSTRAINT chk_organizations_locale;
ALTER TABLE organizations ADD CONSTRAINT chk_organizations_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE notification_templates DROP CONSTRAINT notification_templates_language_chk;
ALTER TABLE notification_templates ADD CONSTRAINT notification_templates_language_chk CHECK (
    language IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE contract_template_locales DROP CONSTRAINT chk_contract_template_locales_locale;
ALTER TABLE contract_template_locales ADD CONSTRAINT chk_contract_template_locales_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE contract_instances DROP CONSTRAINT chk_contract_instances_locale;
ALTER TABLE contract_instances ADD CONSTRAINT chk_contract_instances_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE announcements DROP CONSTRAINT chk_announcements_default_locale;
ALTER TABLE announcements ADD CONSTRAINT chk_announcements_default_locale CHECK (
    default_locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE announcement_locales DROP CONSTRAINT chk_announcement_locales_locale;
ALTER TABLE announcement_locales ADD CONSTRAINT chk_announcement_locales_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE library_item_versions DROP CONSTRAINT chk_library_item_versions_locale;
ALTER TABLE library_item_versions ADD CONSTRAINT chk_library_item_versions_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE review_question_locales DROP CONSTRAINT chk_review_question_locales_locale;
ALTER TABLE review_question_locales ADD CONSTRAINT chk_review_question_locales_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE campaign_contents DROP CONSTRAINT chk_campaign_contents_locale;
ALTER TABLE campaign_contents ADD CONSTRAINT chk_campaign_contents_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE campaign_recipients DROP CONSTRAINT chk_campaign_recipients_locale;
ALTER TABLE campaign_recipients ADD CONSTRAINT chk_campaign_recipients_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE fleet_profiles DROP CONSTRAINT chk_fleet_profiles_report_locale;
ALTER TABLE fleet_profiles ADD CONSTRAINT chk_fleet_profiles_report_locale CHECK (
    report_locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);
ALTER TABLE fleet_reports DROP CONSTRAINT chk_fleet_reports_locale;
ALTER TABLE fleet_reports ADD CONSTRAINT chk_fleet_reports_locale CHECK (
    locale IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
);

-- 2. Dealer showcase locale keyed JSON (000113) -----------------------------
CREATE OR REPLACE FUNCTION dealer_showcase_locale_texts_valid(j JSONB) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE AS $$
    SELECT jsonb_typeof(j) = 'object'
       AND NOT EXISTS (
           SELECT 1 FROM jsonb_each(j) e
           WHERE e.key NOT IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
              OR jsonb_typeof(e.value) <> 'string'
       )
$$;

CREATE OR REPLACE FUNCTION dealer_showcase_content_valid(j JSONB) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE AS $$
    SELECT jsonb_typeof(j) = 'object'
       AND NOT EXISTS (
           SELECT 1 FROM jsonb_each(j) e
           WHERE e.key NOT IN ('tr', 'en', 'bg', 'de', 'el', 'uk', 'ru', 'fr', 'es', 'it', 'zh-CN', 'az', 'ar', 'fa')
              OR jsonb_typeof(e.value) <> 'object'
              OR EXISTS (
                  SELECT 1 FROM jsonb_each(e.value) f
                  WHERE f.key NOT IN ('headline', 'about') OR jsonb_typeof(f.value) <> 'string'
              )
       )
$$;

-- 3. KVKK notice and legal texts (000034, 000110) ---------------------------
INSERT INTO kvkk_notices (locale, version, body) VALUES
    ('fa', 1, 'KVKK: شماره تلفن شما فقط برای احراز هویت و اعلان‌های خدمات و در چارچوب قانون شماره 6698 ترکیه درباره حفاظت از داده‌های شخصی (KVKK) پردازش می‌شود. متن کامل اطلاعیه: olexfilms.app/kvkk')
ON CONFLICT DO NOTHING;

INSERT INTO legal_texts (kind, locale, version, body) VALUES
    ('ai_guidelines', 'fa', 1, $md$## رهنمودهای دستیار هوش مصنوعی

دستیار هوش مصنوعی پورتال برای پاسخ به پرسش‌های شما از سوابق سرویس و گارانتی‌تان استفاده می‌کند. پاسخ‌ها صرفاً جنبه اطلاع‌رسانی دارند؛ برای جزئیات قطعی با نمایندگی خود مشورت کنید.

- اطلاعات سلامت، هویتی یا پرداخت را با دستیار در میان نگذارید.
- گفت‌وگوها برای بهبود خدمات نگهداری می‌شوند.
- می‌توانید هر زمان رضایت خود را از بخش نمایه‌تان پس بگیرید.$md$),
    ('marketing_consent', 'fa', 1, $md$## رضایت به دریافت پیام‌های تجاری

موافقم که کمپین‌ها، تخفیف‌ها و اطلاعیه‌های محصولات و خدمات جدید از طریق اعلان پوش، WhatsApp و ایمیل برایم ارسال شود.

- این رضایت اختیاری است؛ نپذیرفتن آن بر سرویس‌ها و گارانتی‌های شما اثری ندارد.
- اعلان‌های سرویس، نوبت و گارانتی صرف‌نظر از این رضایت ارسال می‌شوند.
- می‌توانید هر زمان رضایت خود را از بخش نمایه‌تان پس بگیرید.$md$)
ON CONFLICT DO NOTHING;

-- 4. Default notification templates (000036, 000054; generic, global) -------
INSERT INTO notification_templates (code, role, channel, language, subject, body, format) VALUES
    ('notifications.test', 'generic', 'inapp', 'fa', 'اعلان آزمایشی', 'این یک اعلان آزمایشی است.', 'text'),
    ('notifications.test', 'generic', 'email', 'fa', 'اعلان آزمایشی', 'این یک ایمیل آزمایشی است.', 'markdown'),
    ('features.module_requested', 'generic', 'inapp', 'fa', 'درخواست ماژول',
     '{{organization_name}} ماژول {{module_key}} را درخواست کرد. {{note}}', 'text'),
    ('features.module_requested', 'generic', 'email', 'fa', 'درخواست ماژول: {{module_key}}',
     '**{{organization_name}}** ماژول **{{module_key}}** را درخواست کرد.

{{note}}', 'markdown'),
    ('auth.welcome', 'generic', 'inapp', 'fa',
     'به App خوش آمدید',
     'سلام {{name}}، حساب شما ایجاد شد.', 'markdown'),
    ('auth.welcome', 'generic', 'email', 'fa',
     'به App خوش آمدید',
     'سلام {{name}}، حساب App شما با موفقیت ایجاد شد.', 'markdown'),
    ('auth.password_reset', 'generic', 'email', 'fa',
     'کد بازنشانی رمز عبور',
     'سلام {{name}}، کد بازنشانی رمز عبور شما: {{code}}. این کد {{expires_minutes}} دقیقه اعتبار دارد.', 'markdown'),
    ('auth.email_verification', 'generic', 'email', 'fa',
     'کد تأیید ایمیل',
     'سلام {{name}}، کد تأیید شما: {{code}}. این کد {{expires_minutes}} دقیقه اعتبار دارد.', 'markdown'),
    ('auth.profile_updated', 'generic', 'inapp', 'fa',
     'نمایه به‌روزرسانی شد',
     'اطلاعات نمایه شما به‌روزرسانی شد.', 'markdown')
ON CONFLICT DO NOTHING;

-- 5. Platform document templates (000094 quote, 000121 fleet report, 000128
-- price list): the ar HTML with Persian texts; variables and content_hash
-- derived from the HTML like the original seeds.
WITH seed (kind, language, name, html) AS (
    VALUES
    ('quote', 'fa', 'پیش‌فاکتور', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header><h1 class="doc-title">پیش‌فاکتور</h1><table class="doc-meta"><tr><th>شماره پیش‌فاکتور</th><td>{{quote_number}}</td><th>تاریخ</th><td>{{document_date}}</td></tr><tr><th>مشتری</th><td>{{customer_name}}</td><th>معتبر تا</th><td>{{valid_until}}</td></tr></table>{{items_table}}<table class="doc-totals"><tr><th>جمع جزء</th><td>{{subtotal}}</td></tr><tr><th>تخفیف</th><td>{{discount_total}}</td></tr><tr><th>مالیات</th><td>{{tax_total}}</td></tr><tr><th>جمع کل</th><td>{{total_amount}}</td></tr></table><p class="doc-footer">{{footer_text}}</p>$html$),
    ('fleet_report', 'fa', 'گزارش ناوگان', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">گزارش ناوگان · {{period_label}}</h1>
<table class="doc-meta"><tr><th>ناوگان</th><td>{{fleet_legal_name}}</td><th>شماره اقتصادی</th><td>{{fleet_tax_number}}</td></tr><tr><th>دوره</th><td>{{period_start}} – {{period_end}}</td><th>تاریخ گزارش</th><td>{{document_date}}</td></tr></table>
<h2>خلاصه</h2><table class="doc-totals"><tr><th>تعداد خودروها</th><td>{{vehicle_count}}</td></tr><tr><th>تعداد سرویس‌ها</th><td>{{service_count}}</td></tr><tr><th>تعداد نمایندگی‌ها</th><td>{{dealer_count}}</td></tr><tr><th>گارانتی‌های فعال</th><td>{{warranty_active_count}}</td></tr><tr><th>گارانتی‌های منقضی‌شده</th><td>{{warranty_expired_count}}</td></tr><tr><th>گارانتی‌های آغازشده در این دوره</th><td>{{warranty_started_count}}</td></tr></table>
<h2>سرویس‌ها به تفکیک نمایندگی</h2>{{dealer_services_table}}
<h2>سرویس‌ها به تفکیک خودرو</h2>{{vehicle_services_table}}
<h2>توزیع قطعات</h2>{{parts_table}}
<h2>محصولات به‌کاررفته</h2>{{products_table}}
<h2>گارانتی‌هایی که به‌زودی منقضی می‌شوند</h2>{{upcoming_expirations_table}}
<h2>مبالغ سرویس و پرداخت‌ها</h2>{{accounts_table}}
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('price_list', 'fa', 'فهرست قیمت پیشنهادی خرده‌فروشی', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">فهرست قیمت پیشنهادی خرده‌فروشی</h1>
<table class="doc-meta"><tr><th>کشور</th><td>{{country_name}}</td><th>ارز</th><td>{{currency}}</td></tr><tr><th>معتبر از</th><td>{{effective_date}}</td><th>تاریخ سند</th><td>{{document_date}}</td></tr></table>
<h2>قیمت‌ها</h2>{{prices_table}}
<p class="doc-note">این فهرست شامل قیمت‌های فروش پیشنهادی برای مصرف‌کننده نهایی است.</p>
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
