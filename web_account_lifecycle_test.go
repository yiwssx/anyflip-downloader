package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newLifecycleFixture(t *testing.T) *webServer {
	t.Helper()
	hash, err := hashAccountPassword("first-admin-password-strong")
	if err != nil { t.Fatal(err) }
	server, err := newWebServer(testWebToken, t.TempDir())
	if err != nil { t.Fatal(err) }
	server.auth, err = loadAccountAuth(server.root, "admin", hash, false)
	if err != nil { t.Fatal(err) }
	for _, u := range []struct{ name, pass, role string }{
		{"anotheradmin", "second-admin-password", "admin"},
		{"alice", "alice-first-password", "user"},
		{"bob", "bob-first-password", "user"},
	} {
		if err := server.auth.addUser(u.name, u.pass, u.role); err != nil { t.Fatal(err) }
	}
	return server
}

func lifecycleRequest(t *testing.T, s *webServer, method, path, origin string, cookie *http.Cookie, payload any, bearer bool) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil { t.Fatal(err) }
	}
	r := httptest.NewRequest(method, "http://account.test" + path, bytes.NewReader(body))
	if origin != "" { r.Header.Set("Origin", origin) }
	if cookie != nil { r.AddCookie(cookie) }
	if bearer { r.Header.Set("Authorization", "Bearer " + testWebToken) }
	if body != nil { r.Header.Set("Content-Type", "application/json") }
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, r)
	return w
}

func lifecycleLogin(t *testing.T, s *webServer, username, password string) *http.Cookie {
	t.Helper()
	w := lifecycleRequest(t, s, "POST", "/auth/login", "http://account.test", nil, map[string]string{"username":username,"password":password}, false)
	if w.Code != http.StatusOK { t.Fatalf("login %s returned %d: %s", username, w.Code, w.Body.String()) }
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode { t.Fatal("missing secure session cookie") }
	return cookies[0]
}

func TestAccountLifecycleDisableReenableAndLastAdmin(t *testing.T) {
	s := newLifecycleFixture(t)
	admin := lifecycleLogin(t,s,"admin","first-admin-password-strong")
	alice := lifecycleLogin(t,s,"alice","alice-first-password")
	change := func(actor *http.Cookie,name string, disabled bool) int {
		t.Helper()
		return lifecycleRequest(t,s,"PATCH","/auth/users/"+name+"/status","http://account.test",actor,map[string]bool{"disabled":disabled},false).Code
	}
	if code:=change(alice,"bob",true);code!=http.StatusForbidden{t.Fatalf("regular user disabled peer: %d",code)}
	if code:=lifecycleRequest(t,s,"PATCH","/auth/users/bob/status","http://evil.test",admin,map[string]bool{"disabled":true},false).Code;code!=http.StatusUnauthorized {t.Fatalf("cross-origin status change returned %d",code)}
	if code:=lifecycleRequest(t,s,"PATCH","/auth/users/bob/status","http://account.test",nil,map[string]bool{"disabled":true},true).Code;code!=http.StatusForbidden {t.Fatalf("machine Bearer token changed accounts: %d",code)}
	if code:=change(admin,"admin",true);code!=http.StatusConflict {t.Fatalf("admin self disable returned %d",code)}
	if code:=change(admin,"anotheradmin",true);code!=http.StatusNoContent {t.Fatalf("disable second admin returned %d",code)}
	if code:=change(admin,"admin",true);code!=http.StatusConflict {t.Fatalf("last admin disable returned %d",code)}
	if code:=change(admin,"alice",true);code!=http.StatusNoContent {t.Fatalf("disable Alice returned %d",code)}
	if code:=lifecycleRequest(t,s,"GET","/auth/me","",alice,nil,false).Code;code!=http.StatusUnauthorized {t.Fatalf("disabled session still accepted: %d",code)}
	if code:=lifecycleRequest(t,s,"GET","/api/jobs","",alice,nil,false).Code;code!=http.StatusUnauthorized {t.Fatalf("disabled user accessed API: %d",code)}
	if code:=lifecycleRequest(t,s,"POST","/auth/login","http://account.test",nil,map[string]string{"username":"alice","password":"alice-first-password"},false).Code;code!=http.StatusUnauthorized {t.Fatalf("disabled user logged in: %d",code)}
	if code:=change(admin,"alice",false);code!=http.StatusNoContent {t.Fatalf("reenable Alice returned %d",code)}
	if code:=lifecycleRequest(t,s,"GET","/auth/me","",alice,nil,false).Code;code!=http.StatusUnauthorized {t.Fatalf("old session resurrected: %d",code)}
	_ = lifecycleLogin(t,s,"alice","alice-first-password")
	again,err:=loadAccountAuth(s.root,"","",false)
	if err!=nil {t.Fatal(err)}
	if !again.users["anotheradmin"].Disabled || again.users["alice"].Disabled {t.Fatal("disabled state did not persist across restart")}
	if code:=change(admin,"missingaccount",true);code!=http.StatusNotFound {t.Fatalf("unknown account status: %d",code)}
	if code:=lifecycleRequest(t,s,"PATCH","/auth/users/bob/status","http://account.test",admin,map[string]string{"unexpected":"value"},false).Code;code!=http.StatusBadRequest {t.Fatalf("unknown JSON field accepted: %d",code)}
	if code:=lifecycleRequest(t,s,"PATCH","/auth/users/bob/status","http://account.test",admin,map[string]string{},false).Code;code!=http.StatusBadRequest {t.Fatalf("missing disabled field accepted: %d",code)}
}

