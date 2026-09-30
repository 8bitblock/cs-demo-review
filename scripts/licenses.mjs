import {readFile,readdir,writeFile,mkdir} from 'node:fs/promises';
import {spawnSync} from 'node:child_process';
import {resolve} from 'node:path';
import {go,root} from './runtime.mjs';
const sections=['CS Demo Review — third-party license texts\nGenerated from the exact installed dependencies. Game assets are not included.'];
async function add(name,directory){
 const names=await readdir(directory).catch(()=>[]);
 const licenses=names.filter(n=>/^(licen[sc]e|copying|copyright|notice|unlicense)(\.|$)/i.test(n));
 for(const file of licenses){const contents=await readFile(resolve(directory,file),'utf8').catch(()=>null);if(contents&&contents.length<500000)sections.push(`${'='.repeat(76)}\n${name} — ${file}\n${'='.repeat(76)}\n${contents}`);}
}
for(const name of ['react','react-dom','scheduler','lucide-react','zod','electron'])await add(name,resolve(root,'node_modules',name));
const modules=spawnSync(go,['list','-m','-f','{{.Path}}|{{.Version}}|{{.Dir}}','all'],{cwd:resolve(root,'worker'),encoding:'utf8'});
if(modules.status!==0)throw new Error(modules.stderr||'Go module listing failed');
for(const line of modules.stdout.trim().split(/\r?\n/)){const[name,version,directory]=line.split('|');if(directory&&version)await add(`${name} ${version}`,directory);}
await add('Source2Viewer 20.0',resolve(root,'.tools/source2viewer'));
await mkdir(resolve(root,'build'),{recursive:true});await writeFile(resolve(root,'build/THIRD-PARTY-LICENSES.txt'),sections.join('\n\n')+'\n');
console.log(`Collected ${sections.length-1} dependency license texts.`);
