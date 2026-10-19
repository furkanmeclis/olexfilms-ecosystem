import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  compactPlate,
  extractPlateCandidates,
  formatTrPlate,
  isStrictTrPlate,
  isValidTrPlate,
  normalizePlateInput,
  padRect,
  PlateConsensus,
  type PlateCandidate,
  pickPictureSize,
  previewRectToImage,
  repairTrPlate,
} from './lib.ts';

const best = (lines: Parameters<typeof extractPlateCandidates>[0], roi?: Parameters<typeof extractPlateCandidates>[1]) =>
  extractPlateCandidates(lines, roi)[0];

describe('TR plate format', () => {
  it('normalizes separators and Turkish letters', () => {
    assert.equal(compactPlate(' 34-abc.123 '), '34ABC123');
    assert.equal(compactPlate('06 şİç 12'), '06SIC12');
    assert.equal(formatTrPlate('34abc123'), '34 ABC 123');
    assert.equal(formatTrPlate('06 A 1234'), '06 A 1234');
  });

  it('checks province codes 01–81', () => {
    assert.equal(isValidTrPlate('01 A 1234'), true);
    assert.equal(isValidTrPlate('81 AB 123'), true);
    assert.equal(isValidTrPlate('00 AB 123'), false);
    assert.equal(isValidTrPlate('82 AB 123'), false);
    assert.equal(isValidTrPlate('99 AB 123'), false);
  });

  it('follows the backend regex for letter and digit counts', () => {
    assert.equal(isValidTrPlate('34 ABCD 12'), false);
    assert.equal(isValidTrPlate('34 ABC 1'), false);
    assert.equal(isValidTrPlate('34 A 123456'), false);
    assert.equal(isValidTrPlate('34 AB 12345'), true);
    assert.equal(isValidTrPlate(''), false);
  });

  it('knows the stricter registration combinations', () => {
    assert.equal(isStrictTrPlate('06 A 1234'), true);
    assert.equal(isStrictTrPlate('34 AB 123'), true);
    assert.equal(isStrictTrPlate('34 ABC 12'), true);
    assert.equal(isStrictTrPlate('34 A 12'), false);
    assert.equal(isStrictTrPlate('34 ABC 1234'), false);
  });

  it('normalizes a manual entry', () => {
    assert.equal(normalizePlateInput('34abc123'), '34 ABC 123');
    assert.equal(normalizePlateInput('  b-md 1234 '), 'B MD 1234');
  });
});

describe('look-alike repair', () => {
  it('fixes O/0 and I/1 by position', () => {
    assert.equal(repairTrPlate('O6ABC1O')[0]?.plate, '06 ABC 10');
    assert.equal(repairTrPlate('34AB1I23')[0]?.plate, '34 AB 1123');
    assert.equal(repairTrPlate('3401C123')[0]?.plate, '34 OIC 123');
  });

  it('prefers the reading with fewest edits', () => {
    const r = repairTrPlate('34AB123');
    assert.equal(r[0]?.plate, '34 AB 123');
    assert.equal(r[0]?.edits, 0);
  });

  it('rejects impossible strings', () => {
    assert.deepEqual(repairTrPlate('ABCDEFG'), []);
    assert.deepEqual(repairTrPlate('34'), []);
    assert.deepEqual(repairTrPlate('9934AB12'), []);
  });
});

