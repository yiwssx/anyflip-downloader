// Same-origin Admin API transport. Authorization is always enforced by Go, not this file.
(() => {
  'use strict';
  async function request(path, options = {}) {
    if (typeof path !== 'string' || !path.startsWith('/api/admin/') || path.startsWith('//')) {
      throw new Error('Admin API path is not permitted');
    }
    const response = await fetch(path, {
      credentials: 'same-origin',
      cache: 'no-store',
      ...options,
    });
    if (!response.ok) {
      const error = new Error(response.status === 401 || response.status === 403
        ? 'สิทธิ์ของบัญชีนี้ไม่อนุญาตให้ดำเนินการดังกล่าว'
        : response.status === 429
          ? 'ดำเนินการบ่อยเกินไป กรุณาลองอีกครั้งภายหลัง'
          : 'คำขอไม่สำเร็จ (HTTP '+response.status+')');
      error.status = response.status;
      throw error;
    }
    return response.json();
  }
  window.AnyflipAdminAPI = Object.freeze({ request });
})();
