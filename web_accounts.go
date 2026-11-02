package main

import (
 "crypto/rand"
 "crypto/subtle"
 "encoding/base64"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "path/filepath"
 "regexp"
 "strings"
 "sync"
 "time"

 "golang.org/x/crypto/argon2"
)

const (
 accountsFile = ".anyflip-accounts.json"
 accountsMaxBytes = 128 << 10
 sessionCookieName = "anyflip_session"
 sessionDuration = 12 * time.Hour
)
var accountNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{2,31}$`)
type accountUser struct {
 Username string `json:"username"`
 Role string `json:"role"`
 PasswordHash string `json:"passwordHash"`
 Disabled bool `json:"disabled,omitempty"`
}
type accountsDocument struct {
 Version int `json:"version"`
 Users []accountUser `json:"users"`
}
type accountSession struct {
 Username string
 Expires time.Time
}
type accountAuth struct {
 mu sync.Mutex
 users map[string]accountUser
 sessions map[string]accountSession // keyed by SHA-256 of random cookie value
 root string
 secureCookie bool
 loginThrottle *authThrottle
 passwordThrottle *authThrottle // Per-account throttle for current-password verification.
 loginGate chan struct{} // Bound concurrent Argon2id computations per server.
}
func validAccountRole(role string) bool { return role == "admin" || role == "user" }
func validAccountUser(user accountUser) bool {
 return accountNamePattern.MatchString(user.Username) && validAccountRole(user.Role) && validPasswordHash(user.PasswordHash)
}
func validPasswordHash(encoded string) bool {
 parts := strings.Split(encoded, ":")
 if len(parts) != 4 || parts[0] != "argon2id" || parts[1] != "v1" { return false }
 salt, err := base64.RawStdEncoding.DecodeString(parts[2]); if err != nil || len(salt)!=16 {return false}
 digest, err := base64.RawStdEncoding.DecodeString(parts[3]); return err == nil && len(digest)==32
}
func hashAccountPassword(password string) (string,error) {
 if len(password)<12 || len(password)>128 { return "", errors.New("password must contain 12 to 128 bytes") }
 var salt [16]byte
 if _,err:=rand.Read(salt[:]);err!=nil{return "",err}
 digest:=argon2.IDKey([]byte(password),salt[:],2,64*1024,1,32)
 return "argon2id:v1:"+base64.RawStdEncoding.EncodeToString(salt[:])+":"+base64.RawStdEncoding.EncodeToString(digest),nil
}
func verifyAccountPassword(hash,password string) bool {
 if !validPasswordHash(hash) || len(password)>128 { return false }
 parts:=strings.Split(hash,":")
 salt,_:=base64.RawStdEncoding.DecodeString(parts[2])
 digest,_:=base64.RawStdEncoding.DecodeString(parts[3])
 actual:=argon2.IDKey([]byte(password),salt,2,64*1024,1,32)
 return subtle.ConstantTimeCompare(actual,digest)==1
}
func loadAccountAuth(root, bootstrapUsername, bootstrapHash string, secure bool) (*accountAuth,error) {
 a:=&accountAuth{root:root,secureCookie:secure,users:map[string]accountUser{},sessions:map[string]accountSession{},loginThrottle:newAuthThrottle(),passwordThrottle:newAuthThrottle(),loginGate:make(chan struct{},2)}
 path:=filepath.Join(root,accountsFile)
 info,err:=os.Lstat(path)
 if errors.Is(err,os.ErrNotExist) {
  user:=accountUser{Username:bootstrapUsername,Role:"admin",PasswordHash:bootstrapHash}
  if !validAccountUser(user) {return nil,errors.New("first account startup requires valid ANYFLIP_AUTH_BOOTSTRAP_USERNAME and ANYFLIP_AUTH_BOOTSTRAP_HASH")}
  a.users[user.Username]=user
  if err:=a.saveUsersLocked();err!=nil{return nil,fmt.Errorf("initialize accounts: %w",err)}
  return a,nil
 }
 if err!=nil{return nil,fmt.Errorf("read account store: %w",err)}
 if !info.Mode().IsRegular() || info.Mode().Perm()&0077!=0 || info.Size()>accountsMaxBytes {return nil,errors.New("unsafe account store permissions or type")}
 f,err:=os.Open(path);if err!=nil{return nil,err};defer f.Close()
 data,err:=io.ReadAll(io.LimitReader(f,accountsMaxBytes+1));if err!=nil || len(data)>accountsMaxBytes{return nil,errors.New("cannot read account store")}
 var doc accountsDocument
 if err:=json.Unmarshal(data,&doc);err!=nil || doc.Version!=1 || len(doc.Users)==0 || len(doc.Users)>200 {return nil,errors.New("invalid account store")}
 admins:=0
 for _,u:=range doc.Users {
  if !validAccountUser(u) {return nil,errors.New("invalid account record")}
  if _,ok:=a.users[u.Username];ok{return nil,errors.New("duplicate username")}
  a.users[u.Username]=u
  if u.Role=="admin" && !u.Disabled {admins++}
 }
 if admins==0{return nil,errors.New("account store requires an administrator")}
 return a,nil
}
func (a *accountAuth) saveUsersLocked() error {
 doc:=accountsDocument{Version:1}
 for _,u:=range a.users {doc.Users=append(doc.Users,u)}
 data,err:=json.Marshal(doc)
 if err!=nil || len(data)>accountsMaxBytes{return errors.New("account store too large")}
 target:=filepath.Join(a.root,accountsFile)
 if info,err:=os.Lstat(target);err==nil {
  if !info.Mode().IsRegular() || info.Mode().Perm()&0077!=0{return errors.New("unsafe account store target")}
 }else if !errors.Is(err,os.ErrNotExist){return err}
 f,err:=os.CreateTemp(a.root,".anyflip-accounts-*.tmp");if err!=nil{return err}
 defer os.Remove(f.Name())
 if err:=f.Chmod(0600);err!=nil{f.Close();return err}
 if _,err=f.Write(data);err!=nil{f.Close();return err}
 if err=f.Sync();err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 if err=os.Rename(f.Name(),target);err!=nil{return err}
 if dir,err:=os.Open(a.root);err==nil { _=dir.Sync();_=dir.Close() }
 return nil
}
func (a *accountAuth) addUser(username,password,role string) error {
 if !accountNamePattern.MatchString(username) || !validAccountRole(role){return errors.New("invalid username or role")}
 if len(password)<12 || len(password)>128{return errors.New("password must contain 12 to 128 bytes")}
 a.mu.Lock();defer a.mu.Unlock()
 if len(a.users)>=200{return errors.New("account capacity exceeded")}
 if _,exists:=a.users[username];exists{return errors.New("username already exists")}
 hash,err:=hashAccountPassword(password);if err!=nil{return err}
 a.users[username]=accountUser{Username:username,Role:role,PasswordHash:hash}
 if err:=a.saveUsersLocked();err!=nil{delete(a.users,username);return err}
 return nil
}
func (a *accountAuth) authenticate(username,password string) (accountUser,bool) {
 a.mu.Lock();user,exists:=a.users[username];a.mu.Unlock()
 // Compute equivalent Argon2id work for unknown usernames to limit user enumeration.
 if !exists {
  _,_ = hashAccountPassword("unused-password-constant")
  return accountUser{},false
 }
 if !verifyAccountPassword(user.PasswordHash,password) || user.Disabled {return accountUser{},false}
 return user,true
}
func (a *accountAuth) newSession(user accountUser) (string,error) {
 var token [32]byte
 if _,err:=rand.Read(token[:]);err!=nil{return "",err}
 value:=base64.RawURLEncoding.EncodeToString(token[:])
 a.mu.Lock();defer a.mu.Unlock()
 now:=time.Now()
 for k,s:=range a.sessions {if !s.Expires.After(now){delete(a.sessions,k)}}
 if len(a.sessions)>=2048 {return "",errors.New("too many active sessions")}
 current, exists := a.users[user.Username]
 if !exists || current.Disabled || current.PasswordHash!=user.PasswordHash { return "",errors.New("account changed during login") }
 a.sessions[sessionDigest(value)]=accountSession{Username:user.Username,Expires:now.Add(sessionDuration)}
 return value,nil
}
func (a *accountAuth) userForSession(r *http.Request) (accountUser,bool) {
 if !sameOriginSessionRequest(r) {return accountUser{},false}
 cookie,err:=r.Cookie(sessionCookieName);if err!=nil || len(cookie.Value)!=43{return accountUser{},false}
 decoded,err:=base64.RawURLEncoding.DecodeString(cookie.Value);if err!=nil || len(decoded)!=32{return accountUser{},false}
 a.mu.Lock();defer a.mu.Unlock()
 key:=sessionDigest(cookie.Value)
 session,exists:=a.sessions[key]
 if !exists {return accountUser{},false}
 if !session.Expires.After(time.Now()){delete(a.sessions,key);return accountUser{},false}
 user,exists:=a.users[session.Username]
 return user,exists && !user.Disabled
}
func (a *accountAuth) revoke(r *http.Request) {
 cookie,err:=r.Cookie(sessionCookieName)
 if err!=nil{return}
 a.mu.Lock();delete(a.sessions,sessionDigest(cookie.Value));a.mu.Unlock()
}
func (a *accountAuth) setCookie(w http.ResponseWriter,r *http.Request,value string) {
 http.SetCookie(w,&http.Cookie{Name:sessionCookieName,Value:value,Path:"/",HttpOnly:true,SameSite:http.SameSiteStrictMode,Secure:a.secureCookie || r.TLS!=nil,MaxAge:int(sessionDuration.Seconds())})
}
func (a *accountAuth) clearCookie(w http.ResponseWriter,r *http.Request) {
 http.SetCookie(w,&http.Cookie{Name:sessionCookieName,Value:"",Path:"/",HttpOnly:true,SameSite:http.SameSiteStrictMode,Secure:a.secureCookie || r.TLS!=nil,MaxAge:-1})
}
