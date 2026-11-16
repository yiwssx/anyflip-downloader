<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import {
    ArrowDownToLine,
    CheckCircle2,
    ChevronRight,
    Clock3,
    FileText,
    History,
    Info,
    LayoutDashboard,
    PanelLeftClose,
    PanelLeftOpen,
    Moon,
    Settings2,
    ShieldCheck,
    Sun,
  } from "@lucide/svelte";

  type View = "overview" | "download" | "history" | "appearance";
  let view = $state<View>("overview");
  let isDark = $state(false);
  let sidebarCollapsed = $state(false);

  const sections: { key: View; title: string }[] = [
    { key: "overview", title: "ภาพรวม" },
    { key: "download", title: "ดาวน์โหลด" },
    { key: "history", title: "ประวัติงาน" },
    { key: "appearance", title: "การแสดงผล" },
  ];

  function selectView(next: View) {
    view = next;
  }

  function toggleTheme() {
    isDark = !isDark;
    document.documentElement.classList.toggle("dark", isDark);
  }
</script>

<svelte:head>
  <title>RCAT AnyFlip · Design Preview</title>
  <meta name="theme-color" content={isDark ? "#1c1c1e" : "#f5f5f7"} />
</svelte:head>

<div class="min-h-svh bg-background text-foreground">
  <a class="sr-only rounded-lg bg-primary px-4 py-3 text-primary-foreground focus:not-sr-only focus:absolute focus:left-3 focus:top-3 focus:z-50" href="#main">ข้ามไปยังเนื้อหาหลัก</a>

  <div class="mx-auto flex min-h-svh w-full max-w-[1600px]">
    <!-- Desktop navigation; mobile gets a compact bottom nav instead. -->
    <aside class={`sticky top-0 hidden h-svh shrink-0 flex-col border-r border-border/75 bg-card/80 px-4 py-6 md:flex ${sidebarCollapsed ? "w-20" : "w-[252px]"}`} aria-label="เมนูหลัก">
      <div class="space-y-3 pb-6">
        <div class={sidebarCollapsed ? "flex justify-center" : "flex items-center gap-3 px-2"}>
          <div class="grid size-11 shrink-0 place-items-center rounded-2xl bg-primary text-primary-foreground shadow-sm" aria-hidden="true"><FileText size={24} strokeWidth={1.8} /></div>
          {#if !sidebarCollapsed}
            <div class="min-w-0">
              <p class="text-base font-bold leading-tight tracking-tight">RCAT AnyFlip</p>
              <p class="mt-1 text-xs font-medium text-muted-foreground">DOWNLOAD WORKSPACE</p>
            </div>
          {/if}
        </div>
        <button
          type="button"
          aria-expanded={!sidebarCollapsed}
          aria-label={sidebarCollapsed ? "ขยายเมนู" : "ยุบเมนู"}
          class="flex min-h-11 w-full items-center justify-center gap-2 rounded-xl border border-border bg-secondary/60 px-2 text-sm font-medium text-muted-foreground hover:bg-accent hover:text-foreground"
          onclick={() => (sidebarCollapsed = !sidebarCollapsed)}
        >
          {#if sidebarCollapsed}<PanelLeftOpen size={18} aria-hidden="true" />{:else}<PanelLeftClose size={18} aria-hidden="true" /> ยุบเมนู{/if}
        </button>
      </div>
      <nav class="grid gap-1.5" aria-label="หน้าในระบบ">
        {#each sections as item (item.key)}
          <button
            type="button"
            aria-current={view === item.key ? "page" : undefined}
            aria-label={sidebarCollapsed ? item.title : undefined}
            title={sidebarCollapsed ? item.title : undefined}
            class:font-semibold={view === item.key}
            class:bg-accent={view === item.key}
            class:text-accent-foreground={view === item.key}
            class={`flex min-h-12 items-center rounded-xl text-left text-sm text-muted-foreground transition-colors hover:bg-accent/70 hover:text-foreground ${sidebarCollapsed ? "justify-center px-2" : "gap-3 px-4"}`}
            onclick={() => selectView(item.key)}
          >
            {#if item.key === "overview"}<LayoutDashboard size={19} aria-hidden="true" />
            {:else if item.key === "download"}<ArrowDownToLine size={19} aria-hidden="true" />
            {:else if item.key === "history"}<History size={19} aria-hidden="true" />
            {:else}<Settings2 size={19} aria-hidden="true" />{/if}
            {#if !sidebarCollapsed}{item.title}{/if}
          </button>
        {/each}
      </nav>
      {#if !sidebarCollapsed}
      <div class="mt-auto space-y-3">
        <div class="rounded-2xl border border-border bg-background p-4">
          <ShieldCheck size={21} class="mb-2 text-primary" aria-hidden="true" />
          <p class="text-sm font-semibold">สิทธิ์การใช้งาน</p>
          <p class="mt-1 text-xs leading-relaxed text-muted-foreground">ใช้กับเอกสารที่ได้รับอนุญาตเท่านั้น ระบบสร้าง Image-based PDF</p>
        </div>
        <p class="px-2 text-xs text-muted-foreground">Svelte UI · Foundation Preview</p>
      </div>
      {/if}
    </aside>

    <div class="min-w-0 flex-1 pb-28 md:pb-8">
      <header class="sticky top-0 z-20 flex min-h-[76px] items-center justify-between gap-4 border-b border-border/70 bg-background/95 px-5 backdrop-blur-xl sm:px-8 lg:px-12">
        <div class="min-w-0">
          <p class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">RCAT DIGITAL SERVICES</p>
          <h1 class="truncate text-lg font-bold tracking-tight sm:text-xl">AnyFlip Downloader</h1>
        </div>
        <div class="flex items-center gap-2">
          <span class="hidden rounded-full border border-border bg-card px-3 py-1.5 text-xs font-medium text-muted-foreground sm:inline-flex">Design Preview</span>
          <Button variant="outline" size="icon" aria-label={isDark ? "เปลี่ยนเป็นธีมสว่าง" : "เปลี่ยนเป็นธีมมืด"} onclick={toggleTheme}>
            {#if isDark}<Sun size={20} aria-hidden="true" />{:else}<Moon size={20} aria-hidden="true" />{/if}
          </Button>
        </div>
      </header>

      <main id="main" tabindex="-1" class="mx-auto w-full max-w-[1200px] space-y-6 px-5 py-7 sm:px-8 lg:px-12 lg:py-9">
        <div class="flex items-center justify-between gap-3">
          <div>
            <p class="mb-1 text-sm font-medium text-muted-foreground">พื้นที่ทำงาน</p>
            <h2 class="text-2xl font-bold tracking-tight sm:text-[30px]">{sections.find((x) => x.key === view)?.title}</h2>
          </div>
          <span class="rounded-full border border-border bg-card px-3 py-1.5 text-[11px] font-semibold text-muted-foreground">SUI-01 · PREVIEW</span>
        </div>

        {#if view === "overview"}
          <Card class="relative overflow-hidden border-none bg-linear-to-br from-primary to-[oklch(0.45_0.17_258)] p-6 text-primary-foreground shadow-lg sm:p-8">
            <div class="relative z-10 max-w-xl">
              <p class="mb-3 text-xs font-bold uppercase tracking-widest text-white/80">RCAT DOWNLOAD CENTER</p>
              <h3 class="text-2xl font-bold leading-tight tracking-tight sm:text-3xl">จัดการเอกสาร ได้ง่ายในที่เดียว</h3>
              <p class="mt-3 max-w-[46ch] text-sm leading-relaxed text-white/85">ออกแบบใหม่สำหรับหน้าจอทุกขนาด ลดขั้นตอนที่ไม่จำเป็น และเน้นการอ่านที่สบายตา</p>
              <div class="mt-6">
                <Button variant="secondary" onclick={() => selectView("download")}>
                  ดูหน้าดาวน์โหลด <ChevronRight size={17} aria-hidden="true" />
                </Button>
              </div>
            </div>
            <FileText class="absolute -bottom-8 -right-5 rotate-[-13deg] text-white/10" size={196} strokeWidth={0.9} aria-hidden="true" />
          </Card>

          <section aria-label="สถานะงานตัวอย่าง" class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <Card class="flex items-center gap-4 p-5">
              <div class="grid size-12 place-items-center rounded-2xl bg-accent text-primary"><FileText size={23} aria-hidden="true" /></div>
              <div><p class="text-sm text-muted-foreground">งานทั้งหมด</p><p class="text-3xl font-bold tracking-tight">—</p></div>
            </Card>
            <Card class="flex items-center gap-4 p-5">
              <div class="grid size-12 place-items-center rounded-2xl bg-secondary text-muted-foreground"><Clock3 size={23} aria-hidden="true" /></div>
              <div><p class="text-sm text-muted-foreground">กำลังดำเนินการ</p><p class="text-3xl font-bold tracking-tight">—</p></div>
            </Card>
            <Card class="flex items-center gap-4 p-5">
              <div class="grid size-12 place-items-center rounded-2xl bg-accent text-success"><CheckCircle2 size={23} aria-hidden="true" /></div>
              <div><p class="text-sm text-muted-foreground">สำเร็จ</p><p class="text-3xl font-bold tracking-tight">—</p></div>
            </Card>
          </section>

          <Card class="p-6 sm:p-7">
            <div class="flex items-start justify-between gap-4">
              <div>
                <h3 class="text-lg font-semibold">งานล่าสุด</h3>
                <p class="mt-1 text-sm text-muted-foreground">ข้อมูลจริงจะปรากฏหลังเชื่อมต่อ Go API ในระยะ SUI-02</p>
              </div>
              <Button size="sm" variant="ghost" onclick={() => selectView("history")}>ดูประวัติ <ChevronRight size={16} aria-hidden="true" /></Button>
            </div>
            <div class="mt-5 grid min-h-32 place-items-center rounded-xl border border-dashed border-border p-6 text-center">
              <div><History class="mx-auto mb-2 text-muted-foreground" size={26} aria-hidden="true" /><p class="text-sm font-medium">ยังไม่มีข้อมูลสำหรับแสดงผลใน Preview</p></div>
            </div>
          </Card>
        {:else if view === "download"}
          <Card class="max-w-3xl space-y-5 p-6 sm:p-8">
            <div>
              <div class="mb-3 flex size-12 items-center justify-center rounded-2xl bg-accent text-primary"><ArrowDownToLine size={24} aria-hidden="true" /></div>
              <h3 class="text-xl font-semibold">สร้างงานดาวน์โหลด</h3>
              <p class="mt-2 text-sm leading-relaxed text-muted-foreground">ตัวอย่าง Component และฟอร์มเท่านั้น ยังไม่มีการส่งงานไปยัง API</p>
            </div>
            <div class="space-y-2">
              <label class="block text-sm font-semibold" for="preview-url">ลิงก์หนังสือ AnyFlip</label>
              <input id="preview-url" type="url" disabled placeholder="https://anyflip.com/publisher/book" class="min-h-12 w-full rounded-xl border border-input bg-secondary/40 px-4 text-sm text-foreground placeholder:text-muted-foreground disabled:opacity-65" />
            </div>
            <div class="space-y-2">
              <label class="block text-sm font-semibold" for="preview-title">ชื่อไฟล์ PDF (ไม่บังคับ)</label>
              <input id="preview-title" type="text" disabled placeholder="ใช้ชื่อหนังสืออัตโนมัติ" class="min-h-12 w-full rounded-xl border border-input bg-secondary/40 px-4 text-sm text-foreground placeholder:text-muted-foreground disabled:opacity-65" />
            </div>
            <Button disabled>เริ่มดาวน์โหลด <ArrowDownToLine size={17} aria-hidden="true" /></Button>
            <p class="flex items-start gap-2 text-xs leading-relaxed text-muted-foreground"><Info size={17} class="mt-0.5 shrink-0" aria-hidden="true" /> การสร้างงานจริง, Progress, Retry และ Cancel จะเชื่อม Go API หลังผ่านเกณฑ์การย้ายใน SUI-02</p>
          </Card>
        {:else if view === "history"}
          <Card class="max-w-3xl p-6 sm:p-8">
            <h3 class="text-xl font-semibold">ประวัติการดาวน์โหลด</h3>
            <p class="mt-2 text-sm text-muted-foreground">สถานะและข้อมูลจะดึงจาก Go Backend หลังเชื่อม API</p>
            <div class="mt-7 flex min-h-56 flex-col items-center justify-center rounded-2xl border border-dashed border-border p-6 text-center">
              <History size={34} class="mb-3 text-muted-foreground" aria-hidden="true" />
              <p class="font-medium">ยังไม่มีรายการให้แสดง</p>
              <p class="mt-2 max-w-sm text-sm text-muted-foreground">Preview นี้ไม่ดึงข้อมูลส่วนตัวหรือจำลองผลดาวน์โหลดว่าเกิดขึ้นจริง</p>
              <div class="mt-4"><Button variant="secondary" onclick={() => selectView("download")}>ไปหน้าดาวน์โหลด</Button></div>
            </div>
          </Card>
        {:else}
          <Card class="max-w-3xl space-y-6 p-6 sm:p-8">
            <div><h3 class="text-xl font-semibold">รูปแบบการแสดงผล</h3><p class="mt-2 text-sm text-muted-foreground">Light และ Dark ใช้ Semantic Tokens ชุดเดียวกัน</p></div>
            <div class="flex flex-wrap items-center justify-between gap-4 rounded-xl border border-border p-4">
              <div><p class="font-semibold">ธีมของแอป</p><p class="mt-1 text-sm text-muted-foreground">เปลี่ยนได้ทันทีใน Preview</p></div>
              <Button variant="outline" onclick={toggleTheme}>
                {#if isDark}<Sun size={18} aria-hidden="true" /> โหมดสว่าง{:else}<Moon size={18} aria-hidden="true" /> โหมดมืด{/if}
              </Button>
            </div>
            <p class="text-sm leading-relaxed text-muted-foreground">ตัวอักษรและปุ่มรองรับคีย์บอร์ดและ Reduced Motion ส่วนการบันทึก Preferences จะจัดทำเมื่อเชื่อมกับ Workspace จริง</p>
          </Card>
        {/if}

        <p class="text-xs leading-relaxed text-muted-foreground">เฉพาะตัวอย่าง UI · ยังไม่ได้เชื่อม Google Workspace, RBAC หรือข้อมูลดาวน์โหลดจริง · Production ใช้ระบบเดิม</p>
      </main>
    </div>
  </div>

  <!-- Compact bottom tabs, respecting phone safe-area. -->
  <nav aria-label="เมนูหลักสำหรับมือถือ" class="fixed inset-x-0 bottom-0 z-30 grid grid-cols-4 border-t border-border bg-card/95 px-2 pt-2 pb-[max(8px,env(safe-area-inset-bottom))] shadow-[0_-6px_26px_rgba(0,0,0,0.035)] backdrop-blur-xl md:hidden">
    {#each sections as item (item.key)}
      <button type="button" class="flex min-h-14 flex-col items-center justify-center gap-1 rounded-xl px-1 text-[11px] font-medium transition-colors" class:text-primary={view === item.key} class:text-muted-foreground={view !== item.key} aria-current={view === item.key ? "page" : undefined} onclick={() => selectView(item.key)}>
        {#if item.key === "overview"}<LayoutDashboard size={22} aria-hidden="true" />
        {:else if item.key === "download"}<ArrowDownToLine size={22} aria-hidden="true" />
        {:else if item.key === "history"}<History size={22} aria-hidden="true" />
        {:else}<Settings2 size={22} aria-hidden="true" />{/if}
        {item.title}
      </button>
    {/each}
  </nav>
</div>
