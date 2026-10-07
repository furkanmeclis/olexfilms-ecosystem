# MCP uçları (Claude, ChatGPT ve diğer AI istemcileri)

Olexfilms verilerine AI istemcilerinden (Claude, ChatGPT, Cursor, VS Code) güvenli erişim sağlayan üç MCP ucu vardır.
Uçların kodu tektir (`backend/internal/modules/mcp`, TEC-402). Kimlik doğrulama OAuth 2.1 ile yapılır
(`backend/internal/modules/oauth`, TEC-400/401). Tool'lar AI asistanının tool kayıt defterinden gelir
([`docs/ai.md`](ai.md)). Panel sohbeti, WhatsApp ve MCP aynı tool'ları ve aynı yetki kontrolünü kullanır.

## 1. Uç adresleri

| Uç | Kim bağlanır | Tool seti |
|---|---|---|
| `https://<alan-adı>/mcp/dealer` | Distribütör ve bayi üyeleri (`mcp.connect` izniyle) | Panel tool'ları, bağlanılan bayi/distribütörün yetkisiyle |
| `https://<alan-adı>/mcp/user` | Merkez dahil tüm panel kullanıcıları (`mcp.connect` izniyle) | Panel tool'ları, onay ekranında seçilen organizasyonun yetkisiyle |
| `https://<alan-adı>/mcp/customer` | Müşteri portalı hesapları | Müşteri tool'ları (yalnız kendi araç, hizmet, garanti ve randevuları) |

- Alan adı markanın alan adıdır (ör. `olexfilms.com`). Next.js `/mcp/*`, `/oauth/*` ve `/.well-known/*` yollarını Go'ya
  aktarır. Dışarıya yalnız frontend açıktır.
- Taşıma: **Streamable HTTP**, durumsuz (stateless). Her istek kendi Bearer token'ıyla gelir ve tool listesi her istekte
  yeniden hesaplanır. `GET`/`DELETE` istekleri 405 döner.
- Kimlik: OAuth 2.1 + PKCE S256 + dinamik istemci kaydı (DCR). Token, onay ekranında seçilen tek bir organizasyona ve tek bir
  uca bağlıdır (RFC 8707 `resource`). Başka ucun token'ı 401 alır.
- Keşif: `/.well-known/oauth-protected-resource/mcp/<uç>` (RFC 9728) ve `/.well-known/oauth-authorization-server`
  (RFC 8414).

## 2. İstemciyi bağlamak

Şifre veya API anahtarı paylaşılmaz. İstemciye yalnızca uç adresi verilir.

- **Claude (claude.ai, masaüstü, mobil):** Ayarlar > Bağlayıcılar > Özel bağlayıcı ekle. Adres olarak örneğin
  `https://olexfilms.com/mcp/dealer` yazılır ve Bağlan'a basılır. Açılan Olexfilms penceresinde giriş yapılır, organizasyon
  seçilir ve **İzin ver**'e basılır.
- **Claude Code:** `claude mcp add --transport http olexfilms https://olexfilms.com/mcp/user` komutu çalıştırılır, ardından
  oturumda `/mcp` ile izin verilir.
- **ChatGPT:** Ayarlar > Bağlayıcılar (Geliştirici modu) > Yeni bağlayıcı. MCP sunucu adresi girilir, kimlik doğrulama
  olarak OAuth seçilir. Aynı Olexfilms onay ekranı açılır.
- **Cursor / VS Code ve diğerleri:** Aynı adres girilir. İstemci "Streamable HTTP + OAuth" desteklemelidir.

Bağlantılar panelde **Bağlı uygulamalar** ekranından tek tıkla kaldırılır (`/v1/oauth/grants`, müşteri için
`/v1/portal/oauth/grants`). Platform yöneticisi kayıtlı tüm istemcileri **MCP istemcileri** ekranından engelleyebilir.

## 3. Erişim kuralları

Her HTTP isteğinde sırasıyla şu kontroller yapılır:

1. **Bearer token.** Token yoksa, geçersizse, süresi dolmuşsa ya da başka ucun token'ıysa yanıt `401` olur. Yanıtta
   `WWW-Authenticate: Bearer resource_metadata="…/.well-known/oauth-protected-resource/mcp/<uç>"` başlığı bulunur. Geçersiz
   token'da başlığa `error="invalid_token"` da eklenir. İstemci bu başlıkla OAuth akışını başlatır.
2. **`mcp` modülü.** Token'ın organizasyonunda `mcp` modülü kapalıysa yanıt `403` olur. Modül kapatıldığında bir sonraki
   çağrı da reddedilir.
3. **Kullanıcının güncel yetkisi.** Rol ve izinler her istekte yeniden okunur. Üyelik veya `mcp.connect` izni kaldırılmışsa,
   organizasyon askıya alınmışsa ya da erişim süresi dolmuşsa yanıt `403` olur. `/mcp/dealer` yalnız distribütör ve bayi
   organizasyonlarında çalışır.
4. **İstek limitleri.** Her bağlantı (token ailesi) dakikada en çok **60** istek yapabilir. Organizasyonun bütün bağlantıları
   toplamda saatte en çok **`mcp.requests_per_hour_per_org`** istek yapabilir. Bu sistem ayarının varsayılanı 600'dür ve
   Platform > Sistem ayarları > MCP bağlantıları bölümünden değiştirilir. Limit aşılınca yanıt HTTP `429` olur; gövdede
   JSON-RPC hatası (`code: -32029`) ve `Retry-After` başlığı bulunur.
