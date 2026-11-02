package main

import (
 "context"
 "crypto/rand"
 "crypto/rsa"
 "crypto/sha256"
 "crypto/subtle"
 "crypto"
 "encoding/base64"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "math/big"
 "net"
 "net/http"
 "net/url"
 "strings"
 "sync"
 "time"
)

const googleWorkspaceDomain="rcat.ac.th"
const googleSessionCookie="anyflip_session"
const googleStateCookie="anyflip_google_state"
const googleAuthTTL=10*time.Minute
const googleSessionTTL=12*time.Hour

type googleUser struct {
 Owner string
 Email string
}
type googlePending struct {
 Nonce string
 Verifier string
 Expires time.Time
}
type googleSession struct {
 User googleUser
 Expires time.Time
}
type googleAuth struct {
 mu sync.Mutex
 clientID,clientSecret,redirectURI string
 httpClient *http.Client
 authURL,tokenURL,jwksURL string
 sessions map[string]googleSession
 pending map[string]googlePending
 keys map[string]*rsa.PublicKey
 keysExpiry time.Time
}

func newGoogleAuth(clientID,clientSecret,redirectURI string)(*googleAuth,error){
 u,err:=url.Parse(redirectURI)
 if err!=nil||u.User!=nil||u.Fragment!=""||u.RawQuery!=""||u.Path!="/auth/google/callback"||u.Host=="" {
  return nil,errors.New("invalid GOOGLE_REDIRECT_URI (expected /auth/google/callback)")
 }
 if u.Scheme!="https"{
  host,_,err:=net.SplitHostPort(u.Host)
  if err!=nil {host=u.Hostname()}
  if u.Scheme!="http" || (host!="localhost" && host!="127.0.0.1" && host!="::1"){
   return nil,errors.New("Google login requires an HTTPS redirect URI (except localhost development)")
  }
 }
 if clientID==""||clientSecret=="" {return nil,errors.New("GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET are required")}
 return &googleAuth{
  clientID:clientID,clientSecret:clientSecret,redirectURI:redirectURI,
  authURL:"https://accounts.google.com/o/oauth2/v2/auth",
  tokenURL:"https://oauth2.googleapis.com/token",
  jwksURL:"https://www.googleapis.com/oauth2/v3/certs",
  httpClient:&http.Client{Timeout:10*time.Second},
  sessions:map[string]googleSession{},pending:map[string]googlePending{},
 },nil
}
func randomGoogleToken()(string,error){
 var b [32]byte
 if _,err:=rand.Read(b[:]);err!=nil{return "",err}
 return base64.RawURLEncoding.EncodeToString(b[:]),nil
}
func googleDigest(s string)string {
 h:=sha256.Sum256([]byte(s))
 return base64.RawURLEncoding.EncodeToString(h[:])
}
func googleOwner(issuer,sub string) string{
 h:=sha256.Sum256([]byte(issuer+"|"+sub))
 return "g-"+fmt.Sprintf("%x",h[:])
}
func googleAllowedIdentity(issuer,sub,audience,nonce,email,hostedDomain string, verified bool,exp,iat int64,clientID,expectedNonce string) (googleUser,error) {
 now:=time.Now().Unix()
 if (issuer!="https://accounts.google.com"&&issuer!="accounts.google.com") ||
  sub==""||len(sub)>255||audience!=clientID||nonce==""||subtle.ConstantTimeCompare([]byte(nonce),[]byte(expectedNonce))!=1 ||
  !verified||hostedDomain!=googleWorkspaceDomain||!strings.HasSuffix(strings.ToLower(email),"@"+googleWorkspaceDomain)||
  exp<=now||iat>now+60||iat<=0||iat<now-3600 {
  return googleUser{},errors.New("Google Workspace identity denied")
 }
 return googleUser{Owner:googleOwner("https://accounts.google.com",sub),Email:email},nil
}
func (g *googleAuth) setCookie(w http.ResponseWriter,name,value string,ttl time.Duration, lax bool){
 mode:=http.SameSiteStrictMode
 if lax {mode=http.SameSiteLaxMode}
 secure:=strings.HasPrefix(g.redirectURI,"https://")
 http.SetCookie(w,&http.Cookie{Name:name,Value:value,Path:"/",HttpOnly:true,Secure:secure,SameSite:mode,MaxAge:int(ttl.Seconds())})
}
func (g *googleAuth) deleteCookie(w http.ResponseWriter,name string,lax bool) {
 g.setCookie(w,name,"",-time.Second,lax)
}
func (g *googleAuth) start(w http.ResponseWriter,r *http.Request){
 state,err:=randomGoogleToken();if err!=nil{http.Error(w,"sign-in unavailable",503);return}
 nonce,err:=randomGoogleToken();if err!=nil{http.Error(w,"sign-in unavailable",503);return}
 verifier,err:=randomGoogleToken();if err!=nil{http.Error(w,"sign-in unavailable",503);return}
 g.mu.Lock()
 now:=time.Now()
 for id,p:=range g.pending{if !p.Expires.After(now){delete(g.pending,id)}}
 if len(g.pending)>=512{g.mu.Unlock();http.Error(w,"sign-in temporarily busy",429);return}
 g.pending[googleDigest(state)]=googlePending{Nonce:nonce,Verifier:verifier,Expires:now.Add(googleAuthTTL)}
 g.mu.Unlock()
 g.setCookie(w,googleStateCookie,state,googleAuthTTL,true)
 u,_:=url.Parse(g.authURL)
 q:=u.Query()
 q.Set("client_id",g.clientID)
 q.Set("redirect_uri",g.redirectURI)
 q.Set("response_type","code")
 q.Set("scope","openid email")
 q.Set("state",state)
 q.Set("nonce",nonce)
 q.Set("hd",googleWorkspaceDomain) // UI hint ONLY; token claims are verified on callback.
 q.Set("code_challenge",googleDigest(verifier))
 q.Set("code_challenge_method","S256")
 q.Set("prompt","select_account")
 u.RawQuery=q.Encode()
 http.Redirect(w,r,u.String(),http.StatusSeeOther)
}
func (g *googleAuth) callback(w http.ResponseWriter,r *http.Request){
 stateCookie,err:=r.Cookie(googleStateCookie)
 g.deleteCookie(w,googleStateCookie,true)
 if err!=nil||len(stateCookie.Value)!=43{http.Error(w,"invalid sign-in state",403);return}
 state:=r.URL.Query().Get("state")
 if len(state)!=43||subtle.ConstantTimeCompare([]byte(state),[]byte(stateCookie.Value))!=1 {
  http.Error(w,"invalid sign-in state",403);return
 }
 g.mu.Lock()
 pending,exists:=g.pending[googleDigest(state)]
 delete(g.pending,googleDigest(state)) // one-time use; cannot replay OAuth callback
 g.mu.Unlock()
 if !exists||!pending.Expires.After(time.Now()){http.Error(w,"expired sign-in state",403);return}
 code:=r.URL.Query().Get("code")
 if code==""||len(code)>4096||r.URL.Query().Get("error")!=""{http.Error(w,"Google sign-in cancelled",403);return}
 idToken,err:=g.exchangeCode(r.Context(),code,pending.Verifier)
 if err!=nil{http.Error(w,"Google sign-in failed",401);return}
 user,err:=g.verifyToken(r.Context(),idToken,pending.Nonce)
 if err!=nil{http.Error(w,"account not permitted",403);return}
 session,err:=randomGoogleToken()
 if err!=nil{http.Error(w,"sign-in unavailable",503);return}
 g.mu.Lock()
 now:=time.Now()
 for k,v:=range g.sessions{if !v.Expires.After(now){delete(g.sessions,k)}}
 if len(g.sessions)>=2048{g.mu.Unlock();http.Error(w,"too many sessions",503);return}
 g.sessions[googleDigest(session)]=googleSession{User:user,Expires:now.Add(googleSessionTTL)}
 g.mu.Unlock()
 g.setCookie(w,googleSessionCookie,session,googleSessionTTL,false)
 http.Redirect(w,r,"/",http.StatusSeeOther)
}
func (g *googleAuth) exchangeCode(ctx context.Context,code,verifier string)(string,error){
 body:=url.Values{
  "client_id":{g.clientID},"client_secret":{g.clientSecret},
  "redirect_uri":{g.redirectURI},"grant_type":{"authorization_code"},
  "code":{code},"code_verifier":{verifier},
 }
 req,err:=http.NewRequestWithContext(ctx,"POST",g.tokenURL,strings.NewReader(body.Encode()))
 if err!=nil{return "",err}
 req.Header.Set("Content-Type","application/x-www-form-urlencoded")
 resp,err:=g.httpClient.Do(req);if err!=nil{return "",err}
 defer resp.Body.Close()
 if resp.StatusCode!=200{return "",errors.New("OAuth code exchange denied")}
 var reply struct{IDToken string `json:"id_token"`}
 if err:=json.NewDecoder(io.LimitReader(resp.Body,1<<20)).Decode(&reply);err!=nil{return "",err}
 if reply.IDToken==""||len(reply.IDToken)>16384{return "",errors.New("no valid ID token")}
 return reply.IDToken,nil
}
func (g *googleAuth) sessionUser(r *http.Request)(googleUser,bool){
 if !sameOriginGoogleRequest(r){return googleUser{},false}
 cookie,err:=r.Cookie(googleSessionCookie)
 if err!=nil||len(cookie.Value)!=43{return googleUser{},false}
 g.mu.Lock();defer g.mu.Unlock()
 s,exists:=g.sessions[googleDigest(cookie.Value)]
 if !exists || !s.Expires.After(time.Now()){delete(g.sessions,googleDigest(cookie.Value));return googleUser{},false}
 return s.User,true
}
func sameOriginGoogleRequest(r *http.Request)bool{
 site:=r.Header.Get("Sec-Fetch-Site")
 if site!=""&&site!="same-origin"&&site!="none"{return false}
 origin:=r.Header.Get("Origin")
 if origin!=""{
  u,err:=url.Parse(origin)
  if err!=nil||u.Host!=r.Host||u.User!=nil||u.Path!=""||u.RawQuery!=""||u.Fragment!=""||(u.Scheme!="http"&&u.Scheme!="https"){return false}
  if r.TLS!=nil&&u.Scheme!="https"{return false}
 }
 if r.Method!="GET"&&r.Method!="HEAD"&&origin==""&&site!="same-origin"{return false}
 return true
}
func (g *googleAuth) logout(w http.ResponseWriter,r *http.Request){
 if !sameOriginGoogleRequest(r){http.Error(w,"forbidden",403);return}
 if c,err:=r.Cookie(googleSessionCookie);err==nil{
  g.mu.Lock();delete(g.sessions,googleDigest(c.Value));g.mu.Unlock()
 }
 g.deleteCookie(w,googleSessionCookie,false)
 w.WriteHeader(http.StatusNoContent)
}
func (g *googleAuth) me(w http.ResponseWriter,r *http.Request){
 user,ok:=g.sessionUser(r)
 if !ok{http.Error(w,"unauthorized",401);return}
 sendJSON(w,200,struct{Email string `json:"email"`}{user.Email})
}
