# i18n terim sözlüğü (TEC-1233)

Uygulamanın 14 dilinde alan terimlerinin kullanılan karşılıkları. Tablo web kataloglarından üretilir (anahtar sütunu kaynaktır); yeni çeviri yazarken bu karşılıkları kullanın. Dile özgü ayrıntılı sözlükler ve üslup notları: `frontend/scripts/i18n-glossary.<dil>.json`.

## Kurallar

- Marka ve ürün adları çevrilmez: Olexfilms, Olex, Olex Films®, Glorian, NexPTG, SmartPTG, WhatsApp.
- Kısaltmalar ve birimler çevrilmez: PPF, PDF, CSV, VIN, SKU, QR, OTP, IBAN, ZPL; mm, m, m², µm, mil, kg.
- `{{param}}` yer tutucuları birebir korunur; çoğul biçimleri dilin CLDR kategorilerine uyar (ar: zero/one/two/few/many/other; ru/uk: one/few/many/other; fr/es/it: one/many/other; zh-CN: other; fa: one/other).
- Barkod, plaka, kod ve tutar alanlarında rakamlar Latin kalır. Arapçada kullanıcı Arap-Hint rakamlarını seçebilir; Farsça (fa) her yerde Latin rakam kullanır (karar: TEC-1233).
- RTL diller: ar, fa (`RTL_LOCALES`: web `frontend/src/config/i18n.ts`, mobil `mobile/src/core/i18n/index.ts`, backend `i18n.IsRTL`).
- Bilinçli olarak kaynakla aynı bırakılan değerler gerekçesiyle `scripts/i18n-allowlist.json` içindedir; `make i18n-scan` bunları saymaz.

## Terimler

