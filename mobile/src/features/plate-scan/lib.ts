/**
 * Turkish licence plate rules. Pure logic (no `@/` imports) so `node --test`
 * runs it directly.
 *
 * Valid form follows the backend's TR plate format (plate_formats, TR):
 * `^(0[1-9]|[1-7][0-9]|8[01])[A-Z]{1,3}[0-9]{2,5}$` on the compact form.
 * The civil registration rules are narrower; they only raise confidence:
 *   1 letter  → 4–5 digits  (06 A 1234)
 *   2 letters → 3–4 digits  (34 AB 123, 34 AB 1234)
 *   3 letters → 2–3 digits  (34 ABC 12, 34 ABC 123)
 * Q, W and X are not used on Turkish plates.
 */

export const TR_PLATE_RE = /^(0[1-9]|[1-7][0-9]|8[01])([A-Z]{1,3})([0-9]{2,5})$/;

const TURKISH_FOLD: Record<string, string> = {
  İ: 'I',
  I: 'I',
  ı: 'I',
  i: 'I',
  Ş: 'S',
  ş: 'S',
  Ğ: 'G',
  ğ: 'G',
  Ü: 'U',
  ü: 'U',
  Ö: 'O',
  ö: 'O',
  Ç: 'C',
  ç: 'C',
};

/** Upper-cases with Turkish letters folded to ASCII (İ→I, Ş→S, …). */
export function foldUpper(raw: string): string {
  let out = '';
  for (const ch of raw) out += TURKISH_FOLD[ch] ?? ch.toUpperCase();
  return out;
}

/** Compact form the backend validates: A–Z and 0–9 only. */
export function compactPlate(raw: string): string {
  return foldUpper(raw).replace(/[^A-Z0-9]/g, '');
}

/** `34ABC123` → `34 ABC 123`; null when not a valid TR plate. */
export function formatTrPlate(raw: string): string | null {
  const m = compactPlate(raw).match(TR_PLATE_RE);
  return m ? `${m[1]} ${m[2]} ${m[3]}` : null;
}

export function isValidTrPlate(raw: string): boolean {
  return TR_PLATE_RE.test(compactPlate(raw));
}

/** Matches the civil registration letter/digit counts (see file header). */
export function isStrictTrPlate(raw: string): boolean {
  const m = compactPlate(raw).match(TR_PLATE_RE);
  if (!m) return false;
  const letters = m[2]!.length;
  const digits = m[3]!.length;
  if (letters === 1) return digits === 4 || digits === 5;
  if (letters === 2) return digits === 3 || digits === 4;
  return digits === 2 || digits === 3;
}

/** Letters that never appear on a Turkish plate. */
export function hasForeignLetters(raw: string): boolean {
  const m = compactPlate(raw).match(TR_PLATE_RE);
  return !!m && /[QWX]/.test(m[2]!);
}

/**
 * Value for a plate text field: a valid TR plate gets its canonical spacing,
 * anything else is upper-cased with separators collapsed (foreign plates are
 * allowed; the backend validates against the vehicle's plate country).
 */
export function normalizePlateInput(raw: string): string {
  return formatTrPlate(raw) ?? foldUpper(raw).replace(/[^A-Z0-9]+/g, ' ').trim();
}

/** OCR look-alikes read as a letter where a digit belongs. */
const TO_DIGIT: Record<string, string> = {
  O: '0',
  Q: '0',
  D: '0',
  U: '0',
  I: '1',
  L: '1',
  J: '1',
  T: '1',
  Z: '2',
  A: '4',
  S: '5',
  G: '6',
  B: '8',
};

/** OCR look-alikes read as a digit where a letter belongs. */
const TO_LETTER: Record<string, string> = {
  '0': 'O',
  '1': 'I',
  '2': 'Z',
  '4': 'A',
  '5': 'S',
  '6': 'G',
  '7': 'T',
  '8': 'B',
};

function mapAll(value: string, table: Record<string, string>, want: RegExp): { out: string; edits: number } | null {
  let out = '';
  let edits = 0;
  for (const ch of value) {
    if (want.test(ch)) {
      out += ch;
      continue;
    }
    const mapped = table[ch];
    if (!mapped) return null;
    out += mapped;
    edits += 1;
  }
  return { out, edits };
}

export type PlateRepair = {
  /** Compact form, e.g. `34ABC123`. */
  compact: string;
  /** Display form, e.g. `34 ABC 123`. */
  plate: string;
  /** Characters swapped to fit the format (O→0, I→1, …). */
  edits: number;
  strict: boolean;
};

/**
 * Every way a compact OCR string can be read as a valid TR plate by fixing
 * look-alike characters (O/0, I/1, S/5, B/8, Z/2, G/6) in the province,
 * letter and number groups. Sorted by fewest edits, then strict form first.
 */
