import {describe,it,expect} from 'vitest';
import {samplesAtTick,visibleRadarBounds} from '../src/ReplayMap';
import type {Sample} from '../shared/types';
const s=(tick:number,yaw=0,alive=true):Sample=>({tick,time:tick/64,playerId:'p',x:tick,y:0,z:0,yaw,pitch:0,health:alive?100:0,armor:0,team:'CT',alive,weapon:'AK-47',scoped:false,flashed:false,crouching:false,velocity:0});
describe('replay display sampling',()=>{
 it('interpolates angles across zero through the short arc, on unsorted frames',()=>{const frames=samplesAtTick([s(12,1),s(10,359)],11,64);expect(frames[0].x).toBe(11);expect(frames[0].yaw%360).toBe(0);});
 it('never invents a future-only player or keeps stale player snapshots',()=>{expect(samplesAtTick([s(10)],1,64)).toEqual([]);expect(samplesAtTick([s(1)],100,64)).toEqual([]);});
 it('does not animate a player through a teleport or new spawn',()=>{const before=s(10),after={...s(12),x:2000};expect(samplesAtTick([before,after],11,64)[0].x).toBe(10);});
 it('does not interpolate through death or sparse gaps',()=>{expect(samplesAtTick([s(10,0),s(12,90,false)],11,64)[0].yaw).toBe(0);expect(samplesAtTick([s(10,0),s(80,90)],20,64)[0].yaw).toBe(0);});
});

describe('radar fitting',()=>{
 it('fits visible radar pixels without changing their original image coordinates',()=>{const pixels=new Uint8ClampedArray(100*100*4);for(let y=20;y<=79;y++)for(let x=10;x<=89;x++)pixels[(y*100+x)*4+3]=255;expect(visibleRadarBounds(pixels,100,100)).toEqual({x:7,y:17,width:86,height:66});});
 it('ignores completely transparent images and retains opaque full images',()=>{expect(visibleRadarBounds(new Uint8ClampedArray(400),10,10)).toBeNull();expect(visibleRadarBounds(new Uint8ClampedArray(400).fill(255),10,10)).toEqual({x:0,y:0,width:10,height:10});});
});
