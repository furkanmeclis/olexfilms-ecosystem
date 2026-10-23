/**
 * SmartPTG characteristic curve (pure, alias-free).
 *
 * The gauge only reports a frequency; each serial has its own factory curve
 * `{SN}.dat` (frequency ↔ thickness pairs for Fe and Al). The file is
 * `IV:Base64(AES-128-CBC(csv))`, keyed with the NexPTG app's constant.
 * Thickness = linear interpolation of (frequency + calibration offset) on it.
 */
import * as aesNamespace from 'aes-js';

type AesJs = typeof aesNamespace;
// CommonJS UMD: Metro/Hermes and Node expose it either as default or as the namespace.
const aesjs: AesJs = (aesNamespace as unknown as { default?: AesJs }).default ?? aesNamespace;

/** NexPTG `guarana` resource; the first 16 bytes are the AES key. */
const CURVE_KEY = '436a1246dbe284b75dc8f4729a962647';

export type CurvePoint = { thickness: number; frequency: number };

export type GaugeCurve = {
  serial: number;
  /** NexPTGConfig.DeviceType (0 Advanced, 1 Standard, 2 Professional, …). */
  deviceType: number;
  /** Header thresholds K (Fe channel) and L (Al channel): the probe in the air. */
  feThreshold: number;
  alThreshold: number;
  /** Header field 4 (NexPTGService.X → the frame's `lang`), null when absent. */
  lang: number | null;
  /** Header fields 6–9: factory frequencies of the zinc (galvanised) model, null when absent. */
  zn: { t: number; u: number; v: number; w: number } | null;
  /** `cal1` / `cal2` / `cal3` somewhere in the file: 90 µm calibration foil (else 150 µm). */
  calVersion: 'cal1' | 'cal2' | 'cal3' | null;
  /** The quoted comment line after the table (author, calX, date), null when absent. */
  comment: string | null;
  fe: CurvePoint[];
  al: CurvePoint[];
};

/** Per-serial frequency corrections (Hz) from a calibration, 0 when never calibrated. */
export type CurveOffsets = { fe: number; al: number };

const DEVICE_TYPES: Record<number, string> = {
  0: 'SmartPTG Advanced',
  1: 'SmartPTG Standard',
  2: 'SmartPTG Professional',
  3: 'SmartPTG Economic',
  4: 'SmartPTG Economic Plus',
  5: 'SmartPTG Standard Plus',
  6: 'SmartPTG Pro Carbon',
};

export function deviceModelName(deviceType: number | null | undefined): string {
  return (deviceType != null && DEVICE_TYPES[deviceType]) || 'SmartPTG';
}

function latin1Bytes(text: string): Uint8Array {
  const out = new Uint8Array(text.length);
  for (let i = 0; i < text.length; i += 1) out[i] = text.charCodeAt(i) & 0xff;
  return out;
}

function stripPkcs7(data: Uint8Array): Uint8Array {
  const pad = data[data.length - 1] ?? 0;
  if (pad < 1 || pad > 16 || pad > data.length) return data;
  for (let i = data.length - pad; i < data.length; i += 1) if (data[i] !== pad) return data;
  return data.subarray(0, data.length - pad);
}

/** `IV:Base64Cipher` → CSV plaintext. Throws on a malformed file. */
export function decryptCurveFile(raw: string): string {
  const trimmed = raw.trim();
  const colon = trimmed.indexOf(':');
  if (colon < 1) throw new Error('curve_format');
  const iv = latin1Bytes(trimmed.slice(0, colon).slice(0, 16));
  if (iv.length !== 16) throw new Error('curve_format');
  const cipher = latin1Bytes(globalThis.atob(trimmed.slice(colon + 1).trim()));
  if (cipher.length === 0 || cipher.length % 16 !== 0) throw new Error('curve_format');
  const cbc = new aesjs.ModeOfOperation.cbc(latin1Bytes(CURVE_KEY.slice(0, 16)), iv);
  const plain = stripPkcs7(cbc.decrypt(cipher));
  let text = '';
  for (let i = 0; i < plain.length; i += 1) text += String.fromCharCode(plain[i]!);
  return text;
}

/** Test helper: the inverse of decryptCurveFile. */
export function encryptCurveFile(plaintext: string, iv: string): string {
  const bytes = latin1Bytes(plaintext);
  const pad = 16 - (bytes.length % 16);
  const padded = new Uint8Array(bytes.length + pad);
  padded.set(bytes);
  padded.fill(pad, bytes.length);
  const cbc = new aesjs.ModeOfOperation.cbc(latin1Bytes(CURVE_KEY.slice(0, 16)), latin1Bytes(iv.slice(0, 16)));
  const cipher = cbc.encrypt(padded);
  let bin = '';
  for (let i = 0; i < cipher.length; i += 1) bin += String.fromCharCode(cipher[i]!);
  return `${iv}:${globalThis.btoa(bin)}`;
}

