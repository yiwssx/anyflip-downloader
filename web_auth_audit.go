package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	authAuditFilename = ".anyflip-auth-audit.jsonl"
	authAuditMaxBytes = 512 << 10
)

// An operational audit trail, not a cryptographically tamper-proof log.
// Only fixed event names and validated account identifiers are written.
// Credentials, request bodies, cookies, tokens, URLs, and IPs are NEVER logged.
type authAudit struct {
	mu sync.Mutex
	root string
}

type authAuditEvent struct {
	Time time.Time `json:"time"`
	Event string `json:"event"`
	Actor string `json:"actor"`
	Target string `json:"target,omitempty"`
	Result string `json:"result"`
}

var allowedAuthAuditEvents = map[string]bool{
	"login": true,
	"logout": true,
	"create_user": true,
	"change_password": true,
	"reset_password": true,
	"suspend_user": true,
	"enable_user": true,
	"offline_admin_recovery": true,
}

func safeAuditAccountName(value string) string {
	if value=="_service" || value=="unknown" || accountNamePattern.MatchString(value) {return value}
	return "unknown"
}

func inspectAuthAuditFile(path string) (int64,error) {
	info, err := os.Lstat(path)
	if errors.Is(err,os.ErrNotExist) {return 0,nil}
	if err!=nil{return 0,err}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077!=0 {
		return 0,fmt.Errorf("unsafe authentication audit file permissions or type")
	}
	if info.Size()>authAuditMaxBytes {
		return 0,fmt.Errorf("authentication audit file exceeds size limit")
	}
	return info.Size(),nil
}

func newAuthAudit(root string) (*authAudit,error) {
	for _,suffix:=range []string{"",".1"} {
		if _,err:=inspectAuthAuditFile(filepath.Join(root,authAuditFilename+suffix));err!=nil{return nil,err}
	}
	return &authAudit{root:root},nil
}

func (a *authAudit) record(event, actor, target, result string) error {
	if a==nil {return nil} // Token-only mode never uses account audit events.
	if !allowedAuthAuditEvents[event] || (result!="ok" && result!="denied" && result!="intent" && result!="failed") {
		return errors.New("unsupported authentication audit event")
	}
	item:=authAuditEvent{
		Time:time.Now().UTC(), Event:event,
		Actor:safeAuditAccountName(actor), Target:safeAuditAccountName(target), Result:result,
	}
	if target=="" {item.Target=""}
	line,err:=json.Marshal(item)
	if err!=nil{return err}
	line=append(line,'\n')
	a.mu.Lock()
	defer a.mu.Unlock()
	path:=filepath.Join(a.root,authAuditFilename)
	archive:=path+".1"
	size,err:=inspectAuthAuditFile(path)
	if err!=nil{return err}
	if size+int64(len(line))>authAuditMaxBytes {
		if _,err:=inspectAuthAuditFile(archive);err!=nil{return err}
		// Atomic replacement of the single bounded archive on the Linux host.
		// This is retention/size control, not tamper evidence.
		if err:=os.Rename(path,archive);err!=nil{return err}
	}
	f,err:=os.OpenFile(path,os.O_APPEND|os.O_CREATE|os.O_WRONLY,0600)
	if err!=nil{return err}
	info,statErr:=f.Stat()
	if statErr!=nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077!=0 {
		_ = f.Close()
		return errors.New("unsafe authentication audit file")
	}
	_,err=f.Write(line)
	if err==nil{err=f.Sync()}
	closeErr:=f.Close()
	if err!=nil{return err}
	if closeErr!=nil{return closeErr}
	if size+int64(len(line))>authAuditMaxBytes {
		if d,err:=os.Open(a.root);err==nil{_ = d.Sync();_ = d.Close()}
	}
	return nil
}

func (a *authAudit) readForTest() ([]authAuditEvent,error) {
	if a==nil{return nil,errors.New("audit disabled")}
	body,err:=os.ReadFile(filepath.Join(a.root,authAuditFilename))
	if err!=nil{return nil,err}
	var result []authAuditEvent
	for len(body)>0{
		var line []byte
		i:=-1
		for k,c:=range body{if c=='\n'{i=k;break}}
		if i<0{return nil,errors.New("unterminated audit record")}
		line,body=body[:i],body[i+1:]
		var event authAuditEvent
		if err:=json.Unmarshal(line,&event);err!=nil{return nil,err}
		result=append(result,event)
	}
	return result,nil
}

// Audit events are intentionally never exposed as an HTTP response.
// Operators review and export the private log using host filesystem access.
