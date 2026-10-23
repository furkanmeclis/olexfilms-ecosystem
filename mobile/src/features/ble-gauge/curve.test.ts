import assert from 'node:assert/strict';
import { createCipheriv } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { describe, it } from 'node:test';

import { BUNDLED_CURVES } from './bundled-curves.ts';
import {
  applyDeviceRules,
  curveCalVersion,
  decryptCurveFile,
  deviceModelName,
  deviceRules,
  encryptCurveFile,
  frequencyToMicrons,
  materialOf,
  parseCurve,
} from './curve.ts';

// Decrypted factory curve of SN 32475 (old app src/data/smartptg-curves/32475.json).
const CSV_32475 = `22291,23490,2,1,32475,18693,20316,18694,20938
0,15836,0,26391
58,17891,52,26250
90,18240,90,26190
169,18848,108,26140
216,19131,159,26026
276,19415,222,25902
334,19647,279,25797
367,19762,311,25745
466,20062,341,25691
588,20356,400,25578
870,20825,797,25077
1255,21247,1203,24712
1816,21615,1756,24377
2240,21781,2272,24175
3200,22002,3205,23947
"Kasia cal1" , 2023-08-10, 13:50
`;

const curve = parseCurve(CSV_32475, 32475);

describe('curve file', () => {
  it('decrypts what node:crypto encrypts (AES-128-CBC, PKCS7)', () => {
    const iv = 'abcdefghijklmnop';
    const c = createCipheriv('aes-128-cbc', Buffer.from('436a1246dbe284b7', 'latin1'), Buffer.from(iv, 'latin1'));
    const file = `${iv}:${Buffer.concat([c.update(CSV_32475, 'latin1'), c.final()]).toString('base64')}`;
    assert.equal(decryptCurveFile(file), CSV_32475);
  });

  it('decrypts the real factory file of SN 32475 (smartptg.pl, 2026-10)', () => {
    const raw = readFileSync(new URL('./fixtures/32475.dat', import.meta.url), 'latin1');
    const real = parseCurve(decryptCurveFile(raw), 32475);
    assert.deepEqual(real.fe, curve.fe);
    assert.deepEqual(real.al, curve.al);
    assert.deepEqual(real, BUNDLED_CURVES[32475]);
  });

  it('round-trips with the helper', () => {
    assert.equal(decryptCurveFile(encryptCurveFile('1,2,3', '0123456789abcdef')), '1,2,3');
  });

  it('rejects malformed files', () => {
    assert.throws(() => decryptCurveFile('no-colon'));
    assert.throws(() => decryptCurveFile('shortiv:AAAA'));
  });

  it('parses header and both material tables', () => {
    assert.equal(curve.serial, 32475);
    assert.equal(curve.deviceType, 2);
    assert.equal(curve.feThreshold, 22291);
    assert.equal(curve.fe.length, 15);
    assert.deepEqual(curve.al[2], { thickness: 90, frequency: 26190 });
    assert.equal(deviceModelName(curve.deviceType), 'SmartPTG Professional');
    assert.equal(deviceModelName(99), 'SmartPTG');
  });

  // TEC-939 (§5.3): lang, the zinc fields T,U,V,W, calX and the comment are kept.
  it('keeps the extra header fields and the comment', () => {
    assert.equal(curve.lang, 1);
    assert.deepEqual(curve.zn, { t: 18693, u: 20316, v: 18694, w: 20938 });
    assert.equal(curve.calVersion, 'cal1');
    assert.equal(curve.comment, '"Kasia cal1" , 2023-08-10, 13:50');
    const plain = parseCurve('22291,23490,1,1,7\n0,15836,0,26391\n90,18240,90,26190\n', 7);
    assert.equal(plain.zn, null);
    assert.equal(plain.calVersion, null);
    assert.equal(plain.comment, null);
    assert.equal(curveCalVersion('x cal3 y'), 'cal3');
    assert.equal(curveCalVersion('"Jan cal2"'), 'cal2');
  });

  it('refuses another serial', () => {
    assert.throws(() => parseCurve(CSV_32475, 1355), /curve_serial/);
  });
});

