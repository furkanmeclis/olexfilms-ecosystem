# Eklentiler kullanıcı kılavuzu

Bu kılavuz Olexfilms panelindeki eklenti modüllerini anlatır: her eklenti ne yapar, kim açar, nasıl talep edilir, hangi ekranda ne yapılır ve hangi izinler gerekir. Kılavuz doküman merkezinde (**Dokümanlar → Doküman kütüphanesi**, "Kılavuzlar" klasörü) tüm ağa açıktır. Türkçe ve İngilizce sürümü vardır; diğer dillerde İngilizce sürüm açılır.

> **Ekran görüntüleri:** `[Ekran görüntüsü: …]` ile işaretli kutular ekran görüntüsü yer tutucusudur. Görseller sonraki sürümde eklenecek.

## 1. Modül seviyeleri

Paneldeki her özellik bir **modüldür**. Modüller üç seviyededir:

| Seviye | Anlamı | Ücret |
| --- | --- | --- |
| Çekirdek | Her zaman açıktır, kapatılamaz (ör. stok, siparişler, hizmetler, müşteriler, muhasebe). | Ücretsiz |
| Standart | Varsayılan olarak açıktır (ör. sözleşme, ölçüm, lead, randevu, duyurular, bayi muhasebesi). | Ücretsiz; hizmet kaydı gerekmez, tekrarlamaz |
| Eklenti | Varsayılan olarak kapalıdır. Talep edilir veya bir üst seviye açar. | Çoğu ücretli; fiyat **Özellikler** sayfasında yazar |

Bu kılavuzdaki eklentiler (modül anahtarı parantez içinde):

| Eklenti | Modül anahtarı | Kim kullanır |
| --- | --- | --- |
| Bayi vitrini ve lead formu | `dealer_showcase` | Bayi, distribütör; onay kuyruğu merkezde |
| Filo müşterileri | `fleet` | Bayi, distribütör, merkez; filo yöneticisi portalda |
| Sertifika | `certificates` | Bayi, distribütör, merkez |
| Stok tahmini ve sipariş önerisi | `stock_forecast` | Bayi, distribütör, merkez |
| Performans ve hedef paneli (prim dahil) | `performance` | Merkez, distribütör, bayi |
| Verimlilik ve fire analizi | `efficiency` | Merkez, distribütör, bayi |
| Fotoğraf standardı | `photo_standard` | Merkez, distribütör; bayi kabulünde uygulanır |
| E-fatura altyapısı (UBL-TR) | `e_invoice` | Yalnız merkez, yalnız Türkiye |
| Tavsiye satış fiyatı | modül değil, izin: `pricing.recommended.read` | Merkez yayınlar; distribütör ve bayi görür |

## 2. Eklentiyi kim açar: zincir kuralları

Bir eklentinin bir organizasyonda açık olup olmadığı yukarıdan aşağıya bir zincirle belirlenir: **platform yöneticisi (merkez) → distribütör → bayi**. Değerler kopyalanmaz; her okumada zincir yeniden hesaplanır ve değişiklik en geç 30 saniyede her yere yansır.

1. **Çekirdek modüller** her zaman açıktır.
2. **Sistem genelinde kapalı** bir modül hiçbir yerde açılamaz. Platform yöneticisi **Modüller** sayfasından (`/platform/modules`) bir modülü sistem genelinde kapatabilir, varsayılan açık/kapalı ve ücretli bilgisini değiştirebilir.
3. **Distribütör, merkez ve distribütörü olmayan bayi** kendi değerini kullanır; değer yoksa modülün varsayılanı geçerlidir.
4. **Distribütöre bağlı bayi** için öncelik sırası:
   1. Platform yöneticisinin bayiye verdiği değer (distribütörde kapalı olsa bile geçerlidir).
   2. Distribütörde kapalıysa bayide de kapalıdır ve modül bayiye **gösterilmez** ("Üst seviyede kapalı").
   3. Bayinin kendi değeri (distribütörün o bayi için verdiği değer).
   4. Distribütörün **bayi standardı**.
   5. Modülün varsayılanı.

