// Browser-level regression coverage for the compiled, authenticated Go Web GUI.
// API routes are intercepted with synthetic responses: no publisher book is fetched.
// Run only against an isolated local CI instance, never against the operator's LAN host.
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { chromium, firefox, webkit } from 'playwright';
const engine = process.env.BROWSER_TEST_ENGINE || 'chromium';
const browserType = { chromium, firefox, webkit }[engine];
if (!browserType) throw new Error('Unsupported BROWSER_TEST_ENGINE: '+engine);

const baseURL = process.env.BROWSER_TEST_BASE_URL || 'http://127.0.0.1:8094';
const token = process.env.ANYFLIP_WEB_TOKEN || 'ci-browser-test-token-strong-only';

const ids = {
  desktop: 'a'.repeat(32),
  batchA: 'b'.repeat(32),
  batchB: 'c'.repeat(32),
  retry: 'd'.repeat(32),
};
const pdfBytes = Buffer.from('%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\n%%EOF\n');

function respond(route, status, data) {
  return route.fulfill({
    status,
    contentType: 'application/json; charset=utf-8',
    body: JSON.stringify(data),
    headers: { 'Cache-Control': 'no-store' },
  });
}

async function withPage(contextOptions, run) {
  const browser = await browserType.launch({ headless: true });
  const errors = [];
  try {
    const options = { ...contextOptions };
    // Playwright Firefox does not support mobile viewport emulation mode.
    if (engine === 'firefox') delete options.isMobile;
    const context = await browser.newContext(options);
    const page = await context.newPage();
    await page.addInitScript(() => {
      window.__cspViolations = [];
      document.addEventListener('securitypolicyviolation', event => {
        window.__cspViolations.push({
          effectiveDirective:event.effectiveDirective,
          blockedURI:event.blockedURI,
          sourceFile:event.sourceFile,
          lineNumber:event.lineNumber,
          sample:event.sample,
        });
      });
    });
    page.on('pageerror', error => errors.push(error.message));
    page.on('console', entry => {
      if (entry.type() === 'error') errors.push(entry.text()+' @ '+JSON.stringify(entry.location()));
    });
    await run(page);
    const violations = await page.evaluate(() => window.__cspViolations || []);
    assert.deepEqual(errors, [], 'JavaScript console or CSP errors were observed; violations='+JSON.stringify(violations));
    await context.close();
  } finally {
    await browser.close();
  }
}

