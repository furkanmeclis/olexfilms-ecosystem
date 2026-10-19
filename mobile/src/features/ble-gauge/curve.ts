/**
 * SmartPTG characteristic curve (pure, alias-free).
 *
 * The gauge only reports a frequency; each serial has its own factory curve
 * `{SN}.dat` (frequency ↔ thickness pairs for Fe and Al). The file is
 * `IV:Base64(AES-128-CBC(csv))`, keyed with the NexPTG app's constant.
 * Thickness = linear interpolation of (frequency + calibration offset) on it.
 */
import * as aesNamespace from 'aes-js';

import type { GaugeFrame } from './protocol.ts';

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
  /** Header thresholds K (Fe) and L (Al). */
  feThreshold: number;
  alThreshold: number;
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

/**
 * Decrypted CSV → curve. Header `K,L,deviceType,X,serial,…`, then rows
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
  for (const line of lines.slice(1)) {
    if (!/^\d/u.test(line)) continue;
    const c = line.split(',').map((x) => Number(x.trim()));
    if (c.length < 4 || c.some((x) => !Number.isFinite(x))) continue;
    fe.push({ thickness: c[0]!, frequency: c[1]! });
    al.push({ thickness: c[2]!, frequency: c[3]! });
  }
  if (fe.length < 2 || al.length < 2) throw new Error('curve_empty');
  return {
    serial,
    deviceType: Number.isFinite(header[2]) ? header[2]! : -1,
    feThreshold: header[0] ?? 0,
    alThreshold: header[1] ?? 0,
    fe,
    al,
  };
}

/**
 * NexPTGService.c: frequency (+ offset) → microns by linear interpolation on
 * the material's curve. Null when outside the curve (probe in the air, wrong
 * material or off-scale).
 */
export function frequencyToMicrons(
  material: 'fe' | 'al',
  frequency: number,
  curve: GaugeCurve,
  offsets?: CurveOffsets | null,
): number | null {
  const f = frequency + (material === 'fe' ? (offsets?.fe ?? 0) : (offsets?.al ?? 0));
  const points = material === 'fe' ? curve.fe : curve.al;
  for (let i = 0; i < points.length - 1; i += 1) {
    const a = points[i]!;
    const b = points[i + 1]!;
    if (f < Math.min(a.frequency, b.frequency) || f > Math.max(a.frequency, b.frequency)) continue;
    if (b.frequency === a.frequency) return Math.round(a.thickness);
    const um = a.thickness + ((f - a.frequency) * (b.thickness - a.thickness)) / (b.frequency - a.frequency);
    return Math.max(0, Math.round(um));
  }
  return null;
}

export type GaugeReading = {
  /** Thickness in microns; null while the probe is in the air / off-curve / curve not loaded. */
  um: number | null;
  material: 'fe' | 'al' | 'unknown';
  frequency: number;
  serial: number;
  batteryVoltage: number;
  /** Epoch ms of reception. */
  at: number;
};

export function frameToReading(
  frame: GaugeFrame,
  curve: GaugeCurve | null,
  offsets: CurveOffsets | null,
  at: number,
): GaugeReading {
  const um =
    curve && curve.serial === frame.serial && frame.material !== 'unknown'
      ? frequencyToMicrons(frame.material, frame.frequency, curve, offsets)
      : null;
  return {
    um,
    material: frame.material,
    frequency: frame.frequency,
    serial: frame.serial,
    batteryVoltage: frame.batteryVoltage,
    at,
  };
}

