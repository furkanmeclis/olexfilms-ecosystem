# Mobil: premium görsel dil, ikon sistemi, müşteri tanıtımı ve güçlü giriş

Tarih: 2026-10-17 · Kapsam: `mobile/` (Expo SDK 58, expo-router, uniwind) · Durum: onay bekliyor

## 1. Niyet ve neden

Mobil uygulama (tek app: bayi, depo, admin, müşteri) işlevsel ama görsel olarak "çıplak" ve yer yer web paneli gibi duruyor. Kullanıcı (ürün sahibi) uygulamanın **premium** görünmesini istiyor; referansı **Apple'ın kendi sistemi**: iOS'ta yerli bir Apple uygulaması gibi sakin, ciddi, tutarlı. "Oyuncaklaştırmadan": renkli/sevimli illüstrasyon, zıplayan animasyon, konfeti yok.

Sonra sırada kalan "Yakında" ekranlarının yapımı var; bu belge yalnızca onların üzerine kurulacağı görsel temeli ve giriş deneyimini tanımlar.

## 2. Bugünkü durum (2026-10-17 taraması)

- İkonlar tamamen Lucide (`src/ui/icons.ts`, ~50 ikon, `strokeWidth=2`, web paneliyle aynı set). `<Icon as={X} />` ~80 yerde, `icon={X}` prop'u (EmptyState vb.) ~48 yerde kullanılıyor.
- `expo-symbols` (SF Symbols), `expo-glass-effect` ve `expo-local-authentication` bağımlılıklarda var, kodda hiç kullanılmıyor.
- İkonsuz ekranlar: 4 giriş ekranı (`features/auth/*-screen.tsx`), menü (`features/menu/menu-screen.tsx`, düz metin satırları), `features/warehouse/entry-new-screen.tsx`.
- 46 `<Input>` kullanımının yalnızca 4'ünde `placeholder` var.
- Giriş: `(public)/index` → `IdentifyScreen` (logo + alt yazı + tek alan "e-posta veya telefon" + Devam). E-posta → `/password` (+ gerekirse `/totp`), telefon → WhatsApp kodu → `/phone`. Dev'de `DevDemoAccounts` (yalnızca `EXPO_PUBLIC_DEMO_PASSWORD` varsa).
- Renk: açık/koyu tema, birincil koyu yeşil `#003d25`, marka altın `#f2ac4a` (`src/global.css`). Fontlar: Plus Jakarta Sans, Outfit, JetBrains Mono.
- i18n: 13 dil; tr/en anahtar anahtar eşit olmak zorunda (`npm run i18n:check`), diğer diller en'e düşer.

## 3. Hedefler

1. **İkon sistemi (anlamsal):** Her kavramın tek bir ikonu olur ve her yerde aynı ikon kullanılır (müşteri, araç, hizmet, garanti, sipariş, stok, depo, tarama, rapor, ayarlar…). iOS'ta **SF Symbols**, Android'de ve SF karşılığı olmayan yerde **Lucide** ile çizilir.
2. **Eksik ikonların doldurulması:** Menü satırları, giriş ekranları, depo yeni giriş ekranı ve boş durumlar ikon alır.
3. **Placeholder'lar:** Tüm form alanlarında yol gösteren örnek metin (örn. `ornek@firma.com`, `+90 5XX XXX XX XX`, `34 ABC 123`), i18n anahtarıyla.
4. **Müşteri tanıtımı (onboarding):** İlk açılışta, girişten önce, müşteriye yönelik 3 sayfalık animasyonlu tanıtım (bkz. §5).
5. **Güçlendirilmiş giriş:** Tanıtımın son sayfası müşteri ve personel girişini ayırır; giriş ekranları premium görünüm alır (bkz. §6).

## 4. Hedef olmayanlar ve anti-hedefler

- **Platform önceliği (ürün sahibi kararı):** Öncelik kesinlikle iOS. Android ayrıca cilalanmaz ama "kötü görünmemeli": Lucide ikonlar, cam yerine düz kart yüzeyi, aynı boşluk/tipografi; bir kez Android emülatör/cihazda göz kontrolü yapılır.
- **Hedef değil:** "Yakında" ekranlarını yapmak (ayrı proje), renk paletini değiştirmek, web panelinin ikonlarına dokunmak, Android'e özel tasarım dili (Material You) çalışması, garanti kartı için Wallet (design.md'de ertelendi).
- **Anti-hedef (teknik olarak çalışsa da başarısızlık sayılır):**
  - Sevimli/renkli illüstrasyonlar, maskot, çizgi film hissi, zıplayan/elastik animasyonlar, konfeti.
  - Aynı kavram için farklı ekranlarda farklı ikon.
  - Personelin her açılışta tanıtımdan geçmek zorunda kalması.
  - Tanıtımda doğrulanmamış iddia: garanti süresi/kapsamı için **sayı yazılmaz** ("10 yıl" gibi); ürün sahibi rakam vermeden metne rakam girmez.
  - Erişilebilirliği bozan hareket: iOS "Hareketi Azalt" açıksa animasyonlar sade geçişe döner.

