export type Block = { kind: 'heading' | 'bullet' | 'paragraph'; text: string };

const ENTITIES: Record<string, string> = {
  '&nbsp;': ' ',
  '&amp;': '&',
  '&lt;': '<',
  '&gt;': '>',
  '&quot;': '"',
  '&#39;': "'",
};

function htmlToMarkdownish(html: string): string {
  return html
    .replace(/<\s*br\s*\/?>/gi, '\n')
    .replace(/<\s*h[1-6][^>]*>/gi, '\n# ')
    .replace(/<\/\s*(p|div|h[1-6]|ul|ol)\s*>/gi, '\n')
    .replace(/<\s*li[^>]*>/gi, '\n- ')
    .replace(/<[^>]+>/g, '')
    .replace(/&(nbsp|amp|lt|gt|quot|#39);/g, (m) => ENTITIES[m] ?? m);
}

function stripInline(s: string): string {
  return s
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/\[([^\]]+)\]\(([^)]*)\)/g, '$1')
    .replace(/(\*\*|__)(.+?)\1/g, '$2')
    .replace(/(\*|_)(.+?)\1/g, '$2')
    .replace(/`([^`]+)`/g, '$1')
    .trim();
}

/** Announcement body (markdown or html) as simple blocks for native text rendering. */
export function toBlocks(body: string, format: 'markdown' | 'html'): Block[] {
  const source = format === 'html' ? htmlToMarkdownish(body) : body;
  const blocks: Block[] = [];
  let para: string[] = [];
  const flush = () => {
    if (para.length) blocks.push({ kind: 'paragraph', text: stripInline(para.join(' ')) });
    para = [];
  };
  for (const raw of source.replace(/\r\n?/g, '\n').split('\n')) {
    const line = raw.trim();
    if (!line) {
      flush();
      continue;
    }
    const h = /^#{1,6}\s+(.*)$/.exec(line);
    const b = /^(?:[-*+]|\d+[.)])\s+(.*)$/.exec(line);
    if (h) {
      flush();
      blocks.push({ kind: 'heading', text: stripInline(h[1]!) });
    } else if (b) {
      flush();
      blocks.push({ kind: 'bullet', text: stripInline(b[1]!) });
    } else {
      para.push(line);
    }
  }
  flush();
  return blocks.filter((x) => x.text);
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
