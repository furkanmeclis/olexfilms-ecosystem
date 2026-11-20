# Bağlantı paketi ortak fixture'ı (v1)

Bu klasör iki repoda birebir aynıdır:

- glorian: `tests/Fixtures/inventory-connection-package/v1/`
- olexfilms-ecosystem: `backend/internal/modules/integrations/glorianadmin/usecase/testdata/connection-package/v1/`

İçerik tamamen sahtedir; gerçek anahtar ya da şifre değildir.

| Dosya | Ne |
|---|---|
| `package.json` | Şifreli paket (dış zarf). Tuz `00..0f`, nonce `10..1b` sabit. |
| `payload.json` | Paketin çözülmüş içeriği, bayt bayt (sondaki satır sonu yok). |
| `password.txt` | Paket şifresi (sondaki satır sonu yok). |

İki taraf da şunu doğrular:

1. `package.json` + `password.txt` çözülünce `payload.json` baytları çıkar.
2. `payload.json` aynı tuz ve nonce ile şifrelenince `package.json` içindeki `ciphertext` birebir çıkar.

Biçim: glorian `docs/inventory-api.md` §1.2, ekosistem `docs/design.md` (Glorian bağlantı paketi).