Üst seviye bir modülü kapattığında alt seviyedeki kayıtlar silinmez; bayi yalnızca kapalı görür. Üst seviye yeniden açınca bayinin eski değeri geri gelir.

### 2.1 Distribütörün bayileri için açması

Distribütör **Özellikler → Bayiler** sekmesinde bir modülü doğrudan bayileri için açar veya kapatır. Kurallar:

- Distribütör kendisinde kapalı olan bir modülü bayileri için açamaz.
- Yalnız kendi doğrudan bayileri seçilebilir; seçimin tamamı uygulanır ya da hiçbiri uygulanmaz.
- Platform yöneticisinin değer verdiği bayi distribütör tarafından değiştirilemez (`MODULE_ADMIN_OVERRIDE`).
- "Temizle" bayiyi bayi standardına döndürür.

### 2.2 Bayi standardı

**Özellikler → Bayi standardı** sekmesi distribütörün bayileri için varsayılan modül setidir. Kendi değeri olmayan bayiler ve yeni açılan bayiler standardı hemen alır. Distribütör kendisinde kapalı bir modülü standarda ekleyemez.

### 2.3 Platform yöneticisinin bağımsız açması

Platform yöneticisi herhangi bir organizasyonda bir modülü **bağımsız** açabilir veya kapatabilir (kaynak: "Platform yöneticisi"). Bu değer distribütörde kapalı olsa bile bayide geçerlidir; yalnız sistem genelinde kapalı modülde reddedilir. "Temizle" organizasyonu yeniden zincire bağlar.

### 2.4 Modül paketi

Merkezin hizmet kataloğunda **modül paketi** (`module_bundle`) türünde hizmetler vardır. Bir organizasyona modül paketi aboneliği atanınca paketteki modüller açılır (kaynak: "Hizmet kaydı", rozet: "Abonelikle açık"). Üst seviyede kapalı bir modül paketle de açılamaz; atama bu durumda tümüyle reddedilir. Abonelik iptal edilince veya süresi dolunca, başka bir aktif abonelik aynı modülü kapsamıyorsa modül kapanır. Süre dolumu saatlik bir işle kontrol edilir.

Bir eklentinin fiyatı, o eklentiyi içeren satıştaki modül paketinden gelir. Birden fazla paket varsa en az modül içeren, sonra en ucuz olan gösterilir. Distribütörün fiyat override'ı varsa distribütör ve bayileri o fiyatı görür. Satışta paket yoksa "Fiyat için iletişime geçin" yazar.

### 2.5 Özellikler sayfasında kaynak rozetleri

| Rozet | Anlamı |
| --- | --- |
| Çekirdek (her zaman açık) | Çekirdek modül |
| Sistem genelinde kapalı | Platform yöneticisi sistemde kapattı |
| Varsayılan | Modülün varsayılan değeri |
| Üst seviyede kapalı | Distribütörde kapalı olduğu için bayide kapalı |
| Bayi standardı | Distribütörün bayi standardından |
| Platform yöneticisi | Admin bağımsız değeri |
| Distribütör | Distribütörün bu bayi için verdiği değer |
| Hizmet kaydı | Modül paketi aboneliğinden |

## 3. Eklenti talep etme

> **[Ekran görüntüsü: features-page]** Özellikler sayfası: seviye grupları, açıklama, fiyat ibaresi, durum ve kaynak rozeti, "Talep et" düğmesi.

1. Sol menüde **İşletme → Özellikler** sayfasını açın (`/t/{organizasyon}/features`). Sayfayı `modules.read` izni olan herkes görür.
2. Kapalı bir eklentinin satırında **Talep et** düğmesine basın.
3. Açılan pencerede isteğe bağlı bir not yazın (en fazla 1000 karakter) ve gönderin.
4. Satırda **Talep bekliyor** rozeti görünür. Karar verilene kadar **Talebi geri çek** ile vazgeçebilirsiniz.

Talep nereye gider:

- Distribütöre bağlı bayinin talebi **distribütörün** **Özellikler → Talepler** sekmesine düşer; distribütör sahiplerine bildirim gider.
- Distribütörün ve distribütörü olmayan bayinin talebi **merkeze** gider: **Modüller** sayfasındaki **Talepler** sekmesi; platform yöneticilerine bildirim gider.