/** `cal1` / `cal2` / `cal3` anywhere in the plain text (NexPTGService.l uses str.contains). */
export function curveCalVersion(plaintext: string): GaugeCurve['calVersion'] {
  for (const v of ['cal3', 'cal2', 'cal1'] as const) if (plaintext.includes(v)) return v;
  return null;
}

/**
 * Decrypted CSV → curve (NexPTGService.l, REVERSE_ENGINEERING §5.3). Header
 * `K,L,deviceType,lang,serial[,T,U,V,W]`, then rows
 * `feThick,feFreq,alThick,alFreq`, then a quoted comment line.
 */
export function parseCurve(plaintext: string, expectedSerial?: number): GaugeCurve {
  const lines = plaintext
    .split(/\r?\n/u)
    .map((l) => l.trim())
    .filter(Boolean);
  if (lines.length < 3) throw new Error('curve_empty');
  const header = lines[0]!.split(',').map((c) => Number(c.trim()));
  const serial = header[4]!;
  if (!Number.isFinite(serial)) throw new Error('curve_format');
  if (expectedSerial != null && serial !== expectedSerial) throw new Error('curve_serial');

  const fe: CurvePoint[] = [];
  const al: CurvePoint[] = [];
  let comment: string | null = null;
  for (const line of lines.slice(1)) {
    if (!/^\d/u.test(line)) {
      comment = line;
      continue;
    }
    const c = line.split(',').map((x) => Number(x.trim()));
    if (c.length < 4 || c.some((x) => !Number.isFinite(x))) continue;
    fe.push({ thickness: c[0]!, frequency: c[1]! });
    al.push({ thickness: c[2]!, frequency: c[3]! });
  }
  if (fe.length < 2 || al.length < 2) throw new Error('curve_empty');
  const zn = header.length >= 9 && header.slice(5, 9).every(Number.isFinite)
    ? { t: header[5]!, u: header[6]!, v: header[7]!, w: header[8]! }
    : null;
  return {
    serial,
    deviceType: Number.isFinite(header[2]) ? header[2]! : -1,
    feThreshold: header[0] ?? 0,
    alThreshold: header[1] ?? 0,
    lang: Number.isFinite(header[3]) ? header[3]! : null,
    zn,
    calVersion: curveCalVersion(plaintext),
    comment,
    fe,
    al,
  };
}

/* ------------------------------------------------------------ device rules */

/**
 * Per device type limits (NexPTGService.a, §5.4): Standard / Economic /
 * Economic Plus / Standard Plus round to 10 µm and stop at 1000 µm;
 * Advanced 2200, Professional 2500, Pro Carbon 3000 (whole microns);
 * Economic (3, 4) has no Al measurement.
 */
export function deviceRules(deviceType: number): { roundTo: number; maxUm: number; measuresAl: boolean } {
  const tenUm = deviceType === 1 || deviceType === 3 || deviceType === 4 || deviceType === 5;
  const maxUm = deviceType === 0 ? 2200 : deviceType === 2 ? 2500 : deviceType === 6 ? 3000 : tenUm ? 1000 : 2500;
  return { roundTo: tenUm ? 10 : 1, maxUm, measuresAl: deviceType !== 3 && deviceType !== 4 };
}

/** Applies deviceRules to a curve value: null (no value) above the limit or for Al on an Economic. */
export function applyDeviceRules(um: number | null, material: Material, deviceType: number): number | null {
  if (um == null) return null;
  const rules = deviceRules(deviceType);
  if (material === 'al' && !rules.measuresAl) return null;
  const v = rules.roundTo > 1 ? Math.round(um / rules.roundTo) * rules.roundTo : um;
  return v > rules.maxUm ? null : v;
}

/* ---------------------------------------------------------------- material */

export type Material = 'fe' | 'al';

/**
 * Substrate of a frame (NexPTGService.a `fe_or_al`, §4.1): on the Fe channel a
 * frequency above K means aluminium, on the Al channel one at or above L.
 */
export function materialOf(channel: Material, frequency: number, curve: Pick<GaugeCurve, 'feThreshold' | 'alThreshold'>): Material {
  return channel === 'fe' ? (frequency > curve.feThreshold ? 'al' : 'fe') : frequency >= curve.alThreshold ? 'al' : 'fe';
}

/**
 * NexPTGService.c (§5.4): frequency (+ calibration offset) → microns by linear
 * interpolation on the material's curve, before the device rules.
 * Null = the probe is in the air: on Fe above the last (thickest) frequency,
 * on Al below it. A frequency the table does not cover otherwise (below the
 * first Fe point, above the first Al point: bare metal) is 0 µm.
 */
