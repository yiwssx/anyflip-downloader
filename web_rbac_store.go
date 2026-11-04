package main

import (
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "regexp"
 "sort"
 "strings"
 "sync"
 "time"
)

const (
 rbacFilename = ".anyflip-rbac.json"
 rbacStoreLimit = 256 << 10
 rbacMaxMembers = 1000
 roleUser = "user"
 roleAdmin = "admin"
 roleSuperAdmin = "super_admin"
)

var rbacEmailPattern = regexp.MustCompile("(?i)^[^\\s@]{1,120}@rcat\\.ac\\.th$")

type rbacMember struct {
 ID string
 Email string
 Role string
 Disabled bool
 LastSeen time.Time
}
type rbacDocument struct {
 Version int
 Members []rbacMember
}
type rbacStore struct {
 mu sync.Mutex
 root string
 path string
 members map[string]rbacMember
}

func validRBACRole(role string) bool {
 return role==roleUser || role==roleAdmin || role==roleSuperAdmin
}

func validateRBACRoot(root string) error {
 info,err:=os.Lstat(root)
 if err!=nil{return fmt.Errorf("inspect private RBAC directory: %w",err)}
 if !info.IsDir() || info.Mode().Perm()&0077!=0 {
  return errors.New("RBAC directory requires private mode 0700")
 }
 return nil
}

func validateRBACFile(info os.FileInfo) error {
 if !info.Mode().IsRegular() || info.Mode().Perm()!=0600 || info.Size()>rbacStoreLimit {
  return errors.New("RBAC store requires a private 0600 regular file")
 }
 return nil
}

func validateRBACDocument(doc rbacDocument) error {
 if doc.Version!=1 || len(doc.Members)==0 || len(doc.Members)>rbacMaxMembers {return errors.New("invalid RBAC version or size")}
 seen:=make(map[string]bool,len(doc.Members))
 superadmins:=0
 for _,m:=range doc.Members {
  if !storedGoogleOwnerPattern.MatchString(m.ID) || seen[m.ID] ||
   !validRBACRole(m.Role) || len(m.Email)>140 ||
   (m.Email!="" && !rbacEmailPattern.MatchString(m.Email)) {
   return errors.New("invalid RBAC member")
  }
  seen[m.ID]=true
  if m.Role==roleSuperAdmin && !m.Disabled {superadmins++}
 }
 if superadmins<1{return errors.New("no active super admin")}
 return nil
}

func readRBACDocument(root string) (rbacDocument,error) {
 var doc rbacDocument
 if err:=validateRBACRoot(root);err!=nil{return doc,err}
 path:=filepath.Join(root,rbacFilename)
 info,err:=os.Lstat(path)
 if err!=nil{return doc,fmt.Errorf("RBAC store missing or inaccessible: %w",err)}
 if err:=validateRBACFile(info);err!=nil{return doc,err}
 f,err:=os.Open(path)
 if err!=nil{return doc,err}
 defer f.Close()
 body,err:=io.ReadAll(io.LimitReader(f,rbacStoreLimit+1))
 if err!=nil || len(body)>rbacStoreLimit{return doc,errors.New("RBAC store unreadable or oversized")}
 if err:=json.Unmarshal(body,&doc);err!=nil{return doc,errors.New("invalid RBAC store JSON")}
 if err:=validateRBACDocument(doc);err!=nil{return doc,err}
 return doc,nil
}

func openRBACStore(root string) (*rbacStore,error) {
 doc,err:=readRBACDocument(root)
 if err!=nil{return nil,err}
 if _,err:=readRBACAuditFile(filepath.Join(root,rbacAuditFilename));err!=nil{return nil,err}
 if _,err:=readRBACAuditFile(filepath.Join(root,rbacAuditFilename)+".1");err!=nil{return nil,err}
 s:=&rbacStore{root:root,path:filepath.Join(root,rbacFilename),members:make(map[string]rbacMember,len(doc.Members))}
 for _,m:=range doc.Members{s.members[m.ID]=m}
 return s,nil
}