Kurallar:

- Çekirdek modül, zaten açık modül veya sistem genelinde kapalı modül talep edilemez.
- Bir organizasyonun bir modül için tek açık talebi olur; yeniden talep etmek mevcut talebi günceller.
- Durumlar: **Bekliyor**, **Onaylandı**, **Reddedildi**, **Geri çekildi**.
- Modül başka bir yoldan açılırsa (admin, distribütör, bayi standardı, modül paketi) açık talep kendiliğinden **Onaylandı** olur ("Otomatik").
- Onay ve ret kararında talep eden kullanıcıya uygulama içi bildirim ve e-posta gider.

### 3.1 Talep kuyruğu (onaylayanlar için)

> **[Ekran görüntüsü: module-requests]** Talepler sekmesi: talep eden, modül, durum, tarih ve karar sütunları, toplu onay.

- Distribütör: **Özellikler → Talepler** (`modules.manage` izni). Onaylanan modül o bayi için açılır. Distribütörün kendisinde kapalı bir modülü onaylamak reddedilir; önce kendisi üst seviyeden talep etmelidir.
- Merkez: **Modüller → Talepler** (`platform.modules.read` görmek, `platform.modules.write` karar için). Onaylanan modül o organizasyon için platform yöneticisi değeriyle açılır.
- Satırdan **Onayla** veya **Reddet**; birden fazla talep seçilip **Seçilenleri onayla** ile toplu onay. Kararla birlikte isteğe bağlı bir not yazılabilir; not listenin **Karar** sütununda görünür.

## 4. İzinler özeti

| İzin | Ne sağlar | Kimde |
| --- | --- | --- |
| `modules.read` | Özellikler sayfasını görmek, talep etmek | Distribütör ve bayi rolleri |
| `modules.manage` | Bayiler ve bayi standardı sekmeleri, talep onayı | Distribütör sahibi |
| `platform.modules.read` / `platform.modules.write` | Modüller sayfası, merkez talep kuyruğu | Platform yöneticisi |

Eklentilerin kendi izinleri aşağıda her eklentinin bölümündedir. Bir ekranı görmek için hem eklentinin açık olması hem de rolünüzde ilgili iznin bulunması gerekir; eklenti kapalıysa menü öğesi gizlenir ve API `FEATURE_DISABLED` döner.

## 5. Bayi vitrini ve lead formu (`dealer_showcase`)

**Ne yapar:** Bayinin herkese açık sayfasını (`/bayi/{kod}`) zenginleştirir: hizmetler, fotoğraf galerisi, çalışma saatleri, Google puanı, teklif formu ve WhatsApp düğmesi. Teklif formundan gelen talep bayide lead olarak açılır.

**Kim açar:** Eklentidir (ücretli). Distribütör bayileri için açar veya bayi talep eder; merkez bağımsız açabilir. Modül kapalıyken public sayfada yalnız temel bilgiler (ad, adres, harita, WhatsApp) kalır.

**Ekranlar:**

> **[Ekran görüntüsü: showcase-editor]** Vitrinim: dil içerikleri, çalışma saatleri, hizmetler, galeri, Google puanı.

- **Vitrinim** (`/t/{organizasyon}/showcase`, bayi ve distribütör):
  - Dil içerikleri: başlık, hakkında metni, SEO anahtar kelimeleri.
  - Çalışma saatleri; "Randevu saatlerinden kopyala" ile hızlı doldurma.
  - SEO ve sosyal bağlantılar.
  - Hizmetler: ekle, gizle, sırala.
  - Fotoğraf galerisi: yükle, açıklama yaz, sırala (varsayılan en fazla 12 fotoğraf, her biri en fazla 5 MB).
  - Google puanı: elle giriş (1,0–5,0) veya Places kimliği.
  - **Önizle**, **Taslağı kaydet**, **Onaya gönder** (onay kapalıysa **Yayınla**).
  - Durumlar: Taslak, Onay bekliyor, Yayında, Reddedildi (ret notu gösterilir).
  - Distribütör "Hedef bayi" seçiciyle bayilerinin vitrinini düzenler.
