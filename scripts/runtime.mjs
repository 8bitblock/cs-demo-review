import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
export const root = resolve(import.meta.dirname, '..');
export const go = process.env.GO_BINARY || (existsSync(resolve(root, '.tools/go/bin/go.exe')) ? resolve(root, '.tools/go/bin/go.exe') : 'go');
