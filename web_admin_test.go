package main

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestRBACAdminRoutesAndOwnerIsolation(t *testing.T){
 s,_,done:=testGoogle(t);defer done()
 if err:=os.Chmod(s.root,0700);err!=nil{t.Fatal(err)}
 super:=rbacTestIdentity("super")
 manager:=rbacTestIdentity("manager")
 teacher:=rbacTestIdentity("teacher")
 if err:=bootstrapRBACStore(s.root,super);err!=nil{t.Fatal(err)}
 roles,err:=openRBACStore(s.root);if err!=nil{t.Fatal(err)}
 for _,u:=range []googleUser{
  {Owner:super,Email:"super@rcat.ac.th"},
  {Owner:manager,Email:"manager@rcat.ac.th"},
  {Owner:teacher,Email:"teacher@rcat.ac.th"},
 }{if err:=roles.registerVerifiedUser(u);err!=nil{t.Fatal(err)}}
 if err:=roles.changeRole(super,manager,roleAdmin);err!=nil{t.Fatal(err)}
 s.google.rbac=roles
 sessions:=make(map[string]*http.Cookie)
 for _,u:=range []googleUser{
  {Owner:super,Email:"super@rcat.ac.th"},
  {Owner:manager,Email:"manager@rcat.ac.th"},
  {Owner:teacher,Email:"teacher@rcat.ac.th"},
 }{
  token,err:=randomGoogleToken();if err!=nil{t.Fatal(err)}
  s.google.sessions[googleDigest(token)]=googleSession{User:u,Expires:time.Now().Add(time.Hour)}
  sessions[u.Owner]=&http.Cookie{Name:googleSessionCookie,Value:token}
 }
 otherID:=strings.Repeat("a",32)
 ownID:=strings.Repeat("b",32)
 _,otherCancel:=context.WithCancel(context.Background());defer otherCancel()
 _,ownCancel:=context.WithCancel(context.Background());defer ownCancel()
 s.jobs[otherID]=&webJob{id:otherID,owner:teacher,stage:"queued",created:time.Now(),cancel:otherCancel}
 s.jobs[ownID]=&webJob{id:ownID,owner:manager,stage:"queued",created:time.Now(),cancel:ownCancel}
 handle:=s.handler()
 call:=func(owner,method,path,body string)*httptest.ResponseRecorder{
  t.Helper()
  r:=httptest.NewRequest(method,"http://localhost:8094"+path,strings.NewReader(body))
  if owner!=""{r.AddCookie(sessions[owner])}
  if method!="GET"{r.Header.Set("Origin","http://localhost:8094")}
  if body!=""{r.Header.Set("Content-Type","application/json")}
  w:=httptest.NewRecorder();handle.ServeHTTP(w,r);return w
 }
 if got:=call(teacher,"GET","/api/admin/summary","").Code;got!=403{t.Fatalf("user metrics=%d",got)}
 if got:=call(teacher,"GET","/api/admin/users","").Code;got!=403{t.Fatalf("user directory=%d",got)}
 if got:=call("","GET","/api/admin/summary","").Code;got!=401{t.Fatalf("anonymous admin=%d",got)}
 if got:=call(manager,"GET","/api/admin/summary","").Code;got!=200{t.Fatalf("admin metrics=%d",got)}
 if got:=call(manager,"GET","/api/admin/jobs?limit=1000","").Code;got!=400{t.Fatalf("unbounded query=%d",got)}
 list:=call(manager,"GET","/api/admin/jobs?limit=2","")
 if list.Code!=200 || strings.Contains(list.Body.String(),"https://") || strings.Contains(list.Body.String(),"downloadReady"){
  t.Fatalf("admin metadata leak: %d %s",list.Code,list.Body.String())
 }
 if got:=call(manager,"GET","/api/jobs/"+otherID+"/download","").Code;got!=404{
  t.Fatalf("admin downloaded another user's PDF: %d",got)
 }
 if got:=call(super,"GET","/api/jobs/"+otherID,"").Code;got!=404{
  t.Fatalf("super admin read another user's private job: %d",got)
 }
 if got:=call(manager,"PUT","/api/admin/users/"+teacher+"/role",`{"role":"admin"}`).Code;got!=403{
  t.Fatalf("non-super role grant=%d",got)
 }
 if got:=call(super,"PUT","/api/admin/users/"+super+"/role",`{"role":"user"}`).Code;got!=403{
  t.Fatalf("self-demotion=%d",got)
 }
 if got:=call(super,"PUT","/api/admin/users/"+manager+"/role",`{"role":"user"}`).Code;got!=200{
  t.Fatalf("super revoke=%d",got)
 }
 // An existing manager session must lose its permissions immediately.
 if got:=call(manager,"GET","/api/admin/summary","").Code;got!=403{
  t.Fatalf("stale elevated session=%d",got)
 }
 if got:=call(super,"POST","/api/admin/jobs/"+otherID+"/cancel",`{"reason":"policy"}`).Code;got!=202{
  t.Fatalf("cross-user cancellation=%d",got)
 }
 if got:=call(teacher,"GET","/api/admin/audit","").Code;got!=403{t.Fatalf("ordinary audit=%d",got)}
 audit:=call(super,"GET","/api/admin/audit","")
 if audit.Code!=200 || !strings.Contains(audit.Body.String(),"job_cancel") || !strings.Contains(audit.Body.String(),otherID) || !strings.Contains(audit.Body.String(),"role_change_intent"){
  t.Fatalf("privileged audit missing: %d %s",audit.Code,audit.Body.String())
 }
 if got:=call(super,"POST","/api/admin/jobs/"+ownID+"/cancel",`{"reason":"untrusted-user-prose"}`).Code;got!=400{
  t.Fatalf("arbitrary audit reason=%d",got)
 }
 // Mutations must be same-origin, even with a valid session cookie.
 r:=httptest.NewRequest("PUT","http://localhost:8094/api/admin/users/"+teacher+"/role",strings.NewReader(`{"role":"admin"}`))
 r.AddCookie(sessions[super]);w:=httptest.NewRecorder();handle.ServeHTTP(w,r)
 if w.Code!=401{t.Fatalf("cross-site/unknown-origin mutation=%d",w.Code)}
 // RBAC is absent entirely in legacy token/auth configurations.
 tokenMode,err:=newWebServer(testWebToken,t.TempDir());if err!=nil{t.Fatal(err)}
 rr:=httptest.NewRecorder()
 tokenMode.handler().ServeHTTP(rr,httptest.NewRequest("GET","http://localhost/api/admin/summary",nil))
 if rr.Code!=404{t.Fatalf("legacy mode exposed admin API: %d",rr.Code)}
}