func TestAccountLifecycleResetRotateAndSessionRevocation(t *testing.T) {
	s := newLifecycleFixture(t)
	admin := lifecycleLogin(t,s,"admin","first-admin-password-strong")
	alice1 := lifecycleLogin(t,s,"alice","alice-first-password")
	alice2 := lifecycleLogin(t,s,"alice","alice-first-password")
	reset := func(actor *http.Cookie, username, password string) int {
		return lifecycleRequest(t,s,"POST","/auth/users/"+username+"/reset-password","http://account.test",actor,map[string]string{"password":password},false).Code
	}
	if code:=reset(alice1,"bob","new-bob-password-123");code!=http.StatusForbidden {t.Fatalf("user reset another account: %d",code)}
	if code:=reset(admin,"admin","new-admin-password-123");code!=http.StatusConflict {t.Fatalf("admin reset own password bypassed reauth: %d",code)}
	if code:=reset(admin,"alice","too-short");code!=http.StatusBadRequest {t.Fatalf("short reset accepted: %d",code)}
	if code:=reset(admin,"alice","alice-reset-password-strong");code!=http.StatusNoContent {t.Fatalf("admin reset failed: %d",code)}
	for _, session:=range []*http.Cookie{alice1,alice2}{
		if code:=lifecycleRequest(t,s,"GET","/auth/me","",session,nil,false).Code;code!=http.StatusUnauthorized {t.Fatalf("old session still active: %d",code)}
	}
	if code:=lifecycleRequest(t,s,"POST","/auth/login","http://account.test",nil,map[string]string{"username":"alice","password":"alice-first-password"},false).Code;code!=http.StatusUnauthorized {t.Fatalf("old password accepted: %d",code)}
	alice := lifecycleLogin(t,s,"alice","alice-reset-password-strong")
	change := func(actor *http.Cookie, oldPass, newPass string) int {
		return lifecycleRequest(t,s,"POST","/auth/change-password","http://account.test",actor,map[string]string{"currentPassword":oldPass,"newPassword":newPass},false).Code
	}
	if code:=change(alice,"incorrect-password","another-strong-password");code!=http.StatusForbidden {t.Fatalf("incorrect old password accepted: %d",code)}
	if code:=change(alice,"alice-reset-password-strong","alice-reset-password-strong");code!=http.StatusBadRequest {t.Fatalf("password reuse accepted: %d",code)}
	if code:=change(alice,"alice-reset-password-strong","alice-rotated-password-strong");code!=http.StatusNoContent {t.Fatalf("password rotation failed: %d",code)}
	if code:=lifecycleRequest(t,s,"GET","/auth/me","",alice,nil,false).Code;code!=http.StatusUnauthorized {t.Fatalf("session remained after password change: %d",code)}
	_ = lifecycleLogin(t,s,"alice","alice-rotated-password-strong")
	again,err:=loadAccountAuth(s.root,"","",false)
	if err!=nil {t.Fatal(err)}
	if _,ok:=again.authenticate("alice","alice-rotated-password-strong");!ok {t.Fatal("rotated credentials lost across restart")}
	if code:=lifecycleRequest(t,s,"GET","/auth/users","",admin,nil,false).Code;code!=http.StatusOK {t.Fatalf("admin account list failed: %d",code)}
	list:=lifecycleRequest(t,s,"GET","/auth/users","",admin,nil,false).Body.String()
	if strings.Contains(list,"passwordHash") || strings.Contains(list,"argon2id:") || !strings.Contains(list,"\"disabled\"") {t.Fatal("list leaked password hashes or omitted disabled state")}
}

