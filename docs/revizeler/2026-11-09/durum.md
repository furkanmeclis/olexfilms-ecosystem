# Revize 09.11 — durum takibi

Kaynak: [huseyin-revize.md](huseyin-revize.md). Linear etiketi: `revize-0911` (proje "Olexfilms Tek App").
Koordinatör gece çalışması, 2026-11-09. PR'lar merge EDİLMEDİ; merge sırası en altta.

Durum kısaltmaları: ⏳ çalışıyor · 🟢 CI yeşil · 🔴 CI kırık · ⏸ sırada · 🟡 karar bekliyor

## Migration numaraları (merkezi dağıtım)

| No | İş | Not |
| --- | --- | --- |
| 000166 | TEC-1047 / TEC-1052 | yalnız gerekirse |
| 000167 | TEC-1050 | yalnız gerekirse |
| 000168 | TEC-1052 | 000166'yı TEC-1047 kullanırsa |

## 1. Hata ve kritikler

| Madde | Linear | PR | CI | Migration | Durum / not |
| --- | --- | --- | --- | --- | --- |
| 7.5 Web+API sabit barkodlu ürün siparişe eklenemiyor | TEC-1047 | | | | ⏳ |
| 5.2 + 5.10 Web+API depo seçimi, bildirim, aramasız ürün listesi, KALEMLER | TEC-1052 | | | | ⏳ (TEC-1047 dalı üzerine) |
| 5.4 Web+API depo fiyat görmesin | TEC-1050 | | | | ⏳ |
| 7 Mobil depo değişince birimler | TEC-1049 | | | — | ⏳ mobil paket 1 |
| 7.5 Mobil | TEC-1048 | | | — | ⏳ mobil paket 1 |
| 5.2.1 Mobil depo seçimi | TEC-1053 | | | — | ⏳ mobil paket 1 |
| 5.4 Mobil fiyat gizleme | TEC-1051 | | | — | ⏳ mobil paket 1 |

## 2. Sipariş akışı

| Madde | Linear | PR | CI | Migration | Durum |
| --- | --- | --- | --- | --- | --- |
| 5 Web+API sipariş listesi alanları | TEC-1054 | | | | ⏸ |
| 5 + 5.1 Mobil | TEC-1055 | | | | ⏸ |
| 5.5 + 5.9 Web+API sipariş düzenleme | TEC-1056 | | | | ⏸ |
| 5.5 + 5.9 Mobil | TEC-1057 | | | | ⏸ |
| 5.6 Web+API plotter işareti + rapor | TEC-1058 | | | | ⏸ |
| 5.6 + 5.4.1 Mobil | TEC-1059 | | | | ⏸ |
| 5.4.1 Web+API hareket geçmişi | TEC-1060 | | | | ⏸ |
| 5.7 Web+API siparişe açık ürün | TEC-1061 | | | | ⏸ |
| KRİTİK Web+API yeni teslim yeri / bekleme havuzu | TEC-1062 | | | | ⏸ |
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
| 1 Web+API genel bakış | TEC-1072 | | | | ⏸ |
| 1.1 + 3.1 Mobil | TEC-1073 | | | | ⏸ |
| 2 + 4 Web menü, Müşteriler ve Araçlar | TEC-1074 | | | | ⏸ |
| 3 Web+API hizmet listesi | TEC-1075 | | | | ⏸ |

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
| Bayi/Distribütör, Olex Merkez = AFM Florya, Florya'dan AFM Florya'ya sipariş | TEC-1080 | 🟡 analiz ⏳ |
| 5.7 distribütör bazlı | TEC-1081 | 🟡 analiz ⏳ |
| 9.2 sözleşme ek fotoğraf | TEC-1082 | 🟡 analiz ⏳ |
| 9.3? mobil araç kabul / sözleşme | TEC-1083 | 🟡 analiz ⏳ |
| Kodların kredili kullanımı | TEC-1084 | 🟡 analiz ⏳ |
| İadelerin oluşturulması (kapsam) | TEC-1085 | 🟡 analiz ⏳ |
| 5.1 dipnot metrajlı sipariş yolu | TEC-1086 | 🟡 analiz ⏳ |
| 7.4 mobil Yeni Giriş (ertelendi) | TEC-1087 | 🟡 analiz ⏳ |
| Kapatılabilecek modüller (sunucuda hiçbir şey kapatılmadı) | TEC-1088 | 🟡 analiz ⏳ |

## Açık sorular

(İşler ilerledikçe eklenecek.)

## Önerilen merge sırası

(Sabah raporunda kesinleşecek.)