export function frequencyToMicrons(
  material: Material,
  frequency: number,
  curve: GaugeCurve,
  offsets?: CurveOffsets | null,
): number | null {
  const f = frequency + (material === 'fe' ? (offsets?.fe ?? 0) : (offsets?.al ?? 0));
  const points = material === 'fe' ? curve.fe : curve.al;
  const last = points[points.length - 1]!;
  if (material === 'fe' ? f > last.frequency : f < last.frequency) return null;
  for (let i = 0; i < points.length - 1; i += 1) {
    const a = points[i]!;
    const b = points[i + 1]!;
    // Closed segments: the interpolation is continuous, so the app's half-open ones give the same value.
    if (f < Math.min(a.frequency, b.frequency) || f > Math.max(a.frequency, b.frequency)) continue;
    if (b.frequency === a.frequency) return Math.round(a.thickness);
    const um = a.thickness + ((f - a.frequency) * (b.thickness - a.thickness)) / (b.frequency - a.frequency);
    return Math.max(0, Math.round(um));
  }
  return 0;
}

export type GaugeReading = {
  /** Thickness in microns; null while the probe is in the air / off-scale / curve not loaded. */
  um: number | null;
  /** Substrate the value was computed for (null without a curve). */
  material: Material | null;
  /** Probe channel of the frame. */
  channel: Material;
  /**
   * The frame came from the other channel than the selected substrate: the
   * value is the previous one, not recomputed (§4.2). The sampler skips it.
   */
  held: boolean;
  /** The gauge's button was pressed on the selected substrate's channel (frame type 3 / 4). */
  button: boolean;
  frequency: number;
  serial: number;
  batteryVoltage: number;
  /** Epoch ms of reception. */
  at: number;
};

/**
 * Where a gauge's factory curve comes from (pure, alias-free; TEC-934).
 *
 * 1. Our backend, `GET /v1/measurements/devices/smartptg/curves/{serial}`
 *    (through the mobile mirror): it fetches the vendor file server side and
 *    keeps it, so a vendor outage or its broken HTTPS does not matter.
 * 2. The curves bundled with the app (SN 32475).
 *
 * The vendor URLs are no longer fetched from the phone: over HTTPS both fail
 * (mysdprog.pl 404, smartptg.pl certificate mismatch) and plain http is
 * blocked by ATS / cleartext policy, so they only cost a timeout.
 */

/**
 * Answer of the backend curve route: the raw `.dat` text with its ETag,
 * `not_modified` (304 for the ETag sent), or `not_found` (404).
 */
export type CurveResponse = { status: 'ok'; text: string; etag: string | null } | { status: 'not_modified' } | { status: 'not_found' };

/** Asks the backend for a serial's curve, conditional on `etag` when given; throws on network / 5xx. */
export type CurveFetcher = (serial: number, etag: string | null) => Promise<CurveResponse>;

/** A curve kept on the phone, with the backend ETag it came with (null: bundled / unknown). */
export type StoredCurve = { curve: GaugeCurve; etag: string | null };

export type CurveResult = StoredCurve & {
  /** backend: a new or changed file; cache: the stored copy (304, offline or backend failing); bundled: shipped with the app. */
  source: 'backend' | 'cache' | 'bundled';
};

/**
 * Backend first, conditional on the stored copy's ETag (a network error is
 * retried once), so a factory update reaches the phone on the next connect
 * (the NexPTG app downloads the curve on every connect, §5.1). Offline, on
 * 404 or a bad file the stored copy is used, then the bundled curve.
 * `online: false` skips the request. Throws the last error when nothing has
 * the serial.
 */
export async function resolveCurve(
  serial: number,
  fetchCurve: CurveFetcher,
  bundled: Record<number, GaugeCurve>,
  stored: StoredCurve | null = null,
  online = true,
): Promise<CurveResult> {
  const cached = stored?.curve.serial === serial ? stored : null;
  let lastError: unknown = new Error('curve_not_found');
  for (let attempt = 0; online && attempt < 2; attempt += 1) {
    let res: CurveResponse;
    try {
      res = await fetchCurve(serial, cached?.etag ?? null);
    } catch (e) {
      lastError = e;
      continue; // network / 5xx: retry once
    }
    if (res.status === 'not_modified' && cached) return { ...cached, source: 'cache' };
    if (res.status !== 'ok') break; // 404 (or a 304 without a stored copy)
    try {
      return { curve: parseCurve(decryptCurveFile(res.text), serial), etag: res.etag, source: 'backend' };
    } catch (e) {
      lastError = e; // corrupt or another serial's file
      break;
    }
  }
  if (cached) return { ...cached, source: 'cache' };
  const local = bundled[serial];
  if (local) return { curve: local, etag: null, source: 'bundled' };
  throw lastError;
}
