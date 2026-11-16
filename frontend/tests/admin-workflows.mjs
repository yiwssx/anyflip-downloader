// SUI-03 synthetic authorization contract checks. The Go backend remains
// authoritative; these tests confirm the new UI does not retain privileged data.
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import AxeBuilder from "@axe-core/playwright";

const base=process.env.SUI_PREVIEW_URL||"http://127.0.0.1:4173";
const screenshotDir=process.env.SUI_SCREENSHOTS_DIR||path.resolve("test-artifacts/screenshots");
const selfRef="g-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
const otherRef="g-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";
const jobID="a".repeat(32);
const jobOther="b".repeat(32);
const json=(route,payload,status=200)=>route.fulfill({status,contentType:"application/json",body:JSON.stringify(payload)});
const storage={totalBytes:6442450944,usedBytes:104857600,availableBytes:6337593344,usedPercent:2};
const summary={total:25,active:1,queued:0,running:1,completed:22,failed:2,cancelled:0,activeReservations:1,maxConcurrent:2,storage};
const job=(id,stage="downloading",overrides={})=>({id,owner:otherRef,stage,errorCode:"",cancelRequested:false,completed:2,total:4,created:"2026-11-16T02:00:00Z",...overrides});
const browser=await chromium.launch({headless:true});
await fs.mkdir(screenshotDir,{recursive:true});
try {
  // Standard users do not get Admin nav; no Admin endpoints are accessed.
  {
    const context=await browser.newContext({viewport:{width:390,height:844}});
    const page=await context.newPage();
    let adminRequests=0;
    await page.route("**/auth/me",r=>json(r,{email:"user@rcat.ac.th",identityRef:selfRef,role:"user"}));
    await page.route("**/api/jobs",r=>json(r,{jobs:[]}));
    await page.route("**/api/admin/**",r=>{adminRequests++;return r.fulfill({status:403,body:"forbidden"});});
    await page.goto(base+"/google.html",{waitUntil:"networkidle"});
    await page.getByRole("navigation",{name:"เมนูหลักสำหรับมือถือ"}).waitFor();
    assert.equal(await page.getByRole("button",{name:"ผู้ดูแล"}).count(),0);
    assert.equal(adminRequests,0);
    await context.close();
    console.log("SUI03_USER_NO_ADMIN_SURFACE_PASS");
  }

  // Admin sees metadata and may cancel with audited reason, but cannot fetch
  // audit log or mutate user roles. Pagination is server-bounded.
  {
    const context=await browser.newContext({viewport:{width:1365,height:900}});
    const page=await context.newPage();
    const calls=[], roleChanges=[];
    let cancelled=false, currentPage=0, reason=null, adminAudit=0;
    await page.route("**/auth/me",r=>json(r,{email:"admin@rcat.ac.th",identityRef:selfRef,role:"admin"}));
    await page.route("**/api/jobs",r=>json(r,{jobs:[]}));
    await page.route("**/api/admin/**",async r=>{
      const req=r.request(), u=new URL(req.url()),p=u.pathname;
      calls.push(p+u.search);
      assert.equal(req.headers().authorization,undefined,"Admin API relies on Google cookie, not bearer");
      if(p==="/api/admin/summary")return json(r,summary);
      if(p==="/api/admin/jobs"){
        currentPage=Number(u.searchParams.get("offset"))/20;
        assert.equal(Number(u.searchParams.get("limit")),20);
        const list=currentPage===0?[job(jobID,cancelled?"cancelled":"downloading",{cancelRequested:cancelled})]:[job(jobOther,"completed")];
        return json(r,{total:25,jobs:list});
      }
      if(p==="/api/admin/users")return json(r,{users:[
        {identityRef:selfRef,email:"admin@rcat.ac.th",role:"admin",disabled:false,lastSeen:"2026-11-16T01:00:00Z"},
        {identityRef:otherRef,email:"other@rcat.ac.th",role:"user",disabled:false,lastSeen:"2026-11-15T01:00:00Z"},
      ]});
      if(p==="/api/admin/audit"){adminAudit++;return r.fulfill({status:403,body:"denied"});}
      if(p==="/api/admin/jobs/"+jobID+"/cancel"){
        reason=req.postDataJSON().reason;cancelled=true;return json(r,{accepted:true,id:jobID},202);
      }
      if(p.includes("/role")){roleChanges.push(p);return r.fulfill({status:403,body:"denied"});}
      return r.fulfill({status:404,body:"unknown"});
    });
    await page.goto(base+"/google.html",{waitUntil:"networkidle"});
    await page.getByRole("navigation",{name:"หน้าในระบบ"}).getByRole("button",{name:"ผู้ดูแล"}).click();
    await page.getByRole("heading",{name:"ศูนย์จัดการระบบ"}).waitFor();
    await page.getByRole("button",{name:/ระงับงาน/}).waitFor({timeout:6000});
    const a11y = await new AxeBuilder({page}).withTags(["wcag2a","wcag2aa","wcag21aa","wcag22aa"]).analyze();
    const serious=a11y.violations.filter(v=>["serious","critical"].includes(v.impact??""));
    assert.deepEqual(serious.map(v=>({id:v.id,impact:v.impact})),[],"Admin surface has serious accessibility violations");
    await page.screenshot({path:path.join(screenshotDir,"sui03-admin-desktop.png"),fullPage:true});
    await page.getByRole("button",{name:"ถัดไป"}).click();
    await page.getByText("หน้า 2 · ทั้งหมด 25 งาน").waitFor();
    assert.equal(currentPage,1);
    await page.getByRole("button",{name:/ก่อนหน้า/}).click();
    await page.getByRole("button",{name:/ระงับงาน/}).click();
    const dialog=page.getByRole("dialog");
    await dialog.waitFor();
    await dialog.getByRole("combobox").selectOption("resources");
    await dialog.getByRole("button",{name:"ยืนยัน"}).click();
    assert.equal(reason,"resources");
    await page.getByText("ร้องขอยกเลิกแล้ว").waitFor();
    await page.getByRole("button",{name:"บัญชีและบทบาท"}).click();
    assert.equal(await page.getByRole("button",{name:"เปลี่ยนบทบาท"}).count(),0);
    assert.equal(adminAudit,0);
    assert.deepEqual(roleChanges,[]);
    await context.close();
    console.log("SUI03_ADMIN_JOB_CANCEL_PAGING_READONLY_USERS_PASS");
  }

  // Super Admin may change another user's role; backend conflict does not get
  // mistaken for success. Any API 403 scrubs Admin and returns to sign-in.
  {
    const context=await browser.newContext({viewport:{width:390,height:844}});
    const page=await context.newPage();
    let roleChanges=0,auditCalls=0,deny=false,newRole="user";
    await page.route("**/auth/me",r=>json(r,{email:"owner@rcat.ac.th",identityRef:selfRef,role:"super_admin"}));
    await page.route("**/api/jobs",r=>json(r,{jobs:[]}));
    await page.route("**/api/admin/**",async r=>{
      const req=r.request(),u=new URL(req.url()),p=u.pathname;
      if(deny)return r.fulfill({status:403,body:"forbidden"});
      if(p==="/api/admin/summary")return json(r,summary);
      if(p==="/api/admin/jobs")return json(r,{total:0,jobs:[]});
      if(p==="/api/admin/users")return json(r,{users:[
        {identityRef:selfRef,email:"owner@rcat.ac.th",role:"super_admin",disabled:false,lastSeen:"2026-11-16T01:00:00Z"},
        {identityRef:otherRef,email:"member@rcat.ac.th",role:newRole,disabled:false,lastSeen:"2026-11-16T01:00:00Z"},
      ]});
      if(p==="/api/admin/audit"){auditCalls++;return json(r,{events:[{event:"role_change",at:"2026-11-16T02:00:00Z",actor:selfRef}]});}
      if(p===`/api/admin/users/${otherRef}/role`){
        roleChanges++;
        assert.equal(req.method(),"PUT");
        assert.equal(req.postDataJSON().role,"admin");
        if(roleChanges===1)return r.fulfill({status:409,body:"revision conflict"});
        newRole="admin";
        return json(r,{identityRef:otherRef,role:"admin"});
      }
      return r.fulfill({status:404,body:"unknown"});
    });
    await page.goto(base+"/google.html",{waitUntil:"networkidle"});
    await page.getByRole("navigation",{name:"เมนูหลักสำหรับมือถือ"}).getByRole("button",{name:"ผู้ดูแล"}).click();
    await page.getByRole("button",{name:"บัญชีและบทบาท"}).click();
    await page.getByRole("button",{name:"เปลี่ยนบทบาท"}).waitFor({timeout:6000});
    assert.equal(await page.getByRole("button",{name:"เปลี่ยนบทบาท"}).count(),1,"self Super Admin must not be editable");
    await page.screenshot({path:path.join(screenshotDir,"sui03-superadmin-mobile.png"),fullPage:true});
    await page.getByRole("button",{name:"เปลี่ยนบทบาท"}).click();
    let dialog=page.getByRole("dialog");
    await dialog.getByRole("combobox").selectOption("admin");
    await dialog.getByRole("button",{name:"ยืนยัน"}).click();
    await page.getByText(/รายการนี้ถูกเปลี่ยนแปลงแล้ว/).first().waitFor();
    assert.equal(newRole,"user");
    await dialog.getByRole("button",{name:"ยืนยัน"}).click();
    await dialog.waitFor({state:"hidden"});
    assert.equal(newRole,"admin");
    assert.ok(auditCalls>0);
    await page.getByRole("button",{name:"Audit Log"}).click();
    await page.getByText("role_change").waitFor();
    deny=true;
    await page.getByRole("button",{name:"รีเฟรช"}).click();
    await page.getByRole("heading",{name:"ยินดีต้อนรับ"}).waitFor({timeout:6000});
    assert.equal(await page.getByText("member@rcat.ac.th").count(),0,"revoked Admin UI must disappear");
    await page.screenshot({path:path.join(screenshotDir,"sui03-role-revoked.png"),fullPage:true});
    await context.close();
    console.log("SUI03_SUPERADMIN_ROLE_CONFLICT_AUDIT_REVOKE_PASS");
  }
}finally{
  await browser.close();
}
