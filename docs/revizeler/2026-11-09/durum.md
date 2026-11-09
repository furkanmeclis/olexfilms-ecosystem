# Revize 09.11 — durum takibi

Kaynak: [huseyin-revize.md](huseyin-revize.md). Linear etiketi: `revize-0911` (proje "Olexfilms Tek App").
Koordinatör gece çalışması, 2026-11-09. PR'lar merge EDİLMEDİ; merge sırası en altta.

Durum kısaltmaları: ⏳ çalışıyor · 🟢 CI yeşil · 🔴 CI kırık · ⏸ sırada · 🟡 karar bekliyor

## Migration numaraları (merkezi dağıtım)

| No | İş | Not |
| --- | --- | --- |
| 000166 | TEC-1047 / TEC-1052 | KULLANILMADI (boşluk) |
| 000167 | TEC-1050 | yalnız gerekirse |
| 000168 | TEC-1052 | KULLANILMADI (boşluk) |
| 000169 | TEC-1062 | bekleme havuzu |
| 000170 | TEC-1054 / TEC-1060 | yalnız gerekirse |

Not: golang-migrate boşluklara izin verir; ancak PR'lar migration numarası **artan sırayla** merge edilmeli (yüksek numara önce uygulanırsa düşük numara prod'da atlanır).

## 1. Hata ve kritikler