func TestAccountMutationStorageFailureRollsBackWithoutRevoking(t *testing.T) {
	s:=newLifecycleFixture(t)
	admin:=lifecycleLogin(t,s,"admin","first-admin-password-strong")
	alice:=lifecycleLogin(t,s,"alice","alice-first-password")
	target:=filepath.Join(s.root,accountsFile)
	if err:=os.Rename(target,target+".bak");err!=nil{t.Fatal(err)}
	if err:=os.Mkdir(target,0700);err!=nil{t.Fatal(err)}
	a:=s.auth
	if err:=a.setAccountDisabled("admin","alice",true);err==nil{t.Fatal("unsafe account store accepted")}
	if a.users["alice"].Disabled {t.Fatal("failed persistence left account disabled")}
	if code:=lifecycleRequest(t,s,"GET","/auth/me","",alice,nil,false).Code;code!=http.StatusOK {t.Fatalf("session incorrectly revoked on failed persistence: %d",code)}
	if code:=lifecycleRequest(t,s,"GET","/auth/me","",admin,nil,false).Code;code!=http.StatusOK {t.Fatalf("admin session lost: %d",code)}
	if err:=os.Remove(target);err!=nil{t.Fatal(err)}
	if err:=os.Rename(target+".bak",target);err!=nil{t.Fatal(err)}
	if err:=a.setAccountDisabled("admin","alice",true);err!=nil{t.Fatal(err)}
}

func TestAccountLifecycleMalformedCookieAndUnknownRoles(t *testing.T) {
	s:=newLifecycleFixture(t)
	admin:=lifecycleLogin(t,s,"admin","first-admin-password-strong")
	tampered:=*admin
	tampered.Value=strings.Repeat("a",43)
	if code:=lifecycleRequest(t,s,"POST","/auth/change-password","http://account.test",&tampered,map[string]string{"currentPassword":"first-admin-password-strong","newPassword":"new-password-strong-123"},false).Code;code!=http.StatusUnauthorized {t.Fatalf("tampered session accepted: %d",code)}
	if code:=lifecycleRequest(t,s,"POST","/auth/change-password","http://evil.test",admin,map[string]string{"currentPassword":"first-admin-password-strong","newPassword":"new-password-strong-123"},false).Code;code!=http.StatusUnauthorized {t.Fatalf("cross-origin change accepted: %d",code)}
	if code:=lifecycleRequest(t,s,"POST","/auth/change-password","http://account.test",nil,map[string]string{"currentPassword":"first-admin-password-strong","newPassword":"new-password-strong-123"},true).Code;code!=http.StatusForbidden {t.Fatalf("service token changed human password: %d",code)}
	if code:=lifecycleRequest(t,s,"POST","/auth/users/alice/reset-password","http://account.test",admin,map[string]string{"password":"new-valid-password-123","role":"admin"},false).Code;code!=http.StatusBadRequest {t.Fatalf("unknown reset fields accepted: %d",code)}
	if !errors.Is(s.auth.setAccountDisabled("admin","missing",true),errAccountNotFound){t.Fatal("missing account did not return sentinel")}
}

func TestSuspensionCancelsQueuedJobsAndBlocksAdmission(t *testing.T) {
	s:=newLifecycleFixture(t)
	admin:=lifecycleLogin(t,s,"admin","first-admin-password-strong")
	ctx,cancel:=context.WithCancel(context.Background())
	defer cancel()
	id:=strings.Repeat("f",32)
	s.mu.Lock()
	s.jobs[id]=&webJob{id:id,owner:"alice",stage:"queued",cancel:cancel}
	s.mu.Unlock()
	code:=lifecycleRequest(t,s,"PATCH","/auth/users/alice/status","http://account.test",admin,map[string]bool{"disabled":true},false).Code
	if code!=http.StatusNoContent {t.Fatalf("suspend returned %d",code)}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("outstanding suspended-user job was not cancelled")
	}
	_,status,err:=s.enqueueWebJobsForOwner([]webJobSpec{{URL:"https://anyflip.com/demo/authorized",Threads:2}},"alice")
	if status!=http.StatusForbidden || err==nil {t.Fatalf("disabled account admitted job: status=%d err=%v",status,err)}
}