## 5. Müşteri tanıtımı (onboarding)

**Kime:** Yalnızca müşteriye; Olexfilms'i tanıyan ya da hiç tanımayan müşteri. Personel için değil (tek dokunuşla geçer).

**Ne anlatır (ürün sahibinin sözüyle):** "Ne yaptığımız, ne güvence verdiğimiz."

**Ne zaman:** Uygulamanın ilk açılışında, oturum yokken, giriş ekranından önce. Görüldü bilgisi cihazda saklanır (`core/storage` / `preferences` kalıbı); bir kez tamamlanan ya da geçilen tanıtım bir daha gösterilmez. Ayarlar'dan yeniden izlenebilir (benim önerim).

**Ayarlar'dan yeniden izleme:** Oturum açıkken izlenen tanıtımın son sayfasında giriş düğmeleri yerine tek "Tamam" düğmesi olur. Aynı ekran bileşeni iki modda çalışır. (Benim kararım.)

**Sayfalar (3; sayı benim önerim, içerik onaylandı):**
1. **Ne yapıyoruz:** Boya koruma filmi (PPF) ve cam filmi; aracın yüzeyini koruyan şeffaf katman. Görsel: araca filmin "yerleştiği" sakin bir animasyon.
2. **Kim uyguluyor:** Yalnızca yetkili Olexfilms bayileri; ölçümle başlayan, araca özel uygulama.
3. **Güvence:** Dijital garanti. Aracınıza, uygulanan parçalara ve süresine bağlı kayıtlı; QR ile doğrulanır; araç satılınca devredilebilir; kâğıt kart kaybolsa da telefonunuzda.

**Son sayfa eylemleri:** Birincil büyük düğme "Telefon numaramla devam et" (müşteri: WhatsApp kodlu mevcut telefon akışı) ve ikincil küçük bağlantı "Personel girişi" (e-posta + şifre akışı). Her sayfada sağ üstte "Geç" (doğrudan son sayfadaki seçime ya da giriş ekranına).

**Animasyon tonu:** Premium ve sakin. Yavaş yumuşak geçişler (ease-out, ~300–600 ms), hafif paralaks, sayfa göstergesi, cam (`expo-glass-effect`) yüzeyler ve SF Symbols. `react-native-reanimated` 4 zaten projede. "Hareketi Azalt" saygısı zorunlu.

**Metinler:** Tüm metinler i18n anahtarı; tr ve en tam, diğer diller en'e düşer.

## 6. Güçlendirilmiş giriş

- Tanıtım sonrası müşteri doğrudan **telefon** girişine, personel **e-posta** girişine yönlenir; böylece tek "e-posta veya telefon" alanının belirsizliği kalkar.
- Backend bu ayrımı zorlamaz (bugün her kullanıcı her iki yolla da girebilir; filo/kurumsal müşteri e-postayla, personel telefonla girebilir). Bu yüzden **her iki ekranda da öbür yola geçiş bağlantısı** bulunur ("E-posta ile giriş yap" / "Telefonla giriş yap"); kimse takılı kalmaz. (Benim kararım.)
- Giriş ekranları: SF Symbols ikonlu alanlar, anlamlı placeholder'lar, doğru klavye tipleri (telefon için `phone-pad`), premium başlık/logo yerleşimi, hata mesajları alan altında.
- **Telefon alanında ülke seçici** (ürün sahibi kararı): bayrak + ülke kodu seçimi, varsayılan Türkiye (+90), aranabilir liste; numara `libphonenumber-js` ile seçili ülkeye göre doğrulanır ve E.164'e çevrilir. Ülke adları i18n'den ya da cihaz yereline göre (`Intl.DisplayNames`) gelir. Yeni native bağımlılık eklenmez.
- Mevcut kimlik doğrulama akışları ve API'leri **değişmez** (identify, password, totp, phone OTP).
- `DevDemoAccounts` dev'de kalır, prod'da görünmez (bugünkü davranış).

