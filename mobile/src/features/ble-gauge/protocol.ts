/**
 * SmartPTG / NexPTG paint thickness gauge: BLE wire protocol (pure, alias-free).
 *
 * The gauge speaks a Nordic UART style service: the app subscribes to RX
 * notifications and receives ASCII lines like `eVo1AFBAADA54F0BE8\r\n`.
 * A line carries the probe frequency (Hz), the frame type, the battery
 * voltage and the device serial; microns are NOT in the frame, they come
 * from the serial's characteristic curve (see curve.ts).
 *
 * The frame type is the probe CHANNEL, not the substrate (REVERSE_ENGINEERING
 * §3.3): 1 = Fe channel, 2 = Al channel, 3 / 4 = the same channels while the
 * gauge's button ("save") is pressed. The substrate is decided from the
 * frequency against the curve's K / L thresholds (measure.ts).
 *
 * Source: docs/smartptg/REVERSE_ENGINEERING.md (pl.nexptg 2.3.0).
 */

export const GAUGE_BLE = {
  serviceUUID: '6e400001-b5a3-f393-e0a9-e50e24dcca9e',
  /** App → gauge (WRITE): `!<mode>\r\n`. */
  txCharacteristicUUID: '6e400002-b5a3-f393-e0a9-e50e24dcca9e',
  /** Gauge → app (NOTIFY): `eVo…\r\n` lines. */
  rxCharacteristicUUID: '6e400003-b5a3-f393-e0a9-e50e24dcca9e',
  /** Advertised names the NexPTG app accepts (lower-case, spaces removed). */
  nameHints: ['smartptg', 'nexptg'],
  scanTimeoutMs: 15_000,
  /** The NexPTG app drops the link after 10 s without a frame ("AUTO DISCONNECT"). */
  noDataTimeoutMs: 10_000,
} as const;

/** Probe channel of a frame: the Fe or the Al (NFe) oscillator of the probe. */
export type Channel = 'fe' | 'al';

/** `!<mode>`: 1 = Fe channel only, 2 = Al channel only, 3 = both alternating (default). */
export const GAUGE_MODE = { fe: 1, al: 2, both: 3 } as const;

export type GaugeFrame = {
  /** Frame type digit: 1 / 3 = Fe channel, 2 / 4 = Al channel (3 / 4: button pressed). */
  type: number;
  /** Null for a type the app does not know (the frame is ignored). */
  channel: Channel | null;
  /** The gauge's button was pressed (types 3 and 4): the "save" event. */
  button: boolean;
  /** Probe oscillator frequency in Hz. */
  frequency: number;
  serial: number;
  batteryVoltage: number;
};

const HEX = /^[0-9a-f]{14}$/i;

/** Frame type → channel (NexPTGConfig.FRAME_FE / FRAME_AL, + 2 with the button). */
export function channelOfType(type: number): Channel | null {
  if (type === 1 || type === 3) return 'fe';
  if (type === 2 || type === 4) return 'al';
  return null;
}

/**
 * Battery voltage → percent, FrameDecoder.a of the NexPTG app (§3.4):
 * ≥ 2.80 V 100 %, then 90/80/…/30/20/15 % steps down to 1.80 V, else 10 %.
 */
export function batteryPercent(voltage: number): number {
  const cv = Math.round(voltage * 100);
  if (cv >= 280) return 100;
  const steps: [number, number][] = [
    [260, 90],
    [250, 80],
    [240, 70],
    [230, 60],
    [220, 50],
    [210, 40],
    [200, 30],
    [190, 20],
    [180, 15],
  ];
  for (const [above, pct] of steps) if (cv > above) return pct;
  return 10;
}

function nibble(c: string): number {
  return parseInt(c, 16);
}

/** Byte from the hex nibbles at index `hi` (high) and `lo` (low). */
function pair(s: string, hi: number, lo: number): number {
  return (nibble(s[hi]!) << 4) + nibble(s[lo]!);
}

/**
 * Decodes one `eVo` line (with or without CRLF). Returns null for anything
 * that is not a well-formed frame, so garbled notifications are dropped.
 */
