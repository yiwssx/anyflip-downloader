(() => {
  const $ = id => document.getElementById(id);
  const labels = {queued:'อยู่ในคิว',preparing:'เตรียมข้อมูล',downloading:'ดาวน์โหลดภาพ',converting:'สร้าง PDF',completed:'สำเร็จ',failed:'ไม่สำเร็จ',cancelled:'ยกเลิกแล้ว'};
  let currentJob = null, poll = null;
  $('token').value = sessionStorage.getItem('anyflip-access-token') || '';
  function error(el,msg){el.textContent=msg;el.hidden=!msg}
  async function request(path, opts={}) {
    const token = $('token').value.trim();
    const headers = {...opts.headers,Authorization:'Bearer '+token};
    const res = await fetch(path,{...opts,headers,cache:'no-store'});
    if(!res.ok){const text=await res.text();if(res.status===401)throw new Error('Access Token ไม่ถูกต้อง');throw new Error(res.status===429?'ขณะนี้คิวงานเต็ม กรุณาลองใหม่อีกครั้ง':text.trim()||'เกิดข้อผิดพลาด ('+res.status+')')}
    return res;
  }
  async function refresh(){
    if(!currentJob)return;
    try{
      const res = await request('/api/jobs/'+currentJob);
      const j = await res.json(), total=j.total||0, completed=j.completed||0;
      const progress=total?Math.min(100,Math.round(completed*100/total)):0;
      $('jobState').textContent=labels[j.stage]||j.stage;
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
  async function loadHistory() {
    if(!$('token').value.trim()) {
      error($('historyError'),'กรุณากรอก Access Token ก่อนตรวจสอบประวัติ');
      return;
    }
    try {
      const res=await request('/api/jobs');
      const history=await res.json();
      const list=$('historyList');
      list.replaceChildren();
      if(!history.jobs.length) {
        const empty=document.createElement('p');
        empty.className='sub';empty.textContent='ยังไม่มีประวัติงาน';
        list.appendChild(empty);
      }
      for(const job of history.jobs) {
        const item=document.createElement('div');
        item.className='historyRow';
        const summary=document.createElement('span');
        summary.textContent=job.id.slice(0,8)+' · '+(labels[job.stage]||job.stage)+' · '+(job.completed||0)+'/'+(job.total||0)+' หน้า'+(job.archived?' · เก็บประวัติ':'');
        const controls=document.createElement('div');controls.className='actions';
        const open=document.createElement('button');
        open.type='button';open.className='secondary';open.textContent=job.archived?'ดูประวัติ':'ดูสถานะ';
        open.addEventListener('click',()=>selectJob(job.id));
        controls.appendChild(open);
        if(!job.archived&&(job.stage==='failed'||job.stage==='cancelled')){
          const retry=document.createElement('button');
          retry.type='button';retry.className='secondary';retry.textContent='ลองใหม่';
          retry.addEventListener('click',async()=>{
            retry.disabled=true;
            try {
              const res=await request('/api/jobs/'+job.id+'/retry',{method:'POST'});
              const next=await res.json();selectJob(next.id);await loadHistory();
            }catch(e){error($('historyError'),e.message)}
            finally{retry.disabled=false}
          });
          controls.appendChild(retry);
        }
        item.appendChild(summary);item.appendChild(controls);list.appendChild(item);
      }
      error($('historyError'),'');
    }catch(e){error($('historyError'),e.message)}
  }
  $('refreshHistory').addEventListener('click',loadHistory);
  $('batchStartButton').addEventListener('click',async()=>{
    error($('batchError'),'');
    const button=$('batchStartButton');
    button.disabled=true;
    try {
      const token=$('token').value.trim();
      if(token.length<16)throw new Error('กรุณากรอก Access Token ก่อนเพิ่มรายการ');
      sessionStorage.setItem('anyflip-access-token',token);
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
      const token=$('token').value.trim();
      if(token.length<16)throw new Error('กรุณากรอก Access Token ที่ผู้ดูแลระบบกำหนด');
      sessionStorage.setItem('anyflip-access-token',token);
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
})();
