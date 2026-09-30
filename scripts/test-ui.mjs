import {_electron as electron} from '@playwright/test';
import {resolve} from 'node:path';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import assert from 'node:assert/strict';
const dataDir=resolve(process.env.DEMO_TEST_DATA_DIR||'.tmp/integration-library-v2');
await mkdir('artifacts',{recursive:true});
const packaged=process.env.DEMO_PACKAGED_EXE;
const app=await electron.launch({...(packaged?{executablePath:resolve(packaged)}:{}),args:packaged?[]:['.'],env:{...process.env,CS_DEMO_REVIEW_DATA_DIR:dataDir,...(packaged?{PATH:`${process.env.SystemRoot}\\System32;${process.env.SystemRoot}`}:{})},timeout:60_000});
const page=await app.firstWindow(),errors=[];page.on('pageerror',e=>errors.push(String(e)));
try{
 await page.waitForFunction(()=>!!window.csDemo);
 const demos=await page.evaluate(()=>window.csDemo.listDemos());const cs2=demos.find(d=>d.engine==='cs2');assert.ok(cs2);
 // Native import of an existing demo must emit completion once, not trigger a refresh feedback loop.
 await app.evaluate(({dialog},path)=>{dialog.showOpenDialog=async()=>({canceled:false,filePaths:[path]});},cs2.path);
 await page.evaluate(()=>{window.__testImportCompletions=0;window.csDemo.onImportProgress(p=>{if(p.stage==='complete')window.__testImportCompletions++;});});
 await page.getByRole('button',{name:'Import demo',exact:true}).click();
 await page.waitForFunction(()=>window.__testImportCompletions>=1,{timeout:30000});await page.waitForTimeout(500);
 assert.equal(await page.evaluate(()=>window.__testImportCompletions),1,'Import completion must not loop through library refreshes');
 await page.locator('.demo-card').filter({hasText:cs2.map}).locator('.demo-select').click();
 await page.waitForFunction(id=>document.body.innerText.includes('Match replay')&&document.querySelector('.demo-card.selected .demo-filename')?.textContent===id,cs2.name);
 await page.waitForTimeout(500);
 // Exercise the real local map pipeline and then reload so the UI obtains its URLs.
 const map=await page.evaluate(async id=>await window.csDemo.getMap(id)||await window.csDemo.extractMap(id),cs2.id);
 assert.ok(map?.image?.startsWith('demomap://asset/'));
 await page.reload();await page.waitForFunction(()=>!!window.csDemo);
 await page.locator('.demo-card').filter({hasText:cs2.map}).locator('.demo-select').click();
 await page.waitForTimeout(1200);
 const slider=page.locator('input[type="range"]').first();
 await slider.fill(String(Math.round(cs2.tickRate*180)));await slider.dispatchEvent('input');await slider.dispatchEvent('change');
 await page.waitForTimeout(500);
 const tickBefore=Number(await slider.inputValue());await page.getByRole('button',{name:'Play',exact:true}).click();await page.waitForTimeout(900);await page.getByRole('button',{name:'Pause',exact:true}).click();
 assert.ok(Number(await slider.inputValue())>tickBefore,'Playback must advance');
 await page.getByRole('button',{name:'Bookmark this tick'}).click();
 await page.getByPlaceholder('What happened? Add context or an alternative explanation…').fill('UI verification note <script> is plain text.');
 await page.getByRole('button',{name:'Save note',exact:true}).click();await page.waitForTimeout(200);
 const match=await page.evaluate(id=>window.csDemo.getMatch(id),cs2.id);assert.ok(match.notes.some(n=>n.text.startsWith('UI verification note')));
 await page.getByRole('button',{name:'Settings',exact:true}).first().click();
 const modal=page.getByRole('dialog',{name:'Workspace settings'});await modal.waitFor();
 assert.ok((await modal.innerText()).includes('Source 2 Viewer'));await page.screenshot({path:resolve('artifacts/settings.png')});
 await page.getByRole('button',{name:'Save settings',exact:true}).click();await modal.waitFor({state:'hidden'});
 await app.evaluate(({dialog},path)=>{dialog.showSaveDialog=async()=>({canceled:false,filePath:path});},resolve('artifacts/test-report.html'));
 const report=await page.evaluate(id=>window.csDemo.exportReport(id,'html'),cs2.id);assert.ok(report);assert.ok((await readFile(report,'utf8')).includes('&lt;script&gt;'));
 await app.evaluate(({dialog})=>{dialog.showMessageBox=async()=>({response:0,checkboxChecked:false});});
 await page.locator('.demo-card.selected .demo-delete').click();await page.waitForTimeout(200);assert.equal((await page.evaluate(()=>window.csDemo.listDemos())).length,demos.length);
 const nowTick=Number(await slider.inputValue());
 const context=await page.evaluate(async ({id,tick})=>window.csDemo.getReplay(id,Math.round(tick),Math.round(tick)+128),{id:cs2.id,tick:nowTick});
 const living=context.samples.find(s=>s.alive&&s.health>0);
 if(living){const name=match.players.find(p=>p.id===living.playerId)?.name;if(name)await page.getByRole('row').filter({hasText:name}).click();}
 await page.getByRole('button',{name:'Aim trace',exact:true}).click();await page.waitForTimeout(250);
 const chart=page.getByRole('slider',{name:/Aim trace for/});await chart.press('ArrowRight');
 await page.screenshot({path:resolve('artifacts/desktop-dust2.png'),fullPage:true});
 await page.setViewportSize({width:1100,height:760});await page.waitForTimeout(300);await page.screenshot({path:resolve('artifacts/desktop-compact.png')});
 const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>window.innerWidth+2);assert.equal(overflow,false,'App should fit compact viewport');
 for(const n of match.notes.filter(n=>n.text.startsWith('UI verification note')||n.kind==='bookmark'))await page.evaluate(id=>window.csDemo.deleteNote(id),n.id);
 assert.deepEqual(errors,[]);console.log(JSON.stringify({passed:true,map:map.map,mapVerified:map.verified,assessments:match.players.map(p=>p.verdict),errors},null,2));
 await writeFile('artifacts/ui-results.json',JSON.stringify({passed:true,map:map.map,errors,scenarios:['native map extraction','playback','seeking','bookmark','note','settings','HTML report','cancel removal','compact layout']},null,2));
}finally{await app.close();}
