package main

import (
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "sort"
 "strconv"
 "time"
)

type adminJobRecord struct {
 ID string `json:"id"`
 Owner string `json:"owner"`
 Stage string `json:"stage"`
 ErrorCode string `json:"errorCode,omitempty"`
 CancelRequested bool `json:"cancelRequested,omitempty"`
 Completed int `json:"completed"`
 Total int `json:"total"`
 Created time.Time `json:"created"`
 Archived bool `json:"archived,omitempty"`
}
type adminUserRecord struct {
 IdentityRef string `json:"identityRef"`
 Email string `json:"email"`
 Role string `json:"role"`
 Disabled bool `json:"disabled"`
 LastSeen time.Time `json:"lastSeen,omitempty"`
}
type adminSummaryRecord struct {
 Total int `json:"total"`
 Active int `json:"active"`
 Queued int `json:"queued"`
 Running int `json:"running"`
 Completed int `json:"completed"`
 Failed int `json:"failed"`
 Cancelled int `json:"cancelled"`
 ActiveReservations int `json:"activeReservations"`
 MaxConcurrent int `json:"maxConcurrent"`
 Storage *adminStorageRecord `json:"storage,omitempty"`
}
// Safe operational aggregates only. Never include host paths or private files.
type adminStorageRecord struct {
 TotalBytes uint64 `json:"totalBytes"`
 UsedBytes uint64 `json:"usedBytes"`
 AvailableBytes uint64 `json:"availableBytes"`
 UsedPercent int `json:"usedPercent"`
}

