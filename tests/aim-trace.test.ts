import {describe,it,expect} from 'vitest';
import {buildAimTrace} from '../src/aim-trace-data';
import type {Sample} from '../shared/types';
const sample=(tick:number,yaw:number,extra:Partial<Sample>={}):Sample=>({tick,time:tick/64,playerId:'p',x:10,y:10,z:10,yaw,pitch:0,health:100,armor:0,team:'CT',alive:true,weapon:'AK-47',scoped:false,flashed:false,crouching:false,velocity:0,...extra});
describe('recorded aim charts',()=>{
 it('measures angle wrapping without artificial spikes',()=>{const trace=buildAimTrace([sample(1,359),sample(2,1)],'p',64);expect(trace.segments[0][0].yawSpeed).toBe(128);expect(trace.intervalMs).toBe(15.625);});
 it('breaks missing/dead/teleport samples rather than fabricating zero speed',()=>{const trace=buildAimTrace([sample(1,0),sample(2,1),sample(100,2),sample(101,3,{alive:false}),sample(102,4,{x:3000})],'p',64);expect(trace.gaps+trace.omitted).toBeGreaterThan(0);expect(trace.segments.flat().length).toBeLessThan(4);});
 it('preserves genuinely stationary aim as a real zero',()=>{const trace=buildAimTrace([sample(1,20),sample(2,20)],'p',64);expect(trace.segments[0][0].yawSpeed).toBe(0);});
});
