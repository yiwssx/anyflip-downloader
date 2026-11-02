// Real Go account login/cookie lifecycle, with synthetic download API responses.
// Run only against an isolated local CI server, never an operator-hosted endpoint.
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { chromium } from 'playwright';

const baseURL=process.env.BROWSER_TEST_BASE_URL || 'http://127.0.0.1:8094';
test('real account login, Admin creates User, isolated GUI without Access Token', {timeout:45000},async()=>{
 const browser=await chromium.launch({headless:true});
 try{
  const context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,acceptDownloads:true});
  const page=await context.newPage();
  const errors=[];
  page.on('pageerror',error=>errors.push(error.message));
  const jobID='e'.repeat(32);
  let created=0;
  await page.route('**/api/**',async route=>{
    const request=route.request(), path=new URL(request.url()).pathname;
    assert.equal(request.headers().authorization,undefined,'browser must not transmit API access tokens');
    if(path==='/api/jobs' && request.method()==='POST'){
      created++;
      return route.fulfill({status:202,contentType:'application/json',body:JSON.stringify({id:jobID,stage:'queued',completed:0,total:2,downloadReady:false})});
    }
    if(path==='/api/jobs' && request.method()==='GET'){
      return route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({jobs:[]})});
    }
    if(path==='/api/jobs/'+jobID){
      return route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({id:jobID,stage:'completed',completed:2,total:2,downloadReady:true})});
    }
    throw new Error('Unexpected account-mode request: '+request.method()+' '+path);
  });
  await page.goto(baseURL+'/',{waitUntil:'domcontentloaded'});
  await page.locator('#loginPanel').waitFor({state:'visible'});
  assert.equal(await page.locator('#token').count(),0);
  assert.equal(await page.locator('#jobForm').isVisible(),false);
  await page.locator('#username').fill('admin');
  await page.locator('#password').fill('ci-browser-password-123456');
  await page.locator('#loginButton').click();
  await page.locator('#jobForm').waitFor({state:'visible'});
  const session=(await context.cookies()).find(c=>c.name==='anyflip_session');
  assert.ok(session && session.httpOnly && session.sameSite==='Strict','missing secure server-managed cookie');
  assert.equal(await page.locator('#adminPanel').isVisible(),true);
  await page.locator('#newUsername').fill('teacher');
  await page.locator('#newPassword').fill('teacher-password-12345');
  await page.locator('#createUserButton').click();
  await page.locator('#createUserSuccess').waitFor({state:'visible'});
  await page.locator('#resetUsername').selectOption('teacher');
  await page.locator('#resetPassword').fill('teacher-reset-password-123');
  await page.locator('#resetPasswordButton').click();
  await page.locator('#resetPasswordSuccess').waitFor({state:'visible'});
  const teacherRow=page.locator('#usersList .historyRow').filter({hasText:'teacher'});
  await teacherRow.getByRole('button',{name:'ระงับบัญชี'}).click();
  await teacherRow.getByRole('button',{name:'เปิดใช้บัญชี'}).waitFor({state:'visible'});
  await page.locator('#bookURL').fill('https://anyflip.com/demo/authorized');
  await page.locator('#startButton').click();
  await page.locator('#downloadButton').waitFor({state:'visible',timeout:12000});
  assert.equal(created,1);
  await page.locator('#logoutButton').click();
  await page.locator('#loginPanel').waitFor({state:'visible'});
  const me=await page.request.get(baseURL+'/auth/me');
  assert.equal(me.status(),401,'logout did not invalidate account session');
  await page.locator('#username').fill('teacher');
  await page.locator('#password').fill('teacher-reset-password-123');
  await page.locator('#loginButton').click();
  await page.locator('#loginError').waitFor({state:'visible'});
  assert.equal(await page.locator('#jobForm').isVisible(),false,'disabled user accessed download form');

  await page.locator('#username').fill('admin');
  await page.locator('#password').fill('ci-browser-password-123456');
  await page.locator('#loginButton').click();
  await page.locator('#adminPanel').waitFor({state:'visible'});
  await teacherRow.getByRole('button',{name:'เปิดใช้บัญชี'}).click();
  await teacherRow.getByRole('button',{name:'ระงับบัญชี'}).waitFor({state:'visible'});
  await page.locator('#logoutButton').click();
  await page.locator('#loginPanel').waitFor({state:'visible'});

  await page.locator('#username').fill('teacher');
  await page.locator('#password').fill('teacher-reset-password-123');
  await page.locator('#loginButton').click();
  await page.locator('#jobForm').waitFor({state:'visible'});
  assert.equal(await page.locator('#adminPanel').isVisible(),false,'regular user can see admin panel');
  await page.locator('#currentPassword').fill('teacher-reset-password-123');
  await page.locator('#changedPassword').fill('teacher-rotated-password-123');
  await page.locator('#changePasswordButton').click();
  await page.locator('#loginPanel').waitFor({state:'visible'});
  const revoked=await page.request.get(baseURL+'/auth/me');
  assert.equal(revoked.status(),401,'password rotation did not revoke the session');
  await page.locator('#username').fill('teacher');
  await page.locator('#password').fill('teacher-rotated-password-123');
  await page.locator('#loginButton').click();
  await page.locator('#jobForm').waitFor({state:'visible'});
  assert.deepEqual(errors,[]);
  await context.close();
 }finally{await browser.close()}
});
