// SUI-04 exact Go-served candidate browser smoke with synthetic local accounts.
// Two isolated local Go processes only; never call live Google or publishers.
import assert from "node:assert/strict";
import path from "node:path";
import fs from "node:fs/promises";
import { chromium } from "playwright";

const tokenBase=process.env.SUI_GO_BASE_URL;
const googleBase=process.env.SUI_GO_GOOGLE_URL;
if(!tokenBase||!googleBase)throw new Error("SUI_GO_BASE_URL and SUI_GO_GOOGLE_URL required");
const artifactDir=process.env.SUI_SCREENSHOTS_DIR||path.resolve("test-artifacts/screenshots");
await fs.mkdir(artifactDir,{recursive:true});
const browser=await chromium.launch({headless:true});
try{
  for(const candidate of [{base:tokenBase,mode:"token"},{base:googleBase,mode:"google"}]){
    const context=await browser.newContext({viewport:{width:390,height:844},reducedMotion:"reduce"});
    const page=await context.newPage();
    const errors=[],remote=[];
    page.on("pageerror",err=>errors.push(err.message));
    page.on("console",msg=>{
      if(msg.type()==="error" && /Content Security Policy|Refused to|unsafe-inline|unsafe-eval/i.test(msg.text()))errors.push(msg.text());
    });
    page.on("request",request=>{
      const dest=new URL(request.url());
      if(dest.origin!==new URL(candidate.base).origin)remote.push(dest.href);
    });
    // Google sign-in may issue session checks/retries, so networkidle is not a reliable page-load gate.
    // Assert the meaningful user-visible state below instead of assuming zero background requests.
    const response=await page.goto(candidate.base+"/",{waitUntil:"domcontentloaded",timeout:15000});
    assert.equal(response?.status(),200);
    const csp=response.headers()["content-security-policy"];
    assert.ok(csp?.includes("script-src 'self'"));
    assert.ok(csp?.includes("style-src 'self'"));
    assert.ok(!csp.includes("unsafe-inline")&&!csp.includes("unsafe-eval"));
    assert.equal(response.headers()["cache-control"],"no-store");
    const scripts=await page.locator('script[src^="/svelte/assets/"]').count();
    const styles=await page.locator('link[rel="stylesheet"][href^="/svelte/assets/"]').count();
    assert.ok(scripts>0&&styles>0,`Svelte assets missing in ${candidate.mode}`);
    await page.getByRole("heading",{name:"AnyFlip Downloader"}).waitFor({timeout:10000});
    // Real Go origin must serve the existing official RCAT brand files in either auth mode.
    assert.equal(await page.locator('link[rel="icon"]').getAttribute("href"),"/svelte/favicon.svg","Vite embedded base must point to the served same-origin favicon");
    for(const brandPath of ["/svelte/favicon.svg","/svelte/logo.svg","/favicon.svg","/logo.svg"]){
      const res=await context.request.get(candidate.base+brandPath);
      assert.equal(res.status(),200,brandPath+" must be accessible on the Go origin");
      assert.match(res.headers()["content-type"]??"",/image\/svg\+xml/);
    }
    const logo=page.locator('img[src$="/logo.svg"]').first();
    await logo.waitFor({timeout:10000});
    assert.equal(await logo.evaluate(el=>el.complete&&el.naturalWidth>0),true,"Official RCAT icon did not render");
    assert.equal(await page.evaluate(()=>getComputedStyle(document.documentElement).getPropertyValue("--primary").trim()),"#176b3c");
    if(candidate.mode==="token"){
      await page.getByRole("navigation",{name:"เมนูหลักสำหรับมือถือ"})
        .getByRole("button",{name:"ดาวน์โหลด"}).click();
      assert.equal(await page.locator("#token").isVisible(),true);
      const unauthorized=await context.request.get(candidate.base+"/api/jobs");
      assert.equal(unauthorized.status(),401,"Svelte UI must never bypass Go API authorization");
      await page.getByRole("button",{name:"เปลี่ยนเป็นธีมมืด"}).click();
      assert.equal(await page.locator("html").evaluate(el=>el.classList.contains("dark")),true);
    }else{
      await page.getByRole("heading",{name:"ยินดีต้อนรับ"}).waitFor({timeout:10000});
      await page.getByRole("link",{name:/เข้าสู่ระบบด้วย Google/}).waitFor();
      const unauth=await context.request.get(candidate.base+"/auth/me");
      assert.equal(unauth.status(),401);
    }
    assert.deepEqual(remote,[],`Remote asset request from ${candidate.mode}`);
    assert.deepEqual(errors,[],`Runtime or CSP error in ${candidate.mode}`);
    await page.screenshot({path:path.join(artifactDir,`sui04-go-embed-${candidate.mode}.png`),fullPage:true});
    await context.close();
    console.log(`SUI04_EMBED_GO_CSP_${candidate.mode.toUpperCase()}_PASS`);
  }
}finally{await browser.close();}
