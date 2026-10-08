-- TEC-476 (F5-02e): `fleet_report` document kind and its platform default
-- templates in the 13 UI languages (periodic fleet report PDF). Same seed
-- shape as 000094 (variables and content_hash derived from the HTML). The
-- tables are html blocks the fleet loader fills in the report language.

ALTER TABLE document_templates DROP CONSTRAINT chk_document_templates_kind;
ALTER TABLE document_templates ADD CONSTRAINT chk_document_templates_kind CHECK (
    kind IN ('service', 'measurement', 'contract', 'order_slip', 'invoice_view', 'warranty', 'quote', 'fleet_report')
);

WITH seed (kind, language, name, html) AS (
    VALUES
    ('fleet_report', 'tr', 'Filo Raporu', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Filo Raporu · {{period_label}}</h1>
<table class="doc-meta"><tr><th>Filo</th><td>{{fleet_legal_name}}</td><th>Vergi no</th><td>{{fleet_tax_number}}</td></tr><tr><th>Dönem</th><td>{{period_start}} – {{period_end}}</td><th>Rapor tarihi</th><td>{{document_date}}</td></tr></table>
<h2>Özet</h2><table class="doc-totals"><tr><th>Araç sayısı</th><td>{{vehicle_count}}</td></tr><tr><th>Hizmet sayısı</th><td>{{service_count}}</td></tr><tr><th>Bayi sayısı</th><td>{{dealer_count}}</td></tr><tr><th>Aktif garanti</th><td>{{warranty_active_count}}</td></tr><tr><th>Biten garanti</th><td>{{warranty_expired_count}}</td></tr><tr><th>Dönemde başlayan garanti</th><td>{{warranty_started_count}}</td></tr></table>
<h2>Bayi bazında hizmetler</h2>{{dealer_services_table}}
<h2>Araç bazında hizmetler</h2>{{vehicle_services_table}}
<h2>Parça dağılımı</h2>{{parts_table}}
<h2>Kullanılan ürünler</h2>{{products_table}}
<h2>Yaklaşan garanti bitişleri</h2>{{upcoming_expirations_table}}
<h2>Hizmet tutarı ve ödemeler</h2>{{accounts_table}}
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('fleet_report', 'en', 'Fleet Report', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Fleet Report · {{period_label}}</h1>
<table class="doc-meta"><tr><th>Fleet</th><td>{{fleet_legal_name}}</td><th>Tax number</th><td>{{fleet_tax_number}}</td></tr><tr><th>Period</th><td>{{period_start}} – {{period_end}}</td><th>Report date</th><td>{{document_date}}</td></tr></table>
<h2>Summary</h2><table class="doc-totals"><tr><th>Vehicles</th><td>{{vehicle_count}}</td></tr><tr><th>Services</th><td>{{service_count}}</td></tr><tr><th>Dealers</th><td>{{dealer_count}}</td></tr><tr><th>Active warranties</th><td>{{warranty_active_count}}</td></tr><tr><th>Expired warranties</th><td>{{warranty_expired_count}}</td></tr><tr><th>Warranties started in period</th><td>{{warranty_started_count}}</td></tr></table>
<h2>Services by dealer</h2>{{dealer_services_table}}
<h2>Services by vehicle</h2>{{vehicle_services_table}}
<h2>Part distribution</h2>{{parts_table}}
<h2>Products used</h2>{{products_table}}
<h2>Upcoming warranty expirations</h2>{{upcoming_expirations_table}}
<h2>Service amounts and payments</h2>{{accounts_table}}
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('fleet_report', 'bg', 'Отчет за автопарка', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Отчет за автопарка · {{period_label}}</h1>
<table class="doc-meta"><tr><th>Автопарк</th><td>{{fleet_legal_name}}</td><th>Данъчен номер</th><td>{{fleet_tax_number}}</td></tr><tr><th>Период</th><td>{{period_start}} – {{period_end}}</td><th>Дата на отчета</th><td>{{document_date}}</td></tr></table>
<h2>Обобщение</h2><table class="doc-totals"><tr><th>Брой автомобили</th><td>{{vehicle_count}}</td></tr><tr><th>Брой услуги</th><td>{{service_count}}</td></tr><tr><th>Брой дилъри</th><td>{{dealer_count}}</td></tr><tr><th>Активни гаранции</th><td>{{warranty_active_count}}</td></tr><tr><th>Изтекли гаранции</th><td>{{warranty_expired_count}}</td></tr><tr><th>Гаранции, започнали през периода</th><td>{{warranty_started_count}}</td></tr></table>
<h2>Услуги по дилъри</h2>{{dealer_services_table}}
<h2>Услуги по автомобили</h2>{{vehicle_services_table}}
<h2>Разпределение по части</h2>{{parts_table}}
<h2>Използвани продукти</h2>{{products_table}}
<h2>Предстоящи изтичания на гаранции</h2>{{upcoming_expirations_table}}
<h2>Суми за услуги и плащания</h2>{{accounts_table}}
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('fleet_report', 'de', 'Flottenbericht', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Flottenbericht · {{period_label}}</h1>
<table class="doc-meta"><tr><th>Flotte</th><td>{{fleet_legal_name}}</td><th>Steuernummer</th><td>{{fleet_tax_number}}</td></tr><tr><th>Zeitraum</th><td>{{period_start}} – {{period_end}}</td><th>Berichtsdatum</th><td>{{document_date}}</td></tr></table>
<h2>Übersicht</h2><table class="doc-totals"><tr><th>Fahrzeuge</th><td>{{vehicle_count}}</td></tr><tr><th>Dienstleistungen</th><td>{{service_count}}</td></tr><tr><th>Händler</th><td>{{dealer_count}}</td></tr><tr><th>Aktive Garantien</th><td>{{warranty_active_count}}</td></tr><tr><th>Abgelaufene Garantien</th><td>{{warranty_expired_count}}</td></tr><tr><th>Im Zeitraum begonnene Garantien</th><td>{{warranty_started_count}}</td></tr></table>
<h2>Dienstleistungen nach Händler</h2>{{dealer_services_table}}
<h2>Dienstleistungen nach Fahrzeug</h2>{{vehicle_services_table}}
<h2>Verteilung nach Bauteilen</h2>{{parts_table}}
<h2>Verwendete Produkte</h2>{{products_table}}
<h2>Bald ablaufende Garantien</h2>{{upcoming_expirations_table}}
<h2>Leistungsbeträge und Zahlungen</h2>{{accounts_table}}
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('fleet_report', 'el', 'Αναφορά στόλου', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Αναφορά στόλου · {{period_label}}</h1>
<table class="doc-meta"><tr><th>Στόλος</th><td>{{fleet_legal_name}}</td><th>ΑΦΜ</th><td>{{fleet_tax_number}}</td></tr><tr><th>Περίοδος</th><td>{{period_start}} – {{period_end}}</td><th>Ημερομηνία αναφοράς</th><td>{{document_date}}</td></tr></table>
<h2>Σύνοψη</h2><table class="doc-totals"><tr><th>Οχήματα</th><td>{{vehicle_count}}</td></tr><tr><th>Υπηρεσίες</th><td>{{service_count}}</td></tr><tr><th>Αντιπρόσωποι</th><td>{{dealer_count}}</td></tr><tr><th>Ενεργές εγγυήσεις</th><td>{{warranty_active_count}}</td></tr><tr><th>Ληγμένες εγγυήσεις</th><td>{{warranty_expired_count}}</td></tr><tr><th>Εγγυήσεις που ξεκίνησαν στην περίοδο</th><td>{{warranty_started_count}}</td></tr></table>
<h2>Υπηρεσίες ανά αντιπρόσωπο</h2>{{dealer_services_table}}
<h2>Υπηρεσίες ανά όχημα</h2>{{vehicle_services_table}}
<h2>Κατανομή ανά τμήμα</h2>{{parts_table}}
<h2>Προϊόντα που χρησιμοποιήθηκαν</h2>{{products_table}}
<h2>Επερχόμενες λήξεις εγγυήσεων</h2>{{upcoming_expirations_table}}
<h2>Ποσά υπηρεσιών και πληρωμές</h2>{{accounts_table}}
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('fleet_report', 'uk', 'Звіт автопарку', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Звіт автопарку · {{period_label}}</h1>
<table class="doc-meta"><tr><th>Автопарк</th><td>{{fleet_legal_name}}</td><th>Податковий номер</th><td>{{fleet_tax_number}}</td></tr><tr><th>Період</th><td>{{period_start}} – {{period_end}}</td><th>Дата звіту</th><td>{{document_date}}</td></tr></table>
<h2>Підсумок</h2><table class="doc-totals"><tr><th>Кількість автомобілів</th><td>{{vehicle_count}}</td></tr><tr><th>Кількість послуг</th><td>{{service_count}}</td></tr><tr><th>Кількість дилерів</th><td>{{dealer_count}}</td></tr><tr><th>Активні гарантії</th><td>{{warranty_active_count}}</td></tr><tr><th>Завершені гарантії</th><td>{{warranty_expired_count}}</td></tr><tr><th>Гарантії, що почалися в періоді</th><td>{{warranty_started_count}}</td></tr></table>
<h2>Послуги за дилерами</h2>{{dealer_services_table}}
<h2>Послуги за автомобілями</h2>{{vehicle_services_table}}
<h2>Розподіл за деталями</h2>{{parts_table}}
<h2>Використані продукти</h2>{{products_table}}
<h2>Найближчі закінчення гарантій</h2>{{upcoming_expirations_table}}
<h2>Суми послуг і платежі</h2>{{accounts_table}}
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('fleet_report', 'ru', 'Отчёт автопарка', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Отчёт автопарка · {{period_label}}</h1>
<table class="doc-meta"><tr><th>Автопарк</th><td>{{fleet_legal_name}}</td><th>ИНН</th><td>{{fleet_tax_number}}</td></tr><tr><th>Период</th><td>{{period_start}} – {{period_end}}</td><th>Дата отчёта</th><td>{{document_date}}</td></tr></table>
<h2>Сводка</h2><table class="doc-totals"><tr><th>Количество автомобилей</th><td>{{vehicle_count}}</td></tr><tr><th>Количество услуг</th><td>{{service_count}}</td></tr><tr><th>Количество дилеров</th><td>{{dealer_count}}</td></tr><tr><th>Активные гарантии</th><td>{{warranty_active_count}}</td></tr><tr><th>Истёкшие гарантии</th><td>{{warranty_expired_count}}</td></tr><tr><th>Гарантии, начатые в периоде</th><td>{{warranty_started_count}}</td></tr></table>
<h2>Услуги по дилерам</h2>{{dealer_services_table}}
<h2>Услуги по автомобилям</h2>{{vehicle_services_table}}
<h2>Распределение по деталям</h2>{{parts_table}}
<h2>Использованные продукты</h2>{{products_table}}
<h2>Ближайшие окончания гарантий</h2>{{upcoming_expirations_table}}
<h2>Суммы услуг и платежи</h2>{{accounts_table}}
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('fleet_report', 'fr', 'Rapport de flotte', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Rapport de flotte · {{period_label}}</h1>
<table class="doc-meta"><tr><th>Flotte</th><td>{{fleet_legal_name}}</td><th>Numéro fiscal</th><td>{{fleet_tax_number}}</td></tr><tr><th>Période</th><td>{{period_start}} – {{period_end}}</td><th>Date du rapport</th><td>{{document_date}}</td></tr></table>
<h2>Synthèse</h2><table class="doc-totals"><tr><th>Véhicules</th><td>{{vehicle_count}}</td></tr><tr><th>Prestations</th><td>{{service_count}}</td></tr><tr><th>Concessionnaires</th><td>{{dealer_count}}</td></tr><tr><th>Garanties actives</th><td>{{warranty_active_count}}</td></tr><tr><th>Garanties expirées</th><td>{{warranty_expired_count}}</td></tr><tr><th>Garanties commencées sur la période</th><td>{{warranty_started_count}}</td></tr></table>
<h2>Prestations par concessionnaire</h2>{{dealer_services_table}}
<h2>Prestations par véhicule</h2>{{vehicle_services_table}}
<h2>Répartition par pièce</h2>{{parts_table}}
<h2>Produits utilisés</h2>{{products_table}}
<h2>Prochaines fins de garantie</h2>{{upcoming_expirations_table}}
<h2>Montants des prestations et paiements</h2>{{accounts_table}}
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('fleet_report', 'es', 'Informe de flota', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Informe de flota · {{period_label}}</h1>
<table class="doc-meta"><tr><th>Flota</th><td>{{fleet_legal_name}}</td><th>Número fiscal</th><td>{{fleet_tax_number}}</td></tr><tr><th>Periodo</th><td>{{period_start}} – {{period_end}}</td><th>Fecha del informe</th><td>{{document_date}}</td></tr></table>
<h2>Resumen</h2><table class="doc-totals"><tr><th>Vehículos</th><td>{{vehicle_count}}</td></tr><tr><th>Servicios</th><td>{{service_count}}</td></tr><tr><th>Concesionarios</th><td>{{dealer_count}}</td></tr><tr><th>Garantías activas</th><td>{{warranty_active_count}}</td></tr><tr><th>Garantías vencidas</th><td>{{warranty_expired_count}}</td></tr><tr><th>Garantías iniciadas en el periodo</th><td>{{warranty_started_count}}</td></tr></table>
<h2>Servicios por concesionario</h2>{{dealer_services_table}}
<h2>Servicios por vehículo</h2>{{vehicle_services_table}}
<h2>Distribución por pieza</h2>{{parts_table}}
<h2>Productos utilizados</h2>{{products_table}}
<h2>Próximos vencimientos de garantía</h2>{{upcoming_expirations_table}}
<h2>Importes de servicios y pagos</h2>{{accounts_table}}
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('fleet_report', 'it', 'Report della flotta', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Report della flotta · {{period_label}}</h1>
<table class="doc-meta"><tr><th>Flotta</th><td>{{fleet_legal_name}}</td><th>Partita IVA</th><td>{{fleet_tax_number}}</td></tr><tr><th>Periodo</th><td>{{period_start}} – {{period_end}}</td><th>Data del report</th><td>{{document_date}}</td></tr></table>
<h2>Riepilogo</h2><table class="doc-totals"><tr><th>Veicoli</th><td>{{vehicle_count}}</td></tr><tr><th>Servizi</th><td>{{service_count}}</td></tr><tr><th>Concessionari</th><td>{{dealer_count}}</td></tr><tr><th>Garanzie attive</th><td>{{warranty_active_count}}</td></tr><tr><th>Garanzie scadute</th><td>{{warranty_expired_count}}</td></tr><tr><th>Garanzie iniziate nel periodo</th><td>{{warranty_started_count}}</td></tr></table>
<h2>Servizi per concessionario</h2>{{dealer_services_table}}
<h2>Servizi per veicolo</h2>{{vehicle_services_table}}
<h2>Distribuzione per parte</h2>{{parts_table}}
<h2>Prodotti utilizzati</h2>{{products_table}}
<h2>Prossime scadenze di garanzia</h2>{{upcoming_expirations_table}}
<h2>Importi dei servizi e pagamenti</h2>{{accounts_table}}
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('fleet_report', 'zh_CN', '车队报告', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">车队报告 · {{period_label}}</h1>
<table class="doc-meta"><tr><th>车队</th><td>{{fleet_legal_name}}</td><th>税号</th><td>{{fleet_tax_number}}</td></tr><tr><th>期间</th><td>{{period_start}} – {{period_end}}</td><th>报告日期</th><td>{{document_date}}</td></tr></table>
<h2>摘要</h2><table class="doc-totals"><tr><th>车辆数</th><td>{{vehicle_count}}</td></tr><tr><th>服务数</th><td>{{service_count}}</td></tr><tr><th>经销商数</th><td>{{dealer_count}}</td></tr><tr><th>有效质保</th><td>{{warranty_active_count}}</td></tr><tr><th>已到期质保</th><td>{{warranty_expired_count}}</td></tr><tr><th>本期开始的质保</th><td>{{warranty_started_count}}</td></tr></table>
<h2>按经销商统计的服务</h2>{{dealer_services_table}}
<h2>按车辆统计的服务</h2>{{vehicle_services_table}}
<h2>部位分布</h2>{{parts_table}}
<h2>使用的产品</h2>{{products_table}}
<h2>即将到期的质保</h2>{{upcoming_expirations_table}}
<h2>服务金额与付款</h2>{{accounts_table}}
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('fleet_report', 'az', 'Avtopark hesabatı', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">Avtopark hesabatı · {{period_label}}</h1>
<table class="doc-meta"><tr><th>Avtopark</th><td>{{fleet_legal_name}}</td><th>Vergi nömrəsi</th><td>{{fleet_tax_number}}</td></tr><tr><th>Dövr</th><td>{{period_start}} – {{period_end}}</td><th>Hesabat tarixi</th><td>{{document_date}}</td></tr></table>
<h2>Xülasə</h2><table class="doc-totals"><tr><th>Avtomobil sayı</th><td>{{vehicle_count}}</td></tr><tr><th>Xidmət sayı</th><td>{{service_count}}</td></tr><tr><th>Diler sayı</th><td>{{dealer_count}}</td></tr><tr><th>Aktiv zəmanətlər</th><td>{{warranty_active_count}}</td></tr><tr><th>Bitmiş zəmanətlər</th><td>{{warranty_expired_count}}</td></tr><tr><th>Dövrdə başlayan zəmanətlər</th><td>{{warranty_started_count}}</td></tr></table>
<h2>Dilerlər üzrə xidmətlər</h2>{{dealer_services_table}}
<h2>Avtomobillər üzrə xidmətlər</h2>{{vehicle_services_table}}
<h2>Hissələr üzrə bölgü</h2>{{parts_table}}
<h2>İstifadə olunan məhsullar</h2>{{products_table}}
<h2>Yaxınlaşan zəmanət bitmələri</h2>{{upcoming_expirations_table}}
<h2>Xidmət məbləğləri və ödənişlər</h2>{{accounts_table}}
<p class="doc-footer">{{footer_text}}</p>$html$),
    ('fleet_report', 'ar', 'تقرير الأسطول', $html$<header class="doc-header"><div class="doc-logo">{{company_logo}}</div><div class="doc-company"><strong>{{company_name}}</strong><br>{{company_address}}<br>{{company_phone}} · {{company_email}}</div></header>
<h1 class="doc-title">تقرير الأسطول · {{period_label}}</h1>
<table class="doc-meta"><tr><th>الأسطول</th><td>{{fleet_legal_name}}</td><th>الرقم الضريبي</th><td>{{fleet_tax_number}}</td></tr><tr><th>الفترة</th><td>{{period_start}} – {{period_end}}</td><th>تاريخ التقرير</th><td>{{document_date}}</td></tr></table>
<h2>الملخص</h2><table class="doc-totals"><tr><th>عدد المركبات</th><td>{{vehicle_count}}</td></tr><tr><th>عدد الخدمات</th><td>{{service_count}}</td></tr><tr><th>عدد الوكلاء</th><td>{{dealer_count}}</td></tr><tr><th>الضمانات السارية</th><td>{{warranty_active_count}}</td></tr><tr><th>الضمانات المنتهية</th><td>{{warranty_expired_count}}</td></tr><tr><th>الضمانات التي بدأت في الفترة</th><td>{{warranty_started_count}}</td></tr></table>
<h2>الخدمات حسب الوكيل</h2>{{dealer_services_table}}
<h2>الخدمات حسب المركبة</h2>{{vehicle_services_table}}
<h2>توزيع الأجزاء</h2>{{parts_table}}
<h2>المنتجات المستخدمة</h2>{{products_table}}
<h2>انتهاءات الضمان القادمة</h2>{{upcoming_expirations_table}}
<h2>مبالغ الخدمات والمدفوعات</h2>{{accounts_table}}
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
