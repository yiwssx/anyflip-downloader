// Focused SUI-01 synthetic browser/visual acceptance; no real institutional API.
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";

const origin = process.env.SUI_PREVIEW_URL || "http://127.0.0.1:4173";
const outputDir = process.env.SUI_SCREENSHOTS_DIR || path.resolve("test-artifacts/screenshots");
const viewports = [
  { width: 320, height: 760 },
  { width: 390, height: 844 },
  { width: 768, height: 900 },
  { width: 1024, height: 900 },
  { width: 1365, height: 900 },
  { width: 1440, height: 900 },
];
const browser = await chromium.launch({ headless: true });
await fs.mkdir(outputDir, { recursive: true });

try {
  for (const viewport of viewports) {
    const context = await browser.newContext({
      viewport,
      reducedMotion: "reduce",
      colorScheme: "light",
    });
    const page = await context.newPage();
    const errors = [];
    const calls = [];

    page.on("pageerror", (error) => errors.push(error.message));
    page.on("request", (request) => {
      const url = new URL(request.url());
      if (url.pathname.startsWith("/api/") || url.pathname.startsWith("/auth/")) {
        calls.push(request.url());
      }
    });
    const response = await page.goto(origin, { waitUntil: "networkidle" });
    assert.equal(response?.status(), 200, "preview must return HTTP 200");
    await page.getByRole("heading", { name: "AnyFlip Downloader" }).waitFor();

    assert.equal(
      await page.locator("html").evaluate((element) => element.classList.contains("dark")),
      false,
      "fresh preview must default to light",
    );
    const overflow = await page.evaluate(() =>
      document.documentElement.scrollWidth - document.documentElement.clientWidth
    );
    assert.ok(overflow <= 2, `Unexpected horizontal overflow at ${viewport.width}px: ${overflow}px`);
    const desktop = page.getByRole("navigation", { name: "หน้าในระบบ" });
    const mobile = page.getByRole("navigation", { name: "เมนูหลักสำหรับมือถือ" });
    if (viewport.width < 768) {
      assert.equal(await mobile.isVisible(), true, "mobile bottom navigation visible");
      assert.equal(await desktop.isVisible(), false, "desktop sidebar hidden on mobile");
    } else {
      assert.equal(await mobile.isVisible(), false, "mobile bottom navigation hidden on desktop");
      assert.equal(await desktop.isVisible(), true, "desktop sidebar visible");
      const collapse = page.getByRole("button", { name: "ยุบเมนู" });
      await collapse.click();
      assert.equal(await page.getByRole("button", { name: "ขยายเมนู" }).getAttribute("aria-expanded"), "false");
      const collapsed = await page.locator("aside[aria-label='เมนูหลัก']").evaluate((el) => ({
        width: el.getBoundingClientRect().width,
        className: el.className,
      }));
      assert.ok(collapsed.width <= 90, `Sidebar failed to collapse at ${viewport.width}px: width=${collapsed.width}, classes=${collapsed.className}`);
      await page.getByRole("button", { name: "ขยายเมนู" }).click();
      assert.equal(await page.getByRole("button", { name: "ยุบเมนู" }).getAttribute("aria-expanded"), "true");
    }

    const axe = await new AxeBuilder({ page }).withTags([
      "wcag2a",
      "wcag2aa",
      "wcag21a",
      "wcag21aa",
      "wcag22aa",
    ]).analyze();
    const serious = axe.violations.filter((violation) => ["serious", "critical"].includes(violation.impact ?? ""));
    assert.deepEqual(
      serious.map((v) => ({ id: v.id, impact: v.impact, nodes: v.nodes.length })),
      [],
      `Serious axe findings at ${viewport.width}px`,
    );
    await page.screenshot({ path: path.join(outputDir, `overview-light-${viewport.width}.png`), fullPage: true });

    await page.getByRole("button", { name: "เปลี่ยนเป็นธีมมืด" }).click();
    assert.equal(await page.locator("html").evaluate((el) => el.classList.contains("dark")), true);
    await page.screenshot({ path: path.join(outputDir, `overview-dark-${viewport.width}.png`), fullPage: true });

    const downloadNav = viewport.width < 768 ? mobile : desktop;
    await downloadNav.getByRole("button", { name: "ดาวน์โหลด" }).click();
    await page.getByRole("heading", { name: "สร้างงานดาวน์โหลด" }).waitFor();
    assert.equal(await page.getByRole("button", { name: /เริ่มดาวน์โหลด/ }).isDisabled(), true);
    assert.equal(await page.getByLabel("ลิงก์หนังสือ AnyFlip").isDisabled(), true);
    await page.screenshot({ path: path.join(outputDir, `download-dark-${viewport.width}.png`), fullPage: true });

    assert.deepEqual(errors, [], `Page errors at ${viewport.width}px`);
    assert.deepEqual(calls, [], "foundation preview must not contact authentication or job APIs");
    await context.close();
    console.log(`SUI_VISUAL_PASS width=${viewport.width} overflow=${overflow}px axe_serious=0`);
  }

  // Keyboard skip-link is reachable without a pointer.
  const context = await browser.newContext({ viewport: { width: 1365, height: 900 } });
  const page = await context.newPage();
  await page.goto(origin, { waitUntil: "networkidle" });
  await page.keyboard.press("Tab");
  assert.equal(await page.evaluate(() => document.activeElement?.textContent?.trim()), "ข้ามไปยังเนื้อหาหลัก");
  await context.close();
  console.log("SUI_KEYBOARD_SKIP_LINK_PASS");
} finally {
  await browser.close();
}
