const test = require('node:test');
const assert = require('node:assert/strict');
const deletion = require('./release-deletions.js');

test('pending never announces success, duplicate and stale events are ignored', () => {
 const state=deletion.createState();deletion.follow(state,'one');
 assert.equal(deletion.apply(state,{id:'one',status:'pending',version:1}).notification.type,'info');
 assert.equal(deletion.apply(state,{id:'one',status:'pending',version:1}),null);
 assert.equal(deletion.apply(state,{id:'one',status:'completed',version:3}).notification.type,'success');
 assert.equal(deletion.apply(state,{id:'one',status:'running',version:2}),null);
});
test('broadcasts update state but only followed actions notify and concurrent actions stay independent',()=>{
 const state=deletion.createState();deletion.follow(state,'mine');
 assert.equal(deletion.apply(state,{id:'other',status:'failed',version:1}).notification,null);
 assert.equal(deletion.apply(state,{id:'mine',status:'failed',version:2}).notification.type,'error');
 assert.equal(state.operations.other.status,'failed');assert.equal(state.operations.mine.status,'failed');
});
test('persisted query after reconnect recovers a lost result without duplicate notifications',()=>{
 const state=deletion.createState();deletion.follow(state,'lost');
 assert.equal(deletion.apply(state,{id:'lost',status:'completed',version:8}).notification.type,'success');
 assert.equal(deletion.apply(state,{id:'lost',status:'completed',version:8}),null);
});
