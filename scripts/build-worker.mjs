import { spawnSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { resolve } from 'node:path';
import { go, root } from './runtime.mjs';
mkdirSync(resolve(root, 'worker/bin'), { recursive: true });
for(const name of ['demo-worker','demo-parser-cs2','demo-parser-csgo']){
  const result = spawnSync(go, ['build', '-trimpath', '-ldflags=-s -w', '-o', `bin/${name}.exe`, `./cmd/${name}`], { cwd: resolve(root, 'worker'), stdio: 'inherit', env: { ...process.env, CGO_ENABLED: '0' } });
  if (result.error) console.error('Install Go 1.24+ or set GO_BINARY:', result.error.message);
  if(result.status!==0)process.exit(result.status??1);
}
