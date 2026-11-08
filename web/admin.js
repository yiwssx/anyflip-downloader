(() => {
  'use strict';
  const $ = id => document.getElementById(id);
  const roleNames = {user:'ผู้ใช้งาน',admin:'ผู้ดูแลระบบ',super_admin:'ผู้ดูแลสูงสุด'};
  const jobNames = {queued:'รอคิว',preparing:'เตรียมงาน',downloading:'ดาวน์โหลด',converting:'สร้าง PDF',completed:'สำเร็จ',failed:'ผิดพลาด',cancelled:'ยกเลิก'};
  let me = null, currentPage = 0, jobTotal = 0, jobs = [], users = [];
  const pageSize = 20;

  function report(message) {
    const el = $('adminError');
    el.textContent = message || '';
    el.hidden = !message;
  }
  async function api(path, options) {
    const response = await fetch(path, {
      credentials:'same-origin', cache:'no-store',
      ...(options || {})
    });
    if (!response.ok) {
      if (response.status === 401 || response.status === 403) {
        // Do not recurse through checkRole/refreshAll on a denied API.
        me=null;
        $('adminNav').hidden=true;
        $('adminConsole').hidden=true;
        throw new Error('สิทธิ์ของบัญชีนี้ไม่อนุญาตให้ดำเนินการดังกล่าว');
      }
      throw new Error(response.status === 429
        ? 'ดำเนินการบ่อยเกินไป กรุณาลองอีกครั้งภายหลัง'
        : 'คำขอไม่สำเร็จ (HTTP '+response.status+')');
    }
    return response.json();
  }
  function node(tag, text, className) {
    const el = document.createElement(tag);
    if (text !== undefined) el.textContent = text;
    if (className) el.className = className;
    return el;
  }
  function dateText(raw) {
    if (!raw || raw.startsWith('0001-')) return '—';
    const d = new Date(raw);
    return Number.isNaN(d.getTime()) ? '—' :
      new Intl.DateTimeFormat('th-TH',{dateStyle:'short',timeStyle:'short'}).format(d);
  }
  function clearPanel(id, message, cols) {
    const body = $(id);
    body.replaceChildren();
    const row = node('tr');
    const td = node('td',message);
    td.colSpan = cols;
    td.className = 'admin-empty';
    row.appendChild(td);
    body.appendChild(row);
  }
  async function refreshSummary() {
    const data = await api('/api/admin/summary');
    $('adminTotal').textContent = data.total;
    $('adminActive').textContent = data.active;
    $('adminCompleted').textContent = data.completed;
    $('adminFailed').textContent = data.failed;
    const queued = Number(data.queued) || 0;
    const running = Number(data.running) || 0;
    const reservations = Number(data.activeReservations) || 0;
    const maxConcurrent = Number(data.maxConcurrent) || 0;
    let storageText = 'พื้นที่จัดเก็บ: ไม่มีข้อมูลจากระบบปฏิบัติการ';
    const storage = data.storage;
    if (storage && Number.isFinite(storage.totalBytes) && storage.totalBytes > 0 &&
        Number.isFinite(storage.usedBytes) && Number.isFinite(storage.availableBytes)) {
      const gib = value => (value / (1024 ** 3)).toLocaleString('th-TH', {maximumFractionDigits:2});
      storageText = 'พื้นที่จัดเก็บ: ใช้ '+gib(storage.usedBytes)+' / '+gib(storage.totalBytes)+
        ' GiB ('+Number(storage.usedPercent)+'%) · เหลือ '+gib(storage.availableBytes)+' GiB';
    }
    $('adminStorageStatus').textContent = storageText+' · รอคิว '+queued+
      ' · กำลังทำ '+running+' · จองทรัพยากร '+reservations+'/'+maxConcurrent;
  }
  function renderJobs() {
    const search = $('adminJobSearch').value.trim().toLowerCase();
    const filtered = jobs.filter(item =>
      item.id.toLowerCase().includes(search) ||
      item.owner.toLowerCase().includes(search) ||
      item.stage.toLowerCase().includes(search)
    );
    if (!filtered.length) {
      clearPanel('adminJobRows','ไม่พบงานในหน้านี้',6);
    } else {
      const body = $('adminJobRows');
      body.replaceChildren();
      for (const item of filtered) {
        const tr = node('tr');
        const id = node('td');
        const code = node('code',item.id.slice(0,12));
        code.title=item.id;
        id.appendChild(code);
        tr.appendChild(id);
        const owner=node('td');
        const ownerCode=node('code',item.owner ? item.owner.slice(0,12)+'…' : 'งานเดิม');
        ownerCode.title=item.owner || '';
        owner.appendChild(ownerCode);
        tr.appendChild(owner);
        tr.appendChild(node('td',(jobNames[item.stage] || item.stage) + (item.errorCode ? ' · '+item.errorCode : '')));
        tr.appendChild(node('td',String(item.completed)+' / '+String(item.total)));
        tr.appendChild(node('td',dateText(item.created)));
        const action = node('td');
        if (['queued','preparing','downloading','converting'].includes(item.stage) && !item.cancelRequested) {
          const cancel = node('button','ระงับงาน','button-quiet');
          cancel.type='button';
          cancel.setAttribute('aria-label','ระงับงาน '+item.id.slice(0,12));
          cancel.addEventListener('click',async () => {
            const reason = $('adminCancelReason').value;
            if (!window.confirm('ยืนยันการยกเลิกงาน #'+item.id.slice(0,12)+' ?')) return;
            cancel.disabled=true;
            try {
              await api('/api/admin/jobs/'+item.id+'/cancel',{
                method:'POST',headers:{'Content-Type':'application/json'},
                body:JSON.stringify({reason})
              });
              report('');
              await refreshAll();
              $('adminJobsTitle').focus();
            } catch(e) {report(e.message)}
            finally {cancel.disabled=false}
          });
          action.appendChild(cancel);
        } else {
          action.textContent=item.cancelRequested ? 'ร้องขอยกเลิกแล้ว' : '—';
        }
        // Put the intervention control first for keyboard and mobile access.
        tr.insertBefore(action,tr.firstChild);
        body.appendChild(tr);
      }
    }
    $('adminPageLabel').textContent='หน้า '+(currentPage+1)+' · ทั้งหมด '+jobTotal+' งาน';
    $('adminPrev').disabled=currentPage===0;
    $('adminNext').disabled=(currentPage+1)*pageSize>=jobTotal;
  }
  async function refreshJobs() {
    const result=await api('/api/admin/jobs?limit='+pageSize+'&offset='+(currentPage*pageSize));
    jobs=Array.isArray(result.jobs)?result.jobs:[];
    jobTotal=result.total || 0;
    if (currentPage>0 && !jobs.length && jobTotal>0) {
      currentPage=Math.max(0,Math.ceil(jobTotal/pageSize)-1);
      return refreshJobs();
    }
    renderJobs();
  }
  function renderUsers() {
    const search=$('adminUserSearch').value.trim().toLowerCase();
    const filtered=users.filter(u => u.email.toLowerCase().includes(search) ||
      u.identityRef.toLowerCase().includes(search)).slice(0,100);
    if (!filtered.length) {
      clearPanel('adminUserRows','ไม่มีข้อมูลบัญชีที่ตรงกับการค้นหา',4);
      return;
    }
    const body=$('adminUserRows');
    body.replaceChildren();
    for (const u of filtered) {
      const tr=node('tr');
      tr.appendChild(node('td',u.email || '(รอเข้าสู่ระบบครั้งแรก)'));
      tr.appendChild(node('td',roleNames[u.role] || u.role));
      tr.appendChild(node('td',dateText(u.lastSeen)));
      const ctrl=node('td');
      if (me.role==='super_admin' && u.identityRef!==me.identityRef && !u.disabled) {
        const select=node('select');
        select.setAttribute('aria-label','เปลี่ยนบทบาท '+(u.email || u.identityRef));
        select.dataset.identityRef=u.identityRef;
        for (const role of ['user','admin','super_admin']) {
          const option=node('option',roleNames[role]);
          option.value=role;
          option.selected=role===u.role;
          select.appendChild(option);
        }
        select.addEventListener('change',async()=>{
          const next=select.value;
          select.disabled=true;
          if (window.confirm('ยืนยันเปลี่ยนบทบาทของ '+(u.email || 'บัญชีนี้')+' เป็น '+roleNames[next]+' ?')) {
            try {
              await api('/api/admin/users/'+encodeURIComponent(u.identityRef)+'/role',{
                method:'PUT',headers:{'Content-Type':'application/json'},
                body:JSON.stringify({role:next})
              });
              report('');
              await refreshUsers();
              await refreshAudit();
              const replacement=Array.from($('adminUserRows').querySelectorAll('select'))
                .find(el=>el.dataset.identityRef===u.identityRef);
              if(replacement) replacement.focus();
            } catch(e) {report(e.message);select.value=u.role}
          } else select.value=u.role;
          select.disabled=false;
        });
        ctrl.appendChild(select);
      } else ctrl.textContent=u.identityRef===me.identityRef?'บัญชีของคุณ':'อ่านอย่างเดียว';
      tr.appendChild(ctrl);
      body.appendChild(tr);
    }
  }
  async function refreshUsers() {
    const result=await api('/api/admin/users');
    users=Array.isArray(result.users)?result.users:[];
    renderUsers();
  }
  async function refreshAudit() {
    if (!me || me.role!=='super_admin') return;
    const result=await api('/api/admin/audit?limit=50');
    const list=$('adminAuditList');
    list.replaceChildren();
    const events=Array.isArray(result.events)?result.events:[];
    if (!events.length) {
      list.appendChild(node('li','ยังไม่มีเหตุการณ์ที่บันทึกไว้'));
      return;
    }
    for (const e of events) {
      const li=node('li');
      li.appendChild(node('strong',e.event));
      li.appendChild(node('span',' · '+dateText(e.at)+' · '+String(e.actor).slice(0,12)+'…'));
      list.appendChild(li);
    }
  }
  async function refreshAll() {
    if (!me) return;
    report('');
    const refreshButton=$('adminRefresh');
    refreshButton.disabled=true;
    refreshButton.setAttribute('aria-busy','true');
    try {
      await Promise.all([refreshSummary(),refreshJobs(),refreshUsers(),refreshAudit()]);
    } catch(e) {report(e.message)}
    finally {
      refreshButton.disabled=false;
      refreshButton.removeAttribute('aria-busy');
    }
  }
  async function checkRole() {
    try {
      const response=await fetch('/auth/me',{credentials:'same-origin',cache:'no-store'});
      if (!response.ok) throw new Error('not signed in');
      const user=await response.json();
      if (user.role!=='admin' && user.role!=='super_admin') throw new Error('no role');
      me=user;
      $('adminNav').hidden=false;
      $('adminConsole').hidden=false;
      $('adminRoleLabel').textContent=roleNames[user.role] || user.role;
      $('adminAuditSection').hidden=user.role!=='super_admin';
      $('adminRoleHelp').textContent=user.role==='super_admin'
        ? 'สามารถจัดการบทบาทแอปพลิเคชันของผู้ใช้ที่ลงชื่อเข้าใช้แล้ว'
        : 'คุณมีสิทธิ์อ่านรายชื่อผู้ใช้ แต่ไม่สามารถเปลี่ยนบทบาท';
      await refreshAll();
    } catch (_) {
      me=null;
      $('adminNav').hidden=true;
      $('adminConsole').hidden=true;
    }
  }
  $('adminRefresh').addEventListener('click',refreshAll);
  $('adminJobSearch').addEventListener('input',renderJobs);
  $('adminUserSearch').addEventListener('input',renderUsers);
  $('adminPrev').addEventListener('click',()=>{if(currentPage>0){currentPage--;refreshJobs().catch(e=>report(e.message))}});
  $('adminNext').addEventListener('click',()=>{if((currentPage+1)*pageSize<jobTotal){currentPage++;refreshJobs().catch(e=>report(e.message))}});
  checkRole();
})();