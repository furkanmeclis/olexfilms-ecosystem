// QR matrisini 1-bit gri PNG'ye çevirir. Saf dosya: sıkıştırma yok, deflate "stored" blokları.
import { encodeQr } from '../services/qr.ts';

const QUIET_MODULES = 4;
const TARGET_PX = 276;
const MAX_STORED = 0xffff;

let crcTable: Uint32Array | null = null;

export function crc32(bytes: Uint8Array): number {
  if (!crcTable) {
    crcTable = new Uint32Array(256);
    for (let n = 0; n < 256; n++) {
      let c = n;
      for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
      crcTable[n] = c >>> 0;
    }
  }
  let crc = 0xffffffff;
  for (let i = 0; i < bytes.length; i++) crc = crcTable[(crc ^ bytes[i]) & 0xff] ^ (crc >>> 8);
  return (crc ^ 0xffffffff) >>> 0;
}

function adler32(bytes: Uint8Array): number {
  let a = 1;
  let b = 0;
  for (let i = 0; i < bytes.length; i++) {
    a = (a + bytes[i]) % 65521;
    b = (b + a) % 65521;
  }
  return ((b << 16) | a) >>> 0;
}

function zlibStored(raw: Uint8Array): Uint8Array {
  const blocks = Math.max(1, Math.ceil(raw.length / MAX_STORED));
  const out = new Uint8Array(2 + raw.length + blocks * 5 + 4);
  const view = new DataView(out.buffer);
  out[0] = 0x78;
  out[1] = 0x01;
  let pos = 2;
  for (let i = 0; i < blocks; i++) {
    const start = i * MAX_STORED;
    const len = Math.min(MAX_STORED, raw.length - start);
    out[pos] = i === blocks - 1 ? 1 : 0;
    view.setUint16(pos + 1, len, true);
    view.setUint16(pos + 3, ~len & 0xffff, true);
    out.set(raw.subarray(start, start + len), pos + 5);
    pos += 5 + len;
  }
  view.setUint32(pos, adler32(raw));
  return out;
}

function chunk(type: string, data: Uint8Array): Uint8Array {
  const out = new Uint8Array(12 + data.length);
  const view = new DataView(out.buffer);
  view.setUint32(0, data.length);
  for (let i = 0; i < 4; i++) out[4 + i] = type.charCodeAt(i);
  out.set(data, 8);
  view.setUint32(8 + data.length, crc32(out.subarray(4, 8 + data.length)));
  return out;
}

/** `matrix` (true = koyu) → 4 modül sessiz alanlı, 1-bit gri PNG baytı. */
export function qrPng(matrix: boolean[][]): Uint8Array {
  const n = matrix.length;
  const modules = n + QUIET_MODULES * 2;
  const scale = Math.ceil(TARGET_PX / modules);
  const size = modules * scale;
  const stride = Math.ceil(size / 8) + 1;
  // 1 = beyaz; her satır filtre baytı 0 ile başlar, artan bitler beyaz kalır.
  const raw = new Uint8Array(stride * size).fill(0xff);
  for (let y = 0; y < size; y++) {
    const row = y * stride;
    raw[row] = 0;
    const my = Math.floor(y / scale) - QUIET_MODULES;
    if (my < 0 || my >= n) continue;
    for (let x = 0; x < size; x++) {
      const mx = Math.floor(x / scale) - QUIET_MODULES;
      if (mx >= 0 && mx < n && matrix[my][mx]) raw[row + 1 + (x >> 3)] &= ~(0x80 >> (x & 7));
    }
  }

  const ihdr = new Uint8Array(13);
  const view = new DataView(ihdr.buffer);
  view.setUint32(0, size);
  view.setUint32(4, size);
  ihdr[8] = 1; // bit derinliği
  ihdr[9] = 0; // gri

  const parts = [
    Uint8Array.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', ihdr),
    chunk('IDAT', zlibStored(raw)),
    chunk('IEND', new Uint8Array(0)),
  ];
  const png = new Uint8Array(parts.reduce((sum, p) => sum + p.length, 0));
  let pos = 0;
  for (const p of parts) {
    png.set(p, pos);
    pos += p.length;
  }
  return png;
}

/** `publicWarrantyUrl(code)` gibi bir metinden QR PNG'si; metin dışarıdan verilir. */
export function qrPngFromText(text: string): Uint8Array {
  return qrPng(encodeQr(text));
}