- **Vitrin onay kuyruğu** (`/platform/showcases`, merkez): İncele, **Onayla**, **Reddet** (ret notu zorunlu).
- **Public bayi sayfası** (`/bayi/{kod}`): "WhatsApp'tan yaz", Hizmetler, Galeri, Google puanı ve **Teklif iste** formu (ad, telefon, e-posta, araç, ilgilenilen hizmetler, tercih edilen iletişim, mesaj, KVKK onayı).

> **[Ekran görüntüsü: showcase-public]** Public bayi sayfası ve teklif formu.

**İzinler:** `showcase.read` (merkez personeli, distribütör sahibi, bayi sahibi ve personeli), `showcase.write` (distribütör sahibi, bayi sahibi), `platform.showcase.review` (merkez personeli, merkez sosyal medya).

**Sık sorulanlar:**

- *Vitrin hemen yayına girer mi?* Merkez onayı varsayılan kapalıdır; gönderince yayına girer. Admin onayı açarsa vitrin "Onay bekliyor" olur, karar bayi sahiplerine bildirilir. Ret durumunda önceki yayın sürümü yayında kalır.
- *Vitrinde fiyat görünür mü?* Hayır. Vitrinde hiçbir fiyat (tavsiye fiyat dahil) gösterilmez.
- *Google puanı nasıl güncellenir?* Sistemde Google Places anahtarı tanımlıysa ve Places kimliği girildiyse günlük otomatik güncellenir; yoksa bayi puanı elle girer.
- *WhatsApp mesajları kime gider?* Merkez numarasına, bayi kodu etiketiyle. Konuşmaları yalnız sistem yöneticisi görür; lead ilgili bayide açılır ve bayiye bildirim gider.

## 6. Filo müşterileri (`fleet`)

**Ne yapar:** Kurumsal filoları (VKN ile) ayrı bir hesap olarak yönetir: araçlar, filo kullanıcıları, toplu hizmet planı, toplu araç kabulü, filo cari ekstresi ve dönemsel PDF raporu. Filo yöneticisi kendi portalını kullanır.

**Kim açar:** Eklentidir (ücretli). Bayi, distribütör veya merkez kullanabilir; zincir kuralları geçerlidir. Filo kendisi ayrı bir organizasyon tipidir, bayi ağacının ve modül zincirinin dışındadır.

**Ekranlar:**

> **[Ekran görüntüsü: fleet-card]** Filo kartı: özet, araçlar, kullanıcılar, hizmetler, hesap ekstresi, raporlar, hizmet planları.

- **Filolar → Filo listesi** (`/t/{organizasyon}/fleets`): **Yeni filo** önce VKN sorar ve filonun zaten olup olmadığını kontrol eder.
- **Filo kartı** (`/t/{organizasyon}/fleets/{filo}`): sekmeler Özet, Araçlar (ekle, içe aktar, filodan çıkar), Kullanıcılar (davet et, pasifleştir), Hizmetler, Hesap ekstresi (açılış, hizmet, tahsilat, kapanış; dışa aktarılabilir), Raporlar, Hizmet planları.
- **Toplu hizmet planı** sihirbazı (`…/plans/new`): Araçlar → Hizmet ve takvim (başlangıç tarihi, günlük en fazla araç, tercih edilen saatler) → Önizleme. Plan detayında iptal ve kabul başlatma.
- **Filo portalı** (filo kullanıcıları için): Filo araçları, Hizmetler, Garantiler, Hesap, Raporlar ve bayilerden gelen bağ isteklerinin onayı.

**İzinler:** `fleets.read`, `fleets.manage` (merkez personeli, distribütör sahibi, bayi sahibi; bayi personeli yalnız okuma), `fleets.plan` (bayi sahibi ve personeli), `fleet.portal.read` (filo rolü, yalnız kendi filosu).

**Sık sorulanlar:**

