package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthenticationAuditRedactionAndOperationCoverage(t *testing.T) {
	s:=newLifecycleFixture(t)
	admin:=lifecycleLogin(t,s,"admin","first-admin-password-strong")
	_ = lifecycleRequest(t,s,"POST","/auth/login","http://account.test",nil,
		map[string]string{"username":"alice","password":"password-secret-never-log"},false)
	result:=lifecycleRequest(t,s,"POST","/auth/users","http://account.test",admin,
		map[string]string{"username":"carol","password":"carol-super-secret-password","role":"user"},false)
	if result.Code!=http.StatusCreated{t.Fatalf("admin create returned %d",result.Code)}
	result=lifecycleRequest(t,s,"POST","/auth/users/alice/reset-password","http://account.test",admin,
		map[string]string{"password":"a-unique-new-secret-password"},false)
	if result.Code!=http.StatusNoContent{t.Fatalf("admin reset returned %d",result.Code)}
	result=lifecycleRequest(t,s,"PATCH","/auth/users/alice/status","http://account.test",admin,
		map[string]bool{"disabled":true},false)
	if result.Code!=http.StatusNoContent{t.Fatalf("admin suspend returned %d",result.Code)}
	events,err:=s.auth.audit.readForTest()
	if err!=nil{t.Fatal(err)}
	present:=map[string]bool{}
	for _,e:=range events {
		if e.Time.IsZero(){t.Fatal("audit has zero timestamp")}
		if e.Actor=="password-secret-never-log" || strings.Contains(e.Target,"password"){t.Fatal("audit contains password as identifier")}
		present[e.Event+":"+e.Result]=true
	}
	for _,k:=range []string{"create_user:intent","create_user:ok","login:denied","login:ok","reset_password:ok","suspend_user:ok"}{
		if !present[k]{t.Errorf("missing event %s",k)}
	}
	raw,err:=os.ReadFile(filepath.Join(s.root,authAuditFilename))
	if err!=nil{t.Fatal(err)}
	for _,secret:=range []string{"password-secret-never-log","carol-super-secret-password","a-unique-new-secret-password","first-admin-password-strong","Bearer ","anyflip_session"}{
		if bytes.Contains(raw,[]byte(secret)){t.Fatalf("audit leaked sensitive value %q",secret)}
	}
	info,err:=os.Stat(filepath.Join(s.root,authAuditFilename))
	if err!=nil{t.Fatal(err)}
	if info.Mode().Perm()!=0600{t.Fatalf("unsafe audit permission: %v",info.Mode().Perm())}
	var decoded authAuditEvent
	if err:=json.Unmarshal(bytes.TrimSpace(bytes.Split(raw,[]byte{10})[0]),&decoded);err!=nil{t.Fatal(err)}
}

func TestAuthenticationAuditUnsafeFileAndRotation(t *testing.T) {
	dir:=t.TempDir()
	a,err:=newAuthAudit(dir);if err!=nil{t.Fatal(err)}
	path:=filepath.Join(dir,authAuditFilename)
	if err:=os.WriteFile(path,bytes.Repeat([]byte("x"),authAuditMaxBytes-10),0600);err!=nil{t.Fatal(err)}
	if err:=a.record("login","admin","","ok");err!=nil{t.Fatal(err)}
	archived,err:=os.Stat(path+".1")
	if err!=nil{t.Fatal(err)}
	if archived.Size()!=authAuditMaxBytes-10{t.Fatalf("rotation lost archived data: %d",archived.Size())}
	current,err:=os.ReadFile(path)
	if err!=nil{t.Fatal(err)}
	if !bytes.Contains(current,[]byte(`"event":"login"`)){t.Fatal("new audit file not written after rotation")}
	if err:=os.Chmod(path,0644);err!=nil{t.Fatal(err)}
	if err:=a.record("login","admin","","ok");err==nil{t.Fatal("accepted world-readable audit")}
	if _,err:=newAuthAudit(dir);err==nil{t.Fatal("startup permitted insecure audit")}
	if err:=os.Remove(path);err!=nil{t.Fatal(err)}
	if err:=os.Symlink(filepath.Join(dir,"other"),path);err!=nil{t.Fatal(err)}
	if _,err:=newAuthAudit(dir);err==nil{t.Fatal("startup permitted symlink audit")}
	if err:=a.record("login","admin","","ok");err==nil{t.Fatal("record followed symlink")}
}

func TestAuthenticationAuditFailureFailsClosed(t *testing.T) {
	s:=newLifecycleFixture(t)
	admin:=lifecycleLogin(t,s,"admin","first-admin-password-strong")
	path:=filepath.Join(s.root,authAuditFilename)
	if err:=os.Chmod(path,0644);err!=nil{t.Fatal(err)}
	before:=len(s.auth.users)
	result:=lifecycleRequest(t,s,"POST","/auth/users","http://account.test",admin,
		map[string]string{"username":"carol","password":"a-very-long-password","role":"user"},false)
	if result.Code!=http.StatusServiceUnavailable {t.Fatalf("create bypassed unavailable audit: %d",result.Code)}
	if len(s.auth.users)!=before {t.Fatal("account was created without an intent audit record")}
	if result:=lifecycleRequest(t,s,"POST","/auth/login","http://account.test",nil,
		map[string]string{"username":"admin","password":"first-admin-password-strong"},false);result.Code!=http.StatusServiceUnavailable {
		t.Fatalf("login bypassed unavailable audit: %d",result.Code)
	}
}
