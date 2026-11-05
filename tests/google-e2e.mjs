import assert from 'node:assert/strict';
import {test} from 'node:test';
import {chromium} from 'playwright';

const base=process.env.BROWSER_TEST_BASE_URL||'http://127.0.0.1:8094';
test('Google-only sign-in: no local passwords, token field or anonymous jobs',async()=>{
 const browser=await chromium.launch({headless:true});
 try{
  const context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true});
  const page=await context.newPage();
  await page.goto(base+'/',{waitUntil:'domcontentloaded'});
  await page.locator('#loginPanel').waitFor({state:'visible'});
  assert.equal(await page.locator('#token').count(),0);
  assert.equal(await page.locator('#password').count(),0);
  assert.equal(await page.locator('#adminPanel').count(),0);
  assert.equal(await page.locator('#jobForm').isVisible(),false);
  const link=page.locator('#googleSignIn');
  assert.equal(await link.getAttribute('href'),'/auth/google');
  assert.match(await page.locator('#loginPanel').innerText(),/@rcat\.ac\.th/);
  const response=await page.request.get(base+'/api/jobs');
  assert.equal(response.status(),401);
  const google=await page.request.get(base+'/auth/google',{maxRedirects:0});
  assert.equal(google.status(),303);
  const redirect=new URL(google.headers().location);
  assert.equal(redirect.hostname,'accounts.google.com');
  assert.equal(redirect.searchParams.get('hd'),'rcat.ac.th');
  assert.equal(redirect.searchParams.get('code_challenge_method'),'S256');
  assert.equal(redirect.searchParams.get('scope'),'openid email');
  const cookies=await context.cookies();
  assert.equal(cookies.some(c=>c.name==='anyflip_session'),false,'anonymous session issued');
  await context.close();
 }finally{await browser.close()}
});


test('Google workspace dashboard: responsive authenticated experience and history controls',async()=>{
 const browser=await chromium.launch({headless:true});
 try{
  for(const viewport of [{width:1365,height:900},{width:390,height:844}]){
   const context=await browser.newContext({viewport,isMobile:viewport.width<600,hasTouch:viewport.width<600});
   const page=await context.newPage();
   const failures=[];
   page.on('pageerror',e=>failures.push(e.message));
   await page.route('**/auth/me',route=>route.fulfill({
    status:200,contentType:'application/json',body:JSON.stringify({email:'person@rcat.ac.th'})
   }));
   await page.route('**/api/jobs',route=>route.fulfill({
    status:200,contentType:'application/json',body:JSON.stringify({jobs:[
     {id:'a'.repeat(32),stage:'completed',completed:5,total:5,downloadReady:true},
     {id:'b'.repeat(32),stage:'queued',completed:0,total:20,downloadReady:false},
     {id:'c'.repeat(32),stage:'cancelled',completed:2,total:10,downloadReady:false,archived:true}
    ]})
   }));
   await page.goto(base+'/',{waitUntil:'networkidle'});
   await page.locator('#workArea').waitFor({state:'visible'});
   assert.equal(await page.locator('#loginPanel').isVisible(),false);
   assert.equal(await page.locator('#signedInUser').textContent(),'person@rcat.ac.th');
   await page.waitForFunction(()=>document.querySelector('#statTotal')?.textContent==='3');
   assert.equal(await page.locator('#statActive').textContent(),'1');
   assert.equal(await page.locator('#statDone').textContent(),'1');
   assert.equal(await page.locator('.nav-link:visible').count(),4);
   assert.equal(await page.locator('#adminNav').isVisible(),false);
   assert.equal(await page.locator('#adminConsole').isVisible(),false);
   assert.equal(await page.locator('#token').count(),0);
   assert.equal(await page.locator('#adminPanel').count(),0);
   await page.locator('#historyFilter').selectOption('completed');
   assert.equal(await page.locator('.historyRow').count(),1);
   await page.locator('#historySearch').fill('does-not-match');
   assert.match(await page.locator('#historyList').innerText(),/ไม่พบรายการ/);
   await page.locator('#historySearch').fill('');
   assert.equal(await page.locator('.historyRow').count(),1);
   await page.locator('#batchURLs').fill('https://anyflip.com/demo/one\nhttps://anyflip.com/demo/two');
   assert.equal(await page.locator('#batchCount').textContent(),'2 / 8');
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>window.innerWidth+2),false,'horizontal overflow');
   assert.deepEqual(failures,[]);
   await context.close();
  }
 }finally{await browser.close()}
});


