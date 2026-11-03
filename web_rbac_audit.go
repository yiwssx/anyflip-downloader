package main

import (
 "bufio"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "sync"
 "time"
)

const (
 rbacAuditFilename = ".anyflip-rbac-audit.jsonl"
 rbacAuditMaxBytes = 1 << 20
)
type rbacAuditEvent struct {
 At time.Time `json:"at"`
 Event string `json:"event"`
 Actor string `json:"actor"`
 Target string `json:"target,omitempty"`
 Reason string `json:"reason,omitempty"`
 OldRole string `json:"oldRole,omitempty"`
 NewRole string `json:"newRole,omitempty"`
}
func validRBACAuditEvent(e rbacAuditEvent)bool{
 if e.At.IsZero() || !storedGoogleOwnerPattern.MatchString(e.Actor) {return false}
 if e.Target!="" && !storedGoogleOwnerPattern.MatchString(e.Target){return false}
 switch e.Event{
 case "role_change_intent":
  if !validRBACRole(e.OldRole) || !validRBACRole(e.NewRole){return false}
 case "job_cancel","role_change_denied":
  if e.OldRole!=""||e.NewRole!=""{return false}
 default:return false
 }
 switch e.Reason{
 case "","policy","resources","duplicate","other":
 default:return false
 }
 return true
}
func readRBACAuditFile(path string)([]rbacAuditEvent,error){
 info,err:=os.Lstat(path)
 if errors.Is(err,os.ErrNotExist){return nil,nil}
 if err!=nil{return nil,err}
 if !info.Mode().IsRegular() || info.Mode().Perm()!=0600 || info.Size()>rbacAuditMaxBytes {
  return nil,errors.New("unsafe RBAC audit file")
 }
 f,err:=os.Open(path);if err!=nil{return nil,err}
 defer f.Close()
 b,err:=io.ReadAll(io.LimitReader(f,rbacAuditMaxBytes+1))
 if err!=nil||len(b)>rbacAuditMaxBytes{return nil,errors.New("oversized RBAC audit")}
 var events []rbacAuditEvent
 scanner:=bufio.NewScanner(bytesReader(b))
 scanner.Buffer(make([]byte,4096),16<<10)
 for scanner.Scan(){
  var e rbacAuditEvent
  if err:=json.Unmarshal(scanner.Bytes(),&e);err!=nil||!validRBACAuditEvent(e){return nil,errors.New("corrupt RBAC audit")}
  events=append(events,e)
 }
 if err:=scanner.Err();err!=nil{return nil,err}
 if len(b)>0 && b[len(b)-1]!='\n'{return nil,errors.New("truncated RBAC audit")}
 return events,nil
}
// byteReader avoids an import dependency from the HTTP layer.
func bytesReader(b []byte)io.Reader{return &readOnceBytes{b:b}}
type readOnceBytes struct{b []byte}
func (r *readOnceBytes)Read(out []byte)(int,error){
 if len(r.b)==0{return 0,io.EOF}
 n:=copy(out,r.b);r.b=r.b[n:];return n,nil
}

func (s *rbacStore) auditLocked(event rbacAuditEvent)error{
 if !validRBACAuditEvent(event){return errors.New("invalid RBAC audit event")}
 if err:=validateRBACRoot(s.root);err!=nil{return err}
 path:=filepath.Join(s.root,rbacAuditFilename)
 line,err:=json.Marshal(event);if err!=nil{return err}
 line=append(line,'\n')
 existing,err:=readRBACAuditFile(path)
 if err!=nil{return fmt.Errorf("RBAC audit validation: %w",err)}
 // We do not discard untrusted audit content. Keep one bounded rotated file.
 // On rollover the prior .1 is replaced only after its mode is verified.
 _ = existing
 info,err:=os.Lstat(path)
 if err==nil && info.Size()+int64(len(line))>rbacAuditMaxBytes {
  rotated:=path+".1"
  if _,err:=readRBACAuditFile(rotated);err!=nil{return err}
  if err:=os.Rename(path,rotated);err!=nil{return err}
 } else if err!=nil && !errors.Is(err,os.ErrNotExist){return err}
 f,err:=os.OpenFile(path,os.O_CREATE|os.O_WRONLY|os.O_APPEND,0600)
 if err!=nil{return err}
 stat,statErr:=f.Stat()
 if statErr!=nil||stat.Mode().Perm()!=0600||!stat.Mode().IsRegular(){f.Close();return errors.New("unsafe opened audit destination")}
 if _,err:=f.Write(line);err!=nil{f.Close();return err}
 if err:=f.Sync();err!=nil{f.Close();return err}
 if err:=f.Close();err!=nil{return err}
 if dir,err:=os.Open(s.root);err==nil{_ = dir.Sync();_ = dir.Close()}
 return nil
}
func (s *rbacStore) audit(event,actor,target,reason string)error{
 s.mu.Lock();defer s.mu.Unlock()
 return s.auditLocked(rbacAuditEvent{At:time.Now().UTC(),Event:event,Actor:actor,Target:target,Reason:reason})
}
func (s *rbacStore) recentAudit(limit int)([]rbacAuditEvent,error){
 if limit<1||limit>100{return nil,errors.New("audit page limit")}
 s.mu.Lock();defer s.mu.Unlock()
 if err:=validateRBACRoot(s.root);err!=nil{return nil,err}
 path:=filepath.Join(s.root,rbacAuditFilename)
 prior,err:=readRBACAuditFile(path+".1");if err!=nil{return nil,err}
 current,err:=readRBACAuditFile(path);if err!=nil{return nil,err}
 events:=append(prior,current...)
 if len(events)>limit{events=events[len(events)-limit:]}
 for i,j:=0,len(events)-1;i<j;i,j=i+1,j-1{events[i],events[j]=events[j],events[i]}
 return events,nil
}

// Rate limit sensitive actions by verified subject, not proxy IP headers.
type rbacRateLimiter struct{
 mu sync.Mutex
 seen map[string]rbacRateState
}
type rbacRateState struct{Start time.Time;Count int}
func (l *rbacRateLimiter) allow(owner string)bool{
 l.mu.Lock();defer l.mu.Unlock()
 if l.seen==nil{l.seen=make(map[string]rbacRateState)}
 now:=time.Now()
 if len(l.seen)>2000 {
  for id,e:=range l.seen{if now.Sub(e.Start)>time.Minute{delete(l.seen,id)}}
 }
 entry:=l.seen[owner]
 if entry.Start.IsZero()||now.Sub(entry.Start)>time.Minute{entry=rbacRateState{Start:now}}
 if entry.Count>=20{return false}
 entry.Count++
 l.seen[owner]=entry
 return true
}