describe('candidate extraction', () => {
  it('reads a clean line', () => {
    const c = best([{ text: '34 ABC 123', confidence: 0.95 }]);
    assert.equal(c?.plate, '34 ABC 123');
    assert.ok((c?.confidence ?? 0) > 0.8);
  });

  it('strips the TR country strip', () => {
    assert.equal(best([{ text: 'TR 34 ABC 123' }])?.plate, '34 ABC 123');
    assert.equal(best([{ text: 'TR34ABC123' }])?.plate, '34 ABC 123');
    assert.equal(best([{ text: 'TR' }, { text: '06 A 1234' }])?.plate, '06 A 1234');
  });

  it('prefers the full plate over a truncated substring', () => {
    const list = extractPlateCandidates([{ text: '34 AB 1234' }]);
    assert.equal(list[0]?.plate, '34 AB 1234');
    const truncated = list.find((c) => c.compact === '34AB123');
    assert.ok(!truncated || truncated.confidence < list[0]!.confidence);
  });

  it('handles O/0 confusion with lower confidence', () => {
    const clean = best([{ text: '06 ABC 10' }]);
    const fixed = best([{ text: 'O6 ABC 1O' }]);
    assert.equal(fixed?.plate, '06 ABC 10');
    assert.ok((fixed?.confidence ?? 1) < (clean?.confidence ?? 0));
  });

  it('joins two-row square plates', () => {
    assert.equal(best([{ text: '34' }, { text: 'ABC 123' }])?.plate, '34 ABC 123');
  });

  it('uses alternative readings', () => {
    assert.equal(best([{ text: '34 A8C 1Z3', alternatives: ['34 ABC 123'] }])?.plate, '34 ABC 123');
  });

  it('down-ranks lines outside the guide frame', () => {
    const roi = { x: 0.1, y: 0.4, width: 0.8, height: 0.2 };
    const c = best(
      [
        { text: '35 XY 999', box: { x: 0.1, y: 0.05, width: 0.3, height: 0.05 } },
        { text: '34 ABC 123', box: { x: 0.2, y: 0.45, width: 0.6, height: 0.1 } },
      ],
      { roi },
    );
    assert.equal(c?.plate, '34 ABC 123');
  });

  it('falls back to a low-confidence foreign reading', () => {
    const c = best([{ text: 'B MD 1234' }]);
    assert.equal(c?.tr, false);
    assert.equal(c?.plate, 'B MD 1234');
    assert.ok((c?.confidence ?? 1) < 0.5);
  });

  it('returns nothing for noise', () => {
    assert.deepEqual(extractPlateCandidates([{ text: 'OLEXFILMS' }, { text: '' }]), []);
  });
});

describe('consensus across frames', () => {
  const c = (plate: string, confidence: number, tr = true): PlateCandidate => ({ plate, compact: compactPlate(plate), confidence, tr });

  it('accepts after two agreeing frames', () => {
    const k = new PlateConsensus();
    assert.equal(k.add([c('34 ABC 123', 0.9)]), null);
    assert.equal(k.add([c('34 ABC 123', 0.85)])?.plate, '34 ABC 123');
  });

  it('needs every agreeing frame to be confident', () => {
    const k = new PlateConsensus();
    assert.equal(k.add([c('34 ABC 123', 0.95)]), null);
    assert.equal(k.add([c('34 ABC 123', 0.5)]), null);
    assert.equal(k.add([c('34 ABC 123', 0.8)])?.plate, '34 ABC 123');
  });

  it('never accepts low confidence or foreign readings', () => {
    const k = new PlateConsensus();
    k.add([c('34 ABC 123', 0.4)]);
    assert.equal(k.add([c('34 ABC 123', 0.5)]), null);
    k.add([c('B MD 1234', 0.9, false)]);
    assert.equal(k.add([c('B MD 1234', 0.9, false)]), null);
    assert.equal(k.ranked()[0]?.plate, '34 ABC 123');
  });
});

describe('picture size for OCR frames', () => {
  it('takes the largest size with a 1280–1920 px long side', () => {
    assert.equal(pickPictureSize(['4032x3024', '1920x1080', '1280x720', '640x480']), '1920x1080');
    assert.equal(pickPictureSize(['1600x1200', '1280x960', '4000x3000']), '1600x1200');
  });

  it('falls back to the smallest larger size, or none for presets', () => {
    assert.equal(pickPictureSize(['4032x3024', '3264x2448', '640x480']), '3264x2448');
    assert.equal(pickPictureSize(['Photo', 'High', 'Medium']), undefined);
    assert.equal(pickPictureSize([]), undefined);
  });
});

describe('preview to image mapping', () => {
  it('pads a rect and clamps it to the image', () => {
    const r = padRect({ x: 0.05, y: 0.4, width: 0.9, height: 0.1 }, 0.1, 0.5);
    assert.equal(r.x, 0);
    assert.ok(Math.abs(r.y - 0.35) < 1e-9);
    assert.ok(Math.abs(r.x + r.width - 1) < 1e-9);
    assert.ok(Math.abs(r.height - 0.2) < 1e-9);
  });

  it('maps the guide through a cover crop', () => {
    // 4:3 landscape photo shown in a 300×600 portrait view: crops the sides.
    const r = previewRectToImage({ width: 300, height: 600 }, { width: 3000, height: 4000 }, { x: 0, y: 250, width: 300, height: 100 });
    // scale = max(0.1, 0.15) = 0.15 → image shown 450×600, 75 pt cropped each side.
    assert.ok(Math.abs(r.x - 75 / 450) < 1e-9);
    assert.ok(Math.abs(r.width - 300 / 450) < 1e-9);
    assert.ok(Math.abs(r.y - 250 / 600) < 1e-9);
    assert.ok(Math.abs(r.height - 100 / 600) < 1e-9);
  });
});
