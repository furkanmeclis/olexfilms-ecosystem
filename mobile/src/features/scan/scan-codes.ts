/**
 * What a scanned code is. The app decides, so the user never picks a mode.
 * Formats: QR web sign-in `olexfilms://qr-login/<code>`, warehouse labels
 * `OFW:LOC:<full_code>` / `OFW:UNIT:<barcode>` (POST /v1/warehouse/scan),
 * public warranty links `https://<host>/warranty/<no>` (old `/garanti/` too).
 */
export type ScannedCode =
  | { kind: 'qr_login'; code: string }
  | { kind: 'location'; code: string }
  | { kind: 'unit'; code: string }
  | { kind: 'warranty'; code: string }
  | { kind: 'unknown'; code: string };

const QR_LOGIN = /^olexfilms:\/\/qr-login\/([^/?#\s]+)/i;
const WARRANTY = /\/(?:warranty|garanti)\/([A-Za-z0-9_-]{4,32})(?:[/?#]|$)/i;

export function classifyCode(raw: string): ScannedCode {
  const v = raw.trim();
  const qr = v.match(QR_LOGIN);
  if (qr) return { kind: 'qr_login', code: decodeURIComponent(qr[1]) };
  if (/^OFW:LOC:/i.test(v)) return { kind: 'location', code: v.slice(8) };
  if (/^OFW:UNIT:/i.test(v)) return { kind: 'unit', code: v.slice(9) };
  const w = v.match(WARRANTY);
  if (w) return { kind: 'warranty', code: decodeURIComponent(w[1]) };
  // A bare unit barcode, SKU or short code: the warehouse scan endpoint resolves it.
  return { kind: /^[A-Z0-9-]{4,40}$/i.test(v) ? 'unit' : 'unknown', code: v };
}
