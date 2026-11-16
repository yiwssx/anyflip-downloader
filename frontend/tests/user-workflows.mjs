// SUI-02 synthetic contract/browser tests. No live Workspace, publisher or production API.
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import { chromium } from "playwright";

const base = process.env.SUI_PREVIEW_URL || "http://127.0.0.1:4173";
const screenshots = process.env.SUI_SCREENSHOTS_DIR || path.resolve("test-artifacts/screenshots");
const ids = ["a", "b", "c", "d", "e"].map(c=>c.repeat(32));
const [singleId,googleId,batchId,batchSecondId,retryId] = ids;
const job=(id,stage,overrides={})=>({
  id,stage,completed:stage==="completed"?4:stage==="queued"?0:1,total:4,
  downloadReady:stage==="completed",archived:false,...overrides,
});
const json=async(route,payload,status=200)=>route.fulfill({
  status,contentType:"application/json",body:JSON.stringify(payload),
});
const browser=await chromium.launch({headless:true});
await fs.mkdir(screenshots,{recursive:true});

try {
  // Legacy token transport: no API calls before token, Bearer on jobs, single
  // create -> polling -> PDF download; 429 remains recoverable.
  {
    const context=await browser.newContext({viewport:{width:1365,height:900},acceptDownloads:true});
    const page=await context.newPage();
    let polls=0,requests=0,queueFull=false;
    const token="synthetic-token-32-characters-long";
    await page.route("**/api/jobs**",async route=>{
      const req=route.request(),url=new URL(req.url()),p=url.pathname;
      assert.equal(req.headers().authorization,"Bearer "+token,"token mode must send Bearer on job API");
      requests++;
      if(p==="/api/jobs" && req.method()==="GET")
        return json(route,{jobs:polls>1?[job(singleId,"completed")]:[]});
      if(p==="/api/jobs" && req.method()==="POST"){
        if(queueFull)return json(route,{error:"rate limited"},429);
        const body=req.postDataJSON();
        assert.equal(body.threads,3);
        assert.equal(body.url,"https://anyflip.com/publisher/book");
        return json(route,{id:singleId},201);
      }
      if(p==="/api/jobs/"+singleId && req.method()==="GET")
        return json(route,job(singleId,++polls>1?"completed":"downloading"));
      if(p==="/api/jobs/"+singleId+"/download")
        return route.fulfill({status:200,contentType:"application/pdf",body:Buffer.from("%PDF-1.4\n%%EOF")});
      return route.fulfill({status:404,body:"unknown"});
    });
    await page.goto(base+"/token.html",{waitUntil:"networkidle"});
    assert.equal(requests,0,"no token should mean no API calls");
    const createNav=page.getByRole("navigation",{name:"หน้าในระบบ"}).getByRole("button",{name:"ดาวน์โหลด"});
    await createNav.click();
    assert.equal(await page.getByRole("button",{name:/เริ่มดาวน์โหลด/}).isDisabled(),true);
    await page.locator("#token").fill(token);
    await page.getByRole("button",{name:"เชื่อมต่อ"}).click();
    await page.locator("#bookURL").fill("https://anyflip.com/publisher/book");
    await page.getByRole("button",{name:/เริ่มดาวน์โหลด/}).click();
    await page.getByRole("button",{name:/ดาวน์โหลด Image-based PDF/}).waitFor({timeout:12000});
    assert.ok(polls>=2);
    await page.screenshot({path:path.join(screenshots,"sui02-token-complete.png"),fullPage:true});
    const downloadPromise=page.waitForEvent("download",{timeout:6000});
    await page.getByRole("button",{name:/ดาวน์โหลด Image-based PDF/}).click();
    const file=await downloadPromise;
    assert.ok(file.suggestedFilename().endsWith(".pdf"));
    queueFull=true;
    await page.getByRole("button",{name:/เริ่มดาวน์โหลด/}).click();
    await page.getByRole("alert").filter({hasText:"คิวงานเต็ม"}).waitFor({timeout:5000});
    assert.equal(await page.getByRole("button",{name:/เริ่มดาวน์โหลด/}).isDisabled(),false);
    await context.close();
    console.log("SUI02_TOKEN_SINGLE_DOWNLOAD_RATE_LIMIT_PASS");
  }

  // Google cookie session: no Bearer, real Go-like /auth/me response, role is
  // not trusted by frontend; 403 during status polling scrubs owner data.
  {
    const context=await browser.newContext({viewport:{width:390,height:844}});
    const page=await context.newPage();
    let revoked=false,meCalls=0;
    await page.route("**/auth/**",async route=>{
      const p=new URL(route.request().url()).pathname;
      if(p==="/auth/me"){meCalls++;return json(route,{email:"synthetic@rcat.ac.th",role:"user"});}
      if(p==="/auth/logout")return route.fulfill({status:204,body:""});
      return route.fulfill({status:404,body:""});
    });
    await page.route("**/api/jobs**",async route=>{
      const req=route.request(),p=new URL(req.url()).pathname;
      assert.equal(req.headers().authorization,undefined,"Google mode must not send Bearer");
      if(p==="/api/jobs")return json(route,{jobs:[job(googleId,"downloading")]});
      if(p==="/api/jobs/"+googleId){
        revoked=true;
        return route.fulfill({status:403,body:"forbidden"});
      }
      return route.fulfill({status:404,body:""});
    });
    await page.goto(base+"/google.html",{waitUntil:"networkidle"});
    await page.getByText("synthetic@rcat.ac.th").waitFor({state:"attached",timeout:5000});
    assert.ok(meCalls>=1);
    await page.getByRole("navigation",{name:"เมนูหลักสำหรับมือถือ"}).getByRole("button",{name:"ประวัติงาน"}).click();
    await page.getByRole("button",{name:"ดูสถานะ"}).click();
    await page.getByRole("heading",{name:"ยินดีต้อนรับ"}).waitFor({timeout:5000});
    assert.equal(revoked,true);
    assert.equal(await page.getByText(googleId.slice(0,8)).count(),0,"revoked owner history must be hidden");
    await page.screenshot({path:path.join(screenshots,"sui02-google-revoked.png"),fullPage:true});
    await context.close();
    console.log("SUI02_GOOGLE_COOKIE_403_OWNER_SCRUB_PASS");
  }

  // Batch size, cancellation, scoped history and Retry preserve the existing
  // Go contract without needing any real book or account.
  {
    const context=await browser.newContext({viewport:{width:1024,height:900}});
    const page=await context.newPage();
    const token="synthetic-token-for-batch-123456789";
    let cancelled=false,batchSeen=false,retried=false;
    await page.route("**/api/jobs**",async route=>{
      const req=route.request(),p=new URL(req.url()).pathname;
      assert.equal(req.headers().authorization,"Bearer "+token);
      if(p==="/api/jobs" && req.method()==="GET")
        return json(route,{jobs:batchSeen?[job(batchId,cancelled?"cancelled":"downloading")]:[]});
      if(p==="/api/jobs/batch" && req.method()==="POST"){
        const data=req.postDataJSON();
        assert.equal(data.jobs.length,2);
        batchSeen=true;
        return json(route,{jobs:[{id:batchId},{id:batchSecondId}]},201);
      }
      if(p==="/api/jobs/"+batchId && req.method()==="GET")
        return json(route,job(batchId,cancelled?"cancelled":"downloading"));
      if(p==="/api/jobs/"+batchId+"/cancel"){cancelled=true;return route.fulfill({status:204,body:""});}
      if(p==="/api/jobs/"+batchId+"/retry"){retried=true;return json(route,{id:retryId},201);}
      if(p==="/api/jobs/"+retryId)return json(route,job(retryId,"completed"));
      return route.fulfill({status:404,body:"unknown"});
    });
    await page.goto(base+"/token.html",{waitUntil:"networkidle"});
    await page.locator("#token").fill(token);
    await page.getByRole("button",{name:"เชื่อมต่อ"}).click();
    await page.getByRole("navigation",{name:"หน้าในระบบ"}).getByRole("button",{name:"ดาวน์โหลด"}).click();
    await page.getByText("ดาวน์โหลดหลายรายการ").click();
    await page.locator("#batchURLs").fill("https://anyflip.com/publisher/one\nhttps://anyflip.com/publisher/two");
    await page.getByRole("button",{name:"เพิ่มรายการเข้าคิว"}).click();
    await page.getByRole("button",{name:"ยกเลิกงาน"}).waitFor({timeout:7000});
    await page.getByRole("button",{name:"ยกเลิกงาน"}).click();
    await page.getByText("งานสิ้นสุดสถานะแล้ว").waitFor({timeout:7000});
    await page.getByRole("navigation",{name:"หน้าในระบบ"}).getByRole("button",{name:"ประวัติงาน"}).click();
    await page.getByRole("button",{name:"ลองใหม่"}).waitFor({timeout:7000});
    await page.getByRole("button",{name:"ลองใหม่"}).click();
    await page.getByRole("button",{name:/ดาวน์โหลด Image-based PDF/}).waitFor({timeout:7000});
    assert.equal(retried,true);
    await page.screenshot({path:path.join(screenshots,"sui02-batch-retry.png"),fullPage:true});
    await context.close();
    console.log("SUI02_BATCH_CANCEL_RETRY_PASS");
  }
} finally {
  await browser.close();
}
