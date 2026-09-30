export interface ByteRange { start: number; end: number }
/** Single byte ranges used by Chromium's media loader. End is inclusive. */
export function parseVideoRange(value: string | null, size: number): ByteRange | null {
  if (value === null) return null;
  const match = /^bytes=(\d*)-(\d*)$/.exec(value.trim());
  if (!match || (!match[1] && !match[2]) || size < 1) throw new RangeError('Unsatisfiable range');
  const parse = (s: string) => { const n = Number(s); if (!Number.isSafeInteger(n) || n < 0) throw new RangeError('Invalid range'); return n; };
  const start = match[1] ? parse(match[1]) : Math.max(0, size - parse(match[2]));
  const end = match[1] && match[2] ? Math.min(parse(match[2]), size - 1) : size - 1;
  if ((!match[1] && parse(match[2]) === 0) || start >= size || end < start) throw new RangeError('Unsatisfiable range');
  return { start, end };
}
