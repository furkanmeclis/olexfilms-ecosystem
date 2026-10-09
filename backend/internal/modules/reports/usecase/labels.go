package usecase

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"

// labelLocales is the column order of every labels entry.
var labelLocales = []i18n.Locale{
	i18n.LocaleTR, i18n.LocaleEN, i18n.LocaleBG, i18n.LocaleDE, i18n.LocaleEL, i18n.LocaleUK, i18n.LocaleRU,
	i18n.LocaleFR, i18n.LocaleES, i18n.LocaleIT, i18n.LocaleZhCN, i18n.LocaleAZ, i18n.LocaleAR,
}

// label is one key in the 13 UI locales (tr, en, bg, de, el, uk, ru, fr,
// es, it, zh-CN, az, ar).
type label [13]string

// labels are the report titles, series, points, cards and columns the
// envelope carries in the request locale (K10).
var labels = map[string]label{
	// Report titles.
	"title.overview":                     {"Genel bakış", "Overview", "Общ преглед", "Übersicht", "Επισκόπηση", "Огляд", "Обзор", "Vue d'ensemble", "Resumen general", "Panoramica", "概览", "Ümumi baxış", "نظرة عامة"},
	"title.services.trend":               {"Hizmet trendi", "Service trend", "Тенденция на услугите", "Serviceverlauf", "Τάση υπηρεσιών", "Динаміка послуг", "Динамика услуг", "Tendance des prestations", "Tendencia de servicios", "Andamento dei servizi", "服务趋势", "Xidmət trendi", "اتجاه الخدمات"},
	"title.services.status_distribution": {"Hizmet durum dağılımı", "Service status distribution", "Разпределение на услугите по статус", "Serviceaufträge nach Zustand", "Κατανομή υπηρεσιών ανά κατάσταση", "Розподіл послуг за статусом", "Распределение услуг по статусу", "Répartition des prestations par statut", "Distribución de servicios por estado", "Distribuzione dei servizi per stato", "服务状态分布", "Xidmətlərin status bölgüsü", "توزيع الخدمات حسب الحالة"},
	"title.services.top_brands":          {"En çok hizmet alan markalar", "Top car brands", "Най-обслужвани марки", "Meistbediente Automarken", "Κορυφαίες μάρκες αυτοκινήτων", "Найпопулярніші марки авто", "Самые популярные марки авто", "Marques les plus traitées", "Marcas más atendidas", "Marche più servite", "热门汽车品牌", "Ən çox xidmət alan markalar", "أكثر العلامات التجارية خدمةً"},
	"title.services.top_models":          {"En çok hizmet alan modeller", "Top car models", "Най-обслужвани модели", "Meistbediente Modelle", "Κορυφαία μοντέλα", "Найпопулярніші моделі", "Самые популярные модели", "Modèles les plus traités", "Modelos más atendidos", "Modelli più serviti", "热门车型", "Ən çox xidmət alan modellər", "أكثر الطرازات خدمةً"},
	"title.services.top_products":        {"En çok kullanılan ürünler", "Top products", "Най-използвани продукти", "Meistverwendete Produkte", "Πιο χρησιμοποιούμενα προϊόντα", "Найуживаніші продукти", "Самые используемые продукты", "Produits les plus utilisés", "Productos más utilizados", "Prodotti più utilizzati", "最常用产品", "Ən çox istifadə olunan məhsullar", "أكثر المنتجات استخدامًا"},
	"title.orders.trend":                 {"Sipariş trendi", "Order trend", "Тенденция на поръчките", "Bestellverlauf", "Τάση παραγγελιών", "Динаміка замовлень", "Динамика заказов", "Tendance des commandes", "Tendencia de pedidos", "Andamento degli ordini", "订单趋势", "Sifariş trendi", "اتجاه الطلبات"},
	"title.orders.status_distribution":   {"Sipariş durum dağılımı", "Order status distribution", "Разпределение на поръчките по статус", "Bestellungen nach Zustand", "Κατανομή παραγγελιών ανά κατάσταση", "Розподіл замовлень за статусом", "Распределение заказов по статусу", "Répartition des commandes par statut", "Distribución de pedidos por estado", "Distribuzione degli ordini per stato", "订单状态分布", "Sifarişlərin status bölgüsü", "توزيع الطلبات حسب الحالة"},
	"title.customers.trend":              {"Müşteri trendi", "Customer trend", "Тенденция на клиентите", "Kundenentwicklung", "Τάση πελατών", "Динаміка клієнтів", "Динамика клиентов", "Tendance des clients", "Tendencia de clientes", "Andamento dei clienti", "客户趋势", "Müştəri trendi", "اتجاه العملاء"},
	"title.stock.summary":                {"Stok özeti", "Stock summary", "Обобщение на наличностите", "Lagerübersicht", "Σύνοψη αποθέματος", "Зведення запасів", "Сводка по запасам", "Synthèse du stock", "Resumen de existencias", "Riepilogo magazzino", "库存概要", "Stok xülasəsi", "ملخص المخزون"},
	"title.warranties.summary":           {"Garanti özeti", "Warranty summary", "Обобщение на гаранциите", "Garantieübersicht", "Σύνοψη εγγυήσεων", "Зведення гарантій", "Сводка по гарантиям", "Synthèse des garanties", "Resumen de garantías", "Riepilogo garanzie", "质保概要", "Zəmanət xülasəsi", "ملخص الضمانات"},
	"title.measurements.summary":         {"Ölçüm özeti", "Measurement summary", "Обобщение на измерванията", "Messübersicht", "Σύνοψη μετρήσεων", "Зведення вимірювань", "Сводка по измерениям", "Synthèse des mesures", "Resumen de mediciones", "Riepilogo misurazioni", "测量概要", "Ölçmə xülasəsi", "ملخص القياسات"},
	"title.dealers.performance":          {"Bayi performansı", "Dealer performance", "Представяне на дилърите", "Händlerleistung", "Απόδοση αντιπροσώπων", "Ефективність дилерів", "Эффективность дилеров", "Performance des concessionnaires", "Rendimiento de concesionarios", "Prestazioni dei concessionari", "经销商绩效", "Diler performansı", "أداء الوكلاء"},
	"title.dealers.top_by_warranty":      {"Garantiye göre en iyi bayiler", "Top dealers by warranty", "Водещи дилъри по гаранции", "Top-Händler nach Garantien", "Κορυφαίοι αντιπρόσωποι κατά εγγυήσεις", "Найкращі дилери за гарантіями", "Лучшие дилеры по гарантиям", "Meilleurs concessionnaires par garanties", "Mejores concesionarios por garantías", "Migliori concessionari per garanzie", "按质保排名的经销商", "Zəmanətə görə ən yaxşı dilerlər", "أفضل الوكلاء حسب الضمانات"},
	"title.activities.recent":            {"Son hizmetler", "Recent services", "Последни услуги", "Neueste Serviceaufträge", "Πρόσφατες υπηρεσίες", "Останні послуги", "Последние услуги", "Prestations récentes", "Servicios recientes", "Servizi recenti", "最近的服务", "Son xidmətlər", "أحدث الخدمات"},

	// Series.
	"series.created":       {"Oluşturulan", "Created", "Създадени", "Erstellt", "Δημιουργήθηκαν", "Створено", "Создано", "Créées", "Creados", "Creati", "已创建", "Yaradılan", "المُنشأة"},
	"series.completed":     {"Tamamlanan", "Completed", "Завършени", "Abgeschlossen", "Ολοκληρώθηκαν", "Завершено", "Завершено", "Terminées", "Completados", "Completati", "已完成", "Tamamlanan", "المكتملة"},
	"series.received":      {"Teslim alınan", "Received", "Получени", "Empfangen", "Παραλήφθηκαν", "Отримано", "Получено", "Réceptionnées", "Recibidos", "Ricevuti", "已收货", "Qəbul edilən", "المستلمة"},
	"series.status":        {"Durum", "Status", "Статус", "Zustand", "Κατάσταση", "Статус", "Статус", "Statut", "Estado", "Stato", "状态", "Vəziyyət", "الحالة"},
	"series.service_count": {"Hizmet sayısı", "Service count", "Брой услуги", "Anzahl Serviceaufträge", "Αριθμός υπηρεσιών", "Кількість послуг", "Количество услуг", "Nombre de prestations", "Número de servicios", "Numero di servizi", "服务数量", "Xidmət sayı", "عدد الخدمات"},
	"series.individual":    {"Bireysel", "Individual", "Физически лица", "Privatkunden", "Ιδιώτες", "Фізичні особи", "Физические лица", "Particuliers", "Particulares", "Privati", "个人", "Fərdi", "أفراد"},
	"series.corporate":     {"Kurumsal", "Corporate", "Юридически лица", "Firmenkunden", "Εταιρικοί", "Юридичні особи", "Юридические лица", "Entreprises", "Empresas", "Aziende", "企业", "Korporativ", "شركات"},
	"series.cards":         {"Özet", "Summary", "Обобщение", "Zusammenfassung", "Σύνοψη", "Підсумок", "Итог", "Résumé", "Resumen", "Sintesi", "摘要", "Xülasə", "ملخص"},
	"series.started":       {"Başlayan garantiler", "Started warranties", "Започнали гаранции", "Begonnene Garantien", "Εγγυήσεις που ξεκίνησαν", "Розпочаті гарантії", "Начатые гарантии", "Garanties démarrées", "Garantías iniciadas", "Garanzie avviate", "已生效质保", "Başlayan zəmanətlər", "الضمانات التي بدأت"},
	"series.measurements":  {"Ölçümler", "Measurements", "Измервания", "Messungen", "Μετρήσεις", "Вимірювання", "Измерения", "Mesures", "Mediciones", "Misurazioni", "测量", "Ölçmələr", "القياسات"},
	"series.units":         {"Seri birimler", "Serial units", "Серийни единици", "Serieneinheiten", "Σειριακές μονάδες", "Серійні одиниці", "Серийные единицы", "Unités sérialisées", "Unidades seriadas", "Unità seriali", "序列单元", "Seriya vahidləri", "الوحدات التسلسلية"},

	// Service statuses.
	"services.status.draft":      {"Taslak", "Draft", "Чернова", "Entwurf", "Πρόχειρο", "Чернетка", "Черновик", "Brouillon", "Borrador", "Bozza", "草稿", "Qaralama", "مسودة"},
	"services.status.pending":    {"Beklemede", "Pending", "Изчакваща", "Ausstehend", "Σε αναμονή", "Очікує", "Ожидает", "En attente", "Pendiente", "In attesa", "待处理", "Gözləmədə", "قيد الانتظار"},
	"services.status.processing": {"İşlemde", "In progress", "В процес", "In Bearbeitung", "Σε εξέλιξη", "В роботі", "В работе", "En cours", "En curso", "In corso", "处理中", "İcrada", "قيد التنفيذ"},
	"services.status.ready":      {"Hazır", "Ready", "Готова", "Fertig", "Έτοιμη", "Готово", "Готово", "Prête", "Listo", "Pronto", "已就绪", "Hazırdır", "جاهزة"},
	"services.status.completed":  {"Tamamlandı", "Completed", "Завършена", "Abgeschlossen", "Ολοκληρώθηκε", "Завершено", "Завершено", "Terminée", "Completado", "Completato", "已完成", "Tamamlandı", "مكتملة"},
	"services.status.cancelled":  {"İptal edildi", "Cancelled", "Отменена", "Storniert", "Ακυρώθηκε", "Скасовано", "Отменено", "Annulée", "Cancelado", "Annullato", "已取消", "Ləğv edildi", "ملغاة"},

	// Order statuses.
	"orders.status.draft":      {"Taslak", "Draft", "Чернова", "Entwurf", "Πρόχειρο", "Чернетка", "Черновик", "Brouillon", "Borrador", "Bozza", "草稿", "Qaralama", "مسودة"},
	"orders.status.submitted":  {"Gönderildi", "Submitted", "Изпратена", "Eingereicht", "Υποβλήθηκε", "Надіслано", "Отправлен", "Soumise", "Enviado", "Inviato", "已提交", "Göndərildi", "مُرسلة"},
	"orders.status.approved":   {"Onaylandı", "Approved", "Одобрена", "Genehmigt", "Εγκρίθηκε", "Схвалено", "Одобрен", "Approuvée", "Aprobado", "Approvato", "已批准", "Təsdiqləndi", "معتمدة"},
	"orders.status.preparing":  {"Hazırlanıyor", "Preparing", "Подготвя се", "In Vorbereitung", "Σε προετοιμασία", "Готується", "Готовится", "En préparation", "En preparación", "In preparazione", "准备中", "Hazırlanır", "قيد التحضير"},
	"orders.status.ready":      {"Hazır", "Ready", "Готова", "Bereit", "Έτοιμη", "Готово", "Готов", "Prête", "Listo", "Pronto", "已就绪", "Hazırdır", "جاهزة"},
	"orders.status.processing": {"İşleniyor", "Processing", "Обработва се", "In Verarbeitung", "Σε επεξεργασία", "Обробляється", "Обрабатывается", "En traitement", "En proceso", "In lavorazione", "处理中", "Emal olunur", "قيد المعالجة"},
	"orders.status.shipped":    {"Kargoya verildi", "Shipped", "Изпратена", "Versandt", "Απεστάλη", "Відвантажено", "Отгружен", "Expédiée", "Despachado", "Spedito", "已发货", "Yola salındı", "تم الشحن"},
	"orders.status.delivered":  {"Teslim edildi", "Delivered", "Доставена", "Zugestellt", "Παραδόθηκε", "Доставлено", "Доставлен", "Livrée", "Entregado", "Consegnato", "已送达", "Çatdırıldı", "تم التسليم"},
	"orders.status.received":   {"Teslim alındı", "Received", "Получена", "Empfangen", "Παραλήφθηκε", "Отримано", "Получен", "Réceptionnée", "Recibido", "Ricevuto", "已收货", "Qəbul edildi", "تم الاستلام"},
	"orders.status.cancelling": {"İptal ediliyor", "Cancelling", "Отменя се", "Wird storniert", "Ακυρώνεται", "Скасовується", "Отменяется", "Annulation en cours", "Cancelándose", "In annullamento", "取消中", "Ləğv edilir", "قيد الإلغاء"},
	"orders.status.cancelled":  {"İptal edildi", "Cancelled", "Отменена", "Storniert", "Ακυρώθηκε", "Скасовано", "Отменён", "Annulée", "Cancelado", "Annullato", "已取消", "Ləğv edildi", "ملغاة"},

	// Warranty statuses.
	"warranties.status.active":  {"Aktif", "Active", "Активна", "Aktiv", "Ενεργή", "Активна", "Активна", "Active", "Activa", "Attiva", "有效", "Aktiv", "نشطة"},
	"warranties.status.expired": {"Süresi doldu", "Expired", "Изтекла", "Abgelaufen", "Έληξε", "Закінчилася", "Истекла", "Expirée", "Vencida", "Scaduta", "已过期", "Müddəti bitib", "منتهية"},
	"warranties.status.void":    {"Geçersiz", "Void", "Анулирана", "Ungültig", "Άκυρη", "Анульована", "Аннулирована", "Annulée", "Anulada", "Annullata", "已作废", "Etibarsız", "ملغاة"},

	// Unit statuses.
	"units.status.reserved":   {"Rezerve", "Reserved", "Резервирана", "Reserviert", "Δεσμευμένη", "Зарезервовано", "Зарезервировано", "Réservée", "Reservada", "Riservata", "已预留", "Rezerv edilib", "محجوزة"},
	"units.status.printed":    {"Basıldı", "Printed", "Отпечатана", "Gedruckt", "Εκτυπώθηκε", "Надруковано", "Напечатано", "Imprimée", "Impresa", "Stampata", "已打印", "Çap edilib", "مطبوعة"},
	"units.status.available":  {"Kullanılabilir", "Available", "Налична", "Verfügbar", "Διαθέσιμη", "Доступно", "Доступно", "Disponible", "Disponible", "Disponibile", "可用", "Mövcuddur", "متاحة"},
	"units.status.placed":     {"Rafta", "Placed", "Разположена", "Eingelagert", "Τοποθετημένη", "Розміщено", "Размещено", "Rangée", "Ubicada", "Collocata", "已上架", "Yerləşdirilib", "موضوعة"},
	"units.status.in_transit": {"Yolda", "In transit", "В транзит", "Unterwegs", "Σε μεταφορά", "У дорозі", "В пути", "En transit", "En tránsito", "In transito", "运输中", "Yoldadır", "قيد النقل"},
	"units.status.used":       {"Kullanıldı", "Used", "Използвана", "Verbraucht", "Χρησιμοποιήθηκε", "Використано", "Использовано", "Utilisée", "Usada", "Utilizzata", "已使用", "İstifadə olunub", "مستخدمة"},
	"units.status.void":       {"Geçersiz", "Void", "Анулирана", "Ungültig", "Άκυρη", "Анульовано", "Аннулировано", "Annulée", "Anulada", "Annullata", "已作废", "Etibarsız", "ملغاة"},

	// Measurement statuses.
	"measurements.status.accepted":    {"Kabul edildi", "Accepted", "Приета", "Angenommen", "Αποδεκτή", "Прийнято", "Принято", "Acceptée", "Aceptada", "Accettata", "已接受", "Qəbul edildi", "مقبولة"},
	"measurements.status.vin_pending": {"Şasi no bekliyor", "VIN pending", "Очаква VIN", "FIN ausstehend", "Εκκρεμεί VIN", "Очікує VIN", "Ожидает VIN", "VIN en attente", "VIN pendiente", "VIN in attesa", "待补 VIN", "VIN gözlənilir", "بانتظار رقم الهيكل"},

	// Summary cards.
	"card.services_created":      {"Açılan hizmetler", "Services opened", "Открити услуги", "Eröffnete Serviceaufträge", "Υπηρεσίες που άνοιξαν", "Відкриті послуги", "Открытые услуги", "Prestations ouvertes", "Servicios abiertos", "Servizi aperti", "新建服务", "Açılan xidmətlər", "الخدمات المفتوحة"},
	"card.services_completed":    {"Tamamlanan hizmetler", "Services completed", "Завършени услуги", "Abgeschlossene Serviceaufträge", "Ολοκληρωμένες υπηρεσίες", "Завершені послуги", "Завершённые услуги", "Prestations terminées", "Servicios completados", "Servizi completati", "已完成服务", "Tamamlanan xidmətlər", "الخدمات المكتملة"},
	"card.services_open":         {"Devam eden hizmetler", "Services in progress", "Текущи услуги", "Laufende Serviceaufträge", "Υπηρεσίες σε εξέλιξη", "Послуги в роботі", "Услуги в работе", "Prestations en cours", "Servicios en curso", "Servizi in corso", "进行中的服务", "Davam edən xidmətlər", "الخدمات الجارية"},
	"card.orders_created":        {"Yeni siparişler", "New orders", "Нови поръчки", "Neue Bestellungen", "Νέες παραγγελίες", "Нові замовлення", "Новые заказы", "Nouvelles commandes", "Pedidos nuevos", "Nuovi ordini", "新订单", "Yeni sifarişlər", "طلبات جديدة"},
	"card.orders_open":           {"Açık siparişler", "Open orders", "Отворени поръчки", "Offene Bestellungen", "Ανοιχτές παραγγελίες", "Відкриті замовлення", "Открытые заказы", "Commandes en cours", "Pedidos abiertos", "Ordini aperti", "未结订单", "Açıq sifarişlər", "طلبات مفتوحة"},
	"card.customers_total":       {"Toplam müşteri", "Total customers", "Общо клиенти", "Kunden gesamt", "Σύνολο πελατών", "Усього клієнтів", "Всего клиентов", "Total des clients", "Total de clientes", "Clienti totali", "客户总数", "Ümumi müştəri", "إجمالي العملاء"},
	"card.customers_new":         {"Yeni müşteriler", "New customers", "Нови клиенти", "Neue Kunden", "Νέοι πελάτες", "Нові клієнти", "Новые клиенты", "Nouveaux clients", "Clientes nuevos", "Nuovi clienti", "新客户", "Yeni müştərilər", "عملاء جدد"},
	"card.warranties_active":     {"Aktif garantiler", "Active warranties", "Активни гаранции", "Aktive Garantien", "Ενεργές εγγυήσεις", "Активні гарантії", "Активные гарантии", "Garanties actives", "Garantías activas", "Garanzie attive", "有效质保", "Aktiv zəmanətlər", "الضمانات النشطة"},
	"card.warranties_expired":    {"Süresi dolan garantiler", "Expired warranties", "Изтекли гаранции", "Abgelaufene Garantien", "Εγγυήσεις που έληξαν", "Прострочені гарантії", "Истёкшие гарантии", "Garanties expirées", "Garantías vencidas", "Garanzie scadute", "已过期质保", "Müddəti bitmiş zəmanətlər", "الضمانات المنتهية"},
	"card.warranties_void":       {"Geçersiz garantiler", "Void warranties", "Анулирани гаранции", "Ungültige Garantien", "Άκυρες εγγυήσεις", "Анульовані гарантії", "Аннулированные гарантии", "Garanties annulées", "Garantías anuladas", "Garanzie annullate", "已作废质保", "Etibarsız zəmanətlər", "الضمانات الملغاة"},
	"card.warranties_expiring":   {"30 gün içinde bitecek garantiler", "Warranties expiring in 30 days", "Гаранции, изтичащи до 30 дни", "In 30 Tagen ablaufende Garantien", "Εγγυήσεις που λήγουν σε 30 ημέρες", "Гарантії, що спливають за 30 днів", "Гарантии, истекающие через 30 дней", "Garanties expirant sous 30 jours", "Garantías que vencen en 30 días", "Garanzie in scadenza entro 30 giorni", "30 天内到期的质保", "30 gün ərzində bitəcək zəmanətlər", "ضمانات تنتهي خلال 30 يومًا"},
	"card.warranties_started":    {"Başlayan garantiler", "Started warranties", "Започнали гаранции", "Begonnene Garantien", "Εγγυήσεις που ξεκίνησαν", "Розпочаті гарантії", "Начатые гарантии", "Garanties démarrées", "Garantías iniciadas", "Garanzie avviate", "已生效质保", "Başlayan zəmanətlər", "الضمانات التي بدأت"},
	"card.stock_products":        {"Stoktaki ürünler", "Products in stock", "Продукти в наличност", "Produkte auf Lager", "Προϊόντα σε απόθεμα", "Продукти на складі", "Товары на складе", "Produits en stock", "Productos en existencia", "Prodotti a magazzino", "在库产品", "Stokdakı məhsullar", "المنتجات في المخزون"},
	"card.stock_quantity":        {"Stok adedi", "Stock quantity", "Количество в наличност", "Lagermenge", "Ποσότητα αποθέματος", "Кількість на складі", "Количество на складе", "Quantité en stock", "Cantidad en existencia", "Quantità a magazzino", "库存数量", "Stok miqdarı", "كمية المخزون"},
	"card.stock_meters":          {"Stok metresi", "Stock meters", "Метри в наличност", "Lagermeter", "Μέτρα αποθέματος", "Метри на складі", "Метры на складе", "Mètres en stock", "Metros en existencia", "Metri a magazzino", "库存米数", "Stok metri", "أمتار المخزون"},
	"card.measurements_total":    {"Ölçümler", "Measurements", "Измервания", "Messungen", "Μετρήσεις", "Вимірювання", "Измерения", "Mesures", "Mediciones", "Misurazioni", "测量", "Ölçmələr", "القياسات"},
	"card.measurements_linked":   {"Hizmete bağlı ölçümler", "Measurements linked to services", "Измервания, свързани с услуги", "Mit Serviceaufträgen verknüpfte Messungen", "Μετρήσεις συνδεδεμένες με υπηρεσίες", "Вимірювання, пов'язані з послугами", "Измерения, привязанные к услугам", "Mesures liées aux prestations", "Mediciones vinculadas a servicios", "Misurazioni collegate ai servizi", "已关联服务的测量", "Xidmətlərə bağlı ölçmələr", "قياسات مرتبطة بالخدمات"},
	"card.measurements_vehicles": {"Ölçülen araçlar", "Measured vehicles", "Измерени автомобили", "Gemessene Fahrzeuge", "Οχήματα που μετρήθηκαν", "Виміряні автомобілі", "Измеренные автомобили", "Véhicules mesurés", "Vehículos medidos", "Veicoli misurati", "已测量车辆", "Ölçülmüş avtomobillər", "المركبات المقاسة"},
	"card.dealers_total":         {"Bayi sayısı", "Dealers", "Дилъри", "Händler", "Αντιπρόσωποι", "Дилери", "Дилеры", "Concessionnaires", "Concesionarios", "Concessionari", "经销商", "Dilerlər", "الوكلاء"},
	"card.dealers_active":        {"Aktif bayiler", "Active dealers", "Активни дилъри", "Aktive Händler", "Ενεργοί αντιπρόσωποι", "Активні дилери", "Активные дилеры", "Concessionnaires actifs", "Concesionarios activos", "Concessionari attivi", "活跃经销商", "Aktiv dilerlər", "الوكلاء النشطون"},

	// Table columns.
	"column.rank":                {"Sıra", "Rank", "Място", "Rang", "Θέση", "Місце", "Место", "Rang", "Puesto", "Posizione", "排名", "Sıra", "الترتيب"},
	"column.organization":        {"İşletme", "Organization", "Организация", "Organisation", "Οργανισμός", "Організація", "Организация", "Organisation", "Organización", "Organizzazione", "机构", "Təşkilat", "المنظمة"},
	"column.type":                {"Tür", "Type", "Вид", "Typ", "Τύπος", "Тип", "Тип", "Type", "Tipo", "Tipo", "类型", "Növ", "النوع"},
	"column.city":                {"İl", "City", "Град", "Stadt", "Πόλη", "Місто", "Город", "Ville", "Ciudad", "Città", "城市", "Şəhər", "المدينة"},
	"column.warranty_count":      {"Garanti sayısı", "Warranties", "Брой гаранции", "Anzahl Garantien", "Αριθμός εγγυήσεων", "Кількість гарантій", "Количество гарантий", "Nombre de garanties", "Número de garantías", "Numero di garanzie", "质保数量", "Zəmanət sayı", "عدد الضمانات"},
	"column.active_count":        {"Aktif garantiler", "Active warranties", "Активни гаранции", "Aktive Garantien", "Ενεργές εγγυήσεις", "Активні гарантії", "Активные гарантии", "Garanties actives", "Garantías activas", "Garanzie attive", "有效质保", "Aktiv zəmanətlər", "الضمانات النشطة"},
	"column.service_no":          {"Hizmet no", "Service no.", "Номер на услуга", "Auftragsnummer", "Αρ. υπηρεσίας", "№ послуги", "№ услуги", "N° de prestation", "N.º de servicio", "N. servizio", "服务编号", "Xidmət nömrəsi", "رقم الخدمة"},
	"column.plate":               {"Plaka", "Plate", "Регистрационен номер", "Kennzeichen", "Πινακίδα", "Номерний знак", "Госномер", "Immatriculation", "Matrícula", "Targa", "车牌", "Nömrə nişanı", "لوحة الترخيص"},
	"column.vehicle":             {"Araç", "Vehicle", "Автомобил", "Fahrzeug", "Όχημα", "Автомобіль", "Автомобиль", "Véhicule", "Vehículo", "Veicolo", "车辆", "Avtomobil", "المركبة"},
	"column.status":              {"Durum", "Status", "Статус", "Zustand", "Κατάσταση", "Статус", "Статус", "Statut", "Estado", "Stato", "状态", "Vəziyyət", "الحالة"},
	"column.created_at":          {"Oluşturulma", "Created at", "Създадено на", "Erstellt am", "Δημιουργήθηκε", "Створено", "Создано", "Créée le", "Creado el", "Creato il", "创建时间", "Yaradılma tarixi", "تاريخ الإنشاء"},
	"column.services_count":      {"Hizmet sayısı", "Service count", "Брой услуги", "Anzahl Serviceaufträge", "Αριθμός υπηρεσιών", "Кількість послуг", "Количество услуг", "Nombre de prestations", "Número de servicios", "Numero di servizi", "服务数量", "Xidmət sayı", "عدد الخدمات"},
	"column.warranty_start_rate": {"Garanti başlatma oranı", "Warranty start rate", "Дял стартирани гаранции", "Garantie-Startquote", "Ποσοστό έναρξης εγγυήσεων", "Частка запуску гарантій", "Доля запуска гарантий", "Taux d'activation des garanties", "Tasa de activación de garantías", "Tasso di attivazione garanzie", "质保启动率", "Zəmanət başlama nisbəti", "معدل بدء الضمان"},
	"column.measurement_rate":    {"Ölçüm oranı", "Measurement rate", "Дял измервания", "Messquote", "Ποσοστό μετρήσεων", "Частка вимірювань", "Доля измерений", "Taux de mesure", "Tasa de medición", "Tasso di misurazione", "测量率", "Ölçmə nisbəti", "معدل القياس"},
	"column.review_avg":          {"Ortalama puan", "Average rating", "Среден рейтинг", "Durchschnittsbewertung", "Μέση βαθμολογία", "Середня оцінка", "Средняя оценка", "Note moyenne", "Valoración media", "Valutazione media", "平均评分", "Orta qiymət", "متوسط التقييم"},
	"column.order_volume":        {"Sipariş hacmi", "Order volume", "Обем на поръчките", "Bestellvolumen", "Όγκος παραγγελιών", "Обсяг замовлень", "Объём заказов", "Volume de commandes", "Volumen de pedidos", "Volume ordini", "订单量", "Sifariş həcmi", "حجم الطلبات"},
}

// translate resolves key for locale; an unknown locale falls back to en and
// an unknown key to the key itself.
func translate(locale i18n.Locale, key string) string {
	l, ok := labels[key]
	if !ok {
		return key
	}
	for i, loc := range labelLocales {
		if loc == locale && l[i] != "" {
			return l[i]
		}
	}
	return l[1]
}
