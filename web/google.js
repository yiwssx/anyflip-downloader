(() => {
  const $ = id => document.getElementById(id);
  const labels = {queued:'อยู่ในคิว',preparing:'เตรียมข้อมูล',downloading:'ดาวน์โหลดภาพ',converting:'สร้าง PDF',completed:'สำเร็จ',failed:'ไม่สำเร็จ',cancelled:'ยกเลิกแล้ว'};
  let currentJob = null, poll = null, historySnapshot = [];
  sessionStorage.removeItem('anyflip-access-token');
  function error(el,msg){el.textContent=msg;el.hidden=!msg}
  async function request(path, opts={}) {
    const res = await fetch(path,{...opts,credentials:'same-origin',cache:'no-store'});
    if(!res.ok){
      const text=await res.text();
      if(res.status===401){showLogin();throw new Error('กรุณาเข้าสู่ระบบด้วย Google อีกครั้ง')}
      throw new Error(res.status===429?'ขณะนี้คิวงานเต็ม กรุณาลองใหม่อีกครั้ง':text.trim()||'เกิดข้อผิดพลาด ('+res.status+')')
    }
    return res;
  }
  async function refresh(){
    if(!currentJob)return;
    try{
      const res = await request('/api/jobs/'+currentJob);
      const j = await res.json(), total=j.total||0, completed=j.completed||0;
      const progress=total?Math.min(100,Math.round(completed*100/total)):0;
      $('jobState').textContent=labels[j.stage]||j.stage;
      $('jobState').dataset.state=j.stage;
      const stageOrder=['queued','preparing','downloading','converting'];
      const currentStage=stageOrder.indexOf(j.stage);
      document.querySelectorAll('.progress-steps [data-stage]').forEach(step=>{
        const index=stageOrder.indexOf(step.dataset.stage);
        step.classList.toggle('is-current',index===currentStage);
        step.classList.toggle('is-done',j.stage==='completed'||(currentStage>index&&index>=0));
      });
      $('jobDescription').textContent=j.stage==='completed'?'สร้าง Image-based PDF สำเร็จแล้ว พร้อมดาวน์โหลด':j.stage==='failed'?'งานนี้ไม่สำเร็จ':j.stage==='cancelled'?'งานถูกยกเลิก':'ระบบกำลังดำเนินการ: '+(labels[j.stage]||j.stage);
      $('progressFill').style.width=progress+'%';
      $('progressTrack').setAttribute('aria-valuenow',progress);
      $('pageCount').textContent=completed+' / '+total+' หน้า';
      $('percent').textContent=progress+'%';
      error($('jobError'),j.error ? j.error+(j.errorCode?' (รหัส: '+j.errorCode+')':'') : '');
      const done=['completed','failed','cancelled'].includes(j.stage);
      $('cancelButton').hidden=done||j.archived;
      $('downloadButton').hidden=!j.downloadReady||j.archived;
      $('startButton').disabled=!done;
      if(done && poll){clearInterval(poll);poll=null;loadHistory();}
    }catch(e){error($('jobError'),e.message);}
  }

  function selectJob(id) {
    currentJob=id;
    $('jobReference').textContent='Job ID: '+id;
    $('jobReference').hidden=false;
    $('jobPanel').hidden=false;
    $('cancelButton').hidden=false;
    $('downloadButton').hidden=true;
    if(poll)clearInterval(poll);
    poll=setInterval(refresh,1100);
    refresh();
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
    try{
      const response=await request('/api/jobs');
      const data=await response.json();
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
    if(currentJob && poll)return;
    $('startButton').disabled=true;
    try{
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
  function showLogin(){
    if(poll){clearInterval(poll);poll=null}
    currentJob=null;
    $('jobPanel').hidden=true;
    $('workArea').hidden=true;
    $('loginPanel').hidden=false;
  }
  function showUser(user){
    $('signedInUser').textContent=user.email;
    $('loginPanel').hidden=true;
    $('workArea').hidden=false;
    loadHistory();
  }
  $('logoutButton').addEventListener('click',async()=>{
    try {
      const res=await fetch('/auth/logout',{method:'POST',credentials:'same-origin',cache:'no-store'});
      if(!res.ok)throw new Error('ออกจากระบบไม่สำเร็จ');
      showLogin();
    } catch(e) {error($('logoutError'),e.message)}
  });
  fetch('/auth/me',{credentials:'same-origin',cache:'no-store'})
    .then(async res=>{if(!res.ok){showLogin();return}showUser(await res.json())})
    .catch(()=>showLogin());
})();
