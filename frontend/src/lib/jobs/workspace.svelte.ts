import { JobsClient, ApiFailure } from "../api/client";
import {
  ACTIVE_STAGES, TERMINAL_STAGES,
  isJobId, type Job, type CreateJobInput, type BatchJobInput,
} from "../api/contracts";

// User-facing state only. Jobs, roles and PDF ownership remain Go-authorized.
export class WorkspaceModel {
  jobs = $state<Job[]>([]);
  selectedJob = $state<Job | null>(null);
  selectedId = $state<string | null>(null);
  creating = $state(false);
  batching = $state(false);
  cancelling = $state(false);
  downloading = $state(false);
  historyLoading = $state(false);
  polling = $state(false);
  formError = $state("");
  batchError = $state("");
  jobError = $state("");
  historyError = $state("");

  private generation = 0;
  private historyVersion = 0;
  private historyAbort: AbortController | null = null;
  private pollAbort: AbortController | null = null;
  private pollTimer: ReturnType<typeof setTimeout> | null = null;
  private failures = 0;

  constructor(private readonly api:JobsClient) {}

  get total():number { return this.jobs.length; }
  get active():number { return this.jobs.filter(j=>ACTIVE_STAGES.includes(j.stage)).length; }
  get completed():number { return this.jobs.filter(j=>j.stage==="completed").length; }
  get progress():number {
    if(!this.selectedJob || this.selectedJob.total<=0)return 0;
    return Math.min(100,Math.round(this.selectedJob.completed*100/this.selectedJob.total));
  }

  private stopPolling():void {
    ++this.generation;
    if(this.pollTimer!==null) clearTimeout(this.pollTimer);
    this.pollTimer=null;
    this.pollAbort?.abort();
    this.pollAbort=null;
    this.polling=false;
  }

  /** Owner-scoped data must be discarded on logout/revocation, including stale responses. */
  stop():void {
    this.stopPolling();
    ++this.historyVersion;
    this.historyAbort?.abort();
    this.historyAbort=null;
    this.jobs=[];
    this.selectedJob=null;
    this.selectedId=null;
    this.creating=false;
    this.batching=false;
    this.cancelling=false;
    this.downloading=false;
    this.historyLoading=false;
    this.formError="";
    this.batchError="";
    this.jobError="";
    this.historyError="";
  }

  async loadHistory():Promise<void> {
    const requestVersion=++this.historyVersion;
    const version=this.generation;
    this.historyAbort?.abort();
    const controller=new AbortController();
    this.historyAbort=controller;
    this.historyLoading=true;
    try {
      const jobs=await this.api.listJobs(controller.signal);
      if(requestVersion!==this.historyVersion || version!==this.generation)return;
      this.jobs=jobs;
      this.historyError="";
    }catch(err) {
      if(controller.signal.aborted || requestVersion!==this.historyVersion || version!==this.generation)return;
      this.historyError=this.describeError(err);
    }finally {
      if(this.historyAbort===controller){this.historyAbort=null;this.historyLoading=false;}
    }
  }

  selectJob(id:string):void {
    if(!isJobId(id)){this.jobError="รหัสงานไม่ถูกต้อง";return;}
    this.stopPolling();
    this.selectedId=id;
    this.selectedJob=null;
    this.failures=0;
    this.jobError="";
    this.polling=true;
    void this.refreshJob(this.generation);
  }

