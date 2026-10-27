# Demo kılavuzu: merkez → bayi sipariş akışı (3 telefon)

Merkez **Olex Merkez**, hizmet noktası ve depo **AFM Florya Merkez**. Üç kişi, üç telefon:

| Telefon | Kim | Rol | Uygulamada açılan alan |
|---|---|---|---|
| 1 | Zorbey | `center_owner` (Olex Merkez sahibi) | Bayi çalışma alanı + Menü'de depo/admin ekranları; web panel |
| 2 | Depo çalışanı | `center_warehouse`, AFM Florya Merkez deposunun üyesi | Depo (İşler, Tara, Stok) |
| 3 | Bayi sahibi | `dealer_owner` (yeni bayi) | Bayi |

Bu senaryonun tamamını backend'de `TestIntegrationDemoOrderFlow` testi koşuyor
(`backend/internal/httpserver/demo_order_flow_integration_test.go`).

## 0. Demo öncesi kontrol (bir gün önce)

- [ ] Sunucuda bu PR'ın migration'ı uygulanmış olmalı (`000165_center_owner_accounts`: center_owner'a
      `members.write` + `organizations.write`). Zorbey'in hesap açma menüsü buna bağlı.
- [ ] **Worker çalışıyor** (`cmd/worker`, `QUEUE_ENABLED=true`, `WORKER_QUEUES` boşsa ya da `notifications`
      içeriyorsa). Bildirimler outbox → worker → in-app/push yolundan gider; worker durursa push gelmez.
- [ ] **WhatsApp (wuzapi) bağlı**: telefonla giriş kodu WhatsApp'tan gelir. Platform > Entegrasyonlar > WhatsApp
      durumu "bağlı" olmalı.
- [ ] Erişim politikası personel için TOTP **zorunlu değil** (zorunluysa yeni hesap `MFA_NOT_ENROLLED` alır).
- [ ] **Mobil uygulama push destekli sürüm**: bu PR `expo-notifications` native modülünü ekliyor; main'e merge
      sonrası CI yeni bir TestFlight build'i başlatır (OTA değil). Üç telefona **yeni TestFlight sürümü** kurulmalı.
- [ ] **iOS push anahtarı**: EAS'ta iOS Push Notifications anahtarı tanımlı olmalı
      (`cd mobile && npx eas-cli@latest credentials -p ios` → Push Notifications: anahtar oluştur/yükle).
      Yoksa token kaydolur ama Apple bildirimi teslim etmez.
