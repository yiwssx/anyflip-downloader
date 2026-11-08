// UIH guardrails: fail CI if embedding, asset load order, or strict CSP regresses.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const read = path => readFileSync(resolve(root, path), 'utf8');
const tokens=read('web/tokens.css');
const styles=read('web/style.css');
const go=read('web.go');
const workspace=read('web/workspace.js');
assert.equal((tokens.match(/:root\s*\{/g)||[]).length,1,'exactly one token root');
assert.equal((styles.match(/:root\s*\{/g)||[]).length,0,'no competing design-token roots');
for (const [mode, html] of [['token',read('web/index.html')],['google',read('web/google.html')]]) {
  let pos=-1;
  for (const asset of ['/tokens.css','/style.css','/workspace.js','/app.js']) {
    const attr=(asset.endsWith('.css')?'href="':'src="')+asset+'"';
    const next=html.indexOf(attr);
    assert.ok(next>pos,mode+' dependency order '+asset);
    pos=next;
  }
  assert.doesNotMatch(html,/<script(?!\s+src=)[^>]*>/i,mode+' inline executable script');
  assert.doesNotMatch(html,/\s(?:on[a-z]+|style)=/i,mode+' inline handler/style attribute');
}
const googleHtml=read('web/google.html');
assert.ok(googleHtml.indexOf('src="/admin-api.js"')<googleHtml.indexOf('src="/admin.js"'));
for(const asset of ['web/tokens.css','web/style.css','web/workspace.js','web/admin-api.js']) {
  assert.ok(go.includes(asset),'missing Go embedded asset '+asset);
}
assert.ok(go.includes("script-src 'self'") && go.includes("frame-ancestors 'none'"));
assert.doesNotMatch(workspace,/\.style\.[A-Za-z]/,'no CSP-violating inline DOM style mutations');
console.log('PASS UIH frontend policy: tokens, assets, mode separation and strict CSP');