| Madde | Linear | PR | CI | Migration | Durum / not |
| --- | --- | --- | --- | --- | --- |
| 7.5 Web+API sabit barkodlu ürün siparişe eklenemiyor | TEC-1047 | [#508](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/508) | 🟢 | — | kök neden: hazırlamada adetsiz okutma 400, tekrar okutma 409, öneri yalnız konumlu adet; üçü düzeltildi |
| 5.2 + 5.10 Web+API depo seçimi, bildirim, aramasız ürün listesi, KALEMLER | TEC-1052 | [#510](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/510) | 🟢 | — | #508 üstünde; `self_prepare` ("ben hazırlayacağım") eklendi; ürün popülerlik sıralaması üründe yok, ada göre |
| 5.4 Web+API depo fiyat görmesin | TEC-1050 | | | | ⏳ |
| 7 Mobil depo değişince birimler | TEC-1049 | [#509](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/509) | 🟢 | — | 🟢 kök neden: depo seçimi hiçbir listeye gitmiyordu; sipariş listesi depo filtresi backend'de yok → TEC-1054 |
| 7.5 Mobil | TEC-1048 | [#509](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/509) | 🟢 | — | 🟢 mobil UI engeli yok; depo hazırlamada adet/SKU okutma düzeltildi |
| 5.2.1 Mobil depo seçimi | TEC-1053 | [#509](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/509) | 🟢 | — | 🟢 depo seçimi eklendi; "kendim hazırlayacağım" API'si TEC-1052'den sonra |
| 5.4 Mobil fiyat gizleme | TEC-1051 | [#509](https://github.com/furkanmeclis/olexfilms-ecosystem/pull/509) | 🟢 | — | 🟢 depo alanı/rolünde fiyat gizli |

## 2. Sipariş akışı

| Madde | Linear | PR | CI | Migration | Durum |
| --- | --- | --- | --- | --- | --- |
| 5 Web+API sipariş listesi alanları | TEC-1054 | | | 000170? | ⏳ #510 üstünde |
| 5 + 5.1 Mobil | TEC-1055 | | | | ⏸ |
| 5.5 + 5.9 Web+API sipariş düzenleme | TEC-1056 | | | | ⏸ |
| 5.5 + 5.9 Mobil | TEC-1057 | | | | ⏸ |
| 5.6 Web+API plotter işareti + rapor | TEC-1058 | | | | ⏸ |
| 5.6 + 5.4.1 Mobil | TEC-1059 | | | | ⏸ |
| 5.4.1 Web+API hareket geçmişi | TEC-1060 | | | 000170? | ⏳ TEC-1054 üstünde |
| 5.7 Web+API siparişe açık ürün | TEC-1061 | | | | ⏸ |
| KRİTİK Web+API yeni teslim yeri / bekleme havuzu | TEC-1062 | | | 000169 | ⏳ |
| KRİTİK Mobil | TEC-1063 | | | | ⏸ |

## 3. Fiyatlandırma

| Madde | Linear | PR | CI | Migration | Durum |
| --- | --- | --- | --- | --- | --- |
| 5.3 Web+API USD, iskonto, hediye | TEC-1064 | | | | ⏸ |
| 5.3 Mobil | TEC-1065 | | | | ⏸ |

## 4. Rol ekranları

| Madde | Linear | PR | CI | Migration | Durum |
| --- | --- | --- | --- | --- | --- |
| 7.1 Web depo ana sayfası | TEC-1066 | | | | ⏸ |
| 7.2 + 7.3 Mobil depo ana sayfa + kart | TEC-1067 | | | | ⏸ |
| 5.8 Mobil satış görevlisi | TEC-1068 | | | | ⏸ |
| 6 Mobil stok depo filtresi | TEC-1069 | | | | ⏸ |
| 8 + 8.1 Web+API muhasebe | TEC-1070 | | | | ⏸ |
| 8.1 Mobil muhasebe kuyruğu | TEC-1071 | | | | ⏸ |

## 5. Merkez ekranları

| Madde | Linear | PR | CI | Migration | Durum |
| --- | --- | --- | --- | --- | --- |
| 1 Web+API genel bakış | TEC-1072 | | | | ⏳ |
| 1.1 + 3.1 Mobil | TEC-1073 | | | | ⏸ |
| 2 + 4 Web menü, Müşteriler ve Araçlar | TEC-1074 | | | | ⏳ |
| 3 Web+API hizmet listesi | TEC-1075 | | | | ⏳ |

## 6. Hizmet oluşturma

| Madde | Linear | PR | CI | Migration | Durum |
| --- | --- | --- | --- | --- | --- |
| 9 + 9.1 + 9.2 Web sihirbaz | TEC-1076 | | | | ⏸ |
| 9.3 Mobil gövde tekrar seçimi | TEC-1077 | | | | ⏸ |

## 7. Metrajlı çıkış

| Madde | Linear | PR | CI | Migration | Durum |
| --- | --- | --- | --- | --- | --- |
| Metraj Web+API `KOD-3M` | TEC-1078 | | | | ⏸ |
| Metraj Mobil | TEC-1079 | | | | ⏸ |

## Karar gerektirenler (kodlama yok)

| Madde | Linear | Durum |
| --- | --- | --- |
| Bayi/Distribütör, Olex Merkez = AFM Florya, Florya'dan AFM Florya'ya sipariş | TEC-1080 | 🟡 analiz Linear yorumunda, karar bekliyor |
| 5.7 distribütör bazlı | TEC-1081 | 🟡 analiz Linear yorumunda, karar bekliyor |
| 9.2 sözleşme ek fotoğraf | TEC-1082 | 🟡 analiz Linear yorumunda, karar bekliyor |
| 9.3? mobil araç kabul / sözleşme | TEC-1083 | 🟡 analiz Linear yorumunda, karar bekliyor |
| Kodların kredili kullanımı | TEC-1084 | 🟡 analiz Linear yorumunda, karar bekliyor |
| İadelerin oluşturulması (kapsam) | TEC-1085 | 🟡 analiz Linear yorumunda, karar bekliyor |
| 5.1 dipnot metrajlı sipariş yolu | TEC-1086 | 🟡 analiz Linear yorumunda, karar bekliyor |
| 7.4 mobil Yeni Giriş (ertelendi) | TEC-1087 | 🟡 analiz Linear yorumunda, karar bekliyor |
| Kapatılabilecek modüller (sunucuda hiçbir şey kapatılmadı) | TEC-1088 | 🟡 analiz Linear yorumunda, karar bekliyor |

## Açık sorular

- **K0 (TEC-1080):** AFM Florya ayrı bayi mi olsun? Öneri 1b (yeni bayi org'u aç, eski veri merkezde kalsın) veya 1a (org-merge undo). Kök neden: AFM Florya TEC-1003 ile merkeze birleşti; sipariş alıcısı olarak seçilebilecek org yok, alıcı=satıcı=merkez olunca cari oluşmuyor.
- **Metraj (TEC-1078):** ondalık metre `2.5M` mi `250CM` mi? Aynı metrede ikinci kesim `-3M-2` uygun mu? (Varsayım: 2.5M ve -3M-2.)
- **Modüller (TEC-1088):** hangi modüller geçici kapatılsın? Liste Linear'da; sunucuda hiçbir şey kapatılmadı.
- **İade (TEC-1085):** merkezin bayi adına iade açması + iadenin siparişe bağlanması açılıştan önce mi?
- **Kredi (TEC-1084):** "kredili kod" = veresiye kod kullanımı mı, kredi paketi mi?
- **9.3? (TEC-1083):** mobilde sözleşme — öneri: açılışta sonradan tamamlama, sonra tam adım.
- **5.7 dist. (TEC-1081):** distribütör bazlı görünürlük ilk geçişte gerekli mi?
- **7.4 (TEC-1087):** mobil "Yeni giriş" kısayolu yalnız merkez yöneticisine mi gösterilsin?

## Önerilen merge sırası

(Sabah raporunda kesinleşecek.)