test('Workspace Admin Console: role-aware pages, responsive layout and confirmed privileged actions',async()=>{
 const browser=await chromium.launch({headless:true});
 try{
  for(const viewport of [{width:1365,height:900},{width:390,height:844}]){
   const context=await browser.newContext({viewport,isMobile:viewport.width<600,hasTouch:viewport.width<600});
   const page=await context.newPage();
   const requests=[];
   page.on('dialog',dialog=>dialog.accept());
   page.on('pageerror',e=>{throw e});
   const superRef='g-'+'a'.repeat(64), otherRef='g-'+'b'.repeat(64), jobId='c'.repeat(32);
   await page.route('**/auth/me',route=>route.fulfill({
    status:200,contentType:'application/json',
    body:JSON.stringify({email:'principal@rcat.ac.th',role:'super_admin',identityRef:superRef})
   }));
   await page.route('**/api/jobs',route=>route.fulfill({
    status:200,contentType:'application/json',body:JSON.stringify({jobs:[]})
   }));
   await page.route('**/api/admin/**',route=>{
    const url=new URL(route.request().url()),method=route.request().method();
    let body={};
    if (url.pathname.endsWith('/summary')) body={total:2,active:1,queued:1,running:0,completed:1,failed:0,activeReservations:1,maxConcurrent:2,storage:{totalBytes:6*1024**3,usedBytes:1*1024**3,availableBytes:5*1024**3,usedPercent:17}};
    else if (url.pathname==='/api/admin/jobs') body={total:1,jobs:[{
      id:jobId,owner:otherRef,stage:'queued',completed:0,total:5,created:new Date().toISOString()
    }]};
    else if (url.pathname==='/api/admin/users') body={users:[
      {identityRef:superRef,email:'principal@rcat.ac.th',role:'super_admin'},
      {identityRef:otherRef,email:'teacher@rcat.ac.th',role:'user'}
    ]};
    else if (url.pathname==='/api/admin/audit') body={events:[]};
    else if (method==='PUT') {requests.push({method,path:url.pathname,payload:route.request().postDataJSON()});body={identityRef:otherRef,role:'admin'};}
    else if (method==='POST') {requests.push({method,path:url.pathname,payload:route.request().postDataJSON()});body={accepted:true};}
    return route.fulfill({status:200,contentType:'application/json',body:JSON.stringify(body)});
   });
   await page.goto(base+'/',{waitUntil:'networkidle'});
   await page.locator('#workArea').waitFor({state:'visible'});
   await page.locator('#adminConsole').waitFor({state:'visible'});
   await page.waitForFunction(()=>document.querySelector('#adminTotal')?.textContent==='2');
   assert.equal(await page.locator('#adminNav').isVisible(),true);
   assert.equal(await page.locator('#adminTotal').textContent(),'2');
   assert.match(await page.locator('#adminStorageStatus').textContent(),/6 GiB/);
   assert.match(await page.locator('#adminStorageStatus').textContent(),/จองทรัพยากร 1\/2/);
   assert.equal(await page.locator('#adminUserRows tr').count(),2);
   assert.equal(await page.locator('#adminJobRows tr').count(),1);
   assert.equal(await page.locator('#adminAuditSection').isVisible(),true);
   await page.locator('#adminUserRows select').selectOption('admin');
   await page.waitForTimeout(100);
   assert.equal(requests.some(r=>r.method==='PUT'&&r.payload.role==='admin'),true);
   // The actions column is sticky within the horizontally scrollable
   // mobile table; direct user clicks must remain possible without force.
   await page.locator('#adminJobRows button').scrollIntoViewIfNeeded();
   await page.locator('#adminJobRows button').click();
   await page.waitForTimeout(100);
   assert.equal(requests.some(r=>r.method==='POST'&&r.payload.reason==='policy'),true);
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>window.innerWidth+2),false,'admin mobile overflow');
   await context.close();
  }
 }finally{await browser.close()}
});


