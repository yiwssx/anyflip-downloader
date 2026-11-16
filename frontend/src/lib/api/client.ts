import { parseHistory, parseJob, parseUser, requireJobId, isJobId, type AuthMode, type SessionUser, type Job, type CreateJobInput, type BatchJobInput } from "./contracts";

export class ApiFailure extends Error {
  constructor(message: string, readonly status: number | null = null, readonly authExpired = false) { super(message); this.name = "ApiFailure"; }
}
type Settings = {
  mode: AuthMode;
  getToken: () => string;
  onAuthExpired: (reason: string) => void;
  fetchImpl?: typeof fetch;
};

export class JobsClient {
  private readonly requestImpl: typeof fetch;
  constructor(private readonly settings: Settings) {
    this.requestImpl = settings.fetchImpl ?? fetch.bind(globalThis);
  }

  private async request(path:string, options:RequestInit = {}, checkAuth = true):Promise<Response> {
    // Internal paths only; all authorization and per-user ownership remain in Go.
    if (!path.startsWith("/") || path.startsWith("//") || !(
      path === "/auth/me" || path === "/auth/logout" ||
      path === "/api/jobs" || path === "/api/jobs/batch" ||
      /^\/api\/jobs\/[a-f0-9]{32}(?:\/(?:cancel|retry|download))?$/.test(path)
    )) throw new Error("API path is not permitted");
    const headers = new Headers(options.headers);
    if (this.settings.mode === "token" && path.startsWith("/api/jobs")) {
      const token = this.settings.getToken().trim();
      if (token.length < 16) throw new ApiFailure("กรุณากรอก Access Token ที่ผู้ดูแลระบบกำหนด");
      headers.set("Authorization", "Bearer " + token);
    }
    let response:Response;
    try {
      response=await this.requestImpl(path,{
        ...options,
        headers,
        credentials:"same-origin",
        cache:"no-store",
      });
    } catch (err) {
      if (err instanceof Error && err.name === "AbortError") throw err;
      throw new ApiFailure("เชื่อมต่อเซิร์ฟเวอร์ไม่สำเร็จ กรุณาตรวจสอบเครือข่าย");
    }
    if (response.ok) return response;
    if (!checkAuth && (response.status === 401 || response.status === 403)) return response;
    if ((response.status === 401 || response.status === 403) && this.settings.mode === "google") {
      const reason = response.status === 403
        ? "บัญชีนี้ไม่มีสิทธิ์ใช้งาน กรุณาติดต่อผู้ดูแลระบบ"
        : "กรุณาเข้าสู่ระบบด้วย Google อีกครั้ง";
      this.settings.onAuthExpired(reason);
      throw new ApiFailure(reason,response.status,true);
    }
    if (response.status === 401 && this.settings.mode === "token") {
      throw new ApiFailure("Access Token ไม่ถูกต้อง",401,true);
    }
    if (response.status === 429) throw new ApiFailure("ขณะนี้คิวงานเต็ม กรุณาลองใหม่อีกครั้ง",429);
    const message=(await response.text()).trim().slice(0,500);
    throw new ApiFailure(message || "เกิดข้อผิดพลาด (HTTP "+response.status+")",response.status);
  }

  async getSession():Promise<SessionUser|null> {
    if(this.settings.mode !== "google") return null;
    const response=await this.request("/auth/me",{},false);
    if(!response.ok) return null;
    return parseUser(await response.json());
  }
  async logout():Promise<void> {
    if(this.settings.mode !== "google") return;
    await this.request("/auth/logout",{method:"POST"});
  }

  async listJobs(signal?:AbortSignal):Promise<Job[]> {
    return parseHistory(await (await this.request("/api/jobs",{signal})).json());
  }
  async getJob(id:string,signal?:AbortSignal):Promise<Job> {
    if(!isJobId(id))throw new Error("รหัสงานไม่ถูกต้อง");
    return parseJob(await (await this.request("/api/jobs/"+id,{signal})).json());
  }
  async createJob(input:CreateJobInput):Promise<string> {
    const response=await this.request("/api/jobs",{
      method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(input),
    });
    return requireJobId(await response.json());
  }
  async createBatch(inputs:BatchJobInput[]):Promise<string[]> {
    const response=await this.request("/api/jobs/batch",{
      method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({jobs:inputs}),
    });
    const data=await response.json();
    if(!data || !Array.isArray(data.jobs) || !data.jobs.length)throw new Error("เซิร์ฟเวอร์ส่งรายการงานไม่ถูกต้อง");
    return data.jobs.map(requireJobId);
  }
  async cancelJob(id:string):Promise<void> {
    if(!isJobId(id))throw new Error("รหัสงานไม่ถูกต้อง");
    await this.request("/api/jobs/"+id+"/cancel",{method:"POST"});
  }
  async retryJob(id:string):Promise<string> {
    if(!isJobId(id))throw new Error("รหัสงานไม่ถูกต้อง");
    return requireJobId(await (await this.request("/api/jobs/"+id+"/retry",{method:"POST"})).json());
  }
  async downloadJob(id:string):Promise<Blob> {
    if(!isJobId(id))throw new Error("รหัสงานไม่ถูกต้อง");
    return (await this.request("/api/jobs/"+id+"/download")).blob();
  }
}
