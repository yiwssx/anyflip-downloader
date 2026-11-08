// Legacy token transport; all job UI behavior is shared with Google mode.
(() => {
  'use strict';
  const $ = id => document.getElementById(id);
  const tokenField = $('token');
  tokenField.value = sessionStorage.getItem('anyflip-access-token') || '';

  async function request(path, options = {}) {
    const headers = { ...options.headers, Authorization: 'Bearer '+tokenField.value.trim() };
    const response = await fetch(path, { ...options, headers, cache: 'no-store' });
    if (!response.ok) {
      const message = await response.text();
      if (response.status === 401) {
        const error = new Error('Access Token ไม่ถูกต้อง');
        error.authExpired = true;
        throw error;
      }
      throw new Error(response.status === 429
        ? 'ขณะนี้คิวงานเต็ม กรุณาลองใหม่อีกครั้ง'
        : message.trim() || 'เกิดข้อผิดพลาด ('+response.status+')');
    }
    return response;
  }
  function persistToken() {
    const token = tokenField.value.trim();
    if (token.length < 16) throw new Error('กรุณากรอก Access Token ที่ผู้ดูแลระบบกำหนด');
    sessionStorage.setItem('anyflip-access-token',token);
  }
  window.AnyflipWorkspace.start({
    request,
    canLoadHistory: () => Boolean(tokenField.value.trim()),
    beforeBatch: persistToken,
    beforeCreate: persistToken,
  });
})();