test('Workspace Admin: read-only users and no privileged audit controls on desktop or mobile',async()=>{
 const browser=await chromium.launch({headless:true});
 try{
  for(const viewport of [{width:1365,height:900},{width:390,height:844}]){
   const context=await browser.newContext({viewport,isMobile:viewport.width<600,hasTouch:viewport.width<600});
   const page=await context.newPage();
   const errors=[];
   let auditRequested=false;
   page.on('pageerror',e=>errors.push(e.message));
   const adminRef='g-'+'1'.repeat(64),otherRef='g-'+'2'.repeat(64);
   await page.route('**/auth/me',route=>route.fulfill({
    status:200,contentType:'application/json',
    body:JSON.stringify({email:'admin@rcat.ac.th',role:'admin',identityRef:adminRef})
   }));
   await page.route('**/api/jobs',route=>route.fulfill({
    status:200,contentType:'application/json',body:JSON.stringify({jobs:[]})
   }));
   await page.route('**/api/admin/**',route=>{
    const url=new URL(route.request().url());
    if(url.pathname==='/api/admin/audit') {
     auditRequested=true;
     return route.fulfill({status:403,body:'forbidden'});
    }
    let body={};
    if(url.pathname==='/api/admin/summary')body={total:1,active:0,completed:1,failed:0};
    else if(url.pathname==='/api/admin/jobs')body={total:0,jobs:[]};
    else if(url.pathname==='/api/admin/users')body={users:[
     {identityRef:adminRef,email:'admin@rcat.ac.th',role:'admin'},
     {identityRef:otherRef,email:'teacher@rcat.ac.th',role:'user'}
    ]};
    return route.fulfill({status:200,contentType:'application/json',body:JSON.stringify(body)});
   });
   await page.goto(base+'/',{waitUntil:'networkidle'});
   await page.locator('#adminConsole').waitFor({state:'visible'});
   await page.waitForFunction(()=>Array.from(document.querySelectorAll('#adminUserRows tr')).some(row=>row.textContent.includes('teacher@rcat.ac.th')));
   assert.equal(await page.locator('#adminNav').isVisible(),true);
   assert.equal(await page.locator('#adminAuditSection').isVisible(),false);
   assert.equal(await page.locator('#adminUserRows select').count(),0);
   assert.equal(auditRequested,false,'Admin must not request Super Admin audit');
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>window.innerWidth+2),false);
   assert.deepEqual(errors,[]);
   await context.close();
  }
 }finally{await browser.close()}
});


test('Google Workspace entry: readable sign-in and controls from 320px to desktop', async () => {
 const browser=await chromium.launch({headless:true});
 try{
  for(const width of [320,390,768,1365]){
   const context=await browser.newContext({
    viewport:{width,height:900},
    ...(width<600?{isMobile:true,hasTouch:true}:{})
   });
   const page=await context.newPage();
   await page.goto(base+'/',{waitUntil:'domcontentloaded'});
   await page.locator('#loginPanel').waitFor({state:'visible'});
   const info=await page.evaluate(()=>{
    const css=s=>Number.parseFloat(getComputedStyle(document.querySelector(s)).fontSize);
    const height=s=>document.querySelector(s).getBoundingClientRect().height;
    return {
     viewport:innerWidth,
     scroll:document.documentElement.scrollWidth,
     title:css('#loginTitle'),
     lead:css('.login-form .lead'),
     domain:css('.domain-note'),
     buttonFont:css('#googleSignIn'),
     buttonHeight:height('#googleSignIn'),
     feature:css('.feature-row span')
    };
   });
   assert.ok(info.scroll<=info.viewport+2,'Google login overflows: '+JSON.stringify(info));
   assert.ok(info.title>=30,'Sign-in heading too small: '+JSON.stringify(info));
   assert.ok(info.lead>=16,'Sign-in description too small: '+JSON.stringify(info));
   assert.ok(info.domain>=14,'Account domain note too small: '+JSON.stringify(info));
   assert.ok(info.buttonFont>=16&&info.buttonHeight>=48,'Google sign-in touch target too small: '+JSON.stringify(info));
   assert.ok(info.feature>=14,'Feature labels too small: '+JSON.stringify(info));
   await context.close();
  }
 }finally{await browser.close()}
});