5. **`tools/list`.** Tool listesi AI tool kayıt defterinin `Available(principal)` sonucudur. Principal, token'ın kullanıcısı,
   organizasyonu ve ucun realm'inden oluşur. İzin, organizasyon türü, modül, platform tool anahtarı ve marka (K20) burada
   uygulanır. Sunucu `listChanged: false` bildirir. Yetki değişikliği istemcinin bir sonraki oturumunda listeye yansır.
6. **`tools/call`.** Aynı kontrol çağrı anında tekrar yapılır. Listede olmayan ya da artık izin verilmeyen bir tool çağrılırsa
   tool sonucu `isError: true` + `TOOL_NOT_ALLOWED` olur.

AI token kotası MCP'de uygulanmaz, çünkü model istemci tarafında çalışır. Sınırlama yalnızca istek limitleriyle yapılır.

## 4. Tool çıktısı

- Tool açıklamaları İngilizcedir (model için yazılmıştır).
- Başarılı sonuçta önce kısa bir metin gelir (ör. `list_leads: 10 of 42 items.`), ardından JSON metni gelir. Aynı JSON
  `structuredContent` alanında da bulunur.
- Hatalı sonuçta `isError: true` olur ve `structuredContent.error.code` şu kodlardan biridir: `TOOL_NOT_ALLOWED`,
  `INVALID_INPUT`, `NOT_FOUND`, `RESULT_TOO_LARGE`, `TOOL_FAILED`. Kapsam dışındaki bir kayıt "yok" ile ayırt edilmez
  (`NOT_FOUND`).

## 5. Yazma tool'ları: panelden onay

Veri değiştiren tool'lar (`create_task`, `create_lead`, `set_lead_follow_up`, `create_appointment`,
`cancel_appointment`, `create_order_draft`, `add_service_note`) MCP'den **doğrudan çalışmaz**:

1. Girdi şemaya göre doğrulanır, başvurulan kayıtlar çözülür ve bir önizleme hazırlanır. Bu aşamada hiçbir kayıt yazılmaz.
2. `ai_pending_actions` tablosuna `source = mcp` olan bir satır eklenir. Satır 30 dakika geçerlidir ve `source_ref` OAuth
   token ailesidir.
3. Tool sonucu "panelden onay bekleniyor" metnini, işlemin özetini ve onay linkini döner
   (`/t/<org>/ai/approvals?action=<uuid>`). `structuredContent.status` değeri `PENDING_APPROVAL` olur.
4. Kullanıcı işlemi panelde **Onay bekleyen AI işlemleri** ekranında onaylar veya iptal eder (F4-03d). Onay sırasında izin,
   modül ve kapsam yeniden kontrol edilir. İşlemi yalnız başlatan kullanıcı onaylayabilir.

MCP istemcilerinin elicitation (istemcide onay sorma) özelliği bu fazda kullanılmaz. Onay yalnız panelden verilir.

## 6. Kayıt (activity log)

Her `tools/call` için `mcp.tool_called` olayı yazılır. Kaynak `mcp_tool`, aktör token'ın kullanıcısıdır. Olayda şu alanlar
bulunur: `tool`, `endpoint`, `client_id`, `organization_id`, `duration_ms`, `result` (`ok`, `error`, `not_allowed`,
`pending_approval`), varsa `code` ve `ai_action_uuid`, ve maskeli `arguments`.

Argümanlarda kişisel veri maskelenir (`***`). Adı `name`, `phone`, `email`, `plate`, `tckn`, `address`, `note`, `message`,
`text`, `description`, `vin`, `iban`, `query`, `q`, `contact` ve benzerlerini içeren alanların değeri maskelenir. Diğer
alanlarda geçen e-posta adresleri ve telefon numaraları da maskelenir. Uzun metinler 64 karakterde kesilir. UUID, tarih,
durum, sayı ve hizmet numarası okunur kalır.

## 7. Tool listesi

Ayrıntılı tablolar (usecase, izin, modül) [`docs/ai.md`](ai.md) dosyasındadır. Bir kullanıcının gördüğü liste kendi izin ve
modüllerine göre daralır.

**`/mcp/dealer` ve `/mcp/user` (panel realm'i)**

| Tür | Tool'lar |
|---|---|
| Okuma | `search_services`, `get_service`, `service_activity_summary`, `service_pdf_link`, `lookup_warranties`, `search_customers`, `stock_summary`, `stock_units`, `list_orders`, `get_order`, `balance_summary`, `list_appointments`, `list_leads`, `my_tasks` (yalnız merkez), `search_products`, `list_sub_organizations` (merkez / distribütör) |
| Yazma (panelden onay) | `create_task`, `create_lead`, `set_lead_follow_up`, `create_appointment`, `cancel_appointment`, `create_order_draft`, `add_service_note` |

**`/mcp/customer` (müşteri realm'i)**

| Tür | Tool'lar |
|---|---|
| Okuma | `my_vehicles`, `my_services`, `my_service_detail`, `my_service_pdf_link`, `my_warranties`, `my_profile_link`, `dealer_review_link`, `my_appointments`, `my_warranty_claims` |
| Kişisel tercih (onaysız) | `change_language` |

Ziyaretçi tool'ları (`recommend_products`, `find_nearest_dealers`, `search_knowledge`, `lookup_warranty`) yalnız WhatsApp
ziyaretçi akışında kullanılır. Kimliksiz MCP ucu yoktur.

## 8. Eski uç

Eski hub'daki kimliksiz `/mcp/olex` ucu (olexfilms `app/Mcp/Servers/OlexSupportServer.php`, 12 tool) bu uçlarla
değiştirilir. Eşleştirme [`docs/ai.md`](ai.md) §6 parite tablosundadır. Telefonla kimlik sorgusu (`PhoneLookupTool`) bilinçli
olarak taşınmamıştır. Eski uç, eski sistem kapatılırken (F4-05) kaldırılır.
