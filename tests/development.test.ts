import { describe, expect, it } from 'vitest';
import { developmentOrigin } from '../electron/development';

describe('development server trust boundary', () => {
  it('accepts the actual loopback port when another worktree occupies the default', () => {
    for (const port of [5173, 5174, 49152]) {
      expect(developmentOrigin(`http://127.0.0.1:${port}`, false)).toBe(`http://127.0.0.1:${port}`);
      expect(developmentOrigin(`http://127.0.0.1:${port}/`, false)).toBe(`http://127.0.0.1:${port}`);
    }
  });

  it('rejects remote hosts, credentials, paths and malformed ports', () => {
    for (const value of [undefined, '', 'http://localhost:5173', 'http://example.com:5173',
      'http://127.0.0.1.example.com:5173', 'http://127.0.0.1:5173@example.com',
      'http://user@127.0.0.1:5173', 'https://127.0.0.1:5173', 'file:///tmp/index.html',
      'http://127.0.0.1:5173/other', 'http://127.0.0.1:5173/?remote=true',
      'http://127.0.0.1:5173/#fragment', 'http://127.0.0.1:0', 'http://127.0.0.1:65536']) {
      expect(developmentOrigin(value, false)).toBeNull();
    }
  });

  it('always loads packaged assets in packaged builds', () => {
    expect(developmentOrigin('http://127.0.0.1:5174', true)).toBeNull();
  });
});
