import {_electron as electron} from '@playwright/test';
import {resolve} from 'node:path';
import {mkdir,writeFile} from 'node:fs/promises';
import assert from 'node:assert/strict';
const dataDir=resolve(process.env.DEMO_TEST_DATA_DIR||'.tmp/integration-library');
await mkdir('artifacts',{recursive:true});
const packaged=process.env.DEMO_PACKAGED_EXE;
const app=await electron.launch({...(packaged?{executablePath:resolve(packaged)}:{}),args:packaged?[]:['.'],env:{...process.env,CS_DEMO_REVIEW_DATA_DIR:dataDir,...(packaged?{PATH:`${process.env.SystemRoot}\\System32;${process.env.SystemRoot}`}:{})},timeout:60_000});
const page=await app.firstWindow();const errors=[];
page.on('pageerror',e=>errors.push(String(e)));
try{
  await page.waitForFunction(()=>!!window.csDemo,{timeout:30000});
  await page.waitForTimeout(2500);
  const demos=await page.evaluate(()=>window.csDemo.listDemos());
  assert.ok(Array.isArray(demos));
  const diagnostics=await page.evaluate(()=>window.csDemo.getDiagnostics());assert.ok(diagnostics.workerVersion);
  if(demos.length){
    const match=await page.evaluate(id=>window.csDemo.getMatch(id),demos[0].id);assert.ok(match.players.length);
    const commands=await page.evaluate(id=>window.csDemo.getPlaybackCommands(id,128),demos[0].id);assert.ok(commands.play.startsWith('playdemo'));assert.equal(commands.seek,'demo_gototick 128');
  }
  await page.screenshot({path:resolve('artifacts/desktop-review.png'),fullPage:true});
  const text=await page.locator('body').innerText();
  await writeFile('artifacts/desktop-smoke.json',JSON.stringify({diagnostics,demoCount:demos.length,errors,bodyExcerpt:text.slice(0,6000)},null,2));
  console.log(JSON.stringify({demoCount:demos.length,errors,screenshot:'artifacts/desktop-review.png',body:text.slice(0,2500)},null,2));
  assert.deepEqual(errors,[]);
}finally{await app.close();}
