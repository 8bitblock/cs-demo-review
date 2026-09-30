import { spawnSync } from 'node:child_process';
import { resolve } from 'node:path';
import { go, root } from './runtime.mjs';
const result = spawnSync(go, ['test', './...'], { cwd: resolve(root, 'worker'), stdio: 'inherit' });
process.exit(result.status ?? 1);