test('desktop: CSP, create, progress and image-based PDF download', { timeout: 45_000 }, async () => {
  await withPage({ viewport: { width: 1365, height: 900 }, acceptDownloads: true }, async page => {
    let polls = 0;
    let creates = 0;
    let downloads = 0;
    await page.route('**/api/**', async route => {
      const request = route.request();
      const path = new URL(request.url()).pathname;
      if (path === '/api/jobs' && request.method() === 'POST') {
        creates++;
        assert.equal(request.headers().authorization, 'Bearer ' + token);
        const data = request.postDataJSON();
        assert.equal(data.url, 'https://anyflip.com/demo/authorized');
        assert.equal(data.bestQuality, true);
        return respond(route, 202, { id: ids.desktop, stage: 'queued', completed: 0, total: 2, downloadReady: false });
      }
      if (path === '/api/jobs' && request.method() === 'GET') {
        return respond(route, 200, { jobs: [{ id: ids.desktop, stage: 'completed', completed: 2, total: 2, downloadReady: true }] });
      }
      if (path === '/api/jobs/' + ids.desktop && request.method() === 'GET') {
        polls++;
        return respond(route, 200, {
          id: ids.desktop, stage: polls < 2 ? 'downloading' : 'completed',
          completed: polls < 2 ? 1 : 2, total: 2, downloadReady: polls >= 2,
        });
      }
      if (path === '/api/jobs/' + ids.desktop + '/download' && request.method() === 'GET') {
        downloads++;
        return route.fulfill({ status: 200, contentType: 'application/pdf', body: pdfBytes,
          headers: { 'X-AnyFlip-Output-Type': 'image-based-pdf' } });
      }
      throw new Error('Unexpected desktop mock API: ' + request.method() + ' ' + path);
    });

    const response = await page.goto(baseURL + '/', { waitUntil: 'networkidle' });
    assert.equal(response.status(), 200);
    const csp = response.headers()['content-security-policy'] || '';
    assert.match(csp, /script-src 'self'/);
    assert.doesNotMatch(csp, /unsafe-inline|unsafe-eval/);
    assert.ok(await page.locator('#jobForm').isVisible());
    assert.ok(await page.locator('#batchURLs').isVisible());
    assert.ok(await page.locator('.sidebar .nav-links').isVisible());
    assert.equal(await page.locator('.nav-link').count(),4);
    assert.ok(await page.locator('#historySearch').isVisible());
    assert.ok(await page.locator('#historyFilter').isVisible());
    assert.ok(await page.locator('#statTotal').isVisible());
    assert.ok(await page.locator('img.brand-logo').isVisible());
    await page.locator('#token').fill(token);
    await page.locator('#bookURL').fill('https://anyflip.com/demo/authorized');
    await page.locator('#bestQuality').check();
    await page.locator('#startButton').click();
    await page.locator('#downloadButton').waitFor({ state: 'visible', timeout: 10000 });
    assert.equal(await page.locator('#jobReference').textContent(), 'Job ID: ' + ids.desktop);
    assert.equal(await page.locator('#pageCount').textContent(), '2 / 2 หน้า');
    await page.waitForFunction(() => document.querySelector('#statTotal')?.textContent === '1');
    assert.equal(await page.locator('#statDone').textContent(), '1');
    await page.locator('#historySearch').fill('no-match-id');
    assert.match(await page.locator('#historyList').innerText(), /ไม่พบรายการ/);
    await page.locator('#historySearch').fill('');
    assert.match(await page.locator('#historyList').innerText(), /งาน #aaaaaaaa/);

    const downloadEvent = page.waitForEvent('download');
    await page.locator('#downloadButton').click();
    const downloaded = await downloadEvent;
    assert.equal(downloaded.suggestedFilename(), 'anyflip-' + ids.desktop + '.pdf');
    assert.equal(await downloaded.failure(), null);
    assert.equal(creates, 1);
    assert.ok(polls >= 2);
    assert.equal(downloads, 1);
  });
});

test('mobile: batch jobs, cancellation, history and retry', { timeout: 45_000 }, async () => {
  await withPage({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true }, async page => {
    let cancelled = false;
    let batches = 0;
    let retries = 0;
    let activeID = ids.batchA;
    const snapshot = (id, stage) => ({
      id, stage, completed: stage === 'completed' ? 2 : 0,
      total: 2, downloadReady: stage === 'completed',
    });
    await page.route('**/api/**', async route => {
      const request = route.request();
      const path = new URL(request.url()).pathname;
      if (path === '/api/jobs/batch' && request.method() === 'POST') {
        const jobs = request.postDataJSON().jobs;
        assert.equal(jobs.length, 2);
        assert.equal(request.headers().authorization, 'Bearer ' + token);
        batches++;
        return respond(route, 202, { jobs: [snapshot(ids.batchA, 'queued'), snapshot(ids.batchB, 'queued')] });
      }
      if (path === '/api/jobs' && request.method() === 'GET') {
        return respond(route, 200, { jobs: [
          snapshot(ids.batchA, cancelled ? 'cancelled' : 'queued'),
          snapshot(ids.batchB, 'queued'),
          ...(retries ? [snapshot(ids.retry, 'completed')] : []),
        ] });
      }
      if (path === '/api/jobs/' + ids.batchA && request.method() === 'GET') {
        return respond(route, 200, snapshot(ids.batchA, cancelled ? 'cancelled' : 'queued'));
      }
      if (path === '/api/jobs/' + ids.batchA + '/cancel' && request.method() === 'POST') {
        cancelled = true;
        return respond(route, 202, snapshot(ids.batchA, 'queued'));
      }
      if (path === '/api/jobs/' + ids.batchA + '/retry' && request.method() === 'POST') {
        assert.ok(cancelled, 'retry must follow a failed/cancelled job');
        retries++;
        activeID = ids.retry;
        return respond(route, 202, snapshot(ids.retry, 'queued'));
      }
      if (path === '/api/jobs/' + ids.retry && request.method() === 'GET') {
        return respond(route, 200, snapshot(ids.retry, 'completed'));
      }
      throw new Error('Unexpected mobile mock API: ' + request.method() + ' ' + path);
    });

    await page.goto(baseURL + '/', { waitUntil: 'networkidle' });
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 2);
    assert.equal(overflow, false, 'Mobile UI has horizontal overflow');
    await page.locator('#token').fill(token);
    await page.locator('#batchURLs').fill(
      'https://anyflip.com/demo/authorized-a\nhttps://anyflip.com/demo/authorized-b',
    );
    assert.equal(await page.locator('#batchCount').textContent(), '2 / 8');
    await page.locator('#batchStartButton').click();
    await page.locator('#jobPanel').waitFor({ state: 'visible' });
    assert.equal(await page.locator('#batchCount').textContent(), '2 / 8');
    await page.locator('#cancelButton').click();
    await page.waitForFunction(() => document.querySelector('#jobState')?.textContent === 'ยกเลิกแล้ว');
    assert.equal(batches, 1);

    const layout = await page.evaluate(() => {
      const primary=document.querySelector('.primary-column').getBoundingClientRect();
      const secondary=document.querySelector('.secondary-column').getBoundingClientRect();
      const history=document.querySelector('#refreshHistory').getBoundingClientRect();
      const grid=document.querySelector('.content-grid');
      const style=getComputedStyle(grid);
      return {width:innerWidth,primaryTop:primary.top,primaryBottom:primary.bottom,
        secondaryTop:secondary.top,secondaryBottom:secondary.bottom,
        buttonTop:history.top,buttonLeft:history.left,columns:style.gridTemplateColumns,
        display:style.display,flexDirection:style.flexDirection};
    });
    assert.ok(layout.secondaryTop >= layout.primaryBottom - 2, 'Mobile columns overlap: '+JSON.stringify(layout));
    const targetGeometry=await page.evaluate(()=>{
      const button=document.getElementById('refreshHistory');
      button.scrollIntoView({block:'center',behavior:'instant'});
      const rect=button.getBoundingClientRect();
      const hit=document.elementFromPoint(rect.left+rect.width/2,rect.top+rect.height/2);
      const parent=document.querySelector('.primary-column').getBoundingClientRect();
      return {button:{x:rect.x,y:rect.y,width:rect.width,height:rect.height},
       hit:hit?.outerHTML.slice(0,160),hitID:hit?.id,scrollY,innerHeight,
       primary:{top:parent.top,bottom:parent.bottom},
       clickable:hit===button||button.contains(hit)};
    });
    assert.ok(targetGeometry.clickable,'Mobile history refresh is occluded: '+JSON.stringify(targetGeometry));
    // Playwright mobile emulation repositions clicks after a successful
    // elementFromPoint hit test. Verify unobstructed geometry, then dispatch
    // the DOM action for API behavior; physical-touch acceptance stays open.
    async function activateVisibleControl(locator) {
      const rect=await locator.evaluate(element=>{
        element.scrollIntoView({block:'center',behavior:'instant'});
        const b=element.getBoundingClientRect();
        const x=b.left+b.width/2,y=b.top+b.height/2;
        const hit=document.elementFromPoint(x,y);
        return {x,y,valid:hit===element||element.contains(hit),hit:hit?.outerHTML.slice(0,120)};
      });
      assert.ok(rect.valid,'Touch target is obscured: '+JSON.stringify(rect));
      await locator.evaluate(element=>element.click());
    }
    const refreshed=page.waitForResponse(response=>new URL(response.url()).pathname==='/api/jobs'&&response.request().method()==='GET');
    await activateVisibleControl(page.locator('#refreshHistory'));
    await refreshed;
    const retry = page.getByRole('button', { name: 'ลองใหม่' });
    await retry.waitFor({ state: 'visible' });
    await activateVisibleControl(retry);
    await page.locator('#downloadButton').waitFor({ state: 'visible', timeout: 10000 });
    assert.equal(await page.locator('#jobReference').textContent(), 'Job ID: ' + activeID);
    assert.equal(retries, 1);
    assert.ok(await page.locator('#historyList').isVisible());
    await page.locator('#historyFilter').selectOption('completed');
    assert.match(await page.locator('#historyList').innerText(), /สำเร็จ/);
    await page.locator('#historyFilter').selectOption('all');
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 2), false);
    assert.ok(await page.locator('.nav-link[href="#batch"]').isVisible());
  });
});