// Refresh the authoritative file before any mutation or directory read.
// An operator restore or another writer must never be silently overwritten by
// a stale snapshot retained by this process.
func (s *rbacStore) reloadMembersLocked() error {
 doc,err:=readRBACDocument(s.root)
 if err!=nil{return err}
 fresh:=make(map[string]rbacMember,len(doc.Members))
 for _,member:=range doc.Members{fresh[member.ID]=member}
 s.members=fresh
 return nil
}

func encodeRBAC(members map[string]rbacMember) ([]byte,error) {
 doc:=rbacDocument{Version:1,Members:make([]rbacMember,0,len(members))}
 for _,m:=range members{doc.Members=append(doc.Members,m)}
 sort.Slice(doc.Members,func(i,j int)bool{return doc.Members[i].ID<doc.Members[j].ID})
 if err:=validateRBACDocument(doc);err!=nil{return nil,err}
 b,err:=json.MarshalIndent(doc,"","  ")
 if err!=nil || len(b)>rbacStoreLimit{return nil,errors.New("RBAC serialization limit exceeded")}
 return append(b,'\n'),nil
}

func writeRBACAtomic(root,path string,body []byte) error {
 if err:=validateRBACRoot(root);err!=nil{return err}
 if info,err:=os.Lstat(path);err==nil {
  if err:=validateRBACFile(info);err!=nil{return err}
 }else if !errors.Is(err,os.ErrNotExist){return err}
 f,err:=os.CreateTemp(root,".anyflip-rbac-*.tmp")
 if err!=nil{return err}
 defer os.Remove(f.Name())
 defer f.Close()
 if err:=f.Chmod(0600);err!=nil{return err}
 if _,err:=f.Write(body);err!=nil{return err}
 if err:=f.Sync();err!=nil{return err}
 if err:=f.Close();err!=nil{return err}
 if err:=os.Rename(f.Name(),path);err!=nil{return err}
 if dir,err:=os.Open(root);err==nil{_ = dir.Sync();_ = dir.Close()}
 return nil
}

// Only an operator can invoke bootstrap through the CLI. The owner reference
// must first have been obtained from an authenticated Google OIDC /auth/me.
// Never grant this role on first login; never replace an existing store.
func bootstrapRBACStore(root,identityRef string) error {
 if !storedGoogleOwnerPattern.MatchString(identityRef){return errors.New("invalid verified Workspace identity reference")}
 if err:=validateRBACRoot(root);err!=nil{return err}
 body,err:=encodeRBAC(map[string]rbacMember{
  identityRef:{ID:identityRef,Role:roleSuperAdmin},
 })
 if err!=nil{return err}
 f,err:=os.CreateTemp(root,".anyflip-bootstrap-*.tmp")
 if err!=nil{return err}
 defer os.Remove(f.Name())
 if err:=f.Chmod(0600);err!=nil{f.Close();return err}
 if _,err:=f.Write(body);err!=nil{f.Close();return err}
 if err:=f.Sync();err!=nil{f.Close();return err}
 if err:=f.Close();err!=nil{return err}
 // A hard link creates the initial file atomically and fails if it exists.
 path:=filepath.Join(root,rbacFilename)
 if err:=os.Link(f.Name(),path);err!=nil{return fmt.Errorf("bootstrap refuses existing or unsafe RBAC store: %w",err)}
 if dir,err:=os.Open(root);err==nil{_ = dir.Sync();_ = dir.Close()}
 return nil
}

// Resolve from registry on every request. Sessions never cache privileges.
func (s *rbacStore) currentRole(identityRef string)(string,bool) {
 s.mu.Lock();defer s.mu.Unlock()
 // Fail closed on post-startup file corruption, unsafe permissions or removal.
 doc,err:=readRBACDocument(s.root)
 if err!=nil{return "",false}
 for _,m:=range doc.Members{
  if m.ID==identityRef{return m.Role,!m.Disabled && validRBACRole(m.Role)}
 }
 return "",false
}

func (s *rbacStore) snapshotMembers() ([]rbacMember,error) {
 s.mu.Lock();defer s.mu.Unlock()
 if err:=s.reloadMembersLocked();err!=nil{return nil,err}
 result:=make([]rbacMember,0,len(s.members))
 for _,m:=range s.members{result=append(result,m)}
 sort.Slice(result,func(i,j int)bool{return result[i].ID<result[j].ID})
 return result,nil
}

