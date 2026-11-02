package main

import (
 "context"
 "errors"
 "sort"
 "crypto/sha256"
 "crypto/subtle"
 "encoding/hex"
 "encoding/json"
 "io"
 "log"
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
// Require an intent audit record before security-sensitive state changes.
// State persistence and file-based audit are separate durability domains; the
// follow-up outcome is best-effort, and any IO failure is logged server-side.
func (s *webServer) requireAuthAudit(w http.ResponseWriter,event,actor,target string) bool {
 if err:=s.auth.audit.record(event,actor,target,"intent");err!=nil {
  log.Printf("authentication audit unavailable (event=%s)",event)
  http.Error(w,"authentication audit unavailable",http.StatusServiceUnavailable)
  return false
 }
 return true
}
func (s *webServer) authAuditResult(event,actor,target,result string) {
 if err:=s.auth.audit.record(event,actor,target,result);err!=nil {
  log.Printf("authentication audit result could not be written (event=%s)",event)
 }
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
 select {
 case s.auth.loginGate <- struct{}{}:
  defer func(){ <-s.auth.loginGate }()
 default:
  http.Error(w,"login temporarily busy",http.StatusTooManyRequests);return
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
  s.authAuditResult("login",body.Username,"","denied")
  http.Error(w,"invalid username or password",http.StatusUnauthorized);return
 }
 if !s.requireAuthAudit(w,"login",body.Username,""){return}
 value,err:=s.auth.newSession(user);if err!=nil{http.Error(w,"session unavailable",http.StatusServiceUnavailable);return}
 s.auth.loginThrottle.recordSuccess(r.RemoteAddr)
 s.auth.revoke(r)
 s.auth.setCookie(w,r,value)
 s.authAuditResult("login",body.Username,"","ok")
 sendJSON(w,http.StatusOK,struct{Username string `json:"username"`;Role string `json:"role"`}{user.Username,user.Role})
}
func (s *webServer) authMe(w http.ResponseWriter,r *http.Request) {
 user,ok:=s.auth.userForSession(r)
 if !ok {http.Error(w,"unauthorized",http.StatusUnauthorized);return}
 sendJSON(w,http.StatusOK,struct{Username string `json:"username"`;Role string `json:"role"`}{user.Username,user.Role})
}
func (s *webServer) authLogout(w http.ResponseWriter,r *http.Request) {
 if !sameOriginSessionRequest(r){http.Error(w,"forbidden origin",http.StatusForbidden);return}
 actor:="unknown"
 if user,ok:=s.auth.userForSession(r);ok{actor=user.Username}
 s.auth.revoke(r);s.auth.clearCookie(w,r)
 s.authAuditResult("logout",actor,"","ok")
 w.WriteHeader(http.StatusNoContent)
}
func (s *webServer) authAddUser(w http.ResponseWriter,r *http.Request) {
 if !isHumanAdmin(r) {http.Error(w,"forbidden",http.StatusForbidden);return}
 if !sameOriginSessionRequest(r){http.Error(w,"forbidden origin",http.StatusForbidden);return}
 r.Body=http.MaxBytesReader(w,r.Body,2048)
 var body struct {Username string `json:"username"`;Password string `json:"password"`;Role string `json:"role"`}
 decoder:=json.NewDecoder(r.Body);decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&body);err!=nil{http.Error(w,"invalid request",http.StatusBadRequest);return}
 var extra json.RawMessage
 if err:=decoder.Decode(&extra);err!=io.EOF{http.Error(w,"invalid request",http.StatusBadRequest);return}
 actor:=requestPrincipal(r).Username
 if !s.requireAuthAudit(w,"create_user",actor,body.Username){return}
 if err:=s.auth.addUser(body.Username,body.Password,body.Role);err!=nil {
  s.authAuditResult("create_user",actor,body.Username,"failed")
  http.Error(w,"invalid account or username already exists",http.StatusBadRequest);return
 }
 s.authAuditResult("create_user",actor,body.Username,"ok")
 sendJSON(w,http.StatusCreated,struct{Username string `json:"username"`;Role string `json:"role"`}{body.Username,body.Role})
}
func (s *webServer) authListUsers(w http.ResponseWriter,r *http.Request) {
 if !isHumanAdmin(r){http.Error(w,"forbidden",http.StatusForbidden);return}
 s.auth.mu.Lock()
 type safeUser struct { Username string `json:"username"`; Role string `json:"role"`; Disabled bool `json:"disabled"` }
 users:=make([]safeUser,0,len(s.auth.users))
 for _,u:=range s.auth.users{users=append(users,safeUser{u.Username,u.Role,u.Disabled})}
 s.auth.mu.Unlock()
 sort.Slice(users,func(i,j int)bool{return users[i].Username<users[j].Username})
 sendJSON(w,http.StatusOK,struct{Users any `json:"users"`}{users})
}