export function repairTrPlate(raw: string): PlateRepair[] {
  const s = compactPlate(raw);
  if (s.length < 5 || s.length > 10) return [];
  const province = mapAll(s.slice(0, 2), TO_DIGIT, /[0-9]/);
  if (!province) return [];
  const out = new Map<string, PlateRepair>();
  for (let letterLen = 1; letterLen <= 3; letterLen += 1) {
    const digitLen = s.length - 2 - letterLen;
    if (digitLen < 2 || digitLen > 5) continue;
    const letters = mapAll(s.slice(2, 2 + letterLen), TO_LETTER, /[A-Z]/);
    const digits = mapAll(s.slice(2 + letterLen), TO_DIGIT, /[0-9]/);
    if (!letters || !digits) continue;
    const compact = province.out + letters.out + digits.out;
    const plate = formatTrPlate(compact);
    if (!plate) continue;
    const edits = province.edits + letters.edits + digits.edits;
    const prev = out.get(compact);
    if (!prev || prev.edits > edits) out.set(compact, { compact, plate, edits, strict: isStrictTrPlate(compact) });
  }
  return [...out.values()].sort((a, b) => a.edits - b.edits || Number(b.strict) - Number(a.strict));
}

/*
 * Candidates from on-device OCR output. The engine returns text lines (with
 * an optional confidence, normalized box and alternative readings). Each
 * line, its alternatives and each pair of neighbouring lines (two-row square
 * plates) are searched for TR plates; every hit gets a 0–1 confidence from
 * look-alike fixes, word alignment, the registration rules, the OCR
 * confidence and the guide frame.
 */

/** Normalized rectangle, origin top-left, 0–1 of the image. */
export type NormRect = { x: number; y: number; width: number; height: number };

export type OcrLine = {
  text: string;
  /** Engine confidence 0–1 when it gives one. */
  confidence?: number | null;
  box?: NormRect | null;
  /** Other readings of the same line (best first, excluding `text`). */
  alternatives?: string[] | null;
};

export type PlateCandidate = {
  /** Display form: `34 ABC 123` for TR plates. */
  plate: string;
  /** Compact form: `34ABC123`. */
  compact: string;
  confidence: number;
  /** False for a non-TR reading offered as a last resort. */
  tr: boolean;
};

const COUNTRY_PREFIXES = ['TR', 'T'];
/** TR readings below this are noise (mid-word hits with several fixes). */
const MIN_TR_CONFIDENCE = 0.35;

type Segment = { compact: string; bounds: Set<number> };

/** Compact text plus the indexes where words start and end (alignment check). */
function segment(text: string): Segment {
  const words = foldUpper(text)
    .split(/[^A-Z0-9]+/)
    .filter(Boolean);
  const bounds = new Set<number>([0]);
  let compact = '';
  for (const w of words) {
    // A country code glued to the plate (TR34ABC123, T34…) still ends a word.
    if (compact === '') {
      for (const p of COUNTRY_PREFIXES) {
        if (w.length > p.length + 4 && w.startsWith(p) && /^[0-9OIL]/.test(w.slice(p.length))) {
          bounds.add(p.length);
          break;
        }
      }
    }
    compact += w;
    bounds.add(compact.length);
  }
  return { compact, bounds };
}

export function centerInside(box: NormRect, roi: NormRect): boolean {
  const cx = box.x + box.width / 2;
  const cy = box.y + box.height / 2;
  return cx >= roi.x && cx <= roi.x + roi.width && cy >= roi.y && cy <= roi.y + roi.height;
}

const clamp = (n: number) => Math.max(0, Math.min(1, n));

type Source = { text: string; confidence: number | null; box: NormRect | null; penalty: number };

function scoreSource(src: Source, roi: NormRect | null, into: Map<string, PlateCandidate>) {
  const { compact, bounds } = segment(src.text);
  for (let start = 0; start < compact.length; start += 1) {
    for (let end = start + 5; end <= Math.min(compact.length, start + 10); end += 1) {
      const repairs = repairTrPlate(compact.slice(start, end));
      for (const r of repairs) {
        let score = 1 - src.penalty;
        score -= r.edits * 0.15;
        if (!bounds.has(start)) score -= 0.3;
        if (!bounds.has(end)) score -= 0.3;
        score += r.strict ? 0.05 : -0.1;
        if (hasForeignLetters(r.compact)) score -= 0.15;
        if (src.confidence != null) score *= 0.6 + 0.4 * clamp(src.confidence);
        if (roi && src.box) score += centerInside(src.box, roi) ? 0.05 : -0.2;
        const prev = into.get(r.compact);
        const confidence = clamp(score);
        if (!prev || prev.confidence < confidence) into.set(r.compact, { plate: r.plate, compact: r.compact, confidence, tr: true });
      }
    }
  }
}

function unionBox(a: NormRect | null | undefined, b: NormRect | null | undefined): NormRect | null {
  if (!a || !b) return a ?? b ?? null;
  const x = Math.min(a.x, b.x);
  const y = Math.min(a.y, b.y);
  return { x, y, width: Math.max(a.x + a.width, b.x + b.width) - x, height: Math.max(a.y + a.height, b.y + b.height) - y };
}

