import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  batteryPercent,
  channelOfType,
  createLineSplitter,
  decodeFrame,
  encodeFrame,
  isGaugePeripheral,
  modeCommand,
} from './protocol.ts';

// Real capture from a SmartPTG Professional (old app docs/smartptg, BLE_ANALYSIS.md).
const REAL = 'eVo1AFBAADA54F0BE8\r\n';

describe('decodeFrame', () => {
  it('decodes the captured frame', () => {
    assert.deepEqual(decodeFrame(REAL), {
      type: 1,
      channel: 'fe',
      button: false,
      frequency: 22293,
      serial: 32475,
      batteryVoltage: 2,
    });
  });

  it('accepts lower-case hex and a missing line end', () => {
    assert.equal(decodeFrame('eVo1afbaada54f0be8')?.serial, 32475);
  });

  it('rejects garbled frames', () => {
    assert.equal(decodeFrame(''), null);
    assert.equal(decodeFrame('eVo1AFBAADA54F0BE'), null); // short
    assert.equal(decodeFrame('xVo1AFBAADA54F0BE8'), null); // prefix
    assert.equal(decodeFrame('eVo1AFBAADA54F0BZ8'), null); // not hex
    assert.equal(decodeFrame('eVoXAFBAADA54F0BE8'), null); // type not a digit
  });

  it('round-trips synthetic frames (Fe and Al)', () => {
    for (const f of [
      { type: 1, frequency: 18240, serial: 32475, batteryVoltage: 3.7 },
      { type: 2, frequency: 26190, serial: 1355, batteryVoltage: 2.9 },
      { type: 1, frequency: 15836, serial: 0xabcdef, batteryVoltage: 0 },
    ]) {
      const d = decodeFrame(encodeFrame(f));
      assert.equal(d?.frequency, f.frequency);
      assert.equal(d?.serial, f.serial);
      assert.equal(d?.batteryVoltage, f.batteryVoltage);
      assert.equal(d?.channel, f.type === 1 ? 'fe' : 'al');
      assert.equal(d?.button, false);
    }
  });

  // TEC-939 (§3.3): the type is the channel; 3 / 4 are the same channels with the button pressed.
  it('reads the channel and the button from the type', () => {
    const at = (type: number) => decodeFrame(encodeFrame({ type, frequency: 19000, serial: 32475, batteryVoltage: 2.9 }))!;
    assert.deepEqual([at(1).channel, at(1).button], ['fe', false]);
    assert.deepEqual([at(2).channel, at(2).button], ['al', false]);
    assert.deepEqual([at(3).channel, at(3).button], ['fe', true]);
    assert.deepEqual([at(4).channel, at(4).button], ['al', true]);
    assert.deepEqual([at(5).channel, at(5).button], [null, false]);
    assert.equal(channelOfType(0), null);
  });
});

describe('batteryPercent (FrameDecoder.a, §3.4)', () => {
  it('maps the voltage steps', () => {
    const cases: [number, number][] = [
      [3.0, 100],
      [2.8, 100],
      [2.7, 90],
      [2.6, 80], // > 2.60 is 90, 2.60 itself falls to the next step
      [2.55, 80],
      [2.5, 70],
      [2.45, 70],
      [2.35, 60],
      [2.25, 50],
      [2.15, 40],
      [2.05, 30],
      [2.0, 20],
      [1.95, 20],
      [1.85, 15],
      [1.8, 10],
      [0, 10],
    ];
    for (const [v, pct] of cases) assert.equal(batteryPercent(v), pct, `${v} V`);
  });
});

describe('createLineSplitter', () => {
  it('joins split notifications and splits merged ones', () => {
    const s = createLineSplitter();
    assert.deepEqual(s.push('eVo1AFBAAD'), []);
    assert.deepEqual(s.push('A54F0BE8\r\neVo1AF'), ['eVo1AFBAADA54F0BE8']);
    assert.deepEqual(s.push('BAADA54F0BE8\r\n\r\n'), ['eVo1AFBAADA54F0BE8']);
  });

  it('drops a runaway buffer without line ends', () => {
    const s = createLineSplitter(8);
    s.push('0123456789');
    assert.deepEqual(s.push('eVo\r\n'), ['eVo']);
  });
});

describe('peripheral filter and commands', () => {
  it('matches by name or advertised service', () => {
    assert.ok(isGaugePeripheral('SmartPTG'));
    assert.ok(isGaugePeripheral('Nex PTG 2'));
    assert.ok(isGaugePeripheral(null, ['6E400001-B5A3-F393-E0A9-E50E24DCCA9E']));
    assert.ok(!isGaugePeripheral('AirPods', ['180f']));
    assert.ok(!isGaugePeripheral(undefined));
  });

  it('builds the mode commands', () => {
    assert.equal(globalThis.atob(modeCommand(3)), '!3\r\n');
    assert.equal(globalThis.atob(modeCommand(1)), '!1\r\n');
    assert.equal(globalThis.atob(modeCommand(2)), '!2\r\n');
  });
});
