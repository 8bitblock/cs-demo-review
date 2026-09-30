import { spawn } from 'node:child_process';
import { createRequire } from 'node:module';
import { resolve } from 'node:path';
import { createServer } from 'vite';
import { root } from './runtime.mjs';
import './build-electron.mjs';
const require = createRequire(import.meta.url);
const server = await createServer({ root, configFile: resolve(root, 'vite.config.ts') });
await server.listen();
const address = server.httpServer?.address();
if (!address || typeof address === 'string') {
  await server.close();
  throw new Error('The development server did not open a local TCP port.');
}
const devURL = `http://127.0.0.1:${address.port}`;
console.log(`Development server: ${devURL}`);
const child = spawn(require('electron'), [root], { cwd: root, stdio: 'inherit', env: { ...process.env, DEMO_REVIEW_DEV_URL: devURL } });
child.on('error', async (error) => { console.error(error); await server.close(); process.exit(1); });
child.on('exit', async (code) => { await server.close(); process.exit(code ?? 0); });
process.on('SIGINT', () => child.kill());
