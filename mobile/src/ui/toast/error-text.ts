/**
 * Başarısız bir çağrının kullanıcıya gösterilecek metni (TEC-1235). Uygulamanın
 * mevcut eşlemesiyle aynı sıra (`errorMessage`, `whError`, `failMessage`):
 * bağlantı yoksa ağ metni, özelliğin kendi kataloğu, web'in `errors:codes.*`
 * kataloğu, en son genel metin. Ham teknik kod hiçbir zaman gösterilmez.
 *
 * i18n'e doğrudan bağlanmaz (çevirici dışarıdan verilir) ki `node --test` ile
 * react-native yüklenmeden test edilebilsin.
 */

export type Translator = { exists(key: string): boolean; t(key: string): string };

export type ErrorTextOptions = {
  /** Önce bakılacak özellik katalogları, ör. `['warehouse']` → `warehouse:errors.<KOD>`. */
  namespaces?: string[];
  /** Kod bilinmiyorsa genel metin yerine bu. */
  fallback?: string;
};

/** ApiError'ın kodu (sınıfı içe aktarmadan: `name === 'ApiError'`). */
export function apiErrorCode(e: unknown): string | undefined {
  if (!(e instanceof Error) || e.name !== 'ApiError') return undefined;
  const code = (e as { code?: unknown }).code;
  return typeof code === 'string' && code ? code : undefined;
}

export function describeError(e: unknown, tr: Translator, opts: ErrorTextOptions = {}): string {
  const fallback = opts.fallback ?? tr.t('common:error_generic');
  const code = apiErrorCode(e);
  if (!code) return fallback;
  if (code === 'NETWORK_ERROR') return tr.t('common:error_network');
  for (const ns of opts.namespaces ?? []) {
    const own = `${ns}:errors.${code}`;
    if (tr.exists(own)) return tr.t(own);
  }
  const key = `errors:codes.${code}`;
  return tr.exists(key) ? tr.t(key) : fallback;
}
