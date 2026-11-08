// Synthetic CI-only screenshots: never run this suite against live accounts.
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';

export async function captureVisual(page, engine, name) {
  const root = process.env.BROWSER_VISUAL_ARTIFACT_DIR;
  if (!root) return;
  if (!/^(chromium|firefox|webkit)$/.test(engine) || !/^[a-z0-9-]+$/.test(name)) {
    throw new Error('Invalid synthetic screenshot destination');
  }
  const directory = join(root, engine);
  await mkdir(directory, { recursive: true });
  await page.screenshot({
    path: join(directory, name+'.png'),
    fullPage: true,
    animations: 'allow',
    caret: 'initial',
    timeout: 15000,
  });
}