- *Aynı filo ikinci bir bayiye nasıl bağlanır?* İkinci bayi aynı VKN'yi girince bağ isteği gönderilir; filo kullanıcısı portalda kabul edince bağ kurulur. Talep eden her iki durumda da bildirim alır.
- *Filo kullanıcısı nasıl giriş yapar?* Davet e-postasındaki kodla şifresini belirler. İlk kullanıcı birincil kullanıcıdır ve filonun araçlarının sahibidir.
- *Filo portalında muhasebe ne kadar görünür?* Her bayi için yalnız filo carisi: filoya kesilen hizmetler, tahsilatlar ve bakiye.
- *Dönemsel rapor ne zaman gelir?* Varsayılan aylık; çeyreklik veya kapalı seçilebilir. PDF filo diliyle üretilir ve filo kullanıcılarına e-postayla gider.

## 7. Sertifika (`certificates`)

**Ne yapar:** Personel sertifikalarını (ör. uygulama eğitimi) tutar, geçerlilik süresini izler ve sertifika gerektiren ürün/kategoride sertifikasız personelle yapılan hizmeti işaretler.

**Kim açar:** Eklentidir (ücretli). Sertifika tiplerini merkez tanımlar; sertifikayı bayi/distribütör sahibi kendi personeli için yükler; doğrulamayı yalnız merkez, distribütör ve super_admin yapar.

**Ekranlar:**

> **[Ekran görüntüsü: certificates]** Personel sertifikaları listesi ve doğrulama kuyruğu.

- **Sertifikalar → Personel sertifikaları** (`/t/{organizasyon}/certificates`): **Sertifika yükle** (yalnız PDF, en fazla 10 MB), durum, PDF indirme, iptal, kapsama görünümü.
- **Doğrulama kuyruğu** (`…/certificates/verification`, merkez ve distribütör): doğrula veya gerekçeyle reddet.
- **Sertifika onayları** (`…/certificates/approvals`, merkez): sertifika eksikliği nedeniyle bekleyen hizmet statü değişikliklerini onayla/reddet.
- **Sertifika tipleri** (`/platform/certificate-types`, merkez): geçerlilik süresi ve ürün/kategori bağları.

**İzinler:** `certificate_types.manage` ve `certificates.approve_service` (merkez personeli), `certificates.read` (merkez personeli, distribütör sahibi, bayi sahibi), `certificates.write` (distribütör sahibi, bayi sahibi), `certificates.verify` (merkez personeli, distribütör sahibi).

**Sık sorulanlar:**

- *Sertifikasız personel hizmet yaparsa ne olur?* Hizmete iç uyarı işareti konur. Müşteriye hiçbir bildirim gitmez. Admin "statü değişiminde onay" ayarını açarsa iptal dışındaki statü değişiklikleri merkez onayına kadar bekler (varsayılan kapalı).
- *Süresi dolacak sertifika için uyarı gelir mi?* Bitişten 30 gün önce bildirim gider (admin ayarı). Süre dolunca günlük iş sertifikayı "süresi doldu" yapar.

## 8. Stok tahmini ve sipariş önerisi (`stock_forecast`)

**Ne yapar:** Ürün bazında günlük tüketim hızını, tükenme tarihini ve sipariş önerisini hesaplar. Hesap algoritmiktir (yapay zekâ kullanılmaz).

**Kim açar:** Eklentidir (ücretli). Merkez, distribütör ve bayi kullanabilir.

**Ekranlar:**

> **[Ekran görüntüsü: stock-forecast]** Tahmin listesi, ürün grafiği ve sipariş taslağı.

- **Stok → Tahmin ve öneri** (`/t/{organizasyon}/stock-forecast`): ürün listesi ve arama, ürün grafiği (gerçekleşen / tahmin / eşik), seçilenlerden **Sipariş taslağı oluştur** (miktarlar düzenlenebilir), eşik düzenleme.
- **Ağ talebi** sekmesi (merkez, dışa aktarılabilir) ve **Bayilerim** sekmesi (distribütör).
- Gösterge panelinde **Kritik stok** kartı.

**İzinler:** `stock_forecast.read` (merkez personeli ve depo, distribütör sahibi/personeli/depo, bayi sahibi ve personeli), `stock_forecast.manage` (merkez personeli ve depo, distribütör sahibi, bayi sahibi), `stock_forecast.network.read` (merkez).

