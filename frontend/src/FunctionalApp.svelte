<script lang="ts">
  import { onMount } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { Card } from "$lib/components/ui/card";
  import { JobsClient, ApiFailure } from "$lib/api/client";
  import {
    ACTIVE_STAGES, STAGE_LABEL, TERMINAL_STAGES,
    type AuthMode, type Job, type SessionUser,
  } from "$lib/api/contracts";
  import { WorkspaceModel } from "$lib/jobs/workspace.svelte";
  import AdminPanel from "$lib/admin/AdminPanel.svelte";
  import {
    ArrowDownToLine, CheckCircle2, ChevronRight, Clock3,
    FileText, History, Info, LayoutDashboard, LogOut,
    Moon, PanelLeftClose, PanelLeftOpen, RefreshCw,
    Settings2, ShieldCheck, Sun, XCircle, ShieldCog,
  } from "@lucide/svelte";

  type View = "overview" | "download" | "history" | "appearance" | "admin";
  const mode: AuthMode = document.documentElement.dataset.authMode === "google" ? "google" : "token";
  const sections: {key: View;title:string}[] = [
    {key:"overview", title:"ภาพรวม"},
    {key:"download", title:"ดาวน์โหลด"},
    {key:"history", title:"ประวัติงาน"},
    {key:"appearance", title:"การแสดงผล"},
  ];
  let view = $state<View>("overview");
  let isDark = $state(false);
  let sidebarCollapsed = $state(false);
  let checking = $state(mode === "google");
  let user = $state<SessionUser | null>(null);
  let authError = $state("");
  let token = $state("");
  let bookURL = $state("");
  let bookTitle = $state("");
  let threads = $state(3);
  let bestQuality = $state(false);
  let batchURLs = $state("");
  let historySearch = $state("");
  let historyFilter = $state("all");
  const navSections:{key:View;title:string}[] = $derived(
    mode==="google" && (user?.role==="admin"||user?.role==="super_admin")
      ? [...sections, {key:"admin",title:"ผู้ดูแล"}]
      : sections
  );

  // Scrub identity-specific form state as well as queued/history data.
  // A different Workspace account must never inherit the previous user's URL.
  function scrubOwnerInputs():void {
    bookURL = "";
    bookTitle = "";
    batchURLs = "";
    historySearch = "";
    historyFilter = "all";
    threads = 3;
    bestQuality = false;
  }
  const api = new JobsClient({
    mode,
    getToken: () => token,
    onAuthExpired: (reason) => {
      authError = reason;
      user = null;
      scrubOwnerInputs();
      workspace.stop();
    },
  });
  const workspace = new WorkspaceModel(api);
  const tokenReady = $derived(token.trim().length >= 16);
  const authorized = $derived(mode === "token" ? tokenReady : user !== null);
  const batchLines = $derived(batchURLs.split(/\r?\n/).map(x => x.trim()).filter(Boolean));
  const filteredJobs = $derived(workspace.jobs.filter((job) =>
    job.id.toLowerCase().includes(historySearch.trim().toLowerCase()) &&
    (historyFilter === "all" ||
      (historyFilter === "active" && ACTIVE_STAGES.includes(job.stage)) ||
      job.stage === historyFilter)
  ));
  const selectedTerminal = $derived(workspace.selectedJob
    ? TERMINAL_STAGES.includes(workspace.selectedJob.stage)
    : false);

  onMount(() => {
    if(mode === "google") {
      try { sessionStorage.removeItem("anyflip-access-token"); } catch { /* storage may be unavailable */ }
      void checkSession();
    } else {
      try { token = sessionStorage.getItem("anyflip-access-token") ?? ""; } catch {}
      if(token.trim().length >= 16) void workspace.loadHistory();
    }
    return () => workspace.stop();
  });

  async function checkSession():Promise<void> {
    checking = true;
    authError = "";
    try {
      const session = await api.getSession();
      if (!session) scrubOwnerInputs();
      user = session;
      if(user) await workspace.loadHistory();
    }catch(err) {
      user = null;
      authError = err instanceof Error ? err.message : "ไม่สามารถตรวจสอบการเข้าสู่ระบบได้";
    }finally {
      checking = false;
    }
  }
  function persistToken():boolean {
    if(token.trim().length < 16){
      authError = "กรุณากรอก Access Token ที่ผู้ดูแลระบบกำหนด";
      return false;
    }
    try { sessionStorage.setItem("anyflip-access-token", token.trim()); } catch {}
    authError = "";
    return true;
  }
  function submitToken():void {
    if(!persistToken())return;
    workspace.stop();
    void workspace.loadHistory();
  }
  async function logout():Promise<void> {
    try {
      await api.logout();
      user = null;
      authError = "";
      scrubOwnerInputs();
      workspace.stop();
    }catch(err) {
      authError = err instanceof Error ? err.message : "ออกจากระบบไม่สำเร็จ";
    }
  }
  function toggleTheme():void {
    isDark = !isDark;
    document.documentElement.classList.toggle("dark", isDark);
  }
  function onAdminDenied(reason:string):void {
    authError = reason;
    user = null;
    scrubOwnerInputs();
    workspace.stop();
    view = "overview";
  }
  function selectView(next:View):void {
    if(next==="admin" && !(mode==="google" && (user?.role==="admin"||user?.role==="super_admin")))return;
    view = next;
    if(next === "history" && authorized) void workspace.loadHistory();
  }
  async function createSingle():Promise<void> {
    if(!authorized || (mode==="token" && !persistToken()))return;
    await workspace.createJob({
      url:bookURL.trim(),
      title:bookTitle.trim(),
      threads,
      bestQuality,
    });
  }
  async function createBatch():Promise<void> {
    if(!authorized || (mode==="token" && !persistToken()))return;
    await workspace.createBatch(batchLines.map(url=>({url,threads,bestQuality})));
  }
  function viewJob(job:Job):void {
    workspace.selectJob(job.id);
    view = "download";
  }
  async function retryJob(job:Job):Promise<void> {
    await workspace.retryJob(job.id);
    if(workspace.selectedId && workspace.selectedId !== job.id) view = "download";
  }
