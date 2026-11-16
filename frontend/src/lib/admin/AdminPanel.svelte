<script lang="ts">
  import { onMount, tick } from "svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { AdminClient } from "./client";
  import {
    CANCEL_REASONS, ROLE_NAMES, dateText, gib,
    type AdminSummary, type AdminJob, type AdminUser, type AdminAudit,
    type CancelReason, type RBACRole,
  } from "./contracts";
  import type { SessionUser } from "../api/contracts";
  import { ACTIVE_STAGES, STAGE_LABEL } from "../api/contracts";
  import { ChevronLeft, ChevronRight, RefreshCw, ShieldCheck, Users, HardDrive, ListChecks, ShieldAlert, X } from "@lucide/svelte";

  type Tab = "jobs" | "users" | "audit";
  type Pending = {kind:"cancel"; job:AdminJob} | {kind:"role"; user:AdminUser};
  type Props = { viewer:SessionUser; onDenied:(reason:string)=>void };
  let {viewer,onDenied}:Props = $props();
  const api=new AdminClient({onDenied});
  let tab=$state<Tab>("jobs");
  let summary=$state<AdminSummary|null>(null);
  let jobs=$state<AdminJob[]>([]);
  let users=$state<AdminUser[]>([]);
  let events=$state<AdminAudit[]>([]);
  let currentPage=$state(0);
  let jobTotal=$state(0);
  let jobSearch=$state("");
  let userSearch=$state("");
  let reason=$state<CancelReason>("policy");
  let roleChoice=$state<RBACRole>("user");
  let loading=$state(false);
  let applying=$state(false);
  let error=$state("");
  let pending=$state<Pending|null>(null);
  let dialog:HTMLDialogElement;
  let alive=true;
  const pageSize=20;

  const mayEditRole=$derived(viewer.role==="super_admin");
  const visibleJobs=$derived(jobs.filter(j=>{
    const q=jobSearch.trim().toLowerCase();
    return j.id.toLowerCase().includes(q)||j.owner.toLowerCase().includes(q)||j.stage.includes(q);
  }));
  const visibleUsers=$derived(users.filter(u=>{
    const q=userSearch.trim().toLowerCase();
    return u.email.toLowerCase().includes(q)||u.identityRef.toLowerCase().includes(q);
  }).slice(0,100));

  onMount(()=>{
    alive=true;
    void refreshAll();
    return ()=>{alive=false;pending=null;dialog?.close();};
  });
  function message(err:unknown):string {
    return err instanceof Error ? err.message:"คำขอไม่สำเร็จ";
  }
  async function refreshAll():Promise<void> {
    if(loading)return;
    loading=true;
    error="";
    try {
      const [summaryResult,jobResult,userResult,auditResult]=await Promise.all([
        api.summary(),api.jobs(currentPage),api.users(),
        viewer.role==="super_admin" ? api.audit() : Promise.resolve([]),
      ]);
      if(!alive)return;
      summary=summaryResult;
      jobs=jobResult.jobs;
      jobTotal=jobResult.total;
      users=userResult;
      events=auditResult;
      if(currentPage>0 && jobs.length===0 && jobTotal>0) {
        currentPage=Math.max(0,Math.ceil(jobTotal/pageSize)-1);
        await refreshJobs();
      }
    }catch(err) { if(alive)error=message(err); }
    finally {loading=false;}
  }
  async function refreshJobs():Promise<void> {
    try{
      const result=await api.jobs(currentPage);
      if(!alive)return;
      jobs=result.jobs;
      jobTotal=result.total;
    }catch(err){if(alive)error=message(err);}
  }
  async function changePage(delta:number):Promise<void> {
    const next=currentPage+delta;
    if(next<0 || next*pageSize>=jobTotal)return;
    currentPage=next;
    await refreshJobs();
  }
  async function openCancel(job:AdminJob):Promise<void> {
    if(!ACTIVE_STAGES.includes(job.stage)||job.cancelRequested||job.archived)return;
    reason="policy";
    pending={kind:"cancel",job};
    await tick();
    dialog.showModal();
  }
  async function openRole(user:AdminUser):Promise<void> {
    if(!mayEditRole||user.disabled||user.identityRef===viewer.identityRef)return;
    roleChoice=user.role;
    pending={kind:"role",user};
    await tick();
    dialog.showModal();
  }
  function closeDialog():void {
    if(applying)return;
    dialog.close();
    pending=null;
  }
  async function confirmAction():Promise<void> {
    if(!pending||applying)return;
    applying=true;
    error="";
    const target=pending;
    try {
      if(target.kind==="cancel"){
        await api.cancelJob(target.job.id,reason);
        if(!alive)return;
        await Promise.all([refreshJobs(),refreshSummary()]);
        tab="jobs";
      }else{
        if(!mayEditRole||target.user.identityRef===viewer.identityRef)throw new Error("ไม่อนุญาตให้แก้ไขบทบาทบัญชีของตนเอง");
        await api.setRole(target.user.identityRef,roleChoice);
        if(!alive)return;
        await Promise.all([refreshUsers(),refreshAudit()]);
        tab="users";
      }
      if(!alive)return;
      dialog.close();
      pending=null;
    }catch(err){
      if(alive)error=message(err);
    }finally {applying=false;}
  }
  async function refreshSummary():Promise<void> {
    try{const value=await api.summary();if(alive)summary=value;}catch(err){if(alive)error=message(err);}
  }
  async function refreshUsers():Promise<void> {
    try{const value=await api.users();if(alive)users=value;}catch(err){if(alive)error=message(err);}
  }
  async function refreshAudit():Promise<void> {
    if(!mayEditRole)return;
    try{const value=await api.audit();if(alive)events=value;}catch(err){if(alive)error=message(err);}
  }
