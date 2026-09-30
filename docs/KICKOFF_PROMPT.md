# Kickoff prompt'ları

## 1. İlk oturum (F0'dan başla)

Aşağıdaki metni olduğu gibi gönder.

---

Sen `furkanmeclis/olexfilms-ecosystem` reposunu sıfırdan inşa eden tek geliştiricisin. Amaç: Linear projesi **Olexfilms Tek App (P-TEC-4)** içindeki tüm işleri F0'dan F5'e kadar sırayla bitirmek. Ben araya girmeyeceğim; kararlar verilmiş, her iş yazılmış. Sen uygula.

Önce şunları oku, başka hiçbir şey okuma:
1. `AGENTS.md` (çalışma kuralları; bunlara uy)
2. `docs/design.md` bölüm 3 (kararlar K1–K29) ve bölüm 6 (teknik mimari)
3. Linear'da `list_issues` ile P-TEC-4, milestone "F0 Altyapı" işlerini listele

Sonra çalışma döngün her iş için aynı:
- İşi In Progress yap → Kapsam/Kabul/Referans oku → referans dosyaları GitHub'dan çek (klonlama) → branch `feat/tec-<no>-<slug>` → küçük commit'ler `TEC-<no>: ...` → testler yeşil → PR aç (`gh pr create`, başlıkta TEC-no, açıklamada kabul kriterleri işaretli) → `gh pr merge --auto --squash` → beklemeden sıradaki işe geç. PR merge olunca Linear işi Done olur (olmazsa sen yap).
- Aynı fazdaki işleri `blocked by` sırasına göre al; blokerı bitmemiş işe girme.
- Bir işi bitiremiyorsan Linear'a yorum yaz (ne eksik, neden), Todo'ya çek, sonraki işe geç. Tahminle tasarım değiştirme.

Bağlam disiplini:
- Her işe temiz başla; tüm design.md'yi yeniden yükleme, ilgili bölümü grep'le.
- Büyük dosyaları tamamen okuma; ihtiyacın olan aralığı oku.
- Araştırmayı alt ajana ver, özetini al.
- Test/lint çıktısının yalnızca hata kısmını oku.
- Bağlam dolmaya yaklaşınca özetle: "aktif iş, branch, kalan adımlar" üç satırı yeter.
- 3 saati geçen işi Linear'da alt işlere böl.

Taban kod: otopoly-go'nun altyapı katmanı (auth, BFF, organizations, RBAC, queue, realtime, storage, i18n). Üzerine technowide-ecosystem düzeltmeleri (SeaweedFS, oturum, iOS push, compose prod, gen-env, izin scope'ları, MCP). İkisi de GitHub'da; dosya yollarını Linear işleri veriyor.

Sınırlar: prod sunucuya bağlanma, eski sistem DB'lerine yazma, Linear'da iş silme/kilometre taşı değiştirme yok, kapsam dışı maddeleri (design.md bölüm 9) yapma, secret commit'leme.

Başla: F0-01 (TEC-78).

---

## 2. Devam oturumu (faz kapısında veya bağlam sıfırlanınca)

---

`furkanmeclis/olexfilms-ecosystem` üzerinde çalışmaya devam ediyorsun. Kurallar `AGENTS.md`'de, kararlar `docs/design.md`'de. Linear P-TEC-4'te `state=In Progress` olan işi kontrol et; varsa oradan devam et (branch'i `git branch -a` ile bul). Yoksa milestone sırasına göre (F0→F5) ilk `Backlog/Todo` işi al; blokerı bitmemiş işe girme. Döngü aynı: In Progress → uygula → PR + `gh pr merge --auto --squash` → sıradaki iş. Bağlamı küçük tut, referans dosyaları GitHub'dan çek, tahminle tasarım değiştirme; sorun varsa Linear'a yorum yaz ve geç.

---

## 3. Faz kapısı kontrolü (her fazın sonunda gönder)

---

Faz <F#> işleri Done görünüyor. Kapı şartını doğrula: Linear'daki "<F#>" kilometre taşı açıklamasındaki kapı cümlesini oku, gereken akışı lokalde uçtan uca çalıştır (gerekiyorsa Playwright/Go testi yaz), sonucu Linear'da kilometre taşının işlerinden sonuncusuna yorum olarak ekle: neyi çalıştırdın, çıktı ne. Kapı sağlanmıyorsa eksiği yeni bir iş olarak aç (`save_issue`, milestone aynı, başlık `<F#>-FIX ...`), uygula, sonra sonraki faza geç.

---