function lineConfidence(a: OcrLine, b?: OcrLine): number | null {
  const values = [a.confidence, b?.confidence].filter((c): c is number => typeof c === 'number');
  return values.length ? Math.min(...values) : null;
}

/**
 * Ranked plate candidates (best first) from one OCR pass.
 * `roi` is the guide frame in image coordinates; lines outside it lose score.
 */
export function extractPlateCandidates(lines: OcrLine[], opts: { roi?: NormRect | null; limit?: number } = {}): PlateCandidate[] {
  const roi = opts.roi ?? null;
  const found = new Map<string, PlateCandidate>();
  lines.forEach((line, i) => {
    scoreSource({ text: line.text, confidence: lineConfidence(line), box: line.box ?? null, penalty: 0 }, roi, found);
    (line.alternatives ?? []).forEach((alt, k) => {
      scoreSource({ text: alt, confidence: lineConfidence(line), box: line.box ?? null, penalty: 0.08 * (k + 1) }, roi, found);
    });
    const next = lines[i + 1];
    if (next) {
      scoreSource(
        { text: `${line.text} ${next.text}`, confidence: lineConfidence(line, next), box: unionBox(line.box, next.box), penalty: 0.08 },
        roi,
        found,
      );
    }
  });

  let ranked = [...found.values()].filter((c) => c.confidence >= MIN_TR_CONFIDENCE).sort((a, b) => b.confidence - a.confidence || a.compact.localeCompare(b.compact));
  if (ranked.length === 0) {
    const loose = looseCandidate(lines, roi);
    if (loose) ranked = [loose];
  }
  return ranked.slice(0, opts.limit ?? 5);
}

/** Last resort for foreign plates: the most plate-like line, low confidence. */
function looseCandidate(lines: OcrLine[], roi: NormRect | null): PlateCandidate | null {
  let best: PlateCandidate | null = null;
  for (const line of lines) {
    if (roi && line.box && !centerInside(line.box, roi)) continue;
    const words = foldUpper(line.text)
      .split(/[^A-Z0-9]+/)
      .filter(Boolean);
    const compact = words.join('');
    if (compact.length < 5 || compact.length > 10 || !/[0-9]/.test(compact) || !/[A-Z]/.test(compact)) continue;
    const confidence = clamp(0.35 * (line.confidence ?? 1));
    if (!best || best.confidence < confidence) best = { plate: words.join(' '), compact, confidence, tr: false };
  }
  return best;
}

/**
 * Agreement across camera frames. A plate is accepted automatically once it
 * is the best reading in `hits` frames with enough confidence; a single lucky
 * frame never fills the field on its own.
 */
export class PlateConsensus {
  private readonly tally = new Map<string, { candidate: PlateCandidate; hits: number }>();
  private readonly hits: number;
  private readonly minConfidence: number;

  constructor(hits = 2, minConfidence = 0.7) {
    this.hits = hits;
    this.minConfidence = minConfidence;
  }

  /** Adds one frame's candidates; returns the accepted plate, if any. */
  add(candidates: PlateCandidate[]): PlateCandidate | null {
    const top = candidates[0];
    if (!top) return null;
    const entry = this.tally.get(top.compact);
    const candidate = entry && entry.candidate.confidence > top.confidence ? entry.candidate : top;
    const hits = (entry?.hits ?? 0) + 1;
    this.tally.set(top.compact, { candidate, hits });
    return top.tr && hits >= this.hits && candidate.confidence >= this.minConfidence ? candidate : null;
  }

  /** All readings so far, best first (for the result sheet). */
  ranked(limit = 5): PlateCandidate[] {
    return [...this.tally.values()]
      .sort((a, b) => Number(b.candidate.tr) - Number(a.candidate.tr) || b.hits - a.hits || b.candidate.confidence - a.candidate.confidence)
      .slice(0, limit)
      .map((e) => e.candidate);
  }

  reset() {
    this.tally.clear();
  }
}

/**
 * Maps a rectangle on the camera preview (points) to normalized image
 * coordinates. The preview fills its view like `cover`: the photo is scaled
 * up until both sides fit and the overflow is cropped evenly.
 */
export function previewRectToImage(
  view: { width: number; height: number },
  image: { width: number; height: number },
  rect: NormRect,
): NormRect {
  const scale = Math.max(view.width / image.width, view.height / image.height);
  const offsetX = (image.width * scale - view.width) / 2;
  const offsetY = (image.height * scale - view.height) / 2;
  const x = clamp((rect.x + offsetX) / scale / image.width);
  const y = clamp((rect.y + offsetY) / scale / image.height);
  const right = clamp((rect.x + rect.width + offsetX) / scale / image.width);
  const bottom = clamp((rect.y + rect.height + offsetY) / scale / image.height);
  return { x, y, width: right - x, height: bottom - y };
}