func rbacPrincipal(r *http.Request)googlePrincipal {
 p,_:=r.Context().Value(googlePrincipalContextKey{}).(googlePrincipal)
 return p
}
func (s *webServer) requireAdmin(w http.ResponseWriter,r *http.Request,superOnly bool)bool{
 if s.google==nil||s.google.rbac==nil{http.NotFound(w,r);return false}
 p:=rbacPrincipal(r)
 // Do not trust a client field; the role is injected only by authorizeGoogle.
 actual,ok:=s.google.rbac.currentRole(p.Owner)
 if !ok{http.Error(w,"forbidden",http.StatusForbidden);return false}
 if (superOnly&&actual!=roleSuperAdmin) ||
 (!superOnly && actual!=roleAdmin&&actual!=roleSuperAdmin){
  if s.rbacLimiter.allow(p.Owner) {
   // Best-effort denied-attempt audit. Regardless of audit health, deny.
   _ = s.google.rbac.audit("privileged_denied",p.Owner,"","")
  }
  http.Error(w,"forbidden",http.StatusForbidden);return false
 }
 return true
}
func (s *webServer) limitAdminMutation(w http.ResponseWriter,r *http.Request)bool{
 if !s.rbacLimiter.allow(rbacPrincipal(r).Owner){
  w.Header().Set("Retry-After","60")
  http.Error(w,"privileged action limit reached",http.StatusTooManyRequests)
  return false
 }
 return true
}
func adminPagination(r *http.Request)(int,int,error){
 q:=r.URL.Query()
 if len(q["limit"])>1||len(q["offset"])>1{return 0,0,errors.New("invalid pagination")}
 limit,offset:=20,0
 if q.Has("limit"){
  v,err:=strconv.Atoi(q.Get("limit"))
  if err!=nil||v<1||v>50{return 0,0,errors.New("limit must be 1 to 50")}
  limit=v
 }
 if q.Has("offset"){
  v,err:=strconv.Atoi(q.Get("offset"))
  if err!=nil||v<0||v>10000{return 0,0,errors.New("invalid offset")}
  offset=v
 }
 return limit,offset,nil
}
func (s *webServer) adminJobRecords()[]adminJobRecord{
 s.mu.Lock();defer s.mu.Unlock()
 records:=make([]adminJobRecord,0,len(s.jobs)+len(s.history))
 for _,j:=range s.jobs{
  records=append(records,adminJobRecord{ID:j.id,Owner:j.owner,Stage:j.stage,ErrorCode:j.errorCode,CancelRequested:j.cancelRequested,Completed:j.completed,Total:j.total,Created:j.created})
 }
 for _,h:=range s.history{
  if _,exists:=s.jobs[h.Response.ID];exists{continue}
  v:=h.Response
  records=append(records,adminJobRecord{ID:v.ID,Owner:v.Owner,Stage:v.Stage,ErrorCode:v.ErrorCode,Completed:v.Completed,Total:v.Total,Created:h.Finished,Archived:true})
 }
 sort.Slice(records,func(i,j int)bool{
  if records[i].Created.Equal(records[j].Created){return records[i].ID>records[j].ID}
  return records[i].Created.After(records[j].Created)
 })
 return records
}
func (s *webServer) adminSummary(w http.ResponseWriter,r *http.Request){
 if !s.requireAdmin(w,r,false){return}
 var result adminSummaryRecord
 for _,j:=range s.adminJobRecords(){
  result.Total++
  switch j.Stage{
  case "completed":result.Completed++
  case "failed":result.Failed++
  case "cancelled":result.Cancelled++
  case "queued":result.Active++;result.Queued++
  default:result.Active++;result.Running++
  }
 }
 s.mu.Lock()
 result.ActiveReservations=s.activeReservations
 result.MaxConcurrent=cap(s.sem)
 s.mu.Unlock()
 result.Storage=readAdminStorage(s.root)
 sendJSON(w,http.StatusOK,result)
}
func (s *webServer) adminListJobs(w http.ResponseWriter,r *http.Request){
 if !s.requireAdmin(w,r,false){return}
 limit,offset,err:=adminPagination(r)
 if err!=nil{http.Error(w,"invalid pagination",400);return}
 jobs:=s.adminJobRecords()
 total:=len(jobs)
 if offset>=total {sendJSON(w,200,map[string]any{"jobs":[]adminJobRecord{},"total":total});return}
 end:=offset+limit
 if end>total{end=total}
 sendJSON(w,200,map[string]any{"jobs":jobs[offset:end],"total":total})
}
func (s *webServer) adminListUsers(w http.ResponseWriter,r *http.Request){
 if !s.requireAdmin(w,r,false){return}
 list,err:=s.google.rbac.snapshotMembers()
 if err!=nil{http.Error(w,"RBAC directory unavailable",http.StatusServiceUnavailable);return}
 result:=make([]adminUserRecord,0,len(list))
 for _,member:=range list{
  result=append(result,adminUserRecord{
   IdentityRef:member.ID,Email:member.Email,Role:member.Role,Disabled:member.Disabled,LastSeen:member.LastSeen,
  })
 }
 sendJSON(w,http.StatusOK,map[string]any{"users":result})
}
func decodeStrictAdminBody(w http.ResponseWriter,r *http.Request,destination any)error{
 r.Body=http.MaxBytesReader(w,r.Body,1024)
 dec:=json.NewDecoder(r.Body)
 dec.DisallowUnknownFields()
 if err:=dec.Decode(destination);err!=nil{return err}
 var extra json.RawMessage
 if err:=dec.Decode(&extra);err!=io.EOF{return errors.New("trailing request data")}
 return nil
}
func (s *webServer) adminSetRole(w http.ResponseWriter,r *http.Request){
 if !s.requireAdmin(w,r,true)||!s.limitAdminMutation(w,r){return}
 target:=r.PathValue("id")
 if !storedGoogleOwnerPattern.MatchString(target){http.NotFound(w,r);return}
 var body struct{Role string `json:"role"`}
 if err:=decodeStrictAdminBody(w,r,&body);err!=nil||!validRBACRole(body.Role){
  http.Error(w,"invalid role change",400);return
 }
 actor:=rbacPrincipal(r).Owner
 if actor==target {
  _ = s.google.rbac.audit("role_change_denied",actor,target,"")
  http.Error(w,"cannot modify your own role",http.StatusForbidden);return
 }
 if err:=s.google.rbac.changeRole(actor,target,body.Role);err!=nil{
  // Record denied privilege changes without disclosing target existence.
  _ = s.google.rbac.audit("role_change_denied",actor,target,"")
  // Keep existence, disabling and last-super-admin policy private.
  http.Error(w,"role change denied or unavailable",http.StatusConflict);return
 }
 sendJSON(w,200,map[string]any{"identityRef":target,"role":body.Role})
}
func (s *webServer) adminCancelJob(w http.ResponseWriter,r *http.Request){
 if !s.requireAdmin(w,r,false)||!s.limitAdminMutation(w,r){return}
 id:=r.PathValue("id")
 if !jobIDPattern.MatchString(id){http.NotFound(w,r);return}
 var body struct{Reason string `json:"reason"`}
 if err:=decodeStrictAdminBody(w,r,&body);err!=nil{
  http.Error(w,"invalid request",400);return
 }
 switch body.Reason {
 case "policy","resources","duplicate","other":
 default:http.Error(w,"invalid reason",400);return
 }
 s.mu.Lock()
 job,exists:=s.jobs[id]
 if !exists{ s.mu.Unlock();http.NotFound(w,r);return }
 if job.stage=="completed"||job.stage=="failed"||job.stage=="cancelled"{
  s.mu.Unlock();http.Error(w,"job is already terminal",http.StatusConflict);return
 }
 if job.cancelRequested {
  s.mu.Unlock();http.Error(w,"cancellation already requested",http.StatusConflict);return
 }
 actor:=rbacPrincipal(r).Owner
 if err:=s.google.rbac.auditJobCancellation(actor,job.owner,job.id,body.Reason);err!=nil{
  s.mu.Unlock()
  http.Error(w,"audit unavailable; cancellation blocked",http.StatusServiceUnavailable);return
 }
 job.cancelRequested=true
 job.cancel()
 response:=map[string]any{"id":job.id,"accepted":true}
 s.mu.Unlock()
 sendJSON(w,http.StatusAccepted,response)
}
func (s *webServer) adminAudit(w http.ResponseWriter,r *http.Request){
 if !s.requireAdmin(w,r,true){return}
 limit,offset,err:=adminPagination(r)
 if err!=nil||offset!=0{http.Error(w,"invalid pagination",400);return}
 events,err:=s.google.rbac.recentAudit(limit)
 if err!=nil{http.Error(w,"audit unavailable",http.StatusServiceUnavailable);return}
 if events==nil{events=[]rbacAuditEvent{}}
 sendJSON(w,200,map[string]any{"events":events})
}
