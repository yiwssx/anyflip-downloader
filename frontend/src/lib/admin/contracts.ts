import { isJobId, type JobStage, STAGE_LABEL, type SessionUser } from "../api/contracts";

export type RBACRole = SessionUser["role"];
export type CancelReason = "policy" | "resources" | "duplicate" | "other";
export const ROLE_NAMES:Record<RBACRole,string> = {
  user:"ผู้ใช้งาน",admin:"ผู้ดูแลระบบ",super_admin:"ผู้ดูแลสูงสุด",
};
export const CANCEL_REASONS:Record<CancelReason,string> = {
  policy:"ไม่สอดคล้องกับนโยบาย",
  resources:"ทรัพยากรไม่เพียงพอ",
  duplicate:"งานซ้ำ",
  other:"เหตุผลอื่นที่อนุมัติ",
};
export interface AdminStorage {
  totalBytes:number;usedBytes:number;availableBytes:number;usedPercent:number;
}
export interface AdminSummary {
  total:number;active:number;queued:number;running:number;completed:number;failed:number;cancelled:number;
  activeReservations:number;maxConcurrent:number;storage:AdminStorage|null;
}
export interface AdminJob {
  id:string;owner:string;stage:JobStage;completed:number;total:number;
  created:string;cancelRequested:boolean;archived:boolean;errorCode:string;
}
export interface AdminUser {
  identityRef:string;email:string;role:RBACRole;disabled:boolean;lastSeen:string;
}
export interface AdminAudit {
  event:string;at:string;actor:string;
}

function object(value:unknown):Record<string,unknown> {
  if(!value || typeof value!=="object" || Array.isArray(value))throw new Error("ข้อมูลผู้ดูแลระบบไม่ถูกต้อง");
  return value as Record<string,unknown>;
}
function integer(value:unknown,label:string):number {
  if(typeof value!=="number"||!Number.isSafeInteger(value)||value<0)throw new Error(label+" ไม่ถูกต้อง");
  return value;
}
function optionalNumber(value:unknown):number|null {
  return typeof value==="number" && Number.isFinite(value) && value>=0 ? value : null;
}
function text(value:unknown,max=512):string {
  return typeof value==="string" ? value.slice(0,max) : "";
}
function validRole(value:unknown):value is RBACRole {
  return value==="user" || value==="admin" || value==="super_admin";
}
export function parseSummary(value:unknown):AdminSummary {
  const o=object(value);
  const storage=o.storage ? object(o.storage) : null;
  const numeric=(field:string)=>integer(o[field],field);
  return {
    total:numeric("total"),active:numeric("active"),queued:numeric("queued"),running:numeric("running"),
    completed:numeric("completed"),failed:numeric("failed"),cancelled:numeric("cancelled"),
    activeReservations:numeric("activeReservations"),maxConcurrent:numeric("maxConcurrent"),
    storage:storage && [storage.totalBytes,storage.usedBytes,storage.availableBytes,storage.usedPercent].every(x=>optionalNumber(x)!==null)
      ? {totalBytes:storage.totalBytes as number,usedBytes:storage.usedBytes as number,availableBytes:storage.availableBytes as number,usedPercent:storage.usedPercent as number} : null,
  };
}
export function parseJobs(value:unknown):{jobs:AdminJob[];total:number} {
  const o=object(value);
  if(!Array.isArray(o.jobs))throw new Error("รายการงานผู้ดูแลระบบไม่ถูกต้อง");
  return {
    total:integer(o.total,"จำนวนงาน"),
    jobs:o.jobs.map(raw=>{
      const j=object(raw);
      if(!isJobId(j.id)||typeof j.stage!=="string"||!Object.prototype.hasOwnProperty.call(STAGE_LABEL,j.stage))throw new Error("ข้อมูลสถานะงานผู้ดูแลระบบไม่ถูกต้อง");
      return {
        id:j.id,owner:text(j.owner,200),stage:j.stage as JobStage,
        completed:integer(j.completed,"จำนวนหน้าที่ดาวน์โหลด"),total:integer(j.total,"จำนวนหน้าทั้งหมด"),
        created:text(j.created,50),cancelRequested:j.cancelRequested===true,
        archived:j.archived===true,errorCode:text(j.errorCode,64),
      };
    }),
  };
}
export function parseUsers(value:unknown):AdminUser[] {
  const o=object(value);
  if(!Array.isArray(o.users))throw new Error("ข้อมูลบัญชีผู้ใช้ไม่ถูกต้อง");
  return o.users.map(raw=>{
    const u=object(raw);
    if(typeof u.identityRef!=="string" || !u.identityRef.startsWith("g-") || u.identityRef.length>200 ||
       !validRole(u.role))throw new Error("ข้อมูลบทบาทบัญชีไม่ถูกต้อง");
    return {identityRef:u.identityRef,email:text(u.email,256),role:u.role,disabled:u.disabled===true,lastSeen:text(u.lastSeen,50)};
  });
}
export function parseAudit(value:unknown):AdminAudit[] {
  const o=object(value);
  if(!Array.isArray(o.events))throw new Error("ข้อมูล Audit ไม่ถูกต้อง");
  return o.events.map(raw=>{
    const a=object(raw);
    return {event:text(a.event,128),at:text(a.at,50),actor:text(a.actor,200)};
  });
}
export function dateText(raw:string):string {
  if(!raw || raw.startsWith("0001-"))return "—";
  const d=new Date(raw);
  return Number.isNaN(d.getTime())?"—":new Intl.DateTimeFormat("th-TH",{dateStyle:"short",timeStyle:"short"}).format(d);
}
export function gib(value:number):string {
  return (value/(1024**3)).toLocaleString("th-TH",{maximumFractionDigits:2});
}