**Sık sorulanlar:**

- *Neden "yetersiz veri" görüyorum?* Tahmin için en az 90 günlük hareket gerekir.
- *Eşikler nedir?* Uyarı 14 gün, kritik 7 gün stok kaldığında. Öneri 30 günlük kapsamayı hedefler; eldeki stok ve açık siparişler düşülür.
- *Ne zaman hesaplanır?* Her gece 03:00'te (organizasyon saat dilimi). Bildirim yalnız ürün "normal → uyarı" veya "uyarı → kritik" geçtiğinde gider.

## 9. Performans, hedef ve prim (`performance`)

**Ne yapar:** Aylık performans metrikleri, sıralama ve karşılaştırma, bölge haritası, hedefler ve gerçekleşme, zayıf bayi kuralları; bayi içinde personel hedefi ve hedef bazlı prim.

**Kim açar:** Eklentidir (ücretli). Merkez, distribütör ve bayi kullanabilir. Prim ekranları ayrıca **Bayi muhasebesi** (`dealer_accounting`) modülünü gerektirir.

**Ekranlar** (`/t/{organizasyon}/performance`, menü **Performans**):

> **[Ekran görüntüsü: performance]** Performans paneli, sıralama ve bölge haritası.

- Merkez ve distribütör: **Panel**, **Sıralama** (satırdan "Hedef koy", "Görev aç"; dışa aktarılabilir), **Bölge haritası** (ülke/il/ilçe, bayi noktaları, bayisiz bölgeler), **Kurumsal pano**, **Hedefler** (aylık, çeyreklik, yıllık), **Zayıf bayi kuralları**.
- Bayi: **Panel** ("Ağa göre konumum"; başka bayi adı görünmez), **Sıralama**, **Kurumsal pano**, **Ekip hedefleri** (personel × ay tablosu), **Prim kuralları** (sabit tutar veya hizmet gelirinden pay, gerçekleşme eşiği; **Prim ödeme günü**), **Primler** (onayla, tutarı değiştir — not zorunlu, iptal et, toplu onay).

**İzinler:** `performance.read` (merkez personeli, distribütör sahibi ve personeli, bayi sahibi), `performance.targets.manage` ve `performance.rules.manage` (merkez personeli, distribütör sahibi), `performance.staff_targets.manage` (bayi sahibi), `performance.bonus.manage` (bayi sahibi, bayi muhasebe).

**Sık sorulanlar:**

