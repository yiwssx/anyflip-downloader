// Browser-level regression coverage for the compiled, authenticated Go Web GUI.
// API routes are intercepted with synthetic responses: no publisher book is fetched.
// Run only against an isolated local CI instance, never against the operator's LAN host.
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { chromium } from 'playwright';

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
  const browser = await chromium.launch({ headless: true });
  const errors = [];
  try {
    const context = await browser.newContext(contextOptions);
    const page = await context.newPage();
    page.on('pageerror', error => errors.push(error.message));
    page.on('console', entry => {
      if (entry.type() === 'error') errors.push(entry.text());
    });
    await run(page);
    assert.deepEqual(errors, [], 'JavaScript console or CSP errors were observed');
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

    await page.locator('#token').fill(token);
    await page.locator('#bookURL').fill('https://anyflip.com/demo/authorized');
    await page.locator('#bestQuality').check();
    await page.locator('#startButton').click();
    await page.locator('#downloadButton').waitFor({ state: 'visible', timeout: 10000 });
    assert.equal(await page.locator('#jobReference').textContent(), 'Job ID: ' + ids.desktop);
    assert.equal(await page.locator('#pageCount').textContent(), '2 / 2 หน้า');

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
    await page.locator('#batchStartButton').click();
    await page.locator('#jobPanel').waitFor({ state: 'visible' });
    await page.locator('#cancelButton').click();
    await page.waitForFunction(() => document.querySelector('#jobState')?.textContent === 'ยกเลิกแล้ว');
    assert.equal(batches, 1);

    await page.locator('#refreshHistory').click();
    const retry = page.getByRole('button', { name: 'ลองใหม่' });
    await retry.waitFor({ state: 'visible' });
    await retry.click();
    await page.locator('#downloadButton').waitFor({ state: 'visible', timeout: 10000 });
    assert.equal(await page.locator('#jobReference').textContent(), 'Job ID: ' + activeID);
    assert.equal(retries, 1);
    assert.ok(await page.locator('#historyList').isVisible());
  });
});
