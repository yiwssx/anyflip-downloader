// Google Workspace cookie transport, separate from shared download UI.
(() => {
  'use strict';
  const $ = id => document.getElementById(id);
  sessionStorage.removeItem('anyflip-access-token');
  let workspace = null;
  function showLogin() {
    if (workspace) workspace.stop();
    $('jobPanel').hidden = true;
    $('workArea').hidden = true;
    $('loginPanel').hidden = false;
  }
  function showUser(user) {
    $('signedInUser').textContent = user.email;
    $('loginPanel').hidden = true;
    $('workArea').hidden = false;
    void workspace.loadHistory();
  }
  async function request(path, options = {}) {
    const response = await fetch(path, { ...options, credentials:'same-origin', cache:'no-store' });
    if (!response.ok) {
      const message = await response.text();
      if (response.status === 401 || response.status === 403) {
        showLogin();
        const error = new Error(response.status === 403
          ? 'บัญชีนี้ไม่มีสิทธิ์ใช้งาน กรุณาติดต่อผู้ดูแลระบบ'
          : 'กรุณาเข้าสู่ระบบด้วย Google อีกครั้ง');
        error.authExpired = true;
        throw error;
      }
      throw new Error(response.status === 429
        ? 'ขณะนี้คิวงานเต็ม กรุณาลองใหม่อีกครั้ง'
        : message.trim() || 'เกิดข้อผิดพลาด ('+response.status+')');
    }
    return response;
  }
  workspace = window.AnyflipWorkspace.start({ request });
  $('logoutButton').addEventListener('click',async()=>{
    try {
      const response = await fetch('/auth/logout',{method:'POST',credentials:'same-origin',cache:'no-store'});
      if (!response.ok) throw new Error('ออกจากระบบไม่สำเร็จ');
      showLogin();
    } catch(e) {
      const el = $('logoutError'); el.textContent=e.message; el.hidden=false;
    }
  });
  fetch('/auth/me',{credentials:'same-origin',cache:'no-store'})
    .then(async response=>{ if (!response.ok) {showLogin();return;} showUser(await response.json()); })
    .catch(()=>showLogin());
})();