- *Prim ne zaman ödenir?* Ay sonunda hesaplanır, bayi sahibi onaylar ve bir sonraki ayın prim ödeme gününe (varsayılan ayın 5'i, 1–28 arası ayarlanabilir) planlı personel ödemesi olarak yazılır.
- *Zayıf bayi kuralı ne yapar?* Hedef gerçekleşmesi veya ağ medyanının altında kalma koşuluna uyan bayi için merkez ve distribütör sahiplerine bildirim gider. Otomatik görev yalnız merkez kuralında açılır (bayi × kural × ay için tek görev). Bayiye nötr bir "hedefin altında" bildirimi gider.

## 10. Verimlilik ve fire analizi (`efficiency`)

**Ne yapar:** Tamamlanan hizmetlerde kullanılan film metrajını beklenen tüketimle karşılaştırır; fire oranını bayi, personel, ürün, gövde tipi, parça ve rulo kırılımlarında gösterir.

**Kim açar:** Eklentidir (ücretli). Merkez, distribütör ve bayi kullanabilir. Beklenen tüketim tablosunu merkez yönetir.

**Ekranlar:**

> **[Ekran görüntüsü: efficiency]** Verimlilik özeti, aylık trend ve kırılım sekmeleri.

- **Verimlilik ve fire** (`/t/{organizasyon}/efficiency`): dönem seçici; ortalama fire, uyarı eşiği, toplam metraj, en çok fire veren ürün; Karşılaştırma ve Aylık trend; sekmeler Bayiler, Personel, Ürünler, Gövde tipleri, Parçalar, Rulolar (rulo detayı ve kullanıldığı hizmetler). Dışa aktarılabilir.
- **Beklenen tüketim** (`/platform/part-consumption-expectations`, merkez): ürün/kategori × gövde tipi × parça için beklenen metraj; ekle, düzenle, içe aktar.

**İzinler:** `efficiency.read` (merkez personeli, distribütör sahibi, bayi sahibi), `efficiency.expectations.manage` (merkez personeli).

**Sık sorulanlar:**

- *Fire uyarısı ne zaman çıkar?* Beklenenin %15 üzerinde.
- *Beklenen tüketim tanımlı değilse?* Son 180 günün ağ medyanı kullanılır (en az 20 örnek); daha az örnekte fire hesaplanmaz.
- *Bayi kiminle karşılaştırılır?* Kendi distribütörünün ağıyla.

## 11. Fotoğraf standardı (`photo_standard`)

**Ne yapar:** Araç kabulünde çekilmesi gereken fotoğraf açılarını tanımlar; zorunlu açı eksikken sözleşme oluşturulamaz ve hizmet taslaktan çıkamaz. Fotoğraflar sözleşme PDF'ine gömülür.

**Kim açar:** Eklentidir ama ücretsizdir; platform yöneticisinin açıp kapattığı bir anahtardır. Açıları merkez tanımlar, distribütör kendi ağı için değiştirebilir, bayi kabulde uygular.

**Ekranlar:**

> **[Ekran görüntüsü: intake-photos]** Hizmet sihirbazında kabul fotoğrafları adımı (kamera).

- **Fotoğraf açıları** (`/platform/photo-angles`, merkez): açı seti ve örnek görseller.
- **Fotoğraf standardı** (`/t/{organizasyon}/photo-standard`, distribütör): her açı için Zorunlu, Gizli veya Merkez varsayılanı.
- **Kabul fotoğrafları** adımı (hizmet sihirbazı ve hizmet detayı): **Fotoğraf çek** telefonda kamerayı açar; fotoğraf başına en fazla 12 MB.

**İzinler:** `photo_standard.manage` (merkez personeli), `photo_standard.override` (distribütör sahibi).

**Sık sorulanlar:**

- *Zorunlu açı eksikse ne olur?* Sözleşme oluşturulamaz; hizmet taslaktan çıkamaz ve tamamlanamaz. İptal her zaman mümkündür.
- *Konum bilgisi kimde görünür?* EXIF konum ve cihaz bilgisi yalnız bayi sahibi, merkez ve super_admin tarafından görülür (KVKK). Sözleşme PDF'indeki görsellerde EXIF yoktur.

## 12. E-fatura (UBL-TR) (`e_invoice`)

**Ne yapar:** Merkezin satışları için UBL-TR e-Fatura ve e-Arşiv faturası taslağı oluşturur, doğrular (XSD + Schematron), numaralandırır, XML ve PDF olarak arşivler. Entegratöre gönderim yoktur.

**Kim açar:** Yalnız merkez ve yalnız Türkiye'de kullanılır (TRY). Türkiye dışındaki alıcıya fatura kesilmez.

**Ekranlar** (menü **Muhasebe → e-Fatura**):

> **[Ekran görüntüsü: einvoice]** Fatura listesi, faturalanabilir kayıtlar ve fatura detayı.

- **Fatura listesi** (`/t/{organizasyon}/einvoices`): durumlar Taslak, Doğrulama hatası, Arşivlendi, İptal edildi; CSV indirme.
- **Faturalanabilir kayıtlar** (`…/einvoices/billable`): teslim alınmış distribütör siparişleri ve bayi hizmet kataloğu dönemleri; **Taslak oluştur**. Alıcının fatura profilinde eksik alan varsa listelenir.
- **Fatura detayı**: Önizle, Arşivle, İptal et (gerekçe zorunlu), XML indir, PDF indir, PDF'i yeniden oluştur; doğrulama mesajları ve durum geçmişi.
- **e-Fatura ayarları** (`…/einvoices/settings`): satıcı profili, e-Arşiv ve e-Fatura serileri (3 karakter; numara seri + yıl + 9 hane, ör. EAR2026000000001), sayaçlar, XSLT (GİB varsayılanı veya özel).
- Alıcı fatura profili organizasyon detay sayfasındadır.

**İzinler:** `einvoice.read`, `einvoice.manage` (merkez muhasebe; arşivleme ve iptal kimlik doğrulaması ister), `einvoice.settings` (yalnız super_admin).

**Sık sorulanlar:**

- *Doğrulama hatasında numara harcanır mı?* Hayır. Doğrulamadan geçmeyen fatura arşivlenmez ve numara almaz.
- *İptal ne yapar?* Faturayı iptal edildi olarak işaretler; kaynak kayıt yeniden faturalanabilir. İade faturası bu sürümde yoktur.
- *Varsayılan KDV?* %20. Ayarlar kaydedilmeden taslak ve arşivleme kapalıdır.

## 13. Tavsiye satış fiyatı ve fiyat disiplini

**Ne yapar:** Merkez ürün × ülke × para birimi bazında tavsiye satış fiyatı yayınlar; distribütör ve bayi kendi fiyatlarının yanında tavsiye fiyatı ve sapmayı görür. Her yayından sonra fiyat listesi PDF'i doküman merkezine eklenir.

**Kim açar:** Ayrı bir eklenti değildir; katalog çekirdeğinde izinle çalışır. Fiyat listesi PDF'i doküman merkezi (`announcements` modülü) açıksa yayınlanır.

**Ekranlar:**

> **[Ekran görüntüsü: recommended-prices]** Tavsiye fiyatlar: fiyat listesi, yayın sepeti, sürüm geçmişi.

- **Tavsiye fiyatlar** (`/t/{organizasyon}/catalog/recommended-prices`, merkez): Fiyat listesi ve Sürüm geçmişi sekmeleri, yüzdeyle toplu değişiklik, **Yayın sepeti** → yürürlük tarihiyle **Yayınla** (kimlik doğrulaması ister), CSV dışa/içe aktarma.
- **Fiyat disiplini** (`…/catalog/price-discipline`, merkez ve distribütör): sapma listesi ve "Ülkelere göre ortalama sapma" grafiği.
- Distribütör ve bayide: **Satış fiyatlarım** ve katalog fiyat tablosunda "Tavsiye fiyat" sütunu ve sapma rozeti.
- **Dokümanlar → Fiyat listeleri** klasörü: ülke/para birimi başına fiyat listesi PDF'i.

**İzinler:** `pricing.recommended.read` (merkez personeli ve muhasebe, distribütör sahibi ve muhasebe, bayi sahibi ve muhasebe), `pricing.recommended.write` (yayın; yalnız merkez muhasebe), `pricing.discipline.read` (merkez personeli, distribütör sahibi).

**Sık sorulanlar:**

- *Sapma eşiği nedir?* ±%15. Eşik aşımları tek tek değil, haftalık özetle (pazartesi) merkeze ve her distribütöre kendi ağı için bildirilir.
- *Gelecek tarihli fiyat ne zaman görünür?* Yürürlük gününde geçerli olur; sürümler üzerine yazılmaz.
- *Tavsiye fiyat vitrinde görünür mü?* Hayır, yalnız distribütör ve bayi panelinde.

## 14. Genel sık sorulanlar

- *Bir eklentiyi göremiyorum.* Distribütörünüzde kapalı olabilir ("Üst seviyede kapalı") ya da sistem genelinde kapalıdır. Özellikler sayfasından talep edin; distribütörünüzde de kapalıysa distribütörün önce merkezden talep etmesi gerekir.
- *Eklenti açık ama menüde yok.* Rolünüzde ilgili izin yoktur; organizasyon sahibinize başvurun.
- *Modül paketi aboneliği bitti; verilerim silinir mi?* Hayır. Modül kapanır, veriler korunur; yeniden açıldığında geri gelir.
- *Bu kılavuz nerede güncellenir?* Kılavuz yeni sürüm yayınlandığında doküman merkezindeki aynı kayda yeni sürüm olarak eklenir; içerik değişmediyse yeni sürüm açılmaz.
