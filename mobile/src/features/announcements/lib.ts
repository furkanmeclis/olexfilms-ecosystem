export type Part = { text: string; url?: string };
export type Block = {
  kind: 'heading' | 'bullet' | 'paragraph';
  /** Plain text of the block (links reduced to their label). */
  text: string;
  /** Inline pieces; link parts carry a safe url. */
  parts: Part[];
  /** List marker: a bullet or the original number. */
  marker?: string;
};

const NAMED: Record<string, string> = { nbsp: ' ', amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" };

function decodeEntities(s: string): string {
  return s.replace(/&(?:#x([0-9a-f]+)|#(\d+)|([a-z]+));/gi, (m, hex, dec, name) => {
    if (name) return NAMED[name.toLowerCase()] ?? m;
    const code = hex ? parseInt(hex, 16) : parseInt(dec, 10);
    if (!Number.isFinite(code) || code < 1 || code > 0x10ffff) return m;
    try {
      return String.fromCodePoint(code);
    } catch {
      return m;
    }
  });
}

/** Only http, https and mailto links are ever opened. */
export function isSafeUrl(url: string): boolean {
  return /^(https?:\/\/|mailto:)\S+$/i.test(url.trim());
}

function htmlToMarkdownish(html: string): string {
  const out = html
    .replace(/<\s*(script|style)\b[^>]*>[\s\S]*?<\/\s*\1\s*>/gi, '')
    .replace(/<\s*a\b[^>]*?\bhref\s*=\s*(?:"([^"]*)"|'([^']*)')[^>]*>([\s\S]*?)<\/\s*a\s*>/gi, (_m, d, s, inner) => {
      const label = String(inner).replace(/<[^>]+>/g, '').trim();
      return `[${label || (d ?? s)}](${decodeEntities(d ?? s ?? '')})`;
    })
    .replace(/<\s*br\s*\/?>/gi, '\n')
    .replace(/<\s*h[1-6][^>]*>/gi, '\n# ')
    .replace(/<\/\s*(p|div|h[1-6]|ul|ol|li)\s*>/gi, '\n')
    .replace(/<\s*li[^>]*>/gi, '\n- ')
    .replace(/<[^>]+>/g, '');
  return decodeEntities(out);
}

function stripEmphasis(s: string): string {
  return s
    .replace(/(\*\*|__)(?=\S)(.+?)(?<=\S)\1/g, '$2')
    .replace(/\*(?=\S)(.+?)(?<=\S)\*/g, '$1')
    .replace(/(?<![\w])_(?=\S)(.+?)(?<=\S)_(?![\w])/g, '$1')
    .replace(/`([^`]+)`/g, '$1');
}

/** Splits a line into text and link parts; unsafe link targets keep their label only. */
export function parseInline(raw: string): Part[] {
  const s = raw.replace(/!\[[^\]]*\]\([^)]*\)/g, '');
  const parts: Part[] = [];
  const re = /\[([^\]]+)\]\(([^)\s]*)\)/g;
  let last = 0;
  let m: RegExpExecArray | null;
  const push = (text: string, url?: string) => {
    const t = stripEmphasis(text);
    if (t) parts.push(url ? { text: t, url } : { text: t });
  };
  while ((m = re.exec(s))) {
    push(s.slice(last, m.index));
    push(m[1]!, isSafeUrl(m[2]!) ? m[2]!.trim() : undefined);
    last = m.index + m[0].length;
  }
  push(s.slice(last));
  // Merge neighbouring plain parts so text stays contiguous.
  return parts.reduce<Part[]>((acc, p) => {
    const prev = acc[acc.length - 1];
    if (prev && !prev.url && !p.url) prev.text += p.text;
    else acc.push(p);
    return acc;
  }, []);
}

const plain = (parts: Part[]) => parts.map((p) => p.text).join('').trim();

/** Announcement body (markdown or html) as simple blocks for native text rendering. */
export function toBlocks(body: string, format: 'markdown' | 'html'): Block[] {
  const source = format === 'html' ? htmlToMarkdownish(body) : body;
  const blocks: Block[] = [];
  let para: string[] = [];
  const make = (kind: Block['kind'], text: string, marker?: string) => {
    const parts = parseInline(text);
    const t = plain(parts);
    if (t) blocks.push({ kind, text: t, parts, ...(marker ? { marker } : {}) });
  };
  const flush = () => {
    if (para.length) make('paragraph', para.join(' '));
    para = [];
  };
  for (const raw of source.replace(/\r\n?/g, '\n').split('\n')) {
    const line = raw.trim();
    if (!line) {
      flush();
      continue;
    }
    const h = /^#{1,6}\s+(.*)$/.exec(line);
    const ul = /^[-*+]\s+(.*)$/.exec(line);
    const ol = /^(\d+)[.)]\s+(.*)$/.exec(line);
    if (h) {
      flush();
      make('heading', h[1]!);
    } else if (ul) {
      flush();
      make('bullet', ul[1]!, '•');
    } else if (ol) {
      flush();
      make('bullet', ol[2]!, `${ol[1]}.`);
    } else {
      para.push(line);
    }
  }
  flush();
  return blocks;
}

/** One-line preview of a body for list rows. */
export function previewOf(body: string, format: 'markdown' | 'html', max = 120): string {
  const text = toBlocks(body, format)
    .map((b) => b.text)
    .join(' ');
  return text.length > max ? `${text.slice(0, max - 1).trimEnd()}…` : text;
}

export function isUnread(a: { read_at?: string | null }): boolean {
  return !a.read_at;
}
