package main

import (
 "bytes"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
)

func TestAccountsPasswordHashAndPersistence(t *testing.T) {
 hash,err:=hashAccountPassword("a-different-long-password")
 if err!=nil || !validPasswordHash(hash){t.Fatalf("hash invalid %v",err)}
 if !verifyAccountPassword(hash,"a-different-long-password") || verifyAccountPassword(hash,"not-the-password"){t.Fatal("password hash verification failed")}
 if _,err:=hashAccountPassword("short");err==nil{t.Fatal("weak password was accepted")}
 dir:=t.TempDir()
 auth,err:=loadAccountAuth(dir,"admin",hash,false)
 if err!=nil{t.Fatal(err)}
 if err:=auth.addUser("alice","alice-a-very-long-password","user");err!=nil{t.Fatal(err)}
 if err:=auth.addUser("alice","duplicate-long-password","user");err==nil{t.Fatal("duplicate user accepted")}
 fresh,err:=loadAccountAuth(dir,"","",false)
 if err!=nil{t.Fatal(err)}
 if u,ok:=fresh.authenticate("alice","alice-a-very-long-password");!ok || u.Role!="user"{t.Fatal("persisted account not recovered")}
 if _,ok:=fresh.authenticate("alice","wrong");ok{t.Fatal("wrong password accepted")}
}

func TestAccountLoginSessionRolesOwnershipAndLogout(t *testing.T) {
 hash,err:=hashAccountPassword("long-bootstrap-admin-password")
 if err!=nil{t.Fatal(err)}
 s,err:=newWebServer(testWebToken,t.TempDir())
 if err!=nil{t.Fatal(err)}
 s.auth,err=loadAccountAuth(s.root,"admin",hash,false)
 if err!=nil{t.Fatal(err)}
 if err:=s.auth.addUser("alice","alice-password-12345","user");err!=nil{t.Fatal(err)}
 if err:=s.auth.addUser("bob","bob-password-123456","user");err!=nil{t.Fatal(err)}
 h:=s.handler()
 request:=func(method,path,origin string,cookie *http.Cookie,body string,bearer bool) *httptest.ResponseRecorder {
  t.Helper()
  r:=httptest.NewRequest(method,"http://account.test"+path,bytes.NewBufferString(body))
  if cookie!=nil{r.AddCookie(cookie)}
  if origin!=""{r.Header.Set("Origin",origin)}
  if body!=""{r.Header.Set("Content-Type","application/json")}
  if bearer{r.Header.Set("Authorization","Bearer "+testWebToken)}
  w:=httptest.NewRecorder();h.ServeHTTP(w,r);return w
 }
 login:=func(username,password string) *http.Cookie {
  t.Helper()
  b,_:=json.Marshal(map[string]string{"username":username,"password":password})
  w:=request("POST","/auth/login","http://account.test",nil,string(b),false)
  if w.Code!=200{t.Fatalf("login for %s failed: %d %s",username,w.Code,w.Body.String())}
  cookies:=w.Result().Cookies()
  if len(cookies)!=1 || cookies[0].Name!=sessionCookieName || !cookies[0].HttpOnly || cookies[0].SameSite!=http.SameSiteStrictMode {t.Fatal("unsafe session cookie")}
  return cookies[0]
 }
 if got:=request("GET","/api/jobs","",nil,"",false).Code;got!=401{t.Fatalf("unauthenticated API returned %d",got)}
 if got:=request("POST","/auth/login","http://evil.example",nil,`{"username":"admin","password":"long-bootstrap-admin-password"}`,false).Code;got!=403{t.Fatalf("cross-origin login returned %d",got)}
 a:=login("alice","alice-password-12345")
 b:=login("bob","bob-password-123456")
 admin:=login("admin","long-bootstrap-admin-password")
 if got:=request("GET","/auth/me","",a,"",false).Code;got!=200{t.Fatalf("valid session = %d",got)}
 if got:=request("POST","/auth/users","http://account.test",a,`{"username":"eve","password":"longer-password-xyz","role":"admin"}`,false).Code;got!=403{t.Fatalf("user escalated privilege: %d",got)}
 if got:=request("POST","/auth/users","http://account.test",admin,`{"username":"eve","password":"longer-password-xyz","role":"user"}`,false).Code;got!=201{t.Fatalf("admin user creation failed: %d",got)}
 if got:=request("POST","/auth/users","http://evil.test",admin,`{"username":"pwn","password":"longer-password-xyz","role":"admin"}`,false).Code;got!=401{t.Fatalf("cross-origin admin action should be rejected: %d",got)}
 if got:=request("POST","/auth/users","",admin,`{"username":"pwn","password":"longer-password-xyz","role":"admin"}`,false).Code;got!=401{t.Fatalf("CSRF without origin was accepted: %d",got)}
 aliceID:=strings.Repeat("a",32);bobID:=strings.Repeat("b",32)
 s.mu.Lock()
 s.jobs[aliceID]=&webJob{id:aliceID,owner:"alice",stage:"completed",completed:1,total:1,file:"/nonexistent"}
 s.jobs[bobID]=&webJob{id:bobID,owner:"bob",stage:"completed",completed:1,total:1,file:"/nonexistent"}
 s.mu.Unlock()
 if got:=request("GET","/api/jobs/"+aliceID,"",a,"",false).Code;got!=200{t.Fatalf("owner cannot read job: %d",got)}
 for _,item:=range []struct{method,path string}{
  {"GET","/api/jobs/"+bobID},{"POST","/api/jobs/"+bobID+"/cancel"},
  {"POST","/api/jobs/"+bobID+"/retry"},{"GET","/api/jobs/"+bobID+"/download"},
 }{
  if got:=request(item.method,item.path,"http://account.test",a,"",false).Code;got!=404{t.Fatalf("IDOR %s %s returned %d",item.method,item.path,got)}
 }
 result:=request("GET","/api/jobs","",a,"",false)
 if result.Code!=200{t.Fatalf("list status %d",result.Code)}
 if strings.Contains(result.Body.String(),bobID){t.Fatal("job list leaked another user's job")}
 if got:=request("GET","/api/jobs/"+bobID,"",admin,"",false).Code;got!=200{t.Fatalf("admin cannot view other's job: %d",got)}
 if got:=request("GET","/api/jobs/"+bobID,"",nil,"",true).Code;got!=200{t.Fatalf("service API token cannot view job: %d",got)}
 if got:=request("POST","/auth/logout","http://account.test",a,"",false).Code;got!=204{t.Fatalf("logout returned %d",got)}
 if got:=request("GET","/auth/me","",a,"",false).Code;got!=401{t.Fatalf("revoked cookie accepted: %d",got)}
 // An archived record is owned by the original user too.
 s.mu.Lock()
 s.history=[]archivedWebJob{{Response:webJobResponse{ID:strings.Repeat("c",32),Owner:"bob",Stage:"completed",Completed:1,Total:1},Finished:time.Now()}}
 s.mu.Unlock()
 if got:=request("GET","/api/jobs/"+strings.Repeat("c",32),"",b,"",false).Code;got!=200{t.Fatalf("owner archived status %d",got)}
 if got:=request("GET","/api/jobs/"+strings.Repeat("c",32),"",admin,"",false).Code;got!=200{t.Fatalf("admin archived status %d",got)}
 if got:=request("GET","/api/jobs/"+strings.Repeat("c",32),"",nil,"",false).Code;got!=401{t.Fatalf("anonymous archived status %d",got)}
}