test('responsive readability: text hierarchy, touch controls and overflow at 320–1365px', { timeout: 90_000 }, async () => {
  for (const width of [320, 390, 768, 1365]) {
    await withPage({
      viewport: { width, height: 900 },
      ...(width < 600 ? { isMobile: true, hasTouch: true } : {}),
    }, async page => {
      await page.route('**/api/**', route => respond(route, 200, { jobs: [] }));
      await page.goto(baseURL + '/', { waitUntil: 'networkidle' });
      const measurements = await page.evaluate(() => {
        const size = selector => {
          const element = document.querySelector(selector);
          if (!element) throw new Error('Missing required element: ' + selector);
          return Number.parseFloat(getComputedStyle(element).fontSize);
        };
        const height = selector => document.querySelector(selector).getBoundingClientRect().height;
        return {
          viewport: innerWidth,
          scrollWidth: document.documentElement.scrollWidth,
          body: size('body'),
          nav: size('.nav-link'),
          heading: size('#newJobTitle'),
          intro: size('#createJob .section-heading p'),
          fieldLabel: size('label[for="bookURL"]'),
          fieldInput: size('#bookURL'),
          primaryButton: size('#startButton'),
          qualityHint: size('.quality-toggle small'),
          historyNote: size('.history-footnote'),
          noticeCopy: size('.notice p'),
          inputHeight: height('#bookURL'),
          buttonHeight: height('#startButton'),
          navigationHeight: height('.nav-link'),
          overflowing: [...document.querySelectorAll('body *')]
            .map(el => ({ tag: el.tagName.toLowerCase(), cls: typeof el.className === 'string' ? el.className.slice(0, 45) : '', right: Math.round(el.getBoundingClientRect().right) }))
            .filter(el => el.right > innerWidth + 2)
            .slice(0, 8),
        };
      });
      assert.ok(measurements.scrollWidth <= measurements.viewport + 2,
        'Horizontal page overflow: ' + JSON.stringify(measurements));
      for (const key of ['body', 'intro', 'fieldLabel', 'fieldInput', 'primaryButton']) {
        assert.ok(measurements[key] >= 16, key + ' too small: ' + JSON.stringify(measurements));
      }
      for (const key of ['nav', 'qualityHint', 'historyNote', 'noticeCopy']) {
        assert.ok(measurements[key] >= 14, key + ' too small: ' + JSON.stringify(measurements));
      }
      assert.ok(measurements.heading >= 22, 'Heading hierarchy too small: ' + JSON.stringify(measurements));
      assert.ok(measurements.inputHeight >= 48, 'Form input too small: ' + JSON.stringify(measurements));
      assert.ok(measurements.buttonHeight >= 44, 'Primary touch target too small: ' + JSON.stringify(measurements));
      assert.ok(measurements.navigationHeight >= 44, 'Navigation touch target too small: ' + JSON.stringify(measurements));
    });
  }
});

