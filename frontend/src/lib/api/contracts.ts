// Fixed Go API contracts (web.go/web_google.go); UI never decides authorization.
export type AuthMode = "google" | "token";
export type JobStage = "queued" | "preparing" | "downloading" | "converting" | "completed" | "failed" | "cancelled";

export interface SessionUser { email: string; role: "user" | "admin" | "super_admin"; identityRef?: string }
export interface Job {
  id: string;
  stage: JobStage;
  completed: number;
  total: number;
  downloadReady: boolean;
  archived: boolean;
  error: string;
  errorCode: string;
}
export interface CreateJobInput {
  url: string;
  title?: string;
  threads: number;
  bestQuality: boolean;
}
export type BatchJobInput = Omit<CreateJobInput,"title">;

export const STAGE_LABEL: Record<JobStage,string> = {
  queued: "อยู่ในคิว",
  preparing: "เตรียมข้อมูล",
  downloading: "ดาวน์โหลดภาพ",
  converting: "สร้าง PDF",
  completed: "สำเร็จ",
  failed: "ไม่สำเร็จ",
  cancelled: "ยกเลิกแล้ว",
};
export const ACTIVE_STAGES: JobStage[] = ["queued","preparing","downloading","converting"];
export const TERMINAL_STAGES: JobStage[] = ["completed","failed","cancelled"];

export function isJobId(value: unknown): value is string {
  return typeof value === "string" && /^[a-f0-9]{32}$/.test(value);
}
export function requireJobId(data: unknown): string {
  if (!data || typeof data !== "object" || !isJobId((data as {id?:unknown}).id)) {
    throw new Error("เซิร์ฟเวอร์ส่งรหัสงานไม่ถูกต้อง");
  }
  return (data as {id:string}).id;
}
function count(value: unknown): number {
  if (!Number.isSafeInteger(value) || (value as number) < 0) throw new Error("ข้อมูลจำนวนหน้าไม่ถูกต้อง");
  return value as number;
}
export function parseJob(data: unknown): Job {
  if (!data || typeof data !== "object") throw new Error("ข้อมูลสถานะงานไม่ถูกต้อง");
  const raw = data as Record<string,unknown>;
  if (!isJobId(raw.id) || !(typeof raw.stage === "string" && Object.prototype.hasOwnProperty.call(STAGE_LABEL,raw.stage))) {
    throw new Error("ข้อมูลสถานะงานไม่ถูกต้อง");
  }
  return {
    id: raw.id,
    stage: raw.stage as JobStage,
    completed: count(raw.completed),
    total: count(raw.total),
    downloadReady: raw.downloadReady === true,
    archived: raw.archived === true,
    error: typeof raw.error === "string" ? raw.error.slice(0,500) : "",
    errorCode: typeof raw.errorCode === "string" ? raw.errorCode.slice(0,64) : "",
  };
}
export function parseHistory(data: unknown): Job[] {
  if (!data || typeof data !== "object" || !Array.isArray((data as {jobs?:unknown}).jobs)) {
    throw new Error("ข้อมูลประวัติงานไม่ถูกต้อง");
  }
  return (data as {jobs:unknown[]}).jobs.map(parseJob);
}
export function parseUser(data: unknown): SessionUser {
  if (!data || typeof data !== "object") throw new Error("ข้อมูลบัญชีไม่ถูกต้อง");
  const raw=data as Record<string,unknown>;
  if (typeof raw.email !== "string" || !["user","admin","super_admin"].includes(String(raw.role))) {
    throw new Error("ข้อมูลบัญชีไม่ถูกต้อง");
  }
  return {email:raw.email,role:raw.role as SessionUser["role"]};
}