func isHumanAdmin(r *http.Request) bool {
 p:=requestPrincipal(r)
 return p.Role=="admin" && p.Username!="" && p.Username!="_service"
}
func claimAccountHashSlot(w http.ResponseWriter, a *accountAuth) bool {
 select {
 case a.loginGate <- struct{}{}:
  return true
 default:
  http.Error(w,"authentication temporarily busy",http.StatusTooManyRequests)
  return false
 }
}
func decodeAccountMutation(w http.ResponseWriter, r *http.Request, limit int64, dest any) bool {
 r.Body=http.MaxBytesReader(w,r.Body,limit)
 decoder:=json.NewDecoder(r.Body)
 decoder.DisallowUnknownFields()
 if err:=decoder.Decode(dest);err!=nil{
  http.Error(w,"invalid request",http.StatusBadRequest);return false
 }
 var extra json.RawMessage
 if err:=decoder.Decode(&extra);err!=io.EOF{
  http.Error(w,"invalid request",http.StatusBadRequest);return false
 }
 return true
}
func respondAccountMutationError(w http.ResponseWriter,err error) {
 switch {
 case errors.Is(err,errAccountNotFound):
  http.Error(w,"account not found",http.StatusNotFound)
 case errors.Is(err,errBadCurrentPassword):
  http.Error(w,"current password incorrect",http.StatusForbidden)
 case errors.Is(err,errLastActiveAdmin),errors.Is(err,errSelfDisable),errors.Is(err,errAccountConflict):
  http.Error(w,"account change not permitted",http.StatusConflict)
 default:
  // Other input errors should not reveal storage paths or credential hashes.
  http.Error(w,"account change failed",http.StatusBadRequest)
 }
}
func (s *webServer) authChangePassword(w http.ResponseWriter,r *http.Request) {
 p:=requestPrincipal(r)
 if p.Username=="" || p.Username=="_service" {http.Error(w,"forbidden",http.StatusForbidden);return}
 if !sameOriginSessionRequest(r) {http.Error(w,"forbidden",http.StatusForbidden);return}
 peer:=p.Username+":0"
 if wait:=s.auth.passwordThrottle.retryAfter(peer);wait>0{
  w.Header().Set("Retry-After",strconv.Itoa(int((wait+time.Second-1)/time.Second)))
  http.Error(w,"too many password attempts",http.StatusTooManyRequests)
  return
 }
 var body struct{CurrentPassword string `json:"currentPassword"`;NewPassword string `json:"newPassword"`}
 if !decodeAccountMutation(w,r,1024,&body){return}
 if !claimAccountHashSlot(w,s.auth){return}
 defer func(){<-s.auth.loginGate}()
 if !s.requireAuthAudit(w,"change_password",p.Username,p.Username){return}
 err:=s.auth.changeOwnPassword(p.Username,body.CurrentPassword,body.NewPassword)
 if err!=nil{
  s.authAuditResult("change_password",p.Username,p.Username,"failed")
  if errors.Is(err,errBadCurrentPassword){s.auth.passwordThrottle.recordFailure(peer)}
  respondAccountMutationError(w,err)
  return
 }
 s.auth.passwordThrottle.recordSuccess(peer)
 s.authAuditResult("change_password",p.Username,p.Username,"ok")
 s.auth.clearCookie(w,r)
 w.WriteHeader(http.StatusNoContent)
}
func (s *webServer) authResetPassword(w http.ResponseWriter,r *http.Request) {
 if !isHumanAdmin(r){http.Error(w,"forbidden",http.StatusForbidden);return}
 if !sameOriginSessionRequest(r){http.Error(w,"forbidden",http.StatusForbidden);return}
 target:=r.PathValue("username")
 if !accountNamePattern.MatchString(target){http.NotFound(w,r);return}
 var body struct{Password string `json:"password"`}
 if !decodeAccountMutation(w,r,1024,&body){return}
 if !claimAccountHashSlot(w,s.auth){return}
 defer func(){<-s.auth.loginGate}()
 actor:=requestPrincipal(r).Username
 if !s.requireAuthAudit(w,"reset_password",actor,target){return}
 if err:=s.auth.resetAccountPassword(actor,target,body.Password);err!=nil{
  s.authAuditResult("reset_password",actor,target,"failed")
  respondAccountMutationError(w,err);return
 }
 s.authAuditResult("reset_password",actor,target,"ok")
 w.WriteHeader(http.StatusNoContent)
}
func (s *webServer) authUpdateUserStatus(w http.ResponseWriter,r *http.Request) {
 if !isHumanAdmin(r){http.Error(w,"forbidden",http.StatusForbidden);return}
 if !sameOriginSessionRequest(r){http.Error(w,"forbidden",http.StatusForbidden);return}
 target:=r.PathValue("username")
 if !accountNamePattern.MatchString(target){http.NotFound(w,r);return}
 var body struct{Disabled *bool `json:"disabled"`}
 if !decodeAccountMutation(w,r,1024,&body){return}
 if body.Disabled==nil{http.Error(w,"disabled field required",http.StatusBadRequest);return}
 actor:=requestPrincipal(r).Username
 event:="enable_user"
 if *body.Disabled{event="suspend_user"}
 if !s.requireAuthAudit(w,event,actor,target){return}
 if err:=s.auth.setAccountDisabled(actor,target,*body.Disabled);err!=nil{
  s.authAuditResult(event,actor,target,"failed")
  respondAccountMutationError(w,err);return
 }
 s.authAuditResult(event,actor,target,"ok")
 if *body.Disabled {
  // Suspend outstanding jobs owned by this user without deleting their files
  // or archived history. This locks against concurrent queue admission.
  s.mu.Lock()
  for _, job := range s.jobs {
   if job.owner==target && job.cancel!=nil && job.finished.IsZero() {
    job.cancel()
   }
  }
  s.mu.Unlock()
 }
 w.WriteHeader(http.StatusNoContent)
}