  private async refreshJob(version:number):Promise<void> {
    if(version!==this.generation || !this.selectedId)return;
    const id=this.selectedId;
    const ctrl=new AbortController();
    this.pollAbort=ctrl;
    const timeout=setTimeout(()=>ctrl.abort(),15000);
    let shouldContinue=false;
    try {
      const job=await this.api.getJob(id,ctrl.signal);
      if(version!==this.generation || id!==this.selectedId)return;
      this.selectedJob=job;
      this.jobError=job.error ? job.error+(job.errorCode?" (รหัส: "+job.errorCode+")":""):"";
      this.failures=0;
      if(TERMINAL_STAGES.includes(job.stage)){
        this.stopPolling();
        void this.loadHistory();
      }else shouldContinue=true;
    }catch(err) {
      if(version!==this.generation || id!==this.selectedId)return;
      if(err instanceof ApiFailure && err.authExpired) {
        this.stopPolling();
        this.jobError=err.message;
      } else {
        this.failures++;
        this.jobError=err instanceof Error && err.name==="AbortError"
          ?"การเชื่อมต่อหมดเวลา ระบบจะลองตรวจสถานะอีกครั้ง"
          :this.describeError(err);
        shouldContinue=true;
      }
    }finally {
      clearTimeout(timeout);
      if(this.pollAbort===ctrl)this.pollAbort=null;
      if(shouldContinue && version===this.generation && id===this.selectedId){
        const delay=Math.min(15000,1100*Math.pow(2,Math.min(this.failures,4)));
        this.pollTimer=setTimeout(()=>{this.pollTimer=null;void this.refreshJob(version)},delay);
      }
    }
  }

  async createJob(input:CreateJobInput):Promise<void> {
    if(this.creating || this.polling)return;
    const version=this.generation;
    this.creating=true;
    this.formError="";
    try {
      const id=await this.api.createJob(input);
      if(version!==this.generation)return;
      this.selectJob(id);
      void this.loadHistory();
    }catch(err) {
      if(version===this.generation)this.formError=this.describeError(err);
    }finally {
      if(version===this.generation)this.creating=false;
    }
  }

  async createBatch(inputs:BatchJobInput[]):Promise<void> {
    if(this.batching)return;
    this.batchError="";
    if(inputs.length<1 || inputs.length>8){this.batchError="ต้องมีลิงก์ตั้งแต่ 1 ถึง 8 รายการ";return;}
    const version=this.generation;
    this.batching=true;
    try {
      const ids=await this.api.createBatch(inputs);
      if(version!==this.generation)return;
      this.selectJob(ids[0]);
      void this.loadHistory();
    }catch(err) {
      if(version===this.generation)this.batchError=this.describeError(err);
    }finally {
      if(version===this.generation)this.batching=false;
    }
  }

  async cancelSelected():Promise<void> {
    if(!this.selectedId || this.cancelling || this.selectedJob?.archived)return;
    const id=this.selectedId,version=this.generation;
    this.cancelling=true;
    try {
      await this.api.cancelJob(id);
      if(version!==this.generation)return;
      this.stopPolling();
      this.selectedId=id;
      this.polling=true;
      void this.refreshJob(this.generation);
    }catch(err) {
      if(version===this.generation)this.jobError=this.describeError(err);
    }finally {
      this.cancelling=false;
    }
  }

  async retryJob(id:string):Promise<void> {
    const version=this.generation;
    this.historyError="";
    try {
      const next=await this.api.retryJob(id);
      if(version!==this.generation)return;
      this.selectJob(next);
      void this.loadHistory();
    }catch(err){
      if(version===this.generation)this.historyError=this.describeError(err);
    }
  }

  async downloadSelected():Promise<void> {
    if(!this.selectedJob?.downloadReady || this.selectedJob.archived || this.downloading)return;
    const id=this.selectedJob.id,version=this.generation;
    this.downloading=true;
    try{
      const blob=await this.api.downloadJob(id);
      if(version!==this.generation)return;
      const url=URL.createObjectURL(blob);
      const a=document.createElement("a");
      a.href=url;
      a.download="anyflip-"+id+".pdf";
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(()=>URL.revokeObjectURL(url),10000);
    }catch(err){
      if(version===this.generation)this.jobError=this.describeError(err);
    }finally{
      if(version===this.generation)this.downloading=false;
    }
  }

  private describeError(err:unknown):string {
    return err instanceof Error ? err.message : "เกิดข้อผิดพลาดที่ไม่ทราบสาเหตุ";
  }
}
