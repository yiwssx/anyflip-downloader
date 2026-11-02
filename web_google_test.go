package main

import (
 "crypto"
 "crypto/rand"
 "crypto/rsa"
 "crypto/sha256"
 "encoding/base64"
 "encoding/json"
 "math/big"
 "net/http"
 "net/http/httptest"
 "net/url"
 "strings"
 "testing"
 "time"
)

func testJWT(t *testing.T,key *rsa.PrivateKey,claim map[string]any)string{
 t.Helper()
 hdr,_:=json.Marshal(map[string]string{"alg":"RS256","kid":"test-key"})
 payload,_:=json.Marshal(claim)
 text:=base64.RawURLEncoding.EncodeToString(hdr)+"."+base64.RawURLEncoding.EncodeToString(payload)
 h:=sha256.Sum256([]byte(text))
 sig,err:=rsa.SignPKCS1v15(rand.Reader,key,crypto.SHA256,h[:]);if err!=nil{t.Fatal(err)}
 return text+"."+base64.RawURLEncoding.EncodeToString(sig)
}
func testClaims(nonce,sub,email,hd string)map[string]any{
 return map[string]any{"iss":"https://accounts.google.com","sub":sub,"aud":"test-client",
  "nonce":nonce,"email":email,"email_verified":true,"hd":hd,
  "iat":time.Now().Unix(),"exp":time.Now().Add(time.Hour).Unix()}
}
func testGoogle(t *testing.T)(*webServer,*rsa.PrivateKey,func()){
 t.Helper()
 s,err:=newWebServer(testWebToken,t.TempDir());if err!=nil{t.Fatal(err)}
 g,err:=newGoogleAuth("test-client","test-secret","http://localhost:8094/auth/google/callback")
 if err!=nil{t.Fatal(err)}
 key,err:=rsa.GenerateKey(rand.Reader,2048);if err!=nil{t.Fatal(err)}
 jwks:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  n:=base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
  e:=base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())
  sendJSON(w,200,map[string]any{"keys":[]any{map[string]string{"kty":"RSA","kid":"test-key","alg":"RS256","use":"sig","n":n,"e":e}}})
 }))
 g.jwksURL=jwks.URL
 s.google=g
 return s,key,jwks.Close
}
func TestGoogleDomainClaimsRejectSpoofing(t *testing.T){
 now:=time.Now()
 valid:=testClaims("nonce","unique-google-subject","teacher@rcat.ac.th",googleWorkspaceDomain)
 s,k,done:=testGoogle(t);defer done()
 testCases:=[]struct{name string;mutate func(map[string]any)}{
  {"wrong domain",func(c map[string]any){c["hd"]="gmail.com"}},
  {"missing hosted domain",func(c map[string]any){delete(c,"hd")}},
  {"lookalike address",func(c map[string]any){c["email"]="teacher@rcat.ac.th.attacker.example"}},
  {"personal Google account with claimed institutional email",func(c map[string]any){delete(c,"hd")}},
  {"not verified",func(c map[string]any){c["email_verified"]=false}},
  {"wrong client",func(c map[string]any){c["aud"]="other-client"}},
  {"wrong nonce",func(c map[string]any){c["nonce"]="attacker"}},
  {"expired",func(c map[string]any){c["exp"]=now.Add(-time.Minute).Unix()}},
  {"future iat",func(c map[string]any){c["iat"]=now.Add(time.Hour).Unix()}},
  {"wrong issuer",func(c map[string]any){c["iss"]="https://evil.example"}},
  {"missing subject",func(c map[string]any){delete(c,"sub")}},
  {"wrong authorized presenter",func(c map[string]any){c["azp"]="other"}},
 }
 for _,tc:=range testCases{
  t.Run(tc.name,func(t *testing.T){
   c:=testClaims("nonce","unique-google-subject","teacher@rcat.ac.th",googleWorkspaceDomain)
   tc.mutate(c)
   token:=testJWT(t,k,c)
   if _,err:=s.google.verifyToken(t.Context(),token,"nonce");err==nil{t.Fatal("untrusted identity allowed")}
  })
 }
 good:=testJWT(t,k,valid)
 u,err:=s.google.verifyToken(t.Context(),good,"nonce")
 if err!=nil||!strings.HasPrefix(u.Owner,"g-")||u.Email!="teacher@rcat.ac.th"{t.Fatalf("valid account failed: %+v %v",u,err)}
 tampered:=strings.Split(good,".")
 tampered[1]=base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"https://accounts.google.com","hd":"rcat.ac.th"}`))
 if _,err:=s.google.verifyToken(t.Context(),strings.Join(tampered,"."),"nonce");err==nil{t.Fatal("tampered signature accepted")}
}
func TestGoogleOAuthStatePKCESessionAndJobIsolation(t *testing.T){
 s,key,done:=testGoogle(t);defer done()
 handler:=s.handler()
 makeReq:=func(method,path,host,origin string,cookies ...*http.Cookie)*httptest.ResponseRecorder{
  t.Helper()
  r:=httptest.NewRequest(method,"http://"+host+path,nil)
  if origin!=""{r.Header.Set("Origin",origin)}
  for _,c:=range cookies{r.AddCookie(c)}
  w:=httptest.NewRecorder();handler.ServeHTTP(w,r);return w
 }
 start:=makeReq("GET","/auth/google","localhost:8094","")
 if start.Code!=303{t.Fatalf("start status: %d %s",start.Code,start.Body.String())}
 redirect,err:=url.Parse(start.Header().Get("Location"));if err!=nil{t.Fatal(err)}
 if redirect.Host!="accounts.google.com"||redirect.Query().Get("hd")!=googleWorkspaceDomain||redirect.Query().Get("code_challenge_method")!="S256"{t.Fatal("missing Google OAuth restrictions or PKCE")}
 cookies:=start.Result().Cookies()
 if len(cookies)!=1||cookies[0].SameSite!=http.SameSiteLaxMode||!cookies[0].HttpOnly{t.Fatal("incorrect OAuth state cookie")}
 state:=redirect.Query().Get("state")
 s.google.mu.Lock()
 pending:=s.google.pending[googleDigest(state)]
 s.google.mu.Unlock()
 s.google.tokenURL=""
 tokenServer:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Method!="POST"||r.FormValue("client_secret")!="test-secret"||r.FormValue("code_verifier")!=pending.Verifier||
   r.FormValue("redirect_uri")!="http://localhost:8094/auth/google/callback" {t.Errorf("invalid OAuth code exchange request")}
  token:=testJWT(t,key,testClaims(pending.Nonce,"persistent-subject","prof@rcat.ac.th",googleWorkspaceDomain))
  sendJSON(w,200,map[string]string{"id_token":token})
 }))
 defer tokenServer.Close()
 s.google.tokenURL=tokenServer.URL
 bad:=makeReq("GET","/auth/google/callback?state=WRONG&code=ok","localhost:8094","",cookies[0])
 if bad.Code!=403{t.Fatalf("incorrect state accepted: %d",bad.Code)}
 success:=makeReq("GET","/auth/google/callback?state="+state+"&code=test-code","localhost:8094","",cookies[0])
 if success.Code!=303||success.Header().Get("Location")!="/"{t.Fatalf("callback failed: %d %s",success.Code,success.Body.String())}
 var session *http.Cookie
 for _,c:=range success.Result().Cookies(){if c.Name==googleSessionCookie{session=c}}
 if session==nil||!session.HttpOnly||session.SameSite!=http.SameSiteStrictMode{t.Fatal("invalid user session")}
 if got:=makeReq("GET","/auth/google/callback?state="+state+"&code=test-code","localhost:8094","",cookies[0]).Code;got!=403{t.Fatalf("replay accepted: %d",got)}
 if got:=makeReq("GET","/auth/me","localhost:8094","",session).Code;got!=200{t.Fatalf("session identity rejected: %d",got)}
 if got:=makeReq("GET","/api/jobs","localhost:8094","").Code;got!=401{t.Fatalf("anonymous jobs allowed: %d",got)}
 if got:=makeReq("POST","/auth/logout","localhost:8094","http://evil.test",session).Code;got!=403{t.Fatalf("CSRF logout accepted: %d",got)}
 if got:=makeReq("POST","/auth/logout","localhost:8094","http://localhost:8094",session).Code;got!=204{t.Fatalf("logout failed: %d",got)}
 if got:=makeReq("GET","/auth/me","localhost:8094","",session).Code;got!=401{t.Fatalf("revoked session accepted: %d",got)}
 // SSO mode must never allow bypass via the legacy machine API token.
 req:=httptest.NewRequest("GET","http://localhost:8094/api/jobs",nil)
 req.Header.Set("Authorization","Bearer "+testWebToken)
 w:=httptest.NewRecorder();handler.ServeHTTP(w,req)
 if w.Code!=401{t.Fatalf("legacy Bearer bypassed Google login: %d",w.Code)}
 if !validStoredJobOwner(googleOwner("https://accounts.google.com","sub")){t.Fatal("Google owner invalid for history")}
}
func TestGoogleRejectsPlainHTTPNonlocalConfig(t *testing.T){
 if _,err:=newGoogleAuth("id","secret","http://10.10.1.100:8094/auth/google/callback");err==nil{t.Fatal("HTTP LAN sign-in allowed")}
 if _,err:=newGoogleAuth("id","secret","https://app.rcat.ac.th/evil");err==nil{t.Fatal("unknown redirect path allowed")}
 if _,err:=newGoogleAuth("id","","https://app.rcat.ac.th/auth/google/callback");err==nil{t.Fatal("missing secret allowed")}
 s,_,done:=testGoogle(t);defer done()
 w:=httptest.NewRecorder()
 s.handler().ServeHTTP(w,httptest.NewRequest("GET","http://evil.test/auth/google",nil))
 if w.Code!=403{t.Fatalf("OAuth initiated on unexpected host: %d",w.Code)}
}
