package main

import (
 "net/http"
 "net/http/httptest"
 "os"
 "strings"
 "sync"
 "sync/atomic"
 "testing"
 "time"
)

func TestRBACConcurrentAdminCancelIsSingleAndAuditedOnce(t *testing.T) {
 s,_,done:=testGoogle(t);defer done()
 if err:=os.Chmod(s.root,0700);err!=nil{t.Fatal(err)}
 admin:=rbacTestIdentity("cancel-super-admin")
 other:=rbacTestIdentity("cancel-job-owner")
 if err:=bootstrapRBACStore(s.root,admin);err!=nil{t.Fatal(err)}
 roles,err:=openRBACStore(s.root);if err!=nil{t.Fatal(err)}
 for _,user:=range []googleUser{
  {Owner:admin,Email:"super@rcat.ac.th"},
  {Owner:other,Email:"owner@rcat.ac.th"},
 }{if err:=roles.registerVerifiedUser(user);err!=nil{t.Fatal(err)}}
 s.google.rbac=roles
 sessionToken,err:=randomGoogleToken();if err!=nil{t.Fatal(err)}
 s.google.sessions[googleDigest(sessionToken)]=googleSession{
  User:googleUser{Owner:admin,Email:"super@rcat.ac.th"},
  Expires:time.Now().Add(time.Hour),
 }
 cookie:=&http.Cookie{Name:googleSessionCookie,Value:sessionToken}
 jobID:=strings.Repeat("e",32)
 var cancellations atomic.Int32
 s.jobs[jobID]=&webJob{id:jobID,stage:"queued",owner:other,created:time.Now(),cancel:func(){cancellations.Add(1)}}
 handler:=s.handler()
 const requests=8
 var wg sync.WaitGroup
 results:=make(chan int,requests)
 for i:=0;i<requests;i++{
  wg.Add(1)
  go func(){
   defer wg.Done()
   req:=httptest.NewRequest("POST","http://localhost:8094/api/admin/jobs/"+jobID+"/cancel",strings.NewReader(`{"reason":"policy"}`))
   req.Header.Set("Origin","http://localhost:8094")
   req.Header.Set("Content-Type","application/json")
   req.AddCookie(cookie)
   rr:=httptest.NewRecorder()
   handler.ServeHTTP(rr,req)
   results<-rr.Code
  }()
 }
 wg.Wait();close(results)
 accepted,conflicts:=0,0
 for code:=range results{
  switch code {
  case http.StatusAccepted:accepted++
  case http.StatusConflict:conflicts++
  default:t.Errorf("unexpected concurrent cancellation result %d",code)
  }
 }
 if accepted!=1||conflicts!=requests-1{t.Fatalf("expected one accepted and %d conflicts, got %d and %d",requests-1,accepted,conflicts)}
 if got:=cancellations.Load();got!=1{t.Fatalf("cancel function called %d times",got)}
 if !s.jobs[jobID].cancelRequested{t.Fatal("in-memory job did not mark cancellation requested")}
 events,err:=roles.recentAudit(50);if err!=nil{t.Fatal(err)}
 count:=0
 for _,event:=range events{if event.Event=="job_cancel"&&event.JobID==jobID{count++}}
 if count!=1{t.Fatalf("expected one attributable cancellation audit event, got %d",count)}
 req:=httptest.NewRequest("GET","http://localhost:8094/api/admin/jobs",nil)
 req.AddCookie(cookie)
 rr:=httptest.NewRecorder();handler.ServeHTTP(rr,req)
 if rr.Code!=200||!strings.Contains(rr.Body.String(),`"cancelRequested":true`){
  t.Fatalf("admin view lost requested cancellation status: %d %s",rr.Code,rr.Body.String())
 }
}
