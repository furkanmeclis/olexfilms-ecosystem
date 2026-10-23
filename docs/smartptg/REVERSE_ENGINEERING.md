# SmartPTG / NexPTG tersine mühendislik raporu

Olexfilms'in kullandığı SmartPTG (NexPTG ailesi) boya/kaplama kalınlık ölçerin resmi Android uygulamasının statik analizi. Amaç: cihazın bütün işlevlerini kanıtıyla çıkarmak ve mobil uygulamaya (`mobile/src/features/ble-gauge`) taşınabilir hale getirmek.

Tarih: 2026-10-23. Bu belge, eski Olexfilms uygulamasındaki `docs/smartptg/BLE_ANALYSIS.md` ve `VALIDATION_REPORT_v2.md` çalışmasının üzerine yazıldı. Eski raporda "decompile edilemedi" denen bölümler (`NexPTGService.a`, `a.onConnectionStateChange`, `b.doInBackground`) bu kez okundu. Eski rapordaki bazı yorumlar düzeltildi (bkz. §11).

## 0. Kaynak, yöntem, sınırlar

| | |
|---|---|
| APK | `pl.nexptg`, versionCode 946 (2.3.0), dosya `pl-nexptg-946-…apk`, 98 915 608 bayt |
| APK SHA-256 | `b5bfa8f52d77fde15a86349d4a65d2c940e89fbedf757cc3a8978e9d6a501a8b` |
| Analiz kopyası | Ses/OCR modelleri ve native kütüphaneler atılmış, yalnız `AndroidManifest.xml`, `resources.arsc`, `classes{,2,3}.dex` içeren kopya (SHA-256 `467b128e…`) ve yalnız `classes3.dex` içeren kopya (`8bf0114f…`). Kod bayt bayt aynıdır. |
| Araçlar | REA (`rea-agents@6.5.0`) Android sağlayıcısı = `jadx-headless-mcp 0.7.1` (JAR SHA-256 `6e5eacf5…ff75`, REA'nın sabitlediği sürüm), Temurin JDK 21. Sınıf envanteri REA kanıtı: `ev_550c13384aaa0ee7383a75026ca2c338778984ecf770bab074178b6bfa0ad8e5` (classes3, 1690 sınıf, `coverage: complete`). Makinedeki bellek baskısı yüzünden REA CLI her çağrıda JVM'yi yeniden başlatıp zaman aşımına düşünce, aynı JADX motoru tek kalıcı oturumda açılıp sınıf/metot kaynağı ve smali okundu. `resources.arsc` küçük bir ayrıştırıcıyla okundu. |
| Çalıştırma | APK hiçbir aşamada çalıştırılmadı, emülatör kullanılmadı. Bulgular statiktir. Gerçek davranış için 18.07.2026 tarihli 923 satırlık gerçek BLE kaydı (`tmp_scan/smartptg_logs_2026_07_18.csv`, SN 32475) ile çapraz kontrol yapıldı. |

Güven etiketleri: **[G]** gözlem (kod/smali'de doğrudan görülüyor), **[Ç]** çıkarım (kod + gerçek kayıttan türetildi, doğrulanmadı), **[B]** bilinmiyor.

Kanıtlar `sınıf.metot` biçimindedir. Obfuscation nedeniyle alan adları JADX'in verdiği adlardır (`NexPTGService.K`, `f14441r` gibi). `NexPTGService.a` metodu JADX'te "Code decompiled incorrectly" uyarısı verdiği için karar dalları smali'den okundu.

Repoya APK, dekompile kaynak ve anahtar/sır konmadı. Sabit anahtarlar yalnız adıyla anılıyor.

## 1. Mimari özet

```
pl.nexdiag (uygulama, classes3.dex)            com.nexdiag.nexptgmodule (cihaz modülü, classes2.dex)
 MainActivity ── bindService ──────────────▶ NexPTGService (foreground Service)
  │ NexPTGServiceCallback ◀── onNewData ───┤  ├─ a      : BluetoothGattCallback
  ├ MeasurementFragment (ölçüm ekranı)      │  ├─ b      : .dat indirme (AsyncTask)
  ├ ReportManager / ReportDatabase (Room)   │  ├─ FrameDecoder → libauth.so (native decode)
  ├ CarBodyManager (gövde haritaları)       │  ├─ CryptLib (AES-CBC)
  ├ SyncManager (kullanıcı sunucusuna POST) │  ├─ Interpretation (SAPL)
  ├ BlockedDevicesViewModel (kara liste)    │  └─ NexPTGConfig / NexPTGDataFrame
  └ VINAPIManager (VIN sorgusu)
```

Cihaz yalnızca **frekans** gönderir. µm değeri, uygulamada seri numarasına özel fabrika eğrisiyle hesaplanır. Kalibrasyon da uygulamada yapılır, cihaza kalibrasyon komutu gitmez.

## 2. BLE GATT

| Rol | UUID | Kanıt |
|---|---|---|
| Servis (Nordic UART) | `6e400001-b5a3-f393-e0a9-e50e24dcca9e` | `a.onServicesDiscovered`, `NexPTGService.b` [G] |
| TX, uygulama → cihaz, WRITE | `6e400002-b5a3-f393-e0a9-e50e24dcca9e` | `a.onServicesDiscovered` → `NexPTGService.f14416d0` [G] |
| RX, cihaz → uygulama, NOTIFY | `6e400003-b5a3-f393-e0a9-e50e24dcca9e` | `NexPTGService.b` → `f14418e0` [G] |
| CCCD | `00002902-0000-1000-8000-00805f9b34fb`, `ENABLE_NOTIFICATION_VALUE` | `NexPTGService.b` [G] |

Pil, cihaz bilgisi (DIS), firmware sürümü veya birim için ayrı karakteristik **yok**. Pil ve seri numarası ölçüm çerçevesinin içinde gelir. Birim (µm/mil) yalnız uygulama ayarıdır, cihaza gitmez [G: başka UUID literali yok. `NexPTGService` yalnız bu üç UUID'yi kullanır].

### 2.1 Bağlantı akışı [G]

1. `NexPTGService.n(true)`: tarama. API ≥ 23'te `BluetoothLeScanner.startScan(filtre yok, SCAN_MODE_LOW_LATENCY)`. Tarama zaman aşımı **30 s** (`q7.d` case 1 → `STATUS_DEVICE_NOT_FOUND`).
2. `NexPTGService.d(device)`: reklam adında `SmartPTG` veya `NexPTG` (`BuildConfig.FLAVOR`) geçmeli. Kayıtlı varsayılan cihaz varsa MAC eşleşmeli (`setDefaultDevice`). Üretici verisi ve servis UUID'si okunmaz.
3. `connectGatt(autoConnect=false, TRANSPORT_LE, PHY_LE_1M)`. Bağlantı zaman aşımı **20 s** (`q7.d` case 0: disconnect + yeniden tarama).
4. `a.onConnectionStateChange(STATE_CONNECTED)`: servisler boşsa `discoverServices()`, değilse doğrudan bildirim açılır. Kopunca `gatt.close()` ve servis bağlıysa otomatik yeniden tarama.
5. `a.onServicesDiscovered`: TX bulunur ve **hemen `o(3)` → `!3\r\n` yazılır**.
6. `a.onCharacteristicWrite` (ilk yazma): `NexPTGService.b` RX bildirimini ve CCCD'yi açar. Yani resmi uygulama, bildirimden önce mutlaka `!3` yazar.
7. Her RX bildirimi `NexPTGService.a`'ya gider. **10 s** boyunca çerçeve gelmezse otomatik bağlantı kesilir (`F.postDelayed(d(4), 10000)` → "AUTO DISCONNECT").

Not [G]: 18.07 kaydında hiç yazma yapılmadan bildirim alındı. Cihaz `!3` olmadan da yayın yapıyor.

### 2.2 Komut seti (uygulama → cihaz) [G]

Tek komut biçimi vardır: ASCII `"!" + mod + "\r\n"` (`NexPTGService.o(int)`). Varsayılan `WRITE_TYPE_DEFAULT` (yanıtlı) yazmadır, her yazmadan sonra 5 ms beklenir.

| Komut | Bayt | Anlam |
|---|---|---|
| `!1\r\n` | `21 31 0D 0A` | Yalnız **Fe kanalı** [Ç: mod 1'e geçildiğinde uygulama yalnız tip 1 çerçeveleri işler] |
| `!2\r\n` | `21 32 0D 0A` | Yalnız **Al (NFe) kanalı** [Ç] |
| `!3\r\n` | `21 33 0D 0A` | İki kanal dönüşümlü (varsayılan, bağlantıda gönderilir) [G + gerçek kayıt] |

Kalibrasyon, sıfırlama, birim, uyku, pil veya firmware komutu **yok** [G: `o()` dışında `writeCharacteristic` çağrısı yok].

## 3. Çerçeve formatı ve decode

### 3.1 Satır [G]

```
eVo<T><14 hex>\r\n         ör. eVo1AFBAADA54F0BE8\r\n   (20 bayt)
```

Bir bildirim bir satır taşır. `FrameDecoder.a` yalnız `getStringValue(0)` kullanır, satır birleştirme yapmaz. Mobilde parça birleştirme (`createLineSplitter`) yine de doğru bir savunmadır.

### 3.2 Decode [G]

`FrameDecoder.a(String)` → native `FrameDecoder.decode` (`lib/*/libauth.so`, `Java_com_nexdiag_nexptgmodule_FrameDecoder_decode`) → `int[6]`. Native algoritmanın JS portu eski çalışmada doğrulandı ve `mobile/.../protocol.ts:decodeFrame` içinde. 18.07 kaydındaki 923 gerçek çerçevenin hepsinde frekans kayıtla birebir aynı çıkıyor (bu belge için yeniden koşuldu: 0 uyuşmazlık).

| Alan | Hesap |
|---|---|
| `type` | `str.charAt(3) - '0'` (native'den değil) |
| `frequency` (Hz) | `(a[1] << 8) + a[2]` |
| `serialNumber` | `(a[3] << 16) + (a[4] << 8) + a[5]` |
| `batteryVoltage` (V) | `a[0] / 10` |

### 3.3 Çerçeve tipi [G + Ç]

| `type` | Anlam |
|---|---|
| 1 | Fe kanalı ölçümü (`NexPTGConfig.FRAME_FE`) |
| 2 | Al kanalı ölçümü (`FRAME_AL`) |
| 3 | Fe kanalı + **cihaz düğmesine basıldı** [Ç] |
| 4 | Al kanalı + **cihaz düğmesine basıldı** [Ç] |

Kanıt: `NexPTGService.a`, Zn modunda `J==1 && type==3` veya `J==0 && type==4` olduğunda `j0 = true` yapar. Çerçevede `deviceSaveFlag = j0 && type > 2` olur. `MeasurementFragment.m` bu bayrakla ölçümü rapora kaydeder (`p(...)`). Cihaz düğmesi uygulamada "kaydet" düğmesidir. Gerçek kayıtta tip 3/4 görülmedi (düğmeye basılmamış).

**Tip, alt tabakayı söylemez, kanalı söyler [G, gerçek kayıt].** Mod 3'te cihaz yaklaşık 0,8 s'lik ~8 çerçevelik gruplarla iki kanalı dönüşümlü gönderir:

| Durum (SN 32475, K=22291, L=23490) | Fe kanalı (tip 1) | Al kanalı (tip 2) |
|---|---|---|
| Havada | ≈22 290 (≈K) | ≈23 512 (≈L) |
| Çelik üzerinde ~230 µm | ≈19 190 | ≈20 820 |
| Alüminyum üzerinde | ≈24 073 | ≈26 475 |

### 3.4 Pil [G]

`FrameDecoder.a` voltajı (`V*100`) seviyeye (0–7, ikon `ic_battery_%d`) ve yüzdeye çevirir: ≥2,80 V → %100, >2,60 → 90, >2,50 → 80, >2,40 → 70, >2,30 → 60, >2,20 → 50, >2,10 → 40, >2,00 → 30, >1,90 → 20, >1,80 → 15, diğer → 10. UI pil göstergesini en fazla 30 sn'de bir günceller (`MeasurementFragment.m`).

## 4. Malzeme (Fe / Al / Zn) ayrımı ve kanal seçimi

Kaynak: `NexPTGService.a` smali, `:cond_cb … :cond_14f` ve `:cond_2be … :cond_309`.

### 4.1 Malzeme kararı `J` [G]

`K` ve `L` eğri dosyası başlığından gelir (Fe ve Al kanalının havadaki eşiği, bkz. §5).

```
fe_or_al(frame) = frame.type == 1 ? (frame.freq >  K ? AL : FE)
                                  : (frame.freq >= L ? AL : FE)
```

- **Zn modu açık** (`setModes(znEnabled=true)`, varsayılan açık) ve kalibrasyon yoksa: `J = fe_or_al(son Fe kanalı çerçevesi Y ?? bu çerçeve)`. Cihaz mod 3'te değilse ve `b0` (Al kilidi) yoksa `!3` yazılır ve bu çerçeve `J=-1` ile atılır.
- **Zn kapalı veya kalibrasyonda**: Fe kanalı `freq > K` ise mod 2 istenir, değilse mod 1. Al kanalı `freq < L` ise mod 1, değilse mod 2. İstenen mod cihazdaki mod değilse `!n` yazılır ve çerçeve atılır (`J=-1`). Mod uyuyorsa `J = 1` (Al) veya `0` (Fe).

### 4.2 Hangi çerçeve hesaplanır [G]

`J==FE && type==2` veya `J==AL && type==1` ise **değer yeniden hesaplanmaz, önceki değer (`f14439q`) korunarak çerçeve yayınlanır**. Yani seçili malzemenin karşı kanalından gelen çerçeve "prob kalktı" anlamına gelmez.

Değer `-1` olduğunda (hava): Zn modunda `!3` yazılır, `Y/Z` sıfırlanır, `idle=true`. Değer geçerliyse ve `J==AL` ise `b0=true`, `!2` yazılır (Al kanalına kilitlenme).

### 4.3 Zn (galvaniz) tespiti [G]

Yalnız Zn modu açıkken, hem Fe (`Y`) hem Al (`Z`) kanalından birer çerçeve varken çalışır. Sonuç `materialType = 2` (`MATERIAL_FE_ZN`) olur.

- `cal3` ya da cihaz tipi ∈ {2,4,5,6} (`NexPTGService.j`): 4 parametreli lojistik model `y = d + (a − d) / (1 + (µm / c)^b)`, `a = 375.7367`, `b = 0.4066573`, `c = 3.496022e11`, `d = −1176941`: `dPow = (1177316.7367 / (1 + (µm / 3.496022e11)^0.4066573)) − 1176941 − (W−V − (U−T)) / n` (n = 7 `cal3`, aksi 6). `d12 = (Z.freq − Y.freq) − (U − T)`. `Zn = d12 − dPow > 0`. `T,U,V,W` eğri başlığının 6.–9. alanlarıdır. Aynı sabitler `TestConfigActivity` gizli ayar ekranında (`a=375.7367, b=0.4066573, c=349602200000.0, d=−1176941.0`, `znModifier=6`, `znModifierBall=10`) düzenlenebilir görünür.
- Diğerleri (`cal1`): `|(|L−K|) − |Z.freq − Y.freq||` değeri `cal1` için 105…480, değilse 100…450 aralığındaysa Zn. `cal2`: `(L − Z.freq) − (K − Y.freq)` (> 220 µm ise +45) < 0 ise Zn.
- Zn üst sınırı: tip 2,4,5 → 800 µm, tip 6 → 1000 µm, diğer → 600 µm. Üstünde Zn bayrağı düşer.
- Malzeme etiketi ancak ~8 çerçeve sonra gösterilir (`f14432m > 8` → `showMaterial`).

## 5. Eğri dosyası (`{SN}.dat`)

### 5.1 Kaynak [G]

| Sıra | URL | Kanıt |
|---|---|---|
| 1 | `http://www.smartptg.pl/char/{SN}.dat` (düz HTTP) | `b.doInBackground` (`backup=false`) |
| 2 | `https://mysdprog.pl/char/{SN}.dat` | aynı (`backup=true`, birinci 200 dönmezse `onPostExecute` başlatır) |

- Kimlik doğrulama, başlık, imza veya token **yok**. Düz `GET`, 15 s bağlantı zaman aşımı.
- Dosya `filesDir/{SN}_temp.dat`'a indirilip `{SN}.dat`'a taşınır. İnternet varsa her bağlantıda yeniden indirilir (`NexPTGService.l(sn, true)` → `f()`), yoksa diskteki kopya kullanılır.
- İkisi de başarısızsa ve diskte dosya yoksa `onConfigDataDownloadEnded(1, …)`: "dosya yok". Şifre çözme hatasında string kaynağı `def` (gömülü demo eğri) kullanılır.
- Demo modu `loadDemo()` → SN **809** eğrisi, tip 2.
- Eğri **cihazda değil**, sunucuda tutulur. Cihaz eğri göndermez.

### 5.2 Şifreleme [G]

`IV:Base64(ciphertext)`, `CryptLib.a`: `AES/CBC/PKCS7Padding`. Anahtar, string kaynağı `guarana`'nın UTF-8 baytlarının ilk 16'sı. IV, `:` öncesi metnin ilk 16 baytı. Değer zaten `mobile/.../curve.ts` içinde, burada tekrarlanmadı.

### 5.3 Düz metin [G: `NexPTGService.l`]

```
K,L,deviceType,lang,serial[,T,U,V,W]
feThick0,feFreq0,alThick0,alFreq0
...
feThickN,feFreqN,alThickN,alFreqN
"yorum calX" , tarih, saat            ← başlıkta 4'ten fazla alan varsa son satır yorumdur
```

| Alan | Kullanım |
|---|---|
| `K`, `L` | Fe ve Al kanalı havadaki eşikleri (malzeme kararı) |
| `deviceType` | `NexPTGService.n`: 0 Advanced, 1 Standard, 2 Professional, 3 Economic, 4 Economic Plus, 5 Standard Plus, 6 Pro Carbon |
| `lang` | `NexPTGService.X` → çerçevenin `lang` alanı |
| `T,U,V,W` | Zn modeli için fabrika frekansları |
| `cal1`/`cal2`/`cal3` | Metinde geçmesi yeterli (`str.contains`); kalibrasyon hedefi ve Zn algoritmasını seçer |

Satırlar `Q` (Fe) ve `S` (Al) tablolarına `[öncekiKalınlık, öncekiFrekans, kalınlık, frekans]` segmentleri olarak yüklenir.

### 5.4 Frekans → µm (`NexPTGService.c`) [G]

```
f = freq + (Fe ? calFe + idleFe : calAl + idleAl)
Fe: f > son Fe frekansı → -1 (hava)          Al: f < son Al frekansı → -1
segment ara (Fe: ön ≤ f < son; Al: son ≤ f < ön), doğrusal interpolasyon, round
eşleşme yoksa (Fe'de ilk noktanın altı) → 0 µm
```

Sonraki işlemler (`NexPTGService.a`, smali `0x41200000`):

- Cihaz tipi 1, 3, 4, 5 için değer **10 µm'ye yuvarlanır**. 0, 2 ve 6 tam µm.
- Üst sınır, üstü `-1`: tip 1/3/4/5 → 1000 µm, 0 → 2200, 2 → 2500, 6 → 3000.
- Economic (3, 4) Al ölçemez: `J==AL` → `-1`.
- `idleFe/idleAl` (`h()`, `g()`) pratikte **hep 0**: `M−O` çarpılıyor ama `O` ve `P` `final 0`, koşul `O != 0` hiç sağlanmıyor. Ölü kod.

## 6. Kalibrasyon

### 6.1 Fe/Al sıfırlama (folyo) kalibrasyonu [G]

Cihaza komut gitmez. Akış `startCalibrationStep1()` (Fe) → `onCalibrationShowDialogStep2` → `startCalibrationStep2()` (Al):

1. `x=true`, `y=1` (Fe) / `2` (Al). Uygulama gerekirse `!1` / `!2` yazarak kanalı seçer.
2. Her çerçevede eğriye ofsetsiz bakılır. Ardışık iki frekans farkı < 10 Hz ve değer geçerliyse kararlılık sayacı `B++`, değilse hata sayacı `z++`. **10 kararlı örnek** → ofset. **10 hata** → `onCalibrationError`. Adım zaman aşımı **5 s** (`q7.d` case 2).
3. Hedef frekans `e(material)`: eğride kalınlığı **90 µm** (`cal1/2/3` varsa) veya **150 µm** (yoksa) civarındaki (±10) noktanın frekansı. Bulunamazsa 6. nokta. Yani kullanıcı ilgili plakanın üstüne bu kalınlıkta referans folyo koyar.
4. `calFe = e(0) − ölçülenFrekans`, `calAl = e(1) − ölçülenFrekans`.
5. `p()` SharedPreferences `Settings` içine yazar: `calibratedFrequencyDifferenceFe{SN}`, `calibratedFrequencyDifferenceAl{SN}`, `idleCalFrequencyFe{SN}`, `idleCalFrequencyAl{SN}`, `calibrationDate{SN}`. Sonra `onCalibrationSuccess`.
6. Economic (tip 3) Fe adımından sonra doğrudan kaydeder (Al adımı yok).
7. `clearCalibration()` Fe/Al farklarını ve tarihi 0'lar.

Ofsetler **cihaz değil telefon başına**, SN anahtarlıdır. Yeni telefonda kalibrasyon yoktur.

### 6.2 Referans noktası (SAPL bireysel yorum) [G]

`startReferencePoint()`: aynı 10 kararlı örnek kuralı, 5 s zaman aşımı. Ofsetli µm değeri `referencePointValue` olur (`onReferencePointSuccess`). Kullanıcıdan dış yüzeyde tavana, iç yüzeyde B direğine ölçüm alması istenir (string `alert_reference1/2`).

## 7. Yorumlama (SAPL), eşikler ve renkler

Kaynak: `Interpretation.getInterpretation(saplMode, place, ref, µm)`, `getValues`. `place`: 0 dış gövde (`BODY`), 1 iç (`BODY_INSIDE`). Varsayılan referanslar: dış 105 µm, iç 60 µm (`Settings.referenceSaplPointValueOutside/Inside`).

| Seviye | Metin (`interpretationN`) | Renk (`color_interpretations`) | Genel, dış | Genel, iç | Bireysel, dış (ref=r) | Bireysel, iç |
|---|---|---|---|---|---|---|
| 0 | Boya katmanı çok ince | `#E9DF28` | ≤ 69 | ≤ 39 | ≤ r−36 | ≤ r−31 |
| 1 | Orijinal boya | `#55D37A` | 70–165 | 40–90 | r−35…r+35 | r−30…r+30 |
| 2 | İkinci kat boya | `#DEB50A` | 166–300 | 91–150 | …r+170 | …r+90 |
| 3 | İnce macun | `#EC4A08` | 301–500 | 151–350 | …r+370 | …r+290 |
| 4 | Kalın macun | `#AF0025` | 501–700 | 351–550 | …r+570 | …r+490 |
| 5 | Kalın macun | `#AF0025` | > 700 | > 550 | > r+570 | > r+490 |

SAPL modları: 0 genel, 1 bireysel (referans noktalı), 2 kapalı (seviye −1, renk `button_color2 #4196AF`), 3 genişletilmiş (bireysel ile aynı aralıklar, `SaplExtDialog`). Değer yoksa renk `interpretation_color1`.

## 8. Uygulamanın diğer işlevleri

| İşlev | Kanıt | Not |
|---|---|---|
| Birim µm/mil | `MilsKt.milsToUm` (×25,4), `Utils.getMils`, `Settings.measurementUnit` | mil iki ondalık (`%.2f`) gösterilir |
| Ölçüm modu | `Config.CONTINUOUS_MODE=0`, `SINGLE_MODE=1`, `Settings.measurementMode` (varsayılan 1) | Tekil modda cihaz düğmesi (tip 3/4) kaydı yok sayılır. Economic'te ve rapor ekranında sürekli moda zorlanır. |
| Referans farkı gösterimi | `Settings.referencePointOn` | Değer `+Δ` olarak gösterilir |
| Rapor/oturum | `ReportManager`, Room `ReportDatabase` (göç 3→14), `model.Report` | Araç bilgisi (marka, model, yıl, VIN, yakıt, güç, hacim), dış/iç ölçüm noktaları, değerler, yorumlar, zaman damgaları, renkler, lastikler (`Tire`), fotoğraflar, yorum, cihaz seri no, kalibrasyon zamanı, SAPL ayarları. PDF: `PdfGenerator` (pdfbox). |
| Araç parça haritası | `CarBodyManager`, `model.Project/Sides/Side/Element/Point/PointsPack/ModelType/Transform`, `view.CarView` | Gövde tipleri `https://mysdprog.pl/bodyworks/data.json` listesinden, her tip `https://mysdprog.pl/bodyworks/…` altından ZIP olarak indirilir. Sol, üst, sağ ve arka yüzler (`ConfigKt.Side`). Carbon için ayrı set. |
| Senkronizasyon | `SyncManager.access$httpPost` | Kullanıcının girdiği URL'ye (`Settings.syncApi`) `POST application/json`, isteğe bağlı `Authorization: Basic base64(syncLogin:syncPassword)`. Gövde anahtarları: `reports, id, name, date, brand, maker, model, year, vin, fuelType, power, capacity, typeOfBody, unitOfMeasure, deviceSerialNumber, calibrationDate, data, dataInside, history, section, value, interpretation, comment, extraFields, season, width, diameter`. Nexdiag sunucusu değil. |
| Cihaz kara listesi | `BlockedAPIManager.getBlockedDevices`, `BlockedDevicesViewModel.checkSNIsBlocked` | `POST https://devices.smartptg.pl/list`, gövde `{"timestamp":<int>}`, başlık `X-AUTH-TOKEN: <uygulamaya gömülü sabit anahtar>`. Yanıt `{devices:[…SN], timestamp}`, `SharedPreferences("blocked").blockedSN`. Engelli SN'de demo görünümü açılmaz. |
| VIN sorgusu | `VINAPIManager.getCarInfo` | `POST https://vin.smartptg.pl/vin`, `{"vin": "…"}`, aynı `X-AUTH-TOKEN`. Yanıt `make, model, modelYear, engineDisplacement, enginePowerHp, fuelTypePrimary(Id)`. Kota aşımı `VinLimitExceededException`. VIN için kamera/OCR (`ScannerActivity`, ML Kit) ve QR. |
| Sesli komut | `SpeechManager`, `assets/model-en-us` (Vosk) | APK'nın ~70 MB'ı bu model |
| Uygulama bütünlüğü | `MainActivity.onResume` → `NApp.checkAppSignature`, `checkPackageName` | Yeniden imzalı APK çalışmaz |
| Lisans/destek | `onDeviceWrongLicense`, `onDeviceNotSupported` callback'leri | Modülde çağıran yok [G: `NexPTGService` içinde tetikleyici yok]. Ölü arayüz. |
| Firmware güncelleme | — | **Yok** [G: DEX'lerde `firmware`, `dfu`, `nordicsemi`, `ota`, `bootloader` dizesi yok. GATT'ta DFU servisi kullanılmıyor]. |
| Diğer | Yardım `https://help.nexdiag.com/`, dağıtım `https://nexdiag.com/distribution`, 17 dil (`Config.supportedLanguages`, `tr` dahil), Firebase Analytics, Play inceleme istemi | |

## 9. Olexfilms backend: eğri ve kalibrasyon modeli (öneri)

Gerekçe: iOS ATS düz `http://` indirmeye izin vermez. `https://www.smartptg.pl` geçerli sertifika sunmuyor ve `https://mysdprog.pl` çoğu SN için 404 dönüyor (bkz. `bundled-curves.ts` notu ve VALIDATION_REPORT_v2). Mobil yalnız SN 32475'i gömülü taşıyor. Ayrıca kalibrasyon ofsetleri telefona bağlı, şube/cihaz paylaşımı yok.

### 9.1 Uç

```
GET /api/v1/gauge-curves/{serial}          (mobil, oturum zorunlu)
→ 200 { serial, deviceType, model, feThreshold, alThreshold, lang,
        zn: {t,u,v,w} | null, calVersion: "cal1"|"cal2"|"cal3"|null,
        comment, fe: [{thickness,frequency}], al: [...],
        sourceSha256, fetchedAt }
→ 404 { error: "curve_not_found" }  (iki kaynak da yoksa)
```

Sunucu akışı: önbellekte varsa döner (ETag = `sourceSha256`). Yoksa sırayla `http://www.smartptg.pl/char/{SN}.dat` ve `https://mysdprog.pl/char/{SN}.dat` (15 s zaman aşımı, yalnız `^\d{1,8}$` SN), AES-CBC çözme, §5.3'e göre ayrıştırma, `serial` başlıkla eşleşmeli. Ham dosya da saklanır (yeniden ayrıştırma için). Arka planda periyodik tazeleme (resmi uygulama her bağlantıda indiriyor, fabrika yeniden kalibre ederse dosya değişebilir). Olumsuz sonuç kısa süre (ör. 1 saat) önbelleklenir. Proxy bir SN listesini taramaz; yalnız istenen SN'i çeker.

### 9.2 Tablolar

```sql
gauge_curves (
  serial            INTEGER PRIMARY KEY,
  device_type       SMALLINT NOT NULL,
  fe_threshold      INTEGER NOT NULL,   -- K
  al_threshold      INTEGER NOT NULL,   -- L
  lang              SMALLINT,
  zn_t INTEGER, zn_u INTEGER, zn_v INTEGER, zn_w INTEGER,
  cal_version       TEXT,               -- cal1|cal2|cal3|NULL
  comment           TEXT,
  fe_points         JSONB NOT NULL,     -- [{thickness,frequency}]
  al_points         JSONB NOT NULL,
  raw_file          TEXT NOT NULL,      -- IV:Base64 (şifreli, olduğu gibi)
  source_url        TEXT NOT NULL,
  source_sha256     CHAR(64) NOT NULL,
  fetched_at        TIMESTAMPTZ NOT NULL,
  checked_at        TIMESTAMPTZ NOT NULL
)
-- Eğri global fabrika verisidir: organization_id/brand_id yok (AGENTS.md §3 istisnası; design.md'ye karar olarak eklenmeli).

gauge_calibrations (                     -- append-only ledger
  id BIGSERIAL PRIMARY KEY,
  organization_id, brand_id,
  serial INTEGER NOT NULL,
  fe_offset_hz INTEGER NOT NULL, al_offset_hz INTEGER NOT NULL,
  target_um SMALLINT NOT NULL,          -- 90 veya 150
  samples JSONB,                         -- ölçülen frekanslar (denetim)
  device_user_id, created_at TIMESTAMPTZ NOT NULL
)
-- Güncel ofset = (organization_id, serial) için son satır.
```

Ölçüm kaydına eklenecek ham alanlar: `frequency`, `channel` (fe/al), `material` (fe/al/zn), `curve_sha256`, `fe/al_offset_hz`. Böylece eğri ya da algoritma sonradan düzeltilirse geçmiş µm yeniden hesaplanabilir.

### 9.3 Kara liste ve VIN

`devices.smartptg.pl` ve `vin.smartptg.pl` üreticinin anahtarıyla korunuyor. Olexfilms'in bunları kendi istemcisinden çağırması üreticinin gömülü anahtarını kullanmak anlamına gelir. **Önerilmez.** Kara liste Olexfilms için gereksizdir (cihazlar kendi envanterimizde). VIN için mevcut kendi çözümümüz kullanılmalı.

## 10. Mobil koddaki eksikler ve hatalar (`mobile/src/features/ble-gauge`)

Simülasyon: 18.07 kaydının 923 çerçevesi mobil kodun kendisi (`decodeFrame` → `frameToReading` → `stepSampler`, gömülü 32475 eğrisi) ve §4'ün kanal kuralı ile işlendi. Mobil: 212 geçerli / 711 `null` okuma, 11 commit (`222,221,228,52,45,45,45,223,222,233,228`). Kanal kuralı: 7 commit (`1261,241,52,237,1746,234,233`). Fark, mobilin her çerçeve tipini ayrı malzeme sayması ve karşı kanal çerçevesinde örnekleyiciyi sıfırlamasından geliyor. Kayıtta referans değer olmadığı için hangisinin doğru olduğu bu veriyle kanıtlanamaz [B]. Ölçüm doğrulaması için bilinen folyolarla yeni kayıt gerekir.

| # | Sorun | Yer | Resmi davranış | Öneri |
|---|---|---|---|---|
| 1 | **Kalibrasyon ofseti hiç uygulanmıyor** | `manager.ts:onLine` → `frameToReading(frame, curve, null, …)` | `calFe/calAl` her ölçüme eklenir (§6.1) | Fe/Al kalibrasyon sihirbazı (90/150 µm folyo, 10 kararlı örnek, 5 s), ofset SN başına yerel + backend `gauge_calibrations` |
| 2 | Malzeme = çerçeve tipi | `protocol.ts:decodeFrame` (`type 1→fe, 2→al`) | Malzeme K/L eşiğine göre kanal frekansından (§4.1) | `J` kuralını uygula; tip yalnız kanal |
| 3 | Karşı kanal çerçevesi örnekleyiciyi sıfırlıyor | `readings.ts:stepSampler` (`um == null` → reset) | Önceki değer korunur (§4.2) | Seçili malzemenin karşı kanalındaki çerçeveyi yok say; "prob kalktı" yalnız `-1` (hava) |
| 4 | Tip 3/4 (cihaz düğmesi) `unknown` | `protocol.ts` | Kayıt tetikleyicisi (§3.3) | Düğme çerçevesini bir "commit" olayına bağla |
| 5 | `!3` yazılmıyor, mod kontrolü yok | `manager.ts:connect` | Bağlantıda `!3`, kalibrasyon/Al kilidinde `!1/!2` | En az `!3`; kalibrasyonda kanal seçimi. TX'in varlığı da doğrulanmalı. |
| 6 | Fe'de ilk noktanın altı `null` | `curve.ts:frequencyToMicrons` | `0 µm` (§5.4) | 0 döndür (çıplak metal = 0 µm) |
| 7 | Cihaz tipine göre yuvarlama ve üst sınır yok | `curve.ts` | 10 µm yuvarlama (1,3,4,5); 1000/2200/2500/3000 sınırı; Economic'te Al yok | Uygula |
| 8 | Zn/galvaniz yok | — | §4.3 | İkinci aşama; `T,U,V,W` ve `calX` ayrıştırılmalı |
| 9 | Eğri başlığının 6.–9. alanları ve `calX` atılıyor | `curve.ts:parseCurve` | Zn ve kalibrasyon hedefi bunlara bağlı | `GaugeCurve`'e `lang, zn, calVersion` ekle |
| 10 | Eğri kaynağı: önce `https://mysdprog.pl`, sonra `https://www.smartptg.pl` | `session.ts:curveUrls` | `http://www.smartptg.pl` birinci, mysdprog yedek | Doğrudan üretici yerine backend ucu (§9). Sertifikasız https deneme sırası anlamsız. |
| 11 | Önbellekteki eğri hiç tazelenmiyor | `manager.ts:loadCurve` | Her bağlantıda yeniden indirilir | `sourceSha256`/ETag ile arka planda tazele |
| 12 | Pil yalnız voltaj | `store.ts` | Yüzde ve seviye tablosu (§3.4) | Tabloyu taşı |
| 13 | SAPL yorum ve renkleri yok | — | §7 | Parça alanlarında renk/etiket için `Interpretation`'ı taşı (Olexfilms kendi eşiklerini isterse yapılandırılabilir yap) |
| 14 | 10 s veri gelmeyince bağlantı kesme yok | `manager.ts` | `AUTO DISCONNECT` | İsteğe bağlı; en azından "veri yok" durumu |
| 15 | Ham frekans ve kanal ölçümle saklanmıyor | `Commit` tipi | — | §9.2 son paragraf |

**TEC-939 durumu (mobil + backend):** 1–7, 9, 11, 12, 15 uygulandı (`measure.ts` kanal/malzeme kuralı, `calibration.ts` sihirbaz kuralları, `curve.ts` cihaz kuralları ve başlık alanları, `protocol.ts` pil tablosu, `gauge_calibrations` defteri). 13: SAPL genel mod / dış yüzey canlı etiket ve renk (`interpretation.ts`; bireysel mod yalnız saf fonksiyonda). 14: 10 sn veri yoksa "veri yok" uyarısı (otomatik kesme yok). 8 (Zn) ve ölçüm sırasında Al kilidi (`!2`) yapılmadı: doğruluğu [B]/[Ç], numune ve kayıtla doğrulanmalı. 10 TEC-934'te çözüldü.

Doğru olanlar: UUID'ler, ad filtresi (`isGaugePeripheral` ayrıca NUS servis UUID'sini kabul ediyor, zararsız), decode, AES çözme, temel interpolasyon, iki çerçeve onaylı SN (`createSerialConfirm`), µm/mil dönüşümü (resmi uygulama mil'i 2 ondalık gösteriyor, mobil 1).

## 11. Eski rapordan düzeltmeler

- "Mod 2: Fe + frekans eşik üstü, mod 1: Al" yerine mod 1 = Fe kanalı, mod 2 = Al kanalı, mod 3 = dönüşümlü (§2.2, §4.1).
- "Kalibrasyon hedefi ~90/150 µm" doğru. Ayrıca 10 kararlı örnek kuralı, 5 s zaman aşımı ve 10 hata sınırı eklendi.
- "Idle kalibrasyon ofsetleri" pratikte hep 0 (§5.4).
- "type 1 = Fe frame, 2 = Al frame" yerine tip kanal bildirir, malzemeyi değil (§3.3). Tip 3/4 düğme çerçeveleri.
- `onConnectionStateChange`, `NexPTGService.a` ve `b.doInBackground` artık okundu.

## 12. Açık sorular

| Soru | Durum |
|---|---|
| Mod 1/2'nin cihazda gerçekten tek kanal yayını olduğu | [Ç] Uygulama mantığı bunu varsayıyor. `!1`/`!2` yazılıp kayıt alınarak doğrulanmalı. |
| Tip 3/4 düğme çerçeveleri | [Ç] Düğmeye basılarak kayıt alınmalı. |
| Mobil ve resmi uygulama µm değerlerinin eşitliği | [B] Bilinen kalınlıkta folyolarla iki uygulamada eş zamanlı ölçüm gerekli. |
| `libauth.so` decode | Eski çalışmada disassembly ile çıkarıldı. Bu oturumda native kod yeniden açılmadı; 923 gerçek çerçeve ile tutarlı. |
| Zn modelinin doğruluğu | [B] Galvaniz numune gerekli. |
