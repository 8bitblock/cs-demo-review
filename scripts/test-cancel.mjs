import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {randomUUID} from 'node:crypto';
import {resolve} from 'node:path';
import {stat,mkdir,writeFile} from 'node:fs/promises';
import assert from 'node:assert/strict';
const path=resolve(process.argv[2]||'');if(!process.argv[2])throw new Error('Pass an uncached large .dem file to test cancellation.');
const dataDir=resolve('.tmp/cancellation-library');await mkdir(dataDir,{recursive:true});
const before=await stat(path),jobId=randomUUID(),pending=new Map();let cancelled=false,completed;
const done=new Promise(resolve=>completed=resolve);
const worker=spawn(resolve('worker/bin/demo-worker.exe'),['--data-dir',dataDir],{stdio:'pipe',windowsHide:true});
function rpc(method,params={}){return new Promise((resolve,reject)=>{const id=randomUUID();pending.set(id,{resolve,reject});worker.stdin.write(JSON.stringify({id,method,params})+'\n');});}
createInterface({input:worker.stdout}).on('line',line=>{
 const m=JSON.parse(line);
 if(m.event==='import-progress'&&m.data.jobId===jobId){
  if(m.data.stage==='parsing'&&m.data.progress>0.02&&!cancelled){cancelled=true;void rpc('cancelImport',{jobId});}
  if(['cancelled','error','complete'].includes(m.data.stage))completed(m.data);
 }else if(m.id){const p=pending.get(m.id);if(p){pending.delete(m.id);m.error?p.reject(new Error(m.error.message)):p.resolve(m.result);}}
});
const timeout=setTimeout(()=>{worker.kill();throw new Error('Cancellation test timeout');},90_000);
try{
 await rpc('importDemo',{path,jobId});
 // Query the library during a live job: should not wait for a full parse.
 const t=performance.now();await rpc('listDemos');assert.ok(performance.now()-t<2000,'Library reads stalled during import');
 const result=await done;assert.equal(result.stage,'cancelled',result.message);assert.equal((await rpc('listDemos')).length,0);
 const after=await stat(path);assert.equal(after.size,before.size);assert.equal(after.mtimeMs,before.mtimeMs);
 await writeFile(resolve(dataDir,'cancellation-results.json'),JSON.stringify({passed:true,sourceUnchanged:true,readLatencyMs:performance.now()-t,result},null,2));
 console.log('Cancellation, responsive library read, source preservation, and partial-cache cleanup passed.');
}finally{clearTimeout(timeout);worker.kill();}