| Terim | anahtar | tr | en | bg | de | el | uk | ru | fr | es | it | zh-CN | az | ar | fa |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| PPF (boya koruma filmi) | `literal:PPF` | PPF | PPF | PPF | PPF | PPF | PPF | PPF | PPF | PPF | PPF | PPF | PPF | PPF | PPF |
| Cam filmi | `landing:solutions.window_film.title` | Cam filmi | Window film | Фолио за стъкла | Fensterfolie | Μεμβράνη τζαμιών | Тонувальна плівка | Тонировочная плёнка | Film pour vitrage | Lámina para ventanas | Pellicola per vetri | 车窗膜 | Şüşə filmi | أفلام النوافذ | فیلم شیشه |
| Rulo | `services:stock.mode.roll` | Rulo | Roll | Ролка | Rolle | Ρολό | Рулон | Рулон | Rouleau | Rollo | Rotolo | 卷材 | Rulon | لفّة | رول |
| Barkod | `warehouse:fields.barcode` | Barkod | Barcode | Баркод | Barcode | Barcode | Штрихкод | Штрихкод | Code-barres | Código de barras | Codice a barre | 条码 | Barkod | الباركود | بارکد |
| Garanti | `warranty:detail.title` | Garanti | Warranty | Гаранция | Garantie | Εγγύηση | Гарантія | Гарантия | Garantie | Garantía | Garanzia | 质保 | Zəmanət | الضمان | گارانتی |
| Garanti belgesi | `documents:kinds.warranty` | Garanti belgesi | Warranty certificate | Гаранционен сертификат | Garantiezertifikat | Πιστοποιητικό εγγύησης | Гарантійний сертифікат | Гарантийный сертификат | Certificat de garantie | Certificado de garantía | Certificato di garanzia | 质保证书 | Zəmanət sertifikatı | شهادة الضمان | گواهی گارانتی |
| Bayi | `customers:actions.upgrade.dealer` | Bayi | Dealer | Дилър | Händler | Αντιπρόσωπος | Дилер | Дилер | Revendeur | Concesionario | Rivenditore | 经销商 | Diler | الوكيل | نمایندگی |
| Distribütör | `geo:territories.distributor` | Distribütör | Distributor | Дистрибутор | Vertriebspartner | Διανομέας | Дистриб'ютор | Дистрибьютор | Distributeur | Distribuidor | Distributore | 分销商 | Distribyutor | الموزّع | توزیع‌کننده |
| Merkez | `ai_admin:org_type.center` | Merkez | Center | Център | Zentrale | Κέντρο | Центр | Центр | Centre | Centro | Centro | 中心 | Mərkəz | المركز | مرکز |
| Depo | `warehouse:fields.warehouse` | Depo | Warehouse | Склад | Lager | Αποθήκη | Склад | Склад | Entrepôt | Almacén | Magazzino | 仓库 | Anbar | المستودع | انبار |
| Stok | `stock:nav` | Stok | Stock | Наличност | Bestand | Απόθεμα | Склад | Склад | Stock | Stock | Stock | 库存 | Stok | المخزون | موجودی |
| Sevk (gönder) | `warehouse:transfer.ship` | Sevk et | Ship | Изпращане | Versenden | Αποστολή | Відвантажити | Отгрузить | Expédier | Enviar | Spedisci | 发出 | Göndər | شحن | ارسال |
| İrsaliye | `warehouse:receiving.attachment_kind.waybill` | İrsaliye | Waybill | Товарителница | Lieferschein | Δελτίο αποστολής | Накладна | Накладная | Bon de livraison | Albarán | DDT | 运单 | Qaimə | بوليصة الشحن | بارنامه |
| Fatura | `orders:items.invoice` | Fatura | Invoice | — | Rechnung | — | Рахунок | Счёт | Facture | Factura | Fattura | — | — | — | فاکتور |
| Kullanım kredisi | `catalog:fields.usage_credits` | Kullanım kredisi | Usage credits | Кредити за употреба | Nutzungsguthaben | Πιστώσεις χρήσης | Кредити використання | Кредиты использования | Crédits d'utilisation | Créditos de uso | Crediti di utilizzo | 使用额度 | İstifadə krediti | أرصدة الاستخدام | اعتبار مصرف |
| Kod | `warehouse:fields.code` | Kod | Code | Код | Code | Κωδικός | Код | Код | Code | Código | Codice | 代码 | Kod | الرمز | کد |
| Transfer | `warehouse:transfer.title` | Transfer | Transfer | Трансфер | Umlagerung | Μεταφορά | Переміщення | Перемещение | Transfert | Transferencia | Trasferimento | 调拨 | Transfer | التحويل | انتقال |
| Konum | `warehouse:fields.location` | Konum | Location | Местоположение | Lagerplatz | Θέση | Місце зберігання | Место хранения | Emplacement | Ubicación | Ubicazione | 库位 | Yer | الموقع | مکان |
| Fire | `stock_adjustments:type.waste` | Fire | Waste | Фира | Verschnitt | Φύρα | Відходи | Отходы | Chute | Merma | Sfrido | 损耗 | Tullantı | هدر | ضایعات |
| Sipariş | `orders:detail.title` | Sipariş | Order | Поръчка | Bestellung | Παραγγελία | Замовлення | Заказ | Commande | Pedido | Ordine | 订单 | Sifariş | طلب | سفارش |
| Kısmi sevk / bekleyen sipariş | `orders:partial.badge` | Bakiye | Backorder | Допълнителна | Rückstand | Εκκρεμής | Дозамовлення | Дозаказ | Reliquat | Pendiente | Arretrato | 补发 | Qalıq | متأخر | سفارش معوق |
| Tedarikçi | `warehouse:delivery.supplier` | Tedarikçi | Supplier | Доставчик | Lieferant | Προμηθευτής | Постачальник | Поставщик | Fournisseur | Proveedor | Fornitore | 供应商 | Təchizatçı | المورّد | تأمین‌کننده |
| Plaka | `fleets:fields.plate` | Plaka | Plate | Регистрационен номер | Kennzeichen | Πινακίδα | Держномер | Госномер | Plaque | Matrícula | Targa | 车牌 | Dövlət nömrə nişanı | لوحة المركبة | پلاک |
| Araç | `services:vehicle.title` | Araç | Vehicle | Автомобил | Fahrzeug | Όχημα | Автомобіль | Автомобиль | Véhicule | Vehículo | Veicolo | 车辆 | Avtomobil | المركبة | خودرو |
| Müşteri | `customers:detail.title` | Müşteri | Customer | Клиент | Kunde | Πελάτης | Клієнт | Клиент | Client | Cliente | Cliente | 客户 | Müştəri | العميل | مشتری |
| Ürün | `stock_forecast:columns.product` | Ürün | Product | Продукт | Produkt | Προϊόν | Товар | Товар | Produit | Producto | Prodotto | 产品 | Məhsul | المنتج | محصول |
| Alacak | `accounting:statement.credit` | Alacak | Credit | Кредит | Haben | Πίστωση | Кредит | Кредит | Crédit | Haber | Avere | 贷方 | Kredit | دائن | بستانکار |