export function decodeFrame(raw: string): GaugeFrame | null {
  const s = raw.replace(/[\r\n]+$/u, '').trim();
  if (s.length !== 18 || !s.startsWith('eVo')) return null;
  const typeChar = s.charCodeAt(3) - 48;
  if (typeChar < 0 || typeChar > 9 || !HEX.test(s.slice(4))) return null;

  // XOR chain of libauth.so: seven scrambled bytes → int[6].
  const pDe = pair(s, 0xe, 0xd);
  const pC9 = pair(s, 0x9, 0xc);
  const pA8 = pair(s, 0x8, 0xa);
  const p47 = pair(s, 0x7, 0x4);
  const p56 = pair(s, 0x6, 0x5);
  const pTf = pair(s, 0x10, 0x11);
  const pBf = pair(s, 0xb, 0xf);
  const a = [pTf ^ pBf ^ 0xa7, pTf ^ p56, p56 ^ p47, p47 ^ pA8, pA8 ^ pC9, pC9 ^ pDe];

  return {
    type: typeChar,
    channel: channelOfType(typeChar),
    button: typeChar === 3 || typeChar === 4,
    frequency: (a[1]! << 8) + a[2]!,
    serial: (a[3]! << 16) + (a[4]! << 8) + a[5]!,
    batteryVoltage: a[0]! / 10,
  };
}

/** Inverse of decodeFrame (tests and simulators). The free byte is fixed to 0. */
export function encodeFrame(f: { type: number; frequency: number; serial: number; batteryVoltage: number }): string {
  const a0 = Math.round(f.batteryVoltage * 10) & 0xff;
  const a1 = (f.frequency >> 8) & 0xff;
  const a2 = f.frequency & 0xff;
  const a3 = (f.serial >> 16) & 0xff;
  const a4 = (f.serial >> 8) & 0xff;
  const a5 = f.serial & 0xff;
  const pDe = 0;
  const pC9 = a5 ^ pDe;
  const pA8 = a4 ^ pC9;
  const p47 = a3 ^ pA8;
  const p56 = a2 ^ p47;
  const pTf = a1 ^ p56;
  const pBf = pTf ^ (a0 ^ 0xa7);
  const chars = ['e', 'V', 'o', String(f.type), ...Array<string>(14).fill('0')];
  const put = (hi: number, lo: number, v: number) => {
    chars[hi] = ((v >> 4) & 0xf).toString(16).toUpperCase();
    chars[lo] = (v & 0xf).toString(16).toUpperCase();
  };
  put(0xe, 0xd, pDe);
  put(0x9, 0xc, pC9);
  put(0x8, 0xa, pA8);
  put(0x7, 0x4, p47);
  put(0x6, 0x5, p56);
  put(0x10, 0x11, pTf);
  put(0xb, 0xf, pBf);
  return `${chars.join('')}\r\n`;
}

/**
 * Joins notification chunks into CRLF terminated lines. A frame may arrive
 * split over two notifications, or two frames in one.
 */
export function createLineSplitter(maxBuffer = 256) {
  let buffer = '';
  return {
    push(chunk: string): string[] {
      buffer += chunk;
      const parts = buffer.split(/\r?\n/u);
      buffer = parts.pop() ?? '';
      // A stream without line ends is noise; never let it grow without bound.
      if (buffer.length > maxBuffer) buffer = '';
      return parts.map((p) => p.trim()).filter(Boolean);
    },
    reset() {
      buffer = '';
    },
  };
}

/** Base64 notification value → binary (latin-1) string. */
export function base64ToBinary(value: string): string {
  return globalThis.atob(value);
}

/** `!<mode>\r\n` measuring mode command, base64 for a characteristic write. */
export function modeCommand(mode: number): string {
  return globalThis.btoa(`!${mode}\r\n`);
}

/** True when a scanned peripheral is a SmartPTG / NexPTG gauge (name or advertised service). */
export function isGaugePeripheral(name: string | null | undefined, serviceUUIDs?: string[] | null): boolean {
  const n = (name ?? '').replace(/\s+/gu, '').toLowerCase();
  if (n && GAUGE_BLE.nameHints.some((hint) => n.includes(hint))) return true;
  return (serviceUUIDs ?? []).some((u) => u.toLowerCase() === GAUGE_BLE.serviceUUID);
}
