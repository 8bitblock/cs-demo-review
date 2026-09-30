import {mkdir,writeFile,readFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import {resolve} from 'node:path';
const root=resolve(import.meta.dirname,'..');
const wantGo=process.argv.includes('--go'),wantExtractor=process.argv.includes('--extractor');
if(!wantGo&&!wantExtractor){console.log('Usage: node scripts/setup-tools.mjs --go --extractor');process.exit(0);}
await mkdir(resolve(root,'.tools'),{recursive:true});
async function download(url,file,hash){
  const response=await fetch(url);if(!response.ok)throw new Error(`Download failed: ${response.status} ${url}`);
  const bytes=Buffer.from(await response.arrayBuffer());if(hash&&createHash('sha256').update(bytes).digest('hex')!==hash)throw new Error('Download checksum mismatch');
  await writeFile(file,bytes);
}
function extract(file,destination){
  const quote=s=>"'"+s.replaceAll("'","''")+"'";
  const result=spawnSync('powershell.exe',['-NoProfile','-NonInteractive','-Command',`Expand-Archive -LiteralPath ${quote(file)} -DestinationPath ${quote(destination)} -Force`],{stdio:'inherit',windowsHide:true});
  if(result.status!==0)throw new Error('Archive extraction failed');
}
if(wantGo){
  const file=resolve(root,'.tools/go1.27.1.windows-amd64.zip');
  await download('https://go.dev/dl/go1.27.1.windows-amd64.zip',file,'a3911b5e0e1b1053f25ed0675f4c1c6aad1e2bfcf253df2b9be4caabd2edd95d');
  extract(file,resolve(root,'.tools'));console.log('Go 1.27.1 provisioned locally.');
}
if(wantExtractor){
  const file=resolve(root,'.tools/source2viewer.zip'),destination=resolve(root,'.tools/source2viewer');await mkdir(destination,{recursive:true});
  await download('https://github.com/ValveResourceFormat/ValveResourceFormat/releases/download/20.0/cli-windows-x64.zip',file,'d32ab327b8bbb42a2528866afb03bb582bdb779d0005488da32b90292afd3ff5');
  extract(file,destination);await download('https://raw.githubusercontent.com/ValveResourceFormat/ValveResourceFormat/20.0/LICENSE',resolve(destination,'LICENSE.txt'));console.log('Source2Viewer 20.0 provisioned locally.');
}