describe('frequencyToMicrons', () => {
  it('hits the table points', () => {
    assert.equal(frequencyToMicrons('fe', 18240, curve), 90);
    assert.equal(frequencyToMicrons('al', 26190, curve), 90);
    assert.equal(frequencyToMicrons('fe', 15836, curve), 0);
  });

  it('interpolates between points (Fe rising, Al falling)', () => {
    assert.equal(frequencyToMicrons('fe', 18065, curve), 74);
    assert.equal(frequencyToMicrons('al', 26165, curve), 99);
  });

  it('is null in the air: Fe above the last point, Al below it', () => {
    assert.equal(frequencyToMicrons('fe', 22293, curve), null);
    assert.equal(frequencyToMicrons('fe', 22003, curve), null);
    assert.equal(frequencyToMicrons('al', 23512, curve), null);
    assert.equal(frequencyToMicrons('al', 23946, curve), null);
  });

  // TEC-939 (§5.4): a frequency the table does not cover otherwise is bare metal, 0 µm.
  it('is 0 µm below the first Fe point and above the first Al point', () => {
    assert.equal(frequencyToMicrons('fe', 15000, curve), 0);
    assert.equal(frequencyToMicrons('al', 26475, curve), 0);
    assert.equal(frequencyToMicrons('fe', 22002, curve), 3200);
    assert.equal(frequencyToMicrons('al', 23947, curve), 3205);
  });

  it('applies the calibration offset of the channel', () => {
    assert.equal(frequencyToMicrons('fe', 18140, curve, { fe: 100, al: 0 }), 90);
    assert.equal(frequencyToMicrons('al', 26240, curve, { fe: 0, al: -50 }), 90);
    assert.equal(frequencyToMicrons('al', 26190, curve, { fe: 999, al: 0 }), 90);
  });
});

describe('device rules (§5.4)', () => {
  it('rounds to 10 µm and caps at 1000 µm on types 1, 3, 4, 5', () => {
    for (const t of [1, 3, 4, 5]) {
      assert.equal(applyDeviceRules(124, 'fe', t), 120, `type ${t}`);
      assert.equal(applyDeviceRules(125, 'fe', t), 130, `type ${t}`);
      assert.equal(applyDeviceRules(1000, 'fe', t), 1000);
      assert.equal(applyDeviceRules(1004, 'fe', t), 1000);
      assert.equal(applyDeviceRules(1006, 'fe', t), null);
    }
  });

  it('keeps whole microns with 2200 / 2500 / 3000 µm caps on types 0, 2, 6', () => {
    assert.deepEqual([applyDeviceRules(2200, 'fe', 0), applyDeviceRules(2201, 'fe', 0)], [2200, null]);
    assert.deepEqual([applyDeviceRules(2500, 'fe', 2), applyDeviceRules(2501, 'fe', 2)], [2500, null]);
    assert.deepEqual([applyDeviceRules(3000, 'fe', 6), applyDeviceRules(3001, 'fe', 6)], [3000, null]);
    assert.equal(applyDeviceRules(124, 'al', 2), 124);
  });

  it('has no Al value on Economic (3, 4)', () => {
    assert.equal(applyDeviceRules(120, 'al', 3), null);
    assert.equal(applyDeviceRules(120, 'al', 4), null);
    assert.equal(applyDeviceRules(120, 'al', 5), 120);
    assert.equal(deviceRules(3).measuresAl, false);
    assert.equal(applyDeviceRules(null, 'fe', 2), null);
  });
});

describe('materialOf (§4.1)', () => {
  it('decides the substrate from K on the Fe channel and L on the Al channel', () => {
    // SN 32475: K = 22291, L = 23490 (air ≈ K / ≈ L, steel below, aluminium above).
    assert.equal(materialOf('fe', 19190, curve), 'fe');
    assert.equal(materialOf('fe', 22291, curve), 'fe');
    assert.equal(materialOf('fe', 22292, curve), 'al');
    assert.equal(materialOf('fe', 24073, curve), 'al');
    assert.equal(materialOf('al', 20820, curve), 'fe');
    assert.equal(materialOf('al', 23489, curve), 'fe');
    assert.equal(materialOf('al', 23490, curve), 'al');
    assert.equal(materialOf('al', 26475, curve), 'al');
  });
});