func TestRBACFailClosedOnAuditTampering(t *testing.T){
 root:=rbacTestRoot(t)
 super:=rbacTestIdentity("owner")
 target:=rbacTestIdentity("target")
 if err:=bootstrapRBACStore(root,super);err!=nil{t.Fatal(err)}
 store,err:=openRBACStore(root);if err!=nil{t.Fatal(err)}
 if err:=store.registerVerifiedUser(googleUser{Owner:target,Email:"target@rcat.ac.th"});err!=nil{t.Fatal(err)}
 if err:=store.audit("role_change_denied",super,target,"");err!=nil{t.Fatal(err)}
 auditPath:=filepath.Join(root,rbacAuditFilename)
 if err:=os.Chmod(auditPath,0644);err!=nil{t.Fatal(err)}
 if err:=store.changeRole(super,target,roleAdmin);err==nil{t.Fatal("role changed without reliable audit")}
 if role,_:=store.currentRole(target);role!=roleUser{t.Fatal("role escalated after audit failure")}
 if _,err:=openRBACStore(root);err==nil{t.Fatal("corrupt audit accepted on restart")}
}

func TestRBACAdminResponseJSONShape(t *testing.T){
 rec:=adminUserRecord{IdentityRef:rbacTestIdentity("id"),Role:roleUser,Email:"user@rcat.ac.th"}
 b,err:=json.Marshal(rec);if err!=nil{t.Fatal(err)}
 for _,k:=range []string{`"identityRef"`,`"role"`,`"email"`}{
  if !strings.Contains(string(b),k){t.Fatal("missing JSON contract:",k)}
 }
}