- [ ] AFM Florya Merkez deposu **birincil depo** (Depo > Konumlar'da "birincil"); sipariş buraya düşer.
- [ ] Depoda en az bir ürünün **barkodlu birimi** var (Stok girişi ile), ürün kataloğda ve bayiye fiyatı tanımlı
      (fiyatsız ürün de eklenebilir; satır 0 TL olur ve muhasebeye 0 yazılır, demo için fiyatlı ürün seçin).
- [ ] Teslimat yöntemlerinde en az biri var (Ayarlar > Teslimat yöntemleri, ör. "Aras Kargo"); yoksa depo çalışanı
      sevk ekranında yöntem seçmeden de sevk edebilir.
- [ ] Demo telefon numaraları sistemde **başka bir kullanıcıya ait değil** (aitse o kullanıcı bağlanır, ekranda
      "bu numara zaten bir kullanıcıya aitti" yazar).

## 1. Hesapları açma (Zorbey, web panel)

Zorbey web panelde Olex Merkez'e girer: **Ayarlar > Hesaplar**.

1. **Personel ekle** (`/t/<merkez>/settings/team/new`)
   - Ad, soyad, **telefon** (+90 5xx…), e-posta isteğe bağlı.
   - Rol: **Merkez depo** (`center_warehouse`). Depolar: **AFM Florya Merkez** işaretli (birincil önseçili).
   - "Personeli ekle" → Beklenen: "Personel eklendi" kartı; telefon ve rol görünür.
2. **Yeni bayi** (`/t/<merkez>/settings/dealers/new`)
   - Bayi adı, şehir; bayi sahibi ad, soyad, **telefon**.
   - "Bayiyi oluştur" → Beklenen: "Bayi ve bayi sahibi oluşturuldu". Bayi **aktif**, doğrudan Olex Merkez'e bağlı,
     deneme süresi yok; hemen sipariş alabilir.

API karşılıkları: `POST /v1/tenant/members`, `POST /v1/tenant/dealers`.

## 2. Telefonla giriş (telefon 2 ve 3)

1. Uygulamayı açın → giriş kutusuna **telefon numarası** → WhatsApp'a gelen 6 haneli kod.
2. Beklenen: depo çalışanı doğrudan **Depo** alanında (İşler sekmesi), bayi sahibi **Bayi** alanında açılır.
3. İlk girişte **bildirim izni** sorulur → **İzin ver**. (İzin verilmezse push gelmez; Ayarlar'dan açılabilir.)
   Uygulama push token'ı otomatik kaydeder (`PUT /v1/mobile/push-token`).
4. Zorbey de telefon 1'de aynı şekilde girer (bildirim izni verir), "Teslim alındı" push'u ona gelir.

## 3. Sipariş açma (Zorbey, web ya da mobil)

- **Web**: Siparişler > **Yeni sipariş** (`/t/<merkez>/orders/new`) → "Bayi adına yeni sipariş": bayiyi seçin, ürün ekleyin,
  depo **AFM Florya Merkez** (önseçili) → oluştur.
- **Mobil**: Menü > Katalog → ürünleri sepete ekleyin → sepet → bayiyi seçin → oluştur.

Beklenen:
- Sipariş **Onaylandı** durumunda başlar (bayi onayı beklenmez), sipariş deposu AFM Florya Merkez.
- **Telefon 2 (depo)**: "Yeni sipariş hazırlanacak: ORD-…" push'u gelir; dokununca siparişin **hazırlama ekranı** açılır.
  İşler sekmesi > Siparişler'de de görünür.
- **Telefon 3 (bayi)**: "Merkez sizin için sipariş açtı: ORD-…" push'u (ve in-app bildirim).
- Siparişi açan Zorbey kendine bildirim almaz.

## 4. Hazırlama ve sevk (telefon 2, depo çalışanı)

Siparişin hazırlama ekranında (İşler > Siparişler > sipariş):

1. **Hazırlamaya başla** → durum "Hazırlanıyor".
2. Satırda **Okutarak ata** → ürünün barkodunu okutun → satır "tamam" olur (öneri/konum listesi bu deponun stoğunu gösterir).
3. **Hazır işaretle** (tüm satırlar dolunca etkin) → "Hazır".
4. **Sevk et** → teslimat yöntemi (ör. Aras Kargo) ve **takip no** → onayla → "Sevk edildi".

Beklenen: **telefon 3 (bayi)**'ye "Siparişiniz yola çıktı: ORD-…" push'u; bildirimin içinde teslimat yöntemi ve takip no
sipariş detayında görünür. Depo çalışanı (işlemi yapan) bildirim almaz.

İsteğe bağlı: elden/depodan teslimde depo ya da Zorbey **Teslim edildi** der → bayiye "Siparişiniz teslim edildi" push'u.
Teslim edildi yalnız durum ve zamandır; **stok ve muhasebe bayi "Teslim al" deyince işler**.

## 5. Teslim alma (telefon 3, bayi sahibi)

Bayi alanı: Menü > Siparişler > sipariş (ya da push'a dokunun) → **Teslim aldım** → onay.

Beklenen:
- Durum **Alındı**; barkodlu birim bayinin stoğunda "kullanılabilir".
- **Telefon 1 (Zorbey)**: "Sipariş teslim alındı: ORD-…" push'u.
- **Muhasebe** (aynı işlemde): merkezde bayinin carisine **satış (gelir/alacak)**, bayide merkezin carisine
  **alış (gider/borç)** satırı, kaynak = sipariş. Kur onay anında dondurulan kurdur.

## 6. Muhasebede kontrol (Zorbey, web)

Muhasebe > Cari hesaplar > yeni bayinin carisi → **Ekstre** (`/t/<merkez>/accounting/cari/<uuid>/statement`):
siparişin satırı (ORD-…, tutar) görünür, bakiye sipariş tutarı kadar borç. Bayi de kendi panelinde merkez carisinde
aynı tutarı borç olarak görür.

## Sorun giderme

| Belirti | Olası neden |
|---|---|
| Kod gelmiyor | WhatsApp bağlantısı koptu; Platform > WhatsApp. Numara E.164 mü? |
| Girişte müşteri alanı açıldı | Numara personel hesabına kayıtlı değil (hesap açılırken farklı numara girildi); bilinmeyen numara müşteri hesabı açar. |
| Push gelmiyor, in-app bildirim var | Bildirim izni verilmedi; eski uygulama sürümü (push modülü yok); iOS push anahtarı EAS'ta yok; worker'da `EXPO_ACCESS_TOKEN` gerekiyorsa (Expo "enhanced security" açık) tanımlı değil. |
| In-app bildirim de yok | Worker / outbox yayımcısı çalışmıyor. |
| Depo çalışanı siparişi görmüyor | Sipariş başka bir depoya düştü (birincil depo farklı) ya da çalışan o deponun üyesi değil. |
| "Hazır işaretle" pasif | Tüm satırlara birim atanmadı. |
| Muhasebede satır yok | Sipariş "Teslim edildi"de kaldı; bayi "Teslim aldım" demeli. Sipariş tutarı 0 ise satır yazılmaz. |
