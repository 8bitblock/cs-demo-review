import { readdir, readFile, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
const files=(await readdir('release')).filter(f=>/\.(exe|zip)$/.test(f));
const lines=await Promise.all(files.map(async file=>`${createHash('sha256').update(await readFile(`release/${file}`)).digest('hex')}  ${file}`));
await writeFile('release/SHA256SUMS.txt',lines.join('\n')+'\n');
console.log(`Checksums written for ${files.length} artifacts.`);
