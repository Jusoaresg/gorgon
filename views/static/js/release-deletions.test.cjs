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

function browserFixture(operations, retryOK = false) {
 const previous = { document: global.document, fetch: global.fetch, sessionStorage: global.sessionStorage };
 class Element {
  constructor(tag='div') { this.tag=tag; this.children=[];this.dataset={};this.listeners={};this.className='';this.disabled=false;this.classList={contains:name=>this.className.split(' ').includes(name)}; }
  set textContent(value) { this.text=value;this.children=[]; }
  get textContent() { return this.text || ''; }
  appendChild(child) { child.parent=this;this.children.push(child);return child; }
  setAttribute(name,value) { this[name]=value; }
  addEventListener(name,fn) { this.listeners[name]=fn; }
  remove() { if(this.parent)this.parent.children=this.parent.children.filter(child=>child!==this); }
  querySelectorAll(selector) { const all=this.children.flatMap(child=>[child,...child.querySelectorAll(selector)]);return all.filter(child=>selector==='button,select,input,[hx-delete],[hx-post]'?child.tag==='button':selector.startsWith('.')?child.classList.contains(selector.slice(1)):false); }
  querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
 }
 const body=new Element('body');const card=new Element();card.dataset.episodeId='1';card.tracking='downloaded';const control=new Element('button');card.appendChild(control);body.appendChild(card);
 const document={body,createElement:tag=>new Element(tag),getElementById:id=>{ const visit=node=>node.id===id?node:node.children.map(visit).find(Boolean);return visit(body)||null; },querySelectorAll:()=>[document.card],card};
 const notifications=[];const tracking=[];
 global.document=document;global.sessionStorage={getItem:()=>null,setItem:()=>{}};
 global.fetch=async(path,options)=> options && options.method==='POST' ? {ok:retryOK,json:async()=>({operation:{}})} : {ok:true,json:async()=>({operations})};
 const monitor=deletion.attach((message,type)=>notifications.push({message,type}),(id,value)=>{tracking.push(value);document.card.tracking=value;});
 return {monitor,document,card,control,notifications,tracking,Element,restore(){Object.assign(global,previous);}};
}

test('page load uses current episode tracking rather than historical deletion intention',async()=>{
 const browser=browserFixture([{id:'old',target:'hash:abc',episode_id:1,status:'completed',stage:'completed',intention:'skipped',current_tracking:'downloaded',version:6,created_at:1}]);
 try {await browser.monitor.refresh();assert.equal(browser.card.tracking,'downloaded');assert.ok(!browser.tracking.includes('skipped'));assert.equal(browser.notifications.length,0);}finally{browser.restore();}
});
test('new pending target survives historical events and pending controls are reapplied after swaps',async()=>{
 const old={id:'old',target:'hash:abc',episode_id:1,status:'completed',stage:'completed',intention:'skipped',version:6,created_at:1};
 const current={id:'current',target:'hash:abc',episode_id:1,status:'pending',stage:'queued',version:1,created_at:2};
 const browser=browserFixture([current,old]);
 try {
  await browser.monitor.refresh();assert.equal(browser.control.disabled,true);
  browser.monitor.consume({...old,version:7},false);assert.equal(browser.control.disabled,true);assert.ok(!browser.tracking.includes('skipped'));
  const replacement=new browser.Element();replacement.dataset.episodeId='1';const button=new browser.Element('button');replacement.appendChild(button);browser.document.card=replacement;
  browser.monitor.reapply();assert.equal(button.disabled,true);assert.match(replacement.querySelector('.deletion-state').textContent,/pending/);
 }finally{browser.restore();}
});
test('retry transport rejection restores the button and displays a failure',async()=>{
 const browser=browserFixture([{id:'failed',target:'hash:abc',episode_id:1,status:'failed',stage:'links',version:4,created_at:1}]);
 try {
  await browser.monitor.refresh();const button=browser.card.querySelector('.deletion-retry');assert.ok(button);
  button.listeners.click({stopPropagation(){}});await new Promise(resolve=>setImmediate(resolve));
  assert.equal(button.disabled,false);assert.equal(browser.notifications.at(-1).type,'error');
 }finally{browser.restore();}
});
test('search actions do not subscribe to unrelated deletion broadcasts',async()=>{
 const browser=browserFixture([]);
 try {
  await browser.monitor.refresh();browser.document.body.listeners['htmx:beforeRequest']({detail:{requestConfig:{path:'/api/v1/database/show/episode/search',verb:'post'},elt:{closest:()=>browser.card}}});
  browser.monitor.consume({id:'other',target:'hash:abc',episode_id:1,status:'pending',stage:'queued',version:1,created_at:1},false);
  assert.equal(browser.notifications.length,0);assert.equal(browser.monitor.consumeResponse({operation:{}}),false);
 }finally{browser.restore();}
});

test('older release completion cannot overwrite a newer deletion on the same episode',()=>{
 const state=deletion.createState();deletion.follow(state,'old');
 deletion.apply(state,{id:'new',target:'hash:new',hash:'new',episode_id:1,status:'pending',version:1,created_at:20});
 const late=deletion.apply(state,{id:'old',target:'hash:old',hash:'old',episode_id:1,status:'completed',version:9,created_at:10});
 assert.equal(late.current,false);assert.equal(late.notification,null);
});

test('scheduled retry is pending and exhaustion produces an actionable failure notification',()=>{
 const state=deletion.createState();deletion.follow(state,'retry');
 const pending=deletion.apply(state,{id:'retry',status:'failed',next_attempt_at:123,retryable:true,version:2});
 assert.equal(pending.notification.type,'info');assert.match(pending.notification.message,/automatic retry/);
 const exhausted=deletion.apply(state,{id:'retry',status:'failed',next_attempt_at:0,retryable:true,version:3});
 assert.equal(exhausted.notification.type,'error');
});

test('mixed batch preserves individual failures alongside accepted deletion and deduplicates response',async()=>{
 const browser=browserFixture([]);
 try {
  await browser.monitor.refresh();
  const response={results:[{episode_id:1,status:'pending',operation:{id:'accepted',episode_id:1,status:'pending',version:1}},{episode_id:2,status:'failed',error:'Ownership could not be verified'}]};
  assert.equal(browser.monitor.consumeResponse(response),true);
  assert.ok(browser.notifications.some(note=>note.type==='error' && /Episode 2/.test(note.message)));
  const count=browser.notifications.length;browser.monitor.consumeResponse(response);assert.equal(browser.notifications.length,count);
 }finally{browser.restore();}
});
