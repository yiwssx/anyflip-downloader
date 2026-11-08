// Shared, same-origin workspace controller used by token and Google SSO adapters.
// No secrets or role authority belong in this module. Backend is authoritative.
(() => {
  'use strict';
  function start({ request, canLoadHistory = () => true, beforeBatch = async () => {}, beforeCreate = async () => {} }) {

  const $ = id => document.getElementById(id);
  const labels = {queued:'อยู่ในคิว',preparing:'เตรียมข้อมูล',downloading:'ดาวน์โหลดภาพ',converting:'สร้าง PDF',completed:'สำเร็จ',failed:'ไม่สำเร็จ',cancelled:'ยกเลิกแล้ว'};
  let currentJob = null, pollTimer = null, pollController = null;
  let generation = 0, activeRefresh = null, failures = 0, historySnapshot = [];
  const error = (el, message) => { el.textContent=message; el.hidden=!message; };

  // Single-flight, cancellable polling. A stale response must not paint a new job.
  function stopPolling() {
    generation += 1;
    if (pollTimer !== null) { clearTimeout(pollTimer); pollTimer = null; }
    if (pollController) { pollController.abort(); pollController = null; }
    activeRefresh = null;
  }
  async function refresh() {
    if (!currentJob || activeRefresh === generation) return;
    const jobID = currentJob, version = generation;
    const controller = new AbortController();
    activeRefresh = version;
    pollController = controller;
    const timeout = setTimeout(() => controller.abort(), 15000);
    let retry = false;
    try {
      const res = await request('/api/jobs/'+encodeURIComponent(jobID), { signal: controller.signal });
      const j = await res.json();
      if (version !== generation || currentJob !== jobID) return;
      if (!j || typeof j !== 'object' || typeof j.stage !== 'string') {
        throw new Error('ข้อมูลสถานะงานไม่ถูกต้อง');
      }
      const total = Number.isFinite(j.total) ? j.total : 0;
      const completed = Number.isFinite(j.completed) ? j.completed : 0;
      const progress = total > 0 ? Math.min(100, Math.round(completed*100/total)) : 0;
      $('jobState').textContent = labels[j.stage] || j.stage;
      $('jobState').dataset.state = j.stage;
      const stageOrder = ['queued','preparing','downloading','converting'];
      const currentStage = stageOrder.indexOf(j.stage);
      document.querySelectorAll('.progress-steps [data-stage]').forEach(step => {
        const index = stageOrder.indexOf(step.dataset.stage);
        step.classList.toggle('is-current', index === currentStage);
        step.classList.toggle('is-done', j.stage === 'completed' || (currentStage > index && index >= 0));
      });
      const description = j.stage==='completed' ? 'สร้าง Image-based PDF สำเร็จแล้ว พร้อมดาวน์โหลด'
        : j.stage==='failed' ? 'งานนี้ไม่สำเร็จ'
        : j.stage==='cancelled' ? 'งานถูกยกเลิก'
        : 'ระบบกำลังดำเนินการ: '+(labels[j.stage] || j.stage);
      // Only announce a changed stage; repeating it every poll floods live regions.
      if ($('jobDescription').textContent !== description) $('jobDescription').textContent = description;
      // Native progress value avoids CSP-blocked inline style mutations (WebKit).
      $('progressFill').value = progress;
      $('progressTrack').setAttribute('aria-valuenow', String(progress));
      $('pageCount').textContent = completed+' / '+total+' หน้า';
      $('percent').textContent = progress+'%';
      error($('jobError'), j.error ? j.error+(j.errorCode ? ' (รหัส: '+j.errorCode+')' : '') : '');
      const done = ['completed','failed','cancelled'].includes(j.stage);
      $('cancelButton').hidden = done || j.archived;
      $('downloadButton').hidden = !j.downloadReady || j.archived;
      $('startButton').disabled = !done;
      failures = 0;
      if (done) {
        stopPolling();
        void loadHistory();
      } else {
        retry = true;
      }
    } catch(e) {
      if (version !== generation || currentJob !== jobID) return;
      if (e.authExpired) {
        stopPolling();
        error($('jobError'), e.message);
      } else {
        failures += 1;
        error($('jobError'), e.name === 'AbortError'
          ? 'การเชื่อมต่อหมดเวลา ระบบจะลองตรวจสถานะอีกครั้ง'
          : e.message);
        retry = true;
      }
    } finally {
      clearTimeout(timeout);
      if (pollController === controller) pollController = null;
      if (activeRefresh === version) activeRefresh = null;
      if (retry && generation === version && currentJob === jobID) {
        const delay = Math.min(15000, 1100 * Math.pow(2, Math.min(failures, 4)));
        pollTimer = setTimeout(() => { pollTimer = null; void refresh(); }, delay);
      }
    }
  }
  function selectJob(id) {
    stopPolling();
    currentJob = id;
    failures = 0;
    $('jobReference').textContent = 'Job ID: '+id;
    $('jobReference').hidden = false;
    $('jobPanel').hidden = false;
    $('cancelButton').hidden = false;
    $('downloadButton').hidden = true;
    void refresh();
  }

  function renderHistory() {
    const list=$('historyList');
    const query=$('historySearch').value.trim().toLowerCase();
    const status=$('historyFilter').value;
    const activeStages=['queued','preparing','downloading','converting'];
    const filtered=historySnapshot.filter(job=>
      job.id.toLowerCase().includes(query) &&
      (status==='all'||(status==='active'&&activeStages.includes(job.stage))||job.stage===status)
    );
    list.replaceChildren();
    if(!filtered.length){
      const empty=document.createElement('div');
      empty.className='empty-state';
      const icon=document.createElement('span');icon.textContent='◷';icon.setAttribute('aria-hidden','true');
      const title=document.createElement('strong');
      title.textContent=historySnapshot.length?'ไม่พบรายการที่ตรงกับการค้นหา':'ยังไม่มีประวัติงาน';
      const info=document.createElement('p');
      info.textContent=historySnapshot.length?'ลองเปลี่ยนตัวกรองหรือรหัสงาน':'เมื่อคุณเริ่มดาวน์โหลด งานจะปรากฏที่นี่';
      empty.append(icon,title,info);
      list.appendChild(empty);
      return;
    }
    for(const job of filtered){
      const item=document.createElement('div');item.className='historyRow';
      const heading=document.createElement('div');heading.className='history-summary';
      const details=document.createElement('div');details.className='history-details';
      const id=document.createElement('span');id.className='history-id';
      id.textContent='งาน #'+job.id.slice(0,8);
      id.title='Job ID: '+job.id;
      const pages=document.createElement('span');pages.className='history-pages';
      pages.textContent=(job.completed||0)+' / '+(job.total||0)+' หน้า'+(job.archived?' · ประวัติย้อนหลัง':'');
      details.append(id,pages);
      const chip=document.createElement('span');
      chip.className='status-chip';chip.dataset.state=job.stage;
      chip.textContent=labels[job.stage]||job.stage;
      heading.append(details,chip);
      const controls=document.createElement('div');controls.className='actions';
      const open=document.createElement('button');
      open.type='button';open.className='secondary';open.textContent=job.archived?'ดูประวัติ':'ดูสถานะ';
      open.addEventListener('click',()=>selectJob(job.id));
      controls.appendChild(open);
      if(!job.archived&&(job.stage==='failed'||job.stage==='cancelled')){
        const retry=document.createElement('button');
        retry.type='button';retry.className='secondary';retry.textContent='ลองใหม่';
        retry.addEventListener('click',async()=>{
          retry.disabled=true;error($('historyError'),'');
          try{
            const res=await request('/api/jobs/'+job.id+'/retry',{method:'POST'});
            const next=await res.json();
            selectJob(next.id);
            await loadHistory();
          }catch(e){error($('historyError'),e.message)}
          finally{retry.disabled=false}
        });
        controls.appendChild(retry);
      }
      item.append(heading,controls);
      list.appendChild(item);
    }
  }
  async function loadHistory(){
    if (!canLoadHistory()) {
      historySnapshot = [];
      $('statTotal').textContent = '—';
      $('statActive').textContent = '—';
      $('statDone').textContent = '—';
      renderHistory();
      return;
    }
    const version = generation;
    try{
      const response=await request('/api/jobs');
      const data=await response.json();
      if (version !== generation) return;
      historySnapshot=Array.isArray(data.jobs)?data.jobs:[];
      $('statTotal').textContent=String(historySnapshot.length);
      $('statActive').textContent=String(historySnapshot.filter(j=>!['completed','failed','cancelled'].includes(j.stage)).length);
      $('statDone').textContent=String(historySnapshot.filter(j=>j.stage==='completed').length);
      renderHistory();
      error($('historyError'),'');
    }catch(e){error($('historyError'),e.message)}
  }
  $('historySearch').addEventListener('input',renderHistory);
  $('historyFilter').addEventListener('change',renderHistory);
  const navLinks=Array.from(document.querySelectorAll('.nav-link'));
  function syncNavigation(){
    const fragment=location.hash||'#overview';
    navLinks.forEach(link=>{
      const active=link.getAttribute('href')===fragment;
      link.classList.toggle('is-current',active);
      if(active)link.setAttribute('aria-current','location');
      else link.removeAttribute('aria-current');
    });
  }
  window.addEventListener('hashchange',syncNavigation);
  syncNavigation();
  const batchCount=()=>{
    const count=$('batchURLs').value.split(/\r?\n/).filter(line=>line.trim()).length;
    $('batchCount').textContent=count+' / 8';
    $('batchCount').parentElement.dataset.overLimit=String(count>8);
  };
  $('batchURLs').addEventListener('input',batchCount);
  batchCount();
  $('refreshHistory').addEventListener('click',loadHistory);
  $('batchStartButton').addEventListener('click',async()=>{
    error($('batchError'),'');
    const button=$('batchStartButton');
    button.disabled=true;
    try {
      const urls=$('batchURLs').value.split(/\r?\n/).map(url=>url.trim()).filter(Boolean);
      if(urls.length<1||urls.length>8)throw new Error('ต้องมีลิงก์ตั้งแต่ 1 ถึง 8 รายการ');
      await beforeBatch();
      const jobs=urls.map(url=>({url,threads:Number($('threads').value),bestQuality:$('bestQuality').checked}));
      const res=await request('/api/jobs/batch',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({jobs})});
      const accepted=await res.json();
      selectJob(accepted.jobs[0].id);
      await loadHistory();
    }catch(e){error($('batchError'),e.message)}
    finally{button.disabled=false}
  });
  $('jobForm').addEventListener('submit',async event=>{
    event.preventDefault();error($('formError'),'');
    if(currentJob && (pollTimer !== null || activeRefresh !== null))return;
    $('startButton').disabled=true;
    try{
      await beforeCreate();
      const res=await request('/api/jobs',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({
        url:$('bookURL').value.trim(),title:$('bookTitle').value.trim(),threads:Number($('threads').value),bestQuality:$('bestQuality').checked
      })});
      const j=await res.json();selectJob(j.id);await loadHistory();
    }catch(e){error($('formError'),e.message);$('startButton').disabled=false;}
  });
  $('cancelButton').addEventListener('click',async()=>{if(!currentJob)return;try{await request('/api/jobs/'+currentJob+'/cancel',{method:'POST'});await refresh();}catch(e){error($('jobError'),e.message)}});
  $('downloadButton').addEventListener('click',async()=>{
    if(!currentJob)return;
    try{
      $('downloadButton').disabled=true;
      const res=await request('/api/jobs/'+currentJob+'/download');
      const blob=await res.blob();const url=URL.createObjectURL(blob);
      const a=document.createElement('a');a.href=url;a.download='anyflip-'+currentJob+'.pdf';document.body.appendChild(a);a.click();a.remove();
      setTimeout(()=>URL.revokeObjectURL(url),10000);
    }catch(e){error($('jobError'),e.message)}finally{$('downloadButton').disabled=false;}
  });

    return {
      stop() { stopPolling(); currentJob = null; },
      loadHistory,
    };
  }
  window.AnyflipWorkspace = Object.freeze({ start });
})();
