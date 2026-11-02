package main

import (
 "context"
 "crypto/sha256"
 "crypto/subtle"
 "encoding/hex"
 "encoding/json"
 "io"
 "net/http"
 "net/url"
 "strconv"
 "strings"
 "time"
)

type accountPrincipal struct { Username string; Role string }
type principalContextKey struct{}
func requestPrincipal(r *http.Request) accountPrincipal {
 p,_:=r.Context().Value(principalContextKey{}).(accountPrincipal)
 return p
}
func principalOwner(r *http.Request) string { return requestPrincipal(r).Username }
func mayAccessOwner(r *http.Request, owner string) bool {
 p:=requestPrincipal(r)
 return p.Role=="admin" || (p.Username!="" && owner==p.Username)
}
func sessionDigest(cookie string) string {
 sum:=sha256.Sum256([]byte(cookie))
 return hex.EncodeToString(sum[:])
}
func sameOriginSessionRequest(r *http.Request) bool {
 site:=r.Header.Get("Sec-Fetch-Site")
 if site!="" && site!="same-origin" && site!="none" {return false}
 origin:=r.Header.Get("Origin")
 if origin!="" {
  u,err:=url.Parse(origin)
  if err!=nil || u.Host!=r.Host || (u.Scheme!="http" && u.Scheme!="https") || u.User!=nil || u.Path!="" || u.RawQuery!="" || u.Fragment!="" {return false}
  if r.TLS!=nil && u.Scheme!="https"{return false}
 }
 if r.Method!=http.MethodGet && r.Method!=http.MethodHead {
  // Browser mutations must have same-origin metadata, not just an ambient cookie.
  if origin=="" && site!="same-origin" {return false}
 }
 return true
}
func (s *webServer) authorizeAccounts(next http.Handler) http.Handler {
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if value:=r.Header.Get("Authorization"); value!="" {
   if strings.HasPrefix(value,"Bearer ") && subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(value,"Bearer ")),[]byte(s.token))==1 {
    next.ServeHTTP(w,r.WithContext(context.WithValue(r.Context(),principalContextKey{},accountPrincipal{Username:"_service",Role:"admin"})))
    return
   }
   http.Error(w,"unauthorized",http.StatusUnauthorized)
   return
  }
  user,valid:=s.auth.userForSession(r)
  if !valid {http.Error(w,"unauthorized",http.StatusUnauthorized);return}
  next.ServeHTTP(w,r.WithContext(context.WithValue(r.Context(),principalContextKey{},accountPrincipal{Username:user.Username,Role:user.Role})))
 })
}
func (s *webServer) authLogin(w http.ResponseWriter,r *http.Request) {
 if !sameOriginSessionRequest(r) {http.Error(w,"forbidden origin",http.StatusForbidden);return}
 if s.auth.secureCookie && r.TLS==nil && !strings.HasPrefix(r.Header.Get("Origin"),"https://") {
  http.Error(w,"HTTPS required for account login",http.StatusForbidden);return
 }
 if wait:=s.auth.loginThrottle.retryAfter(r.RemoteAddr);wait>0 {
  w.Header().Set("Retry-After",strconv.Itoa(int((wait+time.Second-1)/time.Second)))
  http.Error(w,"too many login attempts",http.StatusTooManyRequests);return
 }
 r.Body=http.MaxBytesReader(w,r.Body,1024)
 var body struct {Username string `json:"username"`; Password string `json:"password"`}
 decoder:=json.NewDecoder(r.Body);decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&body);err!=nil {http.Error(w,"invalid request",http.StatusBadRequest);return}
 var extra json.RawMessage
 if err:=decoder.Decode(&extra);err!=io.EOF {http.Error(w,"invalid request",http.StatusBadRequest);return}
 user,ok:=s.auth.authenticate(body.Username,body.Password)
 if !ok {
  s.auth.loginThrottle.recordFailure(r.RemoteAddr)
  http.Error(w,"invalid username or password",http.StatusUnauthorized);return
 }
 s.auth.loginThrottle.recordSuccess(r.RemoteAddr)
 value,err:=s.auth.newSession(user);if err!=nil{http.Error(w,"session unavailable",http.StatusServiceUnavailable);return}
 s.auth.revoke(r)
 s.auth.setCookie(w,r,value)
 sendJSON(w,http.StatusOK,struct{Username string `json:"username"`;Role string `json:"role"`}{user.Username,user.Role})
}
func (s *webServer) authMe(w http.ResponseWriter,r *http.Request) {
 user,ok:=s.auth.userForSession(r)
 if !ok {http.Error(w,"unauthorized",http.StatusUnauthorized);return}
 sendJSON(w,http.StatusOK,struct{Username string `json:"username"`;Role string `json:"role"`}{user.Username,user.Role})
}
func (s *webServer) authLogout(w http.ResponseWriter,r *http.Request) {
 if !sameOriginSessionRequest(r){http.Error(w,"forbidden origin",http.StatusForbidden);return}
 s.auth.revoke(r);s.auth.clearCookie(w,r)
 w.WriteHeader(http.StatusNoContent)
}
func (s *webServer) authAddUser(w http.ResponseWriter,r *http.Request) {
 if requestPrincipal(r).Role!="admin" {http.Error(w,"forbidden",http.StatusForbidden);return}
 if !sameOriginSessionRequest(r){http.Error(w,"forbidden origin",http.StatusForbidden);return}
 r.Body=http.MaxBytesReader(w,r.Body,2048)
 var body struct {Username string `json:"username"`;Password string `json:"password"`;Role string `json:"role"`}
 decoder:=json.NewDecoder(r.Body);decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&body);err!=nil{http.Error(w,"invalid request",http.StatusBadRequest);return}
 var extra json.RawMessage
 if err:=decoder.Decode(&extra);err!=io.EOF{http.Error(w,"invalid request",http.StatusBadRequest);return}
 if err:=s.auth.addUser(body.Username,body.Password,body.Role);err!=nil {
  http.Error(w,"invalid account or username already exists",http.StatusBadRequest);return
 }
 sendJSON(w,http.StatusCreated,struct{Username string `json:"username"`;Role string `json:"role"`}{body.Username,body.Role})
}
func (s *webServer) authListUsers(w http.ResponseWriter,r *http.Request) {
 if requestPrincipal(r).Role!="admin"{http.Error(w,"forbidden",http.StatusForbidden);return}
 s.auth.mu.Lock()
 users:=make([]struct{ Username string `json:"username"`;Role string `json:"role"` },0,len(s.auth.users))
 for _,u:=range s.auth.users{users=append(users,struct{Username string `json:"username"`;Role string `json:"role"`}{u.Username,u.Role})}
 s.auth.mu.Unlock()
 sendJSON(w,http.StatusOK,struct{Users any `json:"users"`}{users})
}