</script>

<svelte:head>
  <title>RCAT AnyFlip · Download Workspace</title>
  <meta name="theme-color" content={isDark ? "#1c1c1e" : "#f5f5f7"} />
</svelte:head>

<div class="min-h-svh bg-background text-foreground">
  <a href="#main" class="sr-only rounded-lg bg-primary px-4 py-3 text-primary-foreground focus:not-sr-only focus:absolute focus:left-3 focus:top-3 focus:z-50">ข้ามไปยังเนื้อหาหลัก</a>

  {#if mode === "google" && !user}
    <main id="main" class="mx-auto grid min-h-svh max-w-5xl place-items-center px-5 py-12">
      <Card class="grid w-full overflow-hidden border-none md:grid-cols-[1fr_1.1fr]">
        <div class="space-y-5 bg-linear-to-br from-primary to-[oklch(0.43_0.15_258)] p-8 text-primary-foreground sm:p-12">
          <div class="grid size-16 place-items-center rounded-2xl bg-white/15"><FileText size={32} aria-hidden="true" /></div>
          <p class="text-xs font-bold uppercase tracking-widest text-white/80">RCAT DIGITAL SERVICES</p>
          <h1 class="text-3xl font-bold tracking-tight">AnyFlip Downloader</h1>
          <p class="max-w-md leading-relaxed text-white/85">ดาวน์โหลดและติดตามเอกสารที่ได้รับอนุญาต ด้วยพื้นที่ทำงานส่วนตัวของวิทยาลัย</p>
          <p class="text-sm text-white/85">สร้าง Image-based PDF ไม่ใช่ไฟล์ PDF ต้นฉบับ</p>
        </div>
        <div class="flex flex-col justify-center space-y-5 p-8 sm:p-12">
          <p class="text-xs font-bold tracking-widest text-primary">GOOGLE WORKSPACE</p>
          <h2 class="text-3xl font-bold tracking-tight">ยินดีต้อนรับ</h2>
          <p class="text-sm leading-relaxed text-muted-foreground">ใช้บัญชี Google Workspace ของวิทยาลัยเพื่อเข้าสู่ระบบ</p>
          {#if checking}
            <p role="status" class="text-sm text-muted-foreground">กำลังตรวจสอบสถานะบัญชี...</p>
          {:else}
            <a href="/auth/google" class="inline-flex min-h-12 items-center justify-center rounded-xl bg-primary px-5 text-base font-semibold text-primary-foreground hover:opacity-90">เข้าสู่ระบบด้วย Google <ChevronRight size={18} class="ml-2" aria-hidden="true" /></a>
          {/if}
          {#if authError}<p role="alert" class="rounded-xl bg-secondary p-3 text-sm text-destructive">{authError}</p>{/if}
          <div class="flex items-start gap-2 rounded-xl bg-secondary p-4 text-sm text-muted-foreground"><ShieldCheck size={19} class="shrink-0 text-primary" aria-hidden="true" /> อนุญาตเฉพาะบัญชี Google Workspace ที่ผ่านการตรวจสอบโดยเซิร์ฟเวอร์ (@rcat.ac.th)</div>
        </div>
      </Card>
    </main>
  {:else}
    <div class="mx-auto flex min-h-svh w-full max-w-[1600px]">
      <aside data-collapsed={sidebarCollapsed} aria-label="เมนูหลัก" class="sticky top-0 hidden h-svh shrink-0 flex-col border-r border-border/75 bg-card/80 px-4 py-6 md:flex">
        <div class="space-y-3 pb-6">
          <div class={sidebarCollapsed ? "flex justify-center" : "flex items-center gap-3 px-2"}>
            <div class="grid size-11 shrink-0 place-items-center rounded-2xl bg-primary text-primary-foreground" aria-hidden="true"><FileText size={24} /></div>
            {#if !sidebarCollapsed}<div><p class="text-base font-bold">RCAT AnyFlip</p><p class="text-xs text-muted-foreground">DOWNLOAD WORKSPACE</p></div>{/if}
          </div>
          <button type="button" aria-expanded={!sidebarCollapsed} aria-label={sidebarCollapsed ? "ขยายเมนู" : "ยุบเมนู"} class="flex min-h-11 w-full items-center justify-center gap-2 rounded-xl border border-border bg-secondary/60 px-2 text-sm font-medium hover:bg-accent" onclick={()=>sidebarCollapsed=!sidebarCollapsed}>
            {#if sidebarCollapsed}<PanelLeftOpen size={18} aria-hidden="true" />{:else}<PanelLeftClose size={18} aria-hidden="true" /> ยุบเมนู{/if}
          </button>
        </div>
        <nav aria-label="หน้าในระบบ" class="grid gap-1.5">
          {#each navSections as item (item.key)}
            <button type="button" aria-current={view===item.key ? "page" : undefined} aria-label={sidebarCollapsed ? item.title : undefined} class={`flex min-h-12 items-center rounded-xl text-left text-sm transition-colors hover:bg-accent ${view===item.key ? "bg-accent font-semibold text-accent-foreground" : "text-muted-foreground"} ${sidebarCollapsed ? "justify-center px-2" : "gap-3 px-4"}`} onclick={()=>selectView(item.key)}>
              {#if item.key==="overview"}<LayoutDashboard size={19} aria-hidden="true" />
              {:else if item.key==="download"}<ArrowDownToLine size={19} aria-hidden="true" />
              {:else if item.key==="history"}<History size={19} aria-hidden="true" />
              {:else}<Settings2 size={19} aria-hidden="true" />{/if}
              {#if !sidebarCollapsed}{item.title}{/if}
            </button>
          {/each}
        </nav>
        {#if !sidebarCollapsed}
          <div class="mt-auto rounded-2xl border border-border bg-background p-4 text-xs leading-relaxed text-muted-foreground">
            <ShieldCheck size={22} class="mb-2 text-primary" aria-hidden="true" />
            <strong class="block text-sm text-foreground">ใช้อย่างถูกสิทธิ์</strong>
            ใช้กับเอกสารที่ได้รับอนุญาตเท่านั้น ไม่ข้าม DRM หรือข้อจำกัดของผู้เผยแพร่
          </div>
        {/if}
      </aside>

      <div class="min-w-0 flex-1 pb-28 md:pb-8">
        <header class="sticky top-0 z-20 flex min-h-[76px] items-center justify-between gap-4 border-b border-border/70 bg-background/95 px-5 backdrop-blur-xl sm:px-8 lg:px-12">
          <div class="min-w-0">
            <p class="text-xs font-bold tracking-widest text-muted-foreground">RCAT DIGITAL SERVICES</p>
            <h1 class="truncate text-lg font-bold tracking-tight sm:text-xl">AnyFlip Downloader</h1>
          </div>
          <div class="flex items-center gap-2">
            {#if user}<span class="hidden max-w-48 truncate text-xs text-muted-foreground lg:inline">{user.email}</span>{/if}
            {#if user}<Button variant="ghost" size="icon" aria-label="ออกจากระบบ" onclick={()=>void logout()}><LogOut size={19} aria-hidden="true" /></Button>{/if}
            <Button variant="outline" size="icon" aria-label={isDark ? "เปลี่ยนเป็นธีมสว่าง" : "เปลี่ยนเป็นธีมมืด"} onclick={toggleTheme}>{#if isDark}<Sun size={19} aria-hidden="true" />{:else}<Moon size={19} aria-hidden="true" />{/if}</Button>
          </div>
        </header>

        <main id="main" tabindex="-1" class="mx-auto w-full max-w-[1200px] space-y-6 px-5 py-7 sm:px-8 lg:px-12 lg:py-9">
          <div><p class="mb-1 text-sm font-medium text-muted-foreground">พื้นที่ทำงาน</p><h2 class="text-2xl font-bold tracking-tight sm:text-3xl">{navSections.find(s=>s.key===view)?.title}</h2></div>

          {#if mode==="token"}
            <Card class="space-y-3 p-5 sm:p-6">
              <label for="token" class="block text-sm font-semibold">Access Token สำหรับระบบ LAN เดิม</label>
              <div class="flex flex-col gap-3 sm:flex-row">
                <input id="token" type="password" autocomplete="off" spellcheck="false" bind:value={token} placeholder="กรอกรหัสจากผู้ดูแลระบบ" class="min-h-12 min-w-0 flex-1 rounded-xl border border-input bg-background px-4 text-sm" />
                <Button onclick={submitToken} disabled={!tokenReady}>เชื่อมต่อ</Button>
              </div>
              <p class="text-xs text-muted-foreground">เก็บเฉพาะ Session ของเบราว์เซอร์ ไม่ใช้บัญชี Google ในโหมดนี้</p>
              {#if authError}<p role="alert" class="text-sm text-destructive">{authError}</p>{/if}
            </Card>
          {/if}

          {#if view==="overview"}
            <Card class="relative overflow-hidden border-none bg-linear-to-br from-primary to-[oklch(0.45_0.17_258)] p-6 text-primary-foreground shadow-lg sm:p-8">
              <p class="mb-3 text-xs font-bold uppercase tracking-widest text-white/80">RCAT DOWNLOAD CENTER</p>
              <h3 class="text-2xl font-bold tracking-tight sm:text-3xl">จัดการเอกสาร ได้ง่ายในที่เดียว</h3>
              <p class="mt-3 text-sm leading-relaxed text-white/85">ติดตามงานและดาวน์โหลดไฟล์ PDF ในพื้นที่ทำงานเดียว</p>
              <div class="mt-6"><Button variant="secondary" onclick={()=>selectView("download")}>สร้างงานดาวน์โหลด <ChevronRight size={18} aria-hidden="true" /></Button></div>
            </Card>
            <section aria-label="สรุปงานดาวน์โหลด" class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
              <Card class="flex items-center gap-4 p-5"><FileText size={25} class="text-primary" aria-hidden="true" /><div><p class="text-sm text-muted-foreground">งานทั้งหมด</p><p class="text-3xl font-bold">{authorized ? workspace.total : "—"}</p></div></Card>
              <Card class="flex items-center gap-4 p-5"><Clock3 size={25} class="text-primary" aria-hidden="true" /><div><p class="text-sm text-muted-foreground">กำลังทำงาน</p><p class="text-3xl font-bold">{authorized ? workspace.active : "—"}</p></div></Card>
              <Card class="flex items-center gap-4 p-5"><CheckCircle2 size={25} class="text-success" aria-hidden="true" /><div><p class="text-sm text-muted-foreground">เสร็จสมบูรณ์</p><p class="text-3xl font-bold">{authorized ? workspace.completed : "—"}</p></div></Card>
            </section>
            <Card class="space-y-4 p-6">
              <div class="flex items-center justify-between gap-3"><h3 class="text-lg font-semibold">งานล่าสุด</h3><Button variant="ghost" onclick={()=>selectView("history")}>ดูประวัติ <ChevronRight size={17} aria-hidden="true" /></Button></div>
              {#if workspace.jobs.length}
                {#each workspace.jobs.slice(0,3) as job (job.id)}
                  <div class="flex items-center justify-between gap-3 border-t border-border pt-3">
                    <span class="min-w-0 truncate font-mono text-xs text-muted-foreground">{job.id.slice(0,8)}</span>
                    <span class="text-sm">{STAGE_LABEL[job.stage]}</span>
                    <Button size="sm" variant="outline" onclick={()=>viewJob(job)}>ดูสถานะ</Button>
                  </div>
                {/each}
              {:else}
                <p class="rounded-xl bg-secondary p-5 text-sm text-muted-foreground">{authorized ? "ยังไม่มีประวัติงาน" : "เข้าสู่ระบบเพื่อดูข้อมูลการดาวน์โหลด"}</p>
              {/if}
            </Card>
          {:else if view==="download"}
            <Card class="max-w-3xl space-y-5 p-6 sm:p-8">
              <div><h3 class="text-xl font-semibold">สร้างงานดาวน์โหลด</h3><p class="mt-2 text-sm text-muted-foreground">ระบุลิงก์เอกสาร AnyFlip ที่ได้รับอนุญาต</p></div>
              <form class="space-y-4" onsubmit={(event)=>{event.preventDefault();void createSingle();}}>
                <div class="space-y-2"><label for="bookURL" class="block text-sm font-semibold">ลิงก์หนังสือ AnyFlip</label><input id="bookURL" type="url" required autocomplete="url" spellcheck="false" bind:value={bookURL} placeholder="https://anyflip.com/publisher/book" disabled={!authorized} class="min-h-12 w-full rounded-xl border border-input bg-background px-4 text-sm disabled:opacity-60" /></div>
                <div class="grid gap-4 sm:grid-cols-[1fr_170px]">
                  <div class="space-y-2"><label for="bookTitle" class="block text-sm font-semibold">ชื่อไฟล์ PDF (ไม่บังคับ)</label><input id="bookTitle" maxlength="110" bind:value={bookTitle} disabled={!authorized} class="min-h-12 w-full rounded-xl border border-input bg-background px-4 text-sm disabled:opacity-60" /></div>
                  <div class="space-y-2"><label for="threads" class="block text-sm font-semibold">ดาวน์โหลดพร้อมกัน</label><select id="threads" bind:value={threads} disabled={!authorized} class="min-h-12 w-full rounded-xl border border-input bg-background px-3 text-sm disabled:opacity-60"><option value={1}>1 หน้า</option><option value={2}>2 หน้า</option><option value={3}>3 หน้า</option><option value={4}>4 หน้า</option><option value={6}>6 หน้า</option><option value={8}>8 หน้า</option></select></div>
                </div>
                <label class="flex min-h-12 cursor-pointer items-center gap-3 rounded-xl bg-secondary px-4 py-3"><input type="checkbox" bind:checked={bestQuality} disabled={!authorized} class="size-5 accent-primary" /><span class="text-sm">ใช้ภาพความละเอียดสูงสุด (อาจใช้เวลาเพิ่ม)</span></label>
                <Button type="submit" disabled={!authorized || workspace.creating || workspace.polling || !bookURL.trim()}>{workspace.creating ? "กำลังส่งงาน..." : "เริ่มดาวน์โหลด"} <ArrowDownToLine size={17} aria-hidden="true" /></Button>
                {#if workspace.formError}<p role="alert" class="text-sm text-destructive">{workspace.formError}</p>{/if}
              </form>
            </Card>

            {#if workspace.selectedId}
              <Card class="max-w-3xl space-y-4 p-6 sm:p-8">
                <div class="flex flex-wrap items-center justify-between gap-3"><h3 class="text-xl font-semibold">สถานะการดาวน์โหลด</h3><span class="rounded-full bg-secondary px-3 py-1.5 text-sm font-medium">{workspace.selectedJob ? STAGE_LABEL[workspace.selectedJob.stage] : "กำลังตรวจสอบ..."}</span></div>
                <p class="break-all font-mono text-xs text-muted-foreground">Job ID: {workspace.selectedId}</p>
                <div role="status" aria-live="polite" class="text-sm text-muted-foreground">{workspace.selectedJob ? (selectedTerminal ? "งานสิ้นสุดสถานะแล้ว" : "ระบบกำลังประมวลผล: "+STAGE_LABEL[workspace.selectedJob.stage]) : "กำลังโหลดสถานะงาน"}</div>
                <progress max="100" value={workspace.progress} aria-label="ความคืบหน้าการดาวน์โหลด" class="h-3 w-full overflow-hidden rounded-full accent-primary"></progress>
                <div class="flex items-center justify-between text-sm text-muted-foreground"><span>{workspace.selectedJob?.completed ?? 0} / {workspace.selectedJob?.total ?? 0} หน้า</span><strong class="text-foreground">{workspace.progress}%</strong></div>
                <div class="flex flex-wrap gap-3">
                  {#if workspace.selectedJob && !selectedTerminal && !workspace.selectedJob.archived}
                    <Button variant="destructive" disabled={workspace.cancelling} onclick={()=>void workspace.cancelSelected()}><XCircle size={17} aria-hidden="true" /> ยกเลิกงาน</Button>
                  {/if}
                  {#if workspace.selectedJob?.downloadReady && !workspace.selectedJob.archived}
                    <Button disabled={workspace.downloading} onclick={()=>void workspace.downloadSelected()}><ArrowDownToLine size={17} aria-hidden="true" /> ดาวน์โหลด Image-based PDF</Button>
                  {/if}
                </div>
                {#if workspace.jobError}<p role="alert" class="text-sm text-destructive">{workspace.jobError}</p>{/if}
              </Card>
            {/if}

            <details class="max-w-3xl overflow-hidden rounded-2xl border border-border bg-card">
              <summary class="flex min-h-14 cursor-pointer items-center gap-3 p-5 font-semibold">ดาวน์โหลดหลายรายการ <span class="ml-auto text-xs font-normal text-muted-foreground">เปิด / ปิดส่วนนี้</span></summary>
              <div class="space-y-4 border-t border-border p-5 sm:p-6">
                <label for="batchURLs" class="block text-sm font-semibold">วาง URL ทีละบรรทัด สูงสุด 8 รายการ</label>
                <textarea id="batchURLs" rows="5" bind:value={batchURLs} disabled={!authorized} placeholder="https://anyflip.com/publisher/book-one&#10;https://anyflip.com/publisher/book-two" class="min-h-32 w-full rounded-xl border border-input bg-background p-4 text-sm disabled:opacity-60"></textarea>
                <div class="flex flex-wrap items-center justify-between gap-3"><span class="text-sm text-muted-foreground">{batchLines.length} / 8 รายการ</span><Button variant="secondary" disabled={!authorized || workspace.batching || batchLines.length<1 || batchLines.length>8} onclick={()=>void createBatch()}>เพิ่มรายการเข้าคิว</Button></div>
                {#if workspace.batchError}<p role="alert" class="text-sm text-destructive">{workspace.batchError}</p>{/if}
              </div>
            </details>
          {:else if view==="history"}
            <Card class="space-y-5 p-6 sm:p-8">
              <div class="flex flex-wrap items-center justify-between gap-3"><div><h3 class="text-xl font-semibold">ประวัติการดาวน์โหลด</h3><p class="mt-1 text-sm text-muted-foreground">แสดงเฉพาะงานที่เซิร์ฟเวอร์อนุญาตให้บัญชีนี้เข้าถึง</p></div><Button variant="secondary" disabled={!authorized || workspace.historyLoading} onclick={()=>void workspace.loadHistory()}><RefreshCw size={17} aria-hidden="true" /> รีเฟรช</Button></div>
              <div class="grid gap-3 sm:grid-cols-[1fr_170px]">
                <label class="space-y-2 text-sm"><span class="font-semibold">ค้นหารหัสงาน</span><input type="search" bind:value={historySearch} placeholder="Job ID" class="min-h-11 w-full rounded-xl border border-input bg-background px-4" /></label>
                <label class="space-y-2 text-sm"><span class="font-semibold">สถานะ</span><select bind:value={historyFilter} class="min-h-11 w-full rounded-xl border border-input bg-background px-3"><option value="all">ทั้งหมด</option><option value="active">กำลังทำงาน</option><option value="completed">สำเร็จ</option><option value="failed">ไม่สำเร็จ</option><option value="cancelled">ยกเลิกแล้ว</option></select></label>
              </div>
              {#if workspace.historyError}<p role="alert" class="text-sm text-destructive">{workspace.historyError}</p>{/if}
              {#if filteredJobs.length}
                <div class="divide-y divide-border">
                  {#each filteredJobs as job (job.id)}
                    <article class="flex flex-wrap items-center justify-between gap-3 py-4">
                      <div class="space-y-1"><p class="font-mono text-sm font-semibold">{job.id.slice(0,8)}</p><p class="text-xs text-muted-foreground">{job.completed} / {job.total} หน้า{job.archived ? " · ประวัติย้อนหลัง" : ""}</p><span class="text-xs font-medium text-primary">{STAGE_LABEL[job.stage]}</span></div>
                      <div class="flex flex-wrap gap-2">
                        <Button variant="outline" size="sm" onclick={()=>viewJob(job)}>{job.archived ? "ดูประวัติ" : "ดูสถานะ"}</Button>
                        {#if !job.archived && (job.stage==="failed" || job.stage==="cancelled")}
                          <Button variant="secondary" size="sm" onclick={()=>void retryJob(job)}>ลองใหม่</Button>
                        {/if}
                      </div>
                    </article>
                  {/each}
                </div>
              {:else}
                <div class="grid min-h-48 place-items-center rounded-xl border border-dashed border-border p-6 text-center"><div><History size={28} class="mx-auto mb-3 text-muted-foreground" aria-hidden="true" /><p class="text-sm font-medium">{!authorized ? "เข้าสู่ระบบเพื่อดูประวัติ" : workspace.jobs.length ? "ไม่พบรายการที่ตรงกับการค้นหา" : "ยังไม่มีประวัติงาน"}</p></div></div>
              {/if}
            </Card>
          {:else}
            <Card class="max-w-3xl space-y-5 p-6 sm:p-8">
              <h3 class="text-xl font-semibold">รูปแบบการแสดงผล</h3>
              <p class="text-sm text-muted-foreground">Semantic Colors และระบบตัวอักษรที่อ่านง่ายทั้ง Light และ Dark</p>
              <div class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border p-4"><span class="font-semibold">ธีมแอป</span><Button variant="outline" onclick={toggleTheme}>{#if isDark}<Sun size={18} aria-hidden="true" /> โหมดสว่าง{:else}<Moon size={18} aria-hidden="true" /> โหมดมืด{/if}</Button></div>
            </Card>
          {/if}
          <p class="flex items-start gap-2 text-xs leading-relaxed text-muted-foreground"><Info size={17} class="shrink-0" aria-hidden="true" /> Frontend Candidate SUI-02 · การยืนยันตัวตนและสิทธิ์ดาวน์โหลดเป็นความรับผิดชอบของ Go Backend · ยังไม่ถูกเปิดใช้ใน Production</p>
        </main>
      </div>
    </div>

    <nav aria-label="เมนูหลักสำหรับมือถือ" class="fixed inset-x-0 bottom-0 z-30 grid grid-cols-4 border-t border-border bg-card/95 px-2 pt-2 pb-[max(8px,env(safe-area-inset-bottom))] shadow-[0_-6px_26px_rgba(0,0,0,0.035)] backdrop-blur-xl md:hidden">
      {#each navSections as item (item.key)}
        <button type="button" aria-current={view===item.key ? "page" : undefined} class={`flex min-h-14 flex-col items-center justify-center gap-1 rounded-xl px-1 text-[11px] font-medium ${view===item.key ? "text-primary" : "text-muted-foreground"}`} onclick={()=>selectView(item.key)}>
          {#if item.key==="overview"}<LayoutDashboard size={22} aria-hidden="true" />
          {:else if item.key==="download"}<ArrowDownToLine size={22} aria-hidden="true" />
          {:else if item.key==="history"}<History size={22} aria-hidden="true" />
          {:else}<Settings2 size={22} aria-hidden="true" />{/if}
          {item.title}
        </button>
      {/each}
    </nav>
  {/if}
</div>

<style>
  @media (min-width: 768px) {
    aside[data-collapsed="true"] { flex: 0 0 80px; width:80px; min-width:80px; max-width:80px; overflow-x:clip; }
    aside[data-collapsed="false"] { flex:0 0 252px; width:252px; min-width:252px; max-width:252px; }
  }
</style>