## 7. İkon sistemi ayrıntısı

- Tek giriş noktası: `src/ui/icons.ts` / `src/ui/icon.tsx`. Kavram adı → `{ sf: '<SF Symbol adı>', lucide: <LucideIcon> }` eşlemesi. Çağrı biçimi kurucunun kararı, ama mevcut ~128 kullanım yeni sisteme taşınmalı (kademeli geçiş kabul, sonunda tek yol).
- iOS'ta SF Symbols ağırlığı ve boyutu metinle uyumlu; Lucide `strokeWidth` iOS dışında sakin bir değere (≈1.75) çekilebilir (kurucu kararı).
- Renkler mevcut tema token'larından gelir (`tone`), sabit renk yok.
- SF Symbol adları SDK 58 `expo-symbols` sürümünün desteklediği adlardan seçilir; kurucu versiyonlu Expo belgelerine bakar (mobile/AGENTS.md kuralı).

## 8. Kısıtlar

- Expo SDK 58; `npx expo install` ile paket ekleme; `ios/` ve `android/` elle düzenlenmez (CNG).
- Yeni native bağımlılık gerekirse development build yeniden gerekir (dev client yok; bkz. kullanıcı belleği notları). Mevcut kurulu paketler tercih edilir.
- CI: `mobile` işi (lint, typecheck, i18n:check, api-generate) yeşil olmalı.
- Commit: `TEC-<no>: ...`, PR'da kabul kriterleri; auto-merge yalnızca CI yeşilse.
- Prod DB'ye yazma yok; bu iş backend değişikliği gerektirmez.

## 9. Ürün sahibinin verdiği kararlar

- Görsel dil: premium, Apple sistemine yakın, oyuncak değil.
- Eksik ikonlar ve placeholder'lar doldurulacak.
- Telefon girişinde ülke seçici olacak.
- Öncelik iOS; Android kötü görünmemeli.
- Animasyonlu tanıtım yapılacak; yalnızca müşteriye; tanıyan ve tanımayan müşteriye hitap edecek; ana mesaj "ne yapıyoruz, ne güvence veriyoruz".
- §3 ve §5'teki tarifler "eksik yok" ile onaylandı.

## 10. Benim kararlarım (ürün sahibi açıkça söylemedi)

- iOS'ta SF Symbols, Android'de Lucide.
- Renk paleti korunur.
- Tanıtım 3 sayfa; Ayarlar'dan yeniden izlenebilir.
- Son sayfada müşteri/personel girişinin ayrılması.
- Animasyon süreleri ve "Hareketi Azalt" desteği.
- Face ID ile hızlı giriş bu turda **yok** (kapsamı dar tutmak için); sonraki iş olarak önerilecek.

## 11. Kurucuya bırakılanlar

- Kavram → SF/Lucide eşleme listesinin tamamı ve `Icon` API'sinin son biçimi.
- Tanıtım animasyonunun somut çizimi (SF Symbols + şekil/gradient katmanlarıyla; harici görsel/Lottie eklemek gerekirse önce sorulur).
- Placeholder metinlerinin tamamı (tr + en).
- Tanıtım metinlerini kurucu yazar (tr + en, §4'teki "rakam yok" kuralına uyarak); ürün sahibi PR'daki ekran görüntülerinden onaylar, onaysız merge edilmez. (Benim kararım.)
- İşin PR'lara bölünmesi (öneri: 1) ikon sistemi + eksik ikonlar + placeholder'lar, 2) tanıtım + giriş).

## 12. Kabul

- iOS'ta tüm ikonlar SF Symbols ile, Android'de Lucide ile çiziliyor; ikonsuz liste satırı/giriş ekranı kalmadı.
- Her `<Input>` anlamlı placeholder taşıyor (tr + en).
- Temiz kurulumda ilk açılış tanıtımı gösteriyor; "Geç" ve son sayfa düğmeleri doğru akışa götürüyor; ikinci açılışta tanıtım yok.
- "Hareketi Azalt" açıkken animasyonlar sade.
- Fiziksel iPhone'da development build ile ekran görüntüleri/kısa video ile doğrulandı. (Ürün sahibinin iPhone 16 Pro'su USB ile bağlı, imzalama Team `N5T7KH96U3`, demo API ile 2026-10-17'de kurulum yapıldı; Metro LAN üzerinden, Yerel Ağ izni gerekli.)
- CI `mobile` işi yeşil.
