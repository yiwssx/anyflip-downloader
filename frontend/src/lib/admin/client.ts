import { ApiFailure } from "../api/client";
import { parseAudit, parseJobs, parseSummary, parseUsers, type AdminAudit, type AdminJob, type AdminSummary, type AdminUser, type CancelReason, type RBACRole } from "./contracts";

type AdminConfig = { onDenied: (message:string)=>void; fetchImpl?:typeof fetch };
const JOB_LIMIT = 20;

export class AdminClient {
  private readonly requestImpl:typeof fetch;
  constructor(private readonly config:AdminConfig) {
    this.requestImpl=config.fetchImpl??fetch.bind(globalThis);
  }

  private async request(path:string,options:RequestInit={}):Promise<unknown> {
    // Admin endpoints are server-controlled. These client guards prevent
    // unintentional calls to unrelated origins; only Go enforces permissions.
    if(!path.startsWith("/api/admin/") || path.startsWith("//") ||
      !(
        path==="/api/admin/summary" || path==="/api/admin/users" ||
        path==="/api/admin/audit?limit=50" ||
        /^\/api\/admin\/jobs\?limit=20&offset=\d+$/.test(path) ||
        /^\/api\/admin\/jobs\/[a-f0-9]{32}\/cancel$/.test(path) ||
        /^\/api\/admin\/users\/g-[A-Za-z0-9_%-]+\/role$/.test(path)
      ))throw new Error("Admin API path is not permitted");
    let response:Response;
    try {
      response=await this.requestImpl(path,{...options,credentials:"same-origin",cache:"no-store"});
    }catch(e) {
      if(e instanceof Error && e.name==="AbortError")throw e;
      throw new ApiFailure("เชื่อมต่อ Admin API ไม่สำเร็จ");
    }
    if(!response.ok){
      if(response.status===401 || response.status===403){
        const reason="สิทธิ์ของบัญชีนี้ไม่อนุญาตให้ดำเนินการดังกล่าว";
        this.config.onDenied(reason);
        throw new ApiFailure(reason,response.status,true);
      }
      if(response.status===409 || response.status===428)
        throw new ApiFailure("รายการนี้ถูกเปลี่ยนแปลงแล้ว กรุณารีเฟรชก่อนดำเนินการอีกครั้ง",response.status);
      if(response.status===429)
        throw new ApiFailure("ดำเนินการบ่อยเกินไป กรุณาลองอีกครั้งภายหลัง",response.status);
      throw new ApiFailure("คำขอไม่สำเร็จ (HTTP "+response.status+")",response.status);
    }
    return response.json();
  }

  async summary():Promise<AdminSummary> {
    return parseSummary(await this.request("/api/admin/summary"));
  }
  async jobs(page:number):Promise<{jobs:AdminJob[];total:number}> {
    if(!Number.isSafeInteger(page)||page<0||page>100000)throw new Error("หน้าไม่ถูกต้อง");
    return parseJobs(await this.request("/api/admin/jobs?limit="+JOB_LIMIT+"&offset="+(page*JOB_LIMIT)));
  }
  async users():Promise<AdminUser[]> {
    return parseUsers(await this.request("/api/admin/users"));
  }
  async audit():Promise<AdminAudit[]> {
    return parseAudit(await this.request("/api/admin/audit?limit=50"));
  }
  async cancelJob(id:string,reason:CancelReason):Promise<void> {
    if(!/^[a-f0-9]{32}$/.test(id) || !["policy","resources","duplicate","other"].includes(reason))throw new Error("รายละเอียดการระงับงานไม่ถูกต้อง");
    await this.request("/api/admin/jobs/"+id+"/cancel",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({reason})});
  }
  async setRole(identityRef:string,role:RBACRole):Promise<void> {
    if(!/^g-[A-Za-z0-9_-]{1,198}$/.test(identityRef) || !["user","admin","super_admin"].includes(role))throw new Error("ข้อมูลเปลี่ยนบทบาทไม่ถูกต้อง");
    await this.request("/api/admin/users/"+encodeURIComponent(identityRef)+"/role",{
      method:"PUT",headers:{"Content-Type":"application/json"},body:JSON.stringify({role}),
    });
  }
}
