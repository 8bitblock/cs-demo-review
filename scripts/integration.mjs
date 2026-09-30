import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {randomUUID} from 'node:crypto';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import {resolve,basename} from 'node:path';
import assert from 'node:assert/strict';

const dataDir=resolve(process.env.DEMO_TEST_DATA_DIR||'.tmp/integration-library');
await mkdir(dataDir,{recursive:true});
const worker=spawn(resolve(process.env.DEMO_TEST_WORKER||'worker/bin/demo-worker.exe'),['--data-dir',dataDir],{stdio:'pipe',windowsHide:true});
const pending=new Map(),jobs=new Map();let lastProgress=0;
const stderr=[];worker.stderr.on('data',b=>stderr.push(b.toString()));
createInterface({input:worker.stdout}).on('line',line=>{
  let message;try{message=JSON.parse(line);}catch{console.error('Invalid stdout',line.slice(0,200));return;}
  if(message.event==='import-progress'){
    const p=message.data;
    if(Date.now()-lastProgress>5000||['complete','error','cancelled'].includes(p.stage)){console.log(`${p.name}: ${p.stage} ${Math.round(p.progress*100)}% ${p.message}`);lastProgress=Date.now();}
    const waiter=jobs.get(p.jobId);if(waiter&&['complete','error','cancelled'].includes(p.stage)){jobs.delete(p.jobId);waiter.resolve(p);}
  }else if(message.id){const req=pending.get(message.id);if(req){pending.delete(message.id);message.error?req.reject(new Error(message.error.message)):req.resolve(message.result);}}
});
worker.on('error',e=>{console.error(e);process.exitCode=1;});
function rpc(method,params={}){return new Promise((resolve,reject)=>{const id=randomUUID();pending.set(id,{resolve,reject});worker.stdin.write(JSON.stringify({id,method,params})+'\n');});}
function importDemo(path,jobId=randomUUID()){const finished=new Promise((resolve,reject)=>jobs.set(jobId,{resolve,reject}));return rpc('importDemo',{path:resolve(path),jobId}).then(()=>finished);}
const timeout=setTimeout(()=>{console.error('Integration timeout',stderr.join('\n'));worker.kill();process.exit(1);},20*60_000);
try{
  const demos=process.argv.slice(2);
  assert.ok((await rpc('diagnostics')).workerVersion);
  const results=[];
  for(const path of demos){
    const start=performance.now(), result=await importDemo(path);assert.equal(result.stage,'complete',result.message);assert.ok(result.demoId);
    const match=await rpc('getMatch',{id:result.demoId});assert.ok(match.players.length>0);assert.ok(match.rounds.length>0);assert.ok(match.demo.totalTicks>0);
    if(match.demo.engine==='csgo' && basename(path)==='default.dem'){
      // Independent oracle: demoinfocs v3.3.0 test/default.golden and its pinned cs-demos submodule.
      assert.equal(match.events.filter(e=>e.kind==='kill').length,220,'Kill count disagrees with upstream golden output');
      assert.equal(match.events.filter(e=>e.kind==='damage').length,811,'Damage count disagrees with upstream golden output');
      assert.equal(match.rounds.length,32,'Round count disagrees with upstream golden output');
    }
    const tick=match.rounds[Math.min(1,match.rounds.length-1)].startTick;
    const replay=await rpc('getReplay',{id:result.demoId,fromTick:tick,toTick:Math.min(tick+2048,match.demo.totalTicks)});assert.ok(replay.samples.length>0);
    const note=await rpc('saveNote',{demoId:result.demoId,tick,playerId:match.players[0].id,text:'Integration verification — <>& Unicode ✓',kind:'bookmark'});assert.ok(note.id);
    assert.ok((await rpc('getMatch',{id:result.demoId})).notes.some(n=>n.id===note.id));await rpc('deleteNote',{id:note.id});
    const duplicate=await importDemo(path);assert.equal(duplicate.demoId,result.demoId);
    const summary={file:basename(path),id:result.demoId,engine:match.demo.engine,map:match.demo.map,duration:match.demo.duration,tickRate:match.demo.tickRate,totalTicks:match.demo.totalTicks,players:match.players.length,rounds:match.rounds.length,events:match.events.length,findings:match.findings.length,replaySamples:replay.samples.length,seconds:Math.round((performance.now()-start)/1000),warnings:match.demo.warnings};
    results.push(summary);console.log(JSON.stringify(summary,null,2));
  }
  const corrupt=resolve(dataDir,'corrupt.dem');await writeFile(corrupt,Buffer.from('not a demo'));const corruptResult=await importDemo(corrupt);assert.equal(corruptResult.stage,'error');
  const truncated=resolve(dataDir,'truncated.dem');await writeFile(truncated,Buffer.from('PBDEMS2\0'));const truncatedResult=await importDemo(truncated);assert.equal(truncatedResult.stage,'error');
  const list=await rpc('listDemos');assert.ok(Array.isArray(list));
  await writeFile(resolve(dataDir,'integration-results.json'),JSON.stringify({testedAt:new Date().toISOString(),results,invalidFilesRejected:true},null,2));
  console.log(`Integration checks passed. ${list.length} demos in test library.`);
}catch(error){console.error(error);console.error(stderr.join('\n').slice(-5000));process.exitCode=1;}
finally{clearTimeout(timeout);worker.kill();}
