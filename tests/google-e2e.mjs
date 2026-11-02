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
