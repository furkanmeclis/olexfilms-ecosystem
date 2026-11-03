import assert from 'node:assert/strict';
import { inflateSync } from 'node:zlib';
import { describe, it } from 'node:test';

import { encodeQr } from '../services/qr.ts';
import { crc32, qrPng, qrPngFromText } from './qr-png.ts';

const SIGNATURE = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];

type Chunk = { type: string; data: Uint8Array; crc: number; start: number };

function chunks(png: Uint8Array): Chunk[] {
  const view = new DataView(png.buffer, png.byteOffset, png.byteLength);
  const out: Chunk[] = [];
  let pos = 8;
  while (pos < png.length) {
    const len = view.getUint32(pos);
    const type = String.fromCharCode(...png.subarray(pos + 4, pos + 8));
    out.push({
      type,
      data: png.subarray(pos + 8, pos + 8 + len),
      crc: view.getUint32(pos + 8 + len),
      start: pos + 4,
    });
    pos += 12 + len;
  }
  return out;
}

const URL = 'https://olexfilms.com/garanti/ABC-123';

/** Satırları (filtre baytı atılmış) 1-bit piksel dizisine açar: true = siyah. */
function decode(png: Uint8Array) {
  const all = chunks(png);
  const ihdr = new DataView(all[0].data.buffer, all[0].data.byteOffset, 13);
  const width = ihdr.getUint32(0);
  const height = ihdr.getUint32(4);
  const idat = Buffer.concat(all.filter((c) => c.type === 'IDAT').map((c) => c.data));
  const raw = inflateSync(idat);
  const stride = Math.ceil(width / 8) + 1;
  const px: boolean[][] = [];
  for (let y = 0; y < height; y++) {
    assert.equal(raw[y * stride], 0, 'filter 0');
    const row: boolean[] = [];
    for (let x = 0; x < width; x++) {
      const bit = (raw[y * stride + 1 + (x >> 3)] >> (7 - (x & 7))) & 1;
      row.push(bit === 0);
    }
    px.push(row);
  }
  return { width, height, px, raw, stride };
}

describe('qrPng', () => {
  const matrix = encodeQr(URL);
  const n = matrix.length;
  const png = qrPng(matrix);
  const scale = Math.ceil(276 / (n + 8));

  it('PNG imzası ve IHDR 1-bit gri', () => {
    assert.deepEqual([...png.subarray(0, 8)], SIGNATURE);
    const all = chunks(png);
    assert.deepEqual(all.map((c) => c.type), ['IHDR', 'IDAT', 'IEND']);
    const d = new DataView(all[0].data.buffer, all[0].data.byteOffset, 13);
    const size = (n + 8) * scale;
    assert.equal(d.getUint32(0), size);
    assert.equal(d.getUint32(4), size);
    assert.equal(all[0].data[8], 1);
    assert.equal(all[0].data[9], 0);
    assert.equal(all[0].data[10], 0);
    assert.equal(all[0].data[11], 0);
    assert.equal(all[0].data[12], 0);
  });

  it('ölçek en az 276 px verir', () => {
    assert.ok((n + 8) * scale >= 276);
    assert.ok((n + 8) * (scale - 1) < 276);
  });

  it('CRC her yığın için doğrulanır', () => {
    for (const c of chunks(png)) {
      const body = png.subarray(c.start, c.start + 4 + c.data.length);
      assert.equal(c.crc, crc32(body) >>> 0, c.type);
    }
    assert.equal(crc32(new TextEncoder().encode('123456789')) >>> 0, 0xcbf43926);
  });

  it('IDAT açılınca piksel sayısı boyuta eşittir', () => {
    const { width, height, raw, stride } = decode(png);
    assert.equal(raw.length, stride * height);
    assert.equal(stride, Math.ceil(width / 8) + 1);
  });

  it('matris geri okunur', () => {
    const { px } = decode(png);
    for (let y = 0; y < n; y++)
      for (let x = 0; x < n; x++)
        assert.equal(px[(y + 4) * scale + (scale >> 1)][(x + 4) * scale + (scale >> 1)], matrix[y][x], `${x},${y}`);
  });

  it('sessiz alan açık', () => {
    const { px, width, height } = decode(png);
    const edge = 4 * scale;
    for (let y = 0; y < height; y++)
      for (let x = 0; x < width; x++)
        if (y < edge || x < edge || y >= height - edge || x >= width - edge) assert.equal(px[y][x], false);
  });

  it('metinden üretim matrisle aynı', () => {
    assert.deepEqual(qrPngFromText(URL), png);
  });

  it('boşluk bitleri (satır sonu) açık', () => {
    const { raw, stride, width, height } = decode(png);
    const pad = stride * 8 - 8 - width;
    if (pad > 0) for (let y = 0; y < height; y++) assert.equal(raw[y * stride + stride - 1] & ((1 << pad) - 1), (1 << pad) - 1);
  });
});
