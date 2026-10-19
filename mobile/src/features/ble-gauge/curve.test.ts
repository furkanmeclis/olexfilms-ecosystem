import assert from 'node:assert/strict';
import { createCipheriv } from 'node:crypto';
import { describe, it } from 'node:test';

import {
  decryptCurveFile,
  deviceModelName,
  encryptCurveFile,
  frameToReading,
  frequencyToMicrons,
  parseCurve,
} from './curve.ts';
import { decodeFrame, encodeFrame } from './protocol.ts';

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

  it('is null outside the curve (probe in the air)', () => {
    assert.equal(frequencyToMicrons('fe', 22293, curve), null);
    assert.equal(frequencyToMicrons('al', 27000, curve), null);
  });

  it('applies the calibration offset', () => {
    assert.equal(frequencyToMicrons('fe', 18140, curve, { fe: 100, al: 0 }), 90);
  });
});

describe('frameToReading', () => {
  it('turns the captured idle frame into a reading without microns', () => {
    const r = frameToReading(decodeFrame('eVo1AFBAADA54F0BE8')!, curve, null, 1);
    assert.equal(r.um, null);
    assert.equal(r.serial, 32475);
  });

  it('measures a frame on 90 µm of paint over steel', () => {
    const f = decodeFrame(encodeFrame({ type: 1, frequency: 18240, serial: 32475, batteryVoltage: 3 }))!;
    assert.equal(frameToReading(f, curve, null, 1).um, 90);
  });

  it('has no microns without the matching curve', () => {
    const f = decodeFrame(encodeFrame({ type: 1, frequency: 18240, serial: 1355, batteryVoltage: 3 }))!;
    assert.equal(frameToReading(f, curve, null, 1).um, null);
    assert.equal(frameToReading(f, null, null, 1).um, null);
  });
});