test('single-flight status polling: slow responses never overlap', { timeout: 25_000 }, async () => {
  await withPage({ viewport: { width: 390, height: 844 }, isMobile:true, hasTouch:true }, async page => {
    const jobID = 'e'.repeat(32);
    let active = 0, maximum = 0, requests = 0;
    await page.route('**/api/**', async route => {
      const request = route.request();
      const pathname = new URL(request.url()).pathname;
      if (pathname === '/api/jobs' && request.method() === 'POST') {
        return respond(route, 202, { id:jobID, stage:'queued', completed:0, total:2, downloadReady:false });
      }
      if (pathname === '/api/jobs' && request.method() === 'GET') {
        return respond(route, 200, { jobs:[] });
      }
      if (pathname === '/api/jobs/'+jobID && request.method() === 'GET') {
        active++;
        maximum = Math.max(maximum, active);
        requests++;
        await new Promise(resolve => setTimeout(resolve, 1450));
        active--;
        return respond(route, 200, {
          id: jobID, stage: requests >= 2 ? 'completed' : 'downloading',
          completed: requests >= 2 ? 2 : 1, total: 2, downloadReady: requests >= 2,
        });
      }
      return respond(route, 200, {});
    });
    await page.goto(baseURL+'/', { waitUntil:'networkidle' });
    await page.locator('#token').fill(token);
    await page.locator('#bookURL').fill('https://anyflip.com/demo/authorized');
    await page.locator('#startButton').click();
    await page.waitForFunction(
      () => document.querySelector('#jobState')?.textContent === 'สำเร็จ',
      null, { timeout: 12000 }
    );
    assert.ok(requests >= 2, 'test must observe more than one status check');
    assert.equal(maximum, 1, 'polling requests must never overlap');
    assert.equal(await page.locator('#percent').textContent(), '100%');
  });
});

test('keyboard skip link and stable accessible progress announcements', async () => {
  await withPage({ viewport:{width:390,height:844}, isMobile:true, hasTouch:true }, async page => {
    await page.route('**/api/**', route => respond(route,200,{jobs:[]}));
    await page.goto(baseURL+'/',{waitUntil:'networkidle'});
    await page.keyboard.press('Tab');
    assert.equal(await page.locator('.skip-link').evaluate(el => document.activeElement === el),true,
      'Skip-to-content must be first keyboard stop');
    assert.equal(await page.locator('#jobDescription').getAttribute('role'),'status');
    assert.equal(await page.locator('#jobDescription').getAttribute('aria-live'),'polite');
    assert.equal(await page.locator('#jobDescription').getAttribute('aria-atomic'),'true');
  });
});