func (s *rbacStore) persistLocked(members map[string]rbacMember)error{
 body,err:=encodeRBAC(members);if err!=nil{return err}
 // Reject an unexpected on-disk change between the mutation's initial
 // read and its commit, instead of clobbering a newer operator restore.
 doc,err:=readRBACDocument(s.root)
 if err!=nil{return fmt.Errorf("RBAC primary was modified unsafely: %w",err)}
 if len(doc.Members)!=len(s.members){return errors.New("RBAC primary changed during operation")}
 for _,member:=range doc.Members{
  previous,ok:=s.members[member.ID]
  if !ok || previous.ID!=member.ID || previous.Email!=member.Email ||
   previous.Role!=member.Role || previous.Disabled!=member.Disabled ||
   !previous.LastSeen.Equal(member.LastSeen) {
   return errors.New("RBAC primary changed during operation")
  }
 }
 // Keep previous complete image, not an automatically trusted recovery source.
 old,err:=os.ReadFile(s.path)
 if err!=nil{return err}
 if err:=writeRBACAtomic(s.root,s.path+".bak",old);err!=nil{return fmt.Errorf("RBAC backup: %w",err)}
 if err:=writeRBACAtomic(s.root,s.path,body);err!=nil{return err}
 s.members=members
 return nil
}

// Only call after the Google signature, issuer, audience, hosted domain,
// email_verified, nonce and expiry have been verified server-side.
func (s *rbacStore) registerVerifiedUser(u googleUser)error {
 if !storedGoogleOwnerPattern.MatchString(u.Owner) || !rbacEmailPattern.MatchString(u.Email) {
  return errors.New("unverified Workspace identity")
 }
 s.mu.Lock();defer s.mu.Unlock()
 if err:=s.reloadMembersLocked();err!=nil{return err}
 current,exists:=s.members[u.Owner]
 if exists && current.Disabled{return errors.New("disabled Workspace role")}
 if !exists && len(s.members)>=rbacMaxMembers{return errors.New("RBAC user limit")}
 next:=make(map[string]rbacMember,len(s.members)+1)
 for id,m:=range s.members{next[id]=m}
 if !exists{current=rbacMember{ID:u.Owner,Role:roleUser}}
 current.Email=strings.ToLower(u.Email)
 current.LastSeen=time.Now().UTC()
 next[u.Owner]=current
 return s.persistLocked(next)
}

// This is a store-level primitive. RBAC-3 must add audited HTTP authorization
// before exposing role changes to administrators.
func (s *rbacStore) changeRole(actor,target,role string)error{
 if !validRBACRole(role){return errors.New("unknown role")}
 s.mu.Lock();defer s.mu.Unlock()
 if err:=s.reloadMembersLocked();err!=nil{return err}
 admin,ok:=s.members[actor]
 if !ok || admin.Disabled || admin.Role!=roleSuperAdmin{return errors.New("super admin required")}
 m,exists:=s.members[target]
 if !exists || m.Disabled{return errors.New("target unavailable")}
 if actor==target && role!=roleSuperAdmin{return errors.New("self demotion prohibited")}
 if m.Role==role{return nil}
 active:=0
 for _,member:=range s.members{if member.Role==roleSuperAdmin && !member.Disabled{active++}}
 if m.Role==roleSuperAdmin && role!=roleSuperAdmin && active<2{return errors.New("last super admin protected")}
 next:=make(map[string]rbacMember,len(s.members))
 for id,member:=range s.members{next[id]=member}
 m.Role=role;next[target]=m
 // Write the sanitized intent before a critical privilege mutation.
 // If audit storage fails, the role does not change.
 if err:=s.auditLocked(rbacAuditEvent{At:time.Now().UTC(),Event:"role_change_intent",Actor:actor,Target:target,OldRole:s.members[target].Role,NewRole:role});err!=nil{return fmt.Errorf("RBAC audit failure: %w",err)}
 return s.persistLocked(next)
}
