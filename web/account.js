(() => {
  const $ = id => document.getElementById(id);
  const labels = {queued:'อยู่ในคิว',preparing:'เตรียมข้อมูล',downloading:'ดาวน์โหลดภาพ',converting:'สร้าง PDF',completed:'สำเร็จ',failed:'ไม่สำเร็จ',cancelled:'ยกเลิกแล้ว'};
  let currentJob = null, poll = null, signedInUsername = '';
  sessionStorage.removeItem('anyflip-access-token');
  function error(el,msg){el.textContent=msg;el.hidden=!msg}
  async function request(path, opts={}) {
    const res = await fetch(path,{...opts,credentials:'same-origin',cache:'no-store'});
    if(!res.ok){
      const text=await res.text();
      if(res.status===401){showLogin();throw new Error('เซสชันหมดอายุ กรุณาเข้าสู่ระบบอีกครั้ง')}
      throw new Error(res.status===403?'ไม่มีสิทธิ์ดำเนินการ':res.status===429?'ขณะนี้ระบบจำกัดคำขอ กรุณาลองใหม่อีกครั้ง':text.trim()||'เกิดข้อผิดพลาด ('+res.status+')')
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

  function showLogin() {
    if(poll){clearInterval(poll);poll=null}
    currentJob=null;
    signedInUsername='';
    for(const id of ['password','currentPassword','changedPassword','resetPassword','newPassword']) $(id).value='';
    $('adminPanel').hidden=true;
    $('jobPanel').hidden=true;
    $('workArea').hidden=true;
    $('loginPanel').hidden=false;
  }
  function showAccount(user) {
    signedInUsername=user.username;
    $('signedInUser').textContent=user.username+' ('+user.role+')';
    $('adminPanel').hidden=user.role!=='admin';
    if(user.role==='admin') loadUsers();
    $('loginPanel').hidden=true;
    $('workArea').hidden=false;
    $('password').value='';
    error($('loginError'),'');
    loadHistory();
  }
  $('loginForm').addEventListener('submit',async event=>{
    event.preventDefault();
    const button=$('loginButton');button.disabled=true;
    try {
      const res=await fetch('/auth/login',{method:'POST',credentials:'same-origin',cache:'no-store',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:$('username').value.trim(),password:$('password').value})});
      if(!res.ok){error($('loginError'),res.status===429?'ลองใหม่อีกครั้งภายหลัง':'ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง');return}
      showAccount(await res.json());
    }catch(e){error($('loginError'),'ไม่สามารถเชื่อมต่อระบบยืนยันตัวตน')}
    finally{button.disabled=false}
  });
  $('logoutButton').addEventListener('click',async()=>{
    error($('logoutError'),'');
    try {
      const res=await fetch('/auth/logout',{method:'POST',credentials:'same-origin',cache:'no-store'});
      if(!res.ok)throw new Error('ออกจากระบบไม่สำเร็จ กรุณาลองอีกครั้ง');
      showLogin();
    }catch(e){error($('logoutError'),e.message || 'ไม่สามารถออกจากระบบได้')}
  });
  $('createUserForm').addEventListener('submit',async event=>{
    event.preventDefault();
    const button=$('createUserButton');button.disabled=true;
    error($('createUserError'),'');$('createUserSuccess').hidden=true;
    try{
      const res=await request('/auth/users',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:$('newUsername').value.trim(),password:$('newPassword').value,role:$('newRole').value})});
      if(!res.ok)return;
      const created=await res.json();
      $('createUserSuccess').textContent='สร้างบัญชี '+created.username+' เรียบร้อย';
      $('createUserSuccess').hidden=false;
      $('newPassword').value='';$('newUsername').value='';
      await loadUsers();
    }catch(e){error($('createUserError'),e.message)}
    finally{button.disabled=false}
  });
  $('changePasswordForm').addEventListener('submit',async event=>{
    event.preventDefault();
    const button=$('changePasswordButton');
    button.disabled=true;error($('changePasswordError'),'');
    try {
      await request('/auth/change-password',{method:'POST',headers:{'Content-Type':'application/json'},
        body:JSON.stringify({currentPassword:$('currentPassword').value,newPassword:$('changedPassword').value})});
      showLogin();
      error($('loginError'),'เปลี่ยนรหัสผ่านเรียบร้อยแล้ว กรุณาเข้าสู่ระบบใหม่');
    }catch(e){error($('changePasswordError'),e.message)}
    finally{button.disabled=false}
  });

  async function loadUsers() {
    try {
      const response=await request('/auth/users');
      const data=await response.json();
      const list=$('usersList');
      const selector=$('resetUsername');
      list.replaceChildren();
      selector.replaceChildren();
      for(const user of data.users){
        const row=document.createElement('div');
        row.className='historyRow';
        const name=document.createElement('span');
        name.textContent=user.username+' · '+user.role+' · '+(user.disabled?'ระงับบัญชี':'ใช้งานได้');
        row.appendChild(name);
        const controls=document.createElement('div');
        controls.className='actions';
        const toggle=document.createElement('button');
        toggle.type='button';toggle.className=user.disabled?'secondary':'danger';
        toggle.textContent=user.disabled?'เปิดใช้บัญชี':'ระงับบัญชี';
        toggle.disabled=user.username===signedInUsername;
        toggle.addEventListener('click',async()=>{
          toggle.disabled=true;error($('userActionError'),'');
          try {
            await request('/auth/users/'+encodeURIComponent(user.username)+'/status',{
              method:'PATCH',headers:{'Content-Type':'application/json'},
              body:JSON.stringify({disabled:!user.disabled})
            });
            await loadUsers();
          }catch(e){error($('userActionError'),e.message)}
          finally{toggle.disabled=false}
        });
        controls.appendChild(toggle);
        row.appendChild(controls);
        list.appendChild(row);
        if(user.username!==signedInUsername){
          const option=document.createElement('option');
          option.value=user.username;
          option.textContent=user.username+' ('+user.role+')';
          selector.appendChild(option);
        }
      }
      error($('userActionError'),'');
    }catch(e){error($('userActionError'),e.message)}
  }
  $('refreshUsers').addEventListener('click',loadUsers);
  $('resetPasswordForm').addEventListener('submit',async event=>{
    event.preventDefault();
    const button=$('resetPasswordButton');
    button.disabled=true;error($('resetPasswordError'),'');
    $('resetPasswordSuccess').hidden=true;
    try {
      const username=$('resetUsername').value;
      if(!username)throw new Error('กรุณาเลือกบัญชีผู้ใช้');
      await request('/auth/users/'+encodeURIComponent(username)+'/reset-password',{
        method:'POST',headers:{'Content-Type':'application/json'},
        body:JSON.stringify({password:$('resetPassword').value})
      });
      $('resetPassword').value='';
      $('resetPasswordSuccess').textContent='รีเซ็ตรหัสผ่านให้ '+username+' แล้ว เซสชันเดิมถูกยกเลิกทั้งหมด';
      $('resetPasswordSuccess').hidden=false;
      await loadUsers();
    }catch(e){error($('resetPasswordError'),e.message)}
    finally{button.disabled=false}
  });
  fetch('/auth/me',{credentials:'same-origin',cache:'no-store'}).then(async res=>{
    if(!res.ok){showLogin();return}
    showAccount(await res.json());
  }).catch(()=>showLogin());
})();