</script>

<section aria-labelledby="adminTitle" class="space-y-6">
  <div class="flex flex-wrap items-start justify-between gap-4">
    <div>
      <p class="text-xs font-bold uppercase tracking-widest text-primary">INSTITUTION ADMINISTRATION</p>
      <h2 id="adminTitle" class="mt-1 text-2xl font-bold tracking-tight sm:text-3xl">ศูนย์จัดการระบบ</h2>
      <p class="mt-2 text-sm text-muted-foreground">{ROLE_NAMES[viewer.role]} · ข้อมูลและสิทธิ์ตรวจสอบโดย Go Backend</p>
    </div>
    <Button variant="secondary" disabled={loading} aria-busy={loading} onclick={()=>void refreshAll()}>
      <RefreshCw size={18} aria-hidden="true" /> {loading ? "กำลังโหลด..." : "รีเฟรช"}
    </Button>
  </div>

  {#if error}
    <p role="alert" class="rounded-xl border border-destructive/40 bg-secondary p-4 text-sm text-destructive">{error}</p>
  {/if}

  <section aria-label="ภาพรวมการจัดการ" class="grid grid-cols-2 gap-3 lg:grid-cols-4">
    {#each [
      ["งานทั้งหมด",summary?.total],["กำลังทำงาน",summary?.active],
      ["สำเร็จ",summary?.completed],["ไม่สำเร็จ",summary?.failed],
    ] as [label,value]}
      <Card class="min-w-0 space-y-2 p-4 sm:p-5">
        <p class="text-sm text-muted-foreground">{label}</p>
        <strong class="block text-2xl font-bold tabular-nums">{value??"—"}</strong>
      </Card>
    {/each}
  </section>

  <Card class="space-y-3 p-5">
    <div class="flex items-center gap-3">
      <HardDrive size={22} class="text-primary" aria-hidden="true" />
      <h3 class="font-semibold">ทรัพยากรระบบ</h3>
    </div>
    {#if summary}
      <p class="text-sm text-muted-foreground">
        {summary.storage
          ? `ใช้พื้นที่ ${gib(summary.storage.usedBytes)} / ${gib(summary.storage.totalBytes)} GiB · เหลือ ${gib(summary.storage.availableBytes)} GiB`
          : "พื้นที่จัดเก็บ: ไม่พร้อมให้ตรวจสอบ"}
      </p>
      <p class="text-xs text-muted-foreground">รอคิว {summary.queued} · กำลังทำ {summary.running} · จองทรัพยากร {summary.activeReservations}/{summary.maxConcurrent}</p>
    {:else}<p class="text-sm text-muted-foreground">กำลังตรวจสอบสถานะทรัพยากร...</p>{/if}
  </Card>

  <Card class="overflow-hidden p-4 sm:p-6">
    <div role="tablist" aria-label="ส่วนจัดการระบบ" class="grid grid-cols-2 gap-2 rounded-xl bg-secondary p-1 sm:grid-cols-3">
      {#each [
        {key:"jobs",label:"ติดตามงาน"},
        {key:"users",label:"บัญชีและบทบาท"},
        ...(mayEditRole?[{key:"audit",label:"Audit Log"}]:[]),
      ] as item (item.key)}
        <button type="button" role="tab" aria-selected={tab===item.key} class={`min-h-11 rounded-lg px-3 text-sm font-semibold transition-colors ${tab===item.key?"bg-card text-foreground shadow-sm":"text-muted-foreground hover:text-foreground"}`} onclick={()=>tab=item.key as Tab}>{item.label}</button>
      {/each}
    </div>

    {#if tab==="jobs"}
      <section aria-label="ติดตามและระงับงาน" class="mt-6 space-y-4">
        <div>
          <h3 class="text-lg font-semibold">งานดาวน์โหลดในระบบ</h3>
          <p class="mt-1 text-sm text-muted-foreground">แสดงข้อมูลสถานะขั้นต่ำ ไม่เปิดไฟล์ PDF ของผู้ใช้อื่น</p>
        </div>
        <div class="grid gap-3 sm:grid-cols-[1fr_200px]">
          <label class="space-y-2 text-sm"><span class="font-semibold">ค้นหาในหน้านี้</span><input type="search" bind:value={jobSearch} placeholder="Job ID / เจ้าของ / สถานะ" class="min-h-11 w-full rounded-xl border border-input bg-background px-4" /></label>
          <label class="space-y-2 text-sm"><span class="font-semibold">สาเหตุการระงับงาน</span><select bind:value={reason} class="min-h-11 w-full rounded-xl border border-input bg-background px-3">{#each Object.entries(CANCEL_REASONS) as [value,title]}<option value={value}>{title}</option>{/each}</select></label>
        </div>
        <div tabindex="0" role="region" aria-label="ตารางงานดาวน์โหลด" class="max-w-full overflow-x-auto rounded-xl border border-border">
          <table class="w-full min-w-[780px] border-collapse text-left text-sm">
            <thead><tr class="bg-secondary">
              <th class="px-4 py-3 font-semibold">จัดการ</th><th class="px-4 py-3 font-semibold">รหัสงาน</th>
              <th class="px-4 py-3 font-semibold">เจ้าของ</th><th class="px-4 py-3 font-semibold">สถานะ</th>
              <th class="px-4 py-3 font-semibold">จำนวนหน้า</th><th class="px-4 py-3 font-semibold">เริ่มเมื่อ</th>
            </tr></thead>
            <tbody>
              {#each visibleJobs as job (job.id)}
                <tr class="border-t border-border">
                  <td class="px-4 py-3">
                    {#if ACTIVE_STAGES.includes(job.stage) && !job.cancelRequested && !job.archived}
                      <Button size="sm" variant="outline" aria-label={`ระงับงาน ${job.id.slice(0,12)}`} onclick={()=>void openCancel(job)}>ระงับงาน</Button>
                    {:else}<span class="text-xs text-muted-foreground">{job.cancelRequested?"ร้องขอยกเลิกแล้ว":"—"}</span>{/if}
                  </td>
                  <td class="px-4 py-3 font-mono text-xs" title={job.id}>{job.id.slice(0,12)}</td>
                  <td class="px-4 py-3 font-mono text-xs" title={job.owner}>{job.owner?job.owner.slice(0,12)+"…":"งานเดิม"}</td>
                  <td class="px-4 py-3">{STAGE_LABEL[job.stage]}{job.errorCode?" · "+job.errorCode:""}</td>
                  <td class="px-4 py-3 tabular-nums">{job.completed}/{job.total}</td>
                  <td class="px-4 py-3 whitespace-nowrap">{dateText(job.created)}</td>
                </tr>
              {:else}
                <tr><td colspan="6" class="px-4 py-8 text-center text-muted-foreground">ไม่พบงานในหน้านี้</td></tr>
              {/each}
            </tbody>
          </table>
        </div>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <p class="text-sm text-muted-foreground">หน้า {currentPage+1} · ทั้งหมด {jobTotal} งาน</p>
          <div class="flex gap-2"><Button size="sm" variant="outline" disabled={currentPage===0} onclick={()=>void changePage(-1)}><ChevronLeft size={17} aria-hidden="true" /> ก่อนหน้า</Button><Button size="sm" variant="outline" disabled={(currentPage+1)*pageSize>=jobTotal} onclick={()=>void changePage(1)}>ถัดไป <ChevronRight size={17} aria-hidden="true" /></Button></div>
        </div>
      </section>
    {:else if tab==="users"}
      <section aria-label="บัญชีและบทบาท" class="mt-6 space-y-4">
        <div><h3 class="text-lg font-semibold">บัญชี Google Workspace</h3><p class="mt-1 text-sm text-muted-foreground">{mayEditRole?"ผู้ดูแลสูงสุดสามารถเปลี่ยนบทบาทของผู้อื่นได้":"ผู้ดูแลระบบมีสิทธิ์อ่านรายชื่อ แต่ไม่สามารถเปลี่ยนบทบาท"}</p></div>
        <label class="block max-w-lg space-y-2 text-sm"><span class="font-semibold">ค้นหาบัญชี</span><input type="search" bind:value={userSearch} placeholder="อีเมล / Identity Reference" class="min-h-11 w-full rounded-xl border border-input bg-background px-4" /></label>
        <div tabindex="0" role="region" aria-label="ตารางบัญชีผู้ใช้" class="max-w-full overflow-x-auto rounded-xl border border-border">
          <table class="w-full min-w-[660px] border-collapse text-left text-sm">
            <thead><tr class="bg-secondary"><th class="px-4 py-3">บัญชี</th><th class="px-4 py-3">บทบาท</th><th class="px-4 py-3">เข้าสู่ระบบล่าสุด</th><th class="px-4 py-3">จัดการ</th></tr></thead>
            <tbody>
              {#each visibleUsers as u (u.identityRef)}
                <tr class="border-t border-border">
                  <td class="px-4 py-3">{u.email || "(รอเข้าสู่ระบบครั้งแรก)"}</td>
                  <td class="px-4 py-3">{ROLE_NAMES[u.role]}</td>
                  <td class="px-4 py-3">{dateText(u.lastSeen)}</td>
                  <td class="px-4 py-3">
                    {#if mayEditRole && u.identityRef!==viewer.identityRef && !u.disabled}
                      <Button variant="outline" size="sm" onclick={()=>void openRole(u)}>เปลี่ยนบทบาท</Button>
                    {:else}<span class="text-xs text-muted-foreground">{u.identityRef===viewer.identityRef?"บัญชีของคุณ":"อ่านอย่างเดียว"}</span>{/if}
                  </td>
                </tr>
              {:else}
                <tr><td colspan="4" class="px-4 py-8 text-center text-muted-foreground">ไม่มีข้อมูลบัญชีที่ตรงกับการค้นหา</td></tr>
              {/each}
            </tbody>
          </table>
        </div>
      </section>
    {:else if mayEditRole}
      <section aria-label="บันทึกการจัดการระบบ" class="mt-6 space-y-4">
        <div><h3 class="text-lg font-semibold">Audit Log</h3><p class="mt-1 text-sm text-muted-foreground">เฉพาะผู้ดูแลสูงสุด · ไม่มีรหัสผ่านและรายละเอียดไฟล์ PDF</p></div>
        <ul class="max-h-[460px] divide-y divide-border overflow-y-auto">
          {#each events as event,i (i)}
            <li class="flex flex-col gap-1 py-3 sm:flex-row sm:items-center sm:justify-between">
              <strong class="text-sm font-semibold">{event.event}</strong>
              <span class="text-xs text-muted-foreground">{dateText(event.at)} · {event.actor.slice(0,12)}…</span>
            </li>
          {:else}<li class="py-8 text-center text-sm text-muted-foreground">ยังไม่มีเหตุการณ์ที่บันทึกไว้</li>{/each}
        </ul>
      </section>
    {/if}
  </Card>
</section>

<dialog bind:this={dialog} onclose={()=>{if(!applying)pending=null;}} aria-labelledby="adminConfirmTitle" class="m-auto w-[min(94vw,460px)] max-h-[85vh] overflow-y-auto rounded-2xl border border-border bg-card p-0 text-foreground shadow-2xl backdrop:bg-black/45">
  {#if pending}
    <div class="space-y-5 p-6">
      <div class="flex items-center justify-between">
        <h3 id="adminConfirmTitle" class="text-lg font-bold">{pending.kind==="cancel"?"ยืนยันการระงับงาน":"ยืนยันการเปลี่ยนบทบาท"}</h3>
        <Button variant="ghost" size="icon" disabled={applying} aria-label="ปิด" onclick={closeDialog}><X size={18} /></Button>
      </div>
      {#if pending.kind==="cancel"}
        <p class="text-sm leading-relaxed text-muted-foreground">ต้องการระงับงาน <code>{pending.job.id.slice(0,12)}</code> ใช่หรือไม่? การดำเนินการจะถูกบันทึกใน Audit</p>
        <label class="block space-y-2 text-sm"><span class="font-semibold">สาเหตุ</span>
          <select bind:value={reason} class="min-h-11 w-full rounded-xl border border-input bg-background px-3">
            {#each Object.entries(CANCEL_REASONS) as [value,title]}<option value={value}>{title}</option>{/each}
          </select>
        </label>
      {:else}
        <p class="text-sm leading-relaxed text-muted-foreground">เปลี่ยนบทบาทของ <strong>{pending.user.email || "บัญชีนี้"}</strong> (ไม่สามารถเปลี่ยนบทบาทบัญชีของตนเอง)</p>
        <label class="block space-y-2 text-sm"><span class="font-semibold">บทบาทใหม่</span>
          <select bind:value={roleChoice} class="min-h-11 w-full rounded-xl border border-input bg-background px-3">
            {#each Object.entries(ROLE_NAMES) as [value,title]}<option value={value}>{title}</option>{/each}
          </select>
        </label>
      {/if}
      <div class="flex justify-end gap-3">
        <Button variant="outline" disabled={applying} onclick={closeDialog}>กลับ</Button>
        <Button variant={pending.kind==="cancel"?"destructive":"default"} disabled={applying || (pending.kind==="role" && pending.user.role===roleChoice)} onclick={()=>void confirmAction()}>
          {applying?"กำลังดำเนินการ...":"ยืนยัน"}
        </Button>
      </div>
      {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
    </div>
  {/if}
</dialog>
<style>
  dialog::backdrop { background: rgb(0 0 0 / 45%); }
</style>
