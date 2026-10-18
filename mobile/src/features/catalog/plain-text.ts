/**
 * Product descriptions are Markdown on the web; the app shows them as plain
 * text. Only syntax is removed: line-start markers (headings, quotes,
 * bullets become "•") and emphasis or code spans at word boundaries, so
 * SKUs like PPF_8MIL or "2*3" stay intact. No `@/` imports: node --test runs it.
 */
export function plainText(md: string): string {
  return md
    .split('\n')
    .map((line) => line.replace(/^\s{0,3}(#{1,6}\s+|>\s?)/, '').replace(/^(\s*)[-*+]\s+/, '$1• '))
    .join('\n')
    .replace(/(^|[\s(])(\*\*|__)(?=\S)(.+?)(?<=\S)\2(?=$|[\s).,;:!?])/gm, '$1$3')
    .replace(/(^|[\s(])([*_])(?=\S)([^*_\n]+?)(?<=\S)\2(?=$|[\s).,;:!?])/gm, '$1$3')
    .replace(/`([^`\n]+)`/g, '$1')
    .trim();
}
