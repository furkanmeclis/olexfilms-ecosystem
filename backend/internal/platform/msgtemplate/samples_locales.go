package msgtemplate

// localeSamples are the preview values of language-dependent placeholders for
// the locales beyond tr/en (TEC-138), keyed by placeholder key, then locale.
// Placeholders without an entry (plate, links, codes) use the en sample.
// Notification-center events share the keys (customer_name, note, ...).
var localeSamples = map[string]map[string]string{
	"customer_name": {
		"bg": "Иван Петров", "el": "Γιώργος Παπαδόπουλος", "zh-CN": "张伟",
		"az": "Əli Məmmədov", "ar": "أحمد علي",
	},
	"name": {
		"bg": "Иван Петров", "el": "Γιώργος Παπαδόπουλος", "zh-CN": "张伟",
		"az": "Əli Məmmədov", "ar": "أحمد علي",
	},
	"created_by_name": {
		"bg": "Мария Иванова", "el": "Μαρία Νικολάου", "zh-CN": "李娜",
		"az": "Leyla Həsənova", "ar": "محمد حسن",
	},
	"assignee_name": {
		"bg": "Елена Георгиева", "el": "Ελένη Γεωργίου", "zh-CN": "王芳",
		"az": "Aysel Quliyeva", "ar": "فاطمة يوسف",
	},
	"amount": {
		"bg": "1 250,00", "el": "1.250,00", "zh-CN": "1,250.00", "az": "1 250,00", "ar": "1,250.00",
	},
	"total_amount": {
		"bg": "12 500,00 TRY", "el": "12.500,00 TRY", "zh-CN": "TRY 12,500.00",
		"az": "12 500,00 TRY", "ar": "⁨TRY 12,500.00⁩",
	},
	"valid_until": {
		"bg": "30.09.2026 г.", "el": "30/09/2026", "zh-CN": "2026年9月30日",
		"az": "30.09.2026", "ar": "30/09/2026",
	},
	"due_at": {
		"bg": "25.09.2026 г., 14:30", "el": "25/09/2026 14:30", "zh-CN": "2026年9月25日 14:30",
		"az": "25.09.2026 14:30", "ar": "25/09/2026 14:30",
	},
	"due_in": {
		"bg": "след 15 минути", "el": "σε 15 λεπτά", "zh-CN": "15分钟后",
		"az": "15 dəqiqə sonra", "ar": "بعد 15 دقيقة",
	},
	"contract_title": {
		"bg": "Договор за услуга", "el": "Σύμβαση παροχής υπηρεσιών", "zh-CN": "服务协议",
		"az": "Xidmət müqaviləsi", "ar": "اتفاقية الخدمة",
	},
	"todo_title": {
		"bg":    "Проверка на керамичното покритие 34 ABC 123",
		"el":    "Έλεγχος κεραμικής επικάλυψης 34 ABC 123",
		"zh-CN": "34 ABC 123 陶瓷镀膜检查",
		"az":    "34 ABC 123 keramik örtük yoxlanışı",
		"ar":    "فحص الطلاء السيراميكي للمركبة ⁨34 ABC 123⁩",
	},
	"todo_notes": {
		"bg": "Обадете се на клиента.", "el": "Καλέστε τον πελάτη.", "zh-CN": "请致电客户。",
		"az": "Müştəriyə zəng edin.", "ar": "اتصل بالعميل.",
	},
	"note": {
		"bg": "Моля, включете го.", "el": "Παρακαλώ ενεργοποιήστε το.", "zh-CN": "请开启此功能。",
		"az": "Zəhmət olmasa, aktiv edin.", "ar": "يرجى تفعيله.",
	},
	"intent": {
		"bg": "цени", "el": "τιμές", "zh-CN": "价格咨询", "az": "qiymət", "ar": "الأسعار",
	},
}
