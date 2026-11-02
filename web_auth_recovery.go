package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const recoveryBackupFilename = ".anyflip-accounts.pre-recovery.bak"

// recoverAdminOffline is a break-glass operator action on the private account
// store, never an HTTP endpoint. The Go Web process MUST be stopped first:
// concurrent writers have no cross-process transaction/locking protocol.
//
// It only restores an EXISTING administrator. A lost account database must
// be restored from a separately protected backup, not silently recreated.
func recoverAdminOffline(root, username, password string) error {
	if !accountNamePattern.MatchString(username) {
		return errors.New("invalid administrator username")
	}
	if root=="" {
		return errors.New("explicit recovery directory is required")
	}
	hash,err:=hashAccountPassword(password)
	if err!=nil{return err}
	auth,err:=loadAccountAuth(root,"","",false)
	if err!=nil{return fmt.Errorf("cannot open existing account store: %w",err)}
	auth.mu.Lock()
	defer auth.mu.Unlock()
	admin,exists:=auth.users[username]
	if !exists || admin.Role!="admin" {
		return errors.New("target is not an existing administrator")
	}
	if err:=auth.audit.record("offline_admin_recovery","_service",username,"intent");err!=nil {
		return fmt.Errorf("cannot audit recovery attempt: %w",err)
	}
	// One exclusive, local recovery backup: do not overwrite an earlier
	// recovery snapshot until the operator has copied/verified it elsewhere.
	source:=filepath.Join(root,accountsFile)
	destination:=filepath.Join(root,recoveryBackupFilename)
	info,err:=os.Lstat(source)
	if err!=nil{return err}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077!=0 || info.Size()>accountsMaxBytes {
		return errors.New("unsafe account store for recovery")
	}
	data,err:=os.ReadFile(source)
	if err!=nil{return err}
	if len(data)>accountsMaxBytes{return errors.New("account store exceeds recovery size limit")}
	backup,err:=os.OpenFile(destination,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
	if err!=nil{return fmt.Errorf("cannot create exclusive recovery snapshot: %w",err)}
	_,err=backup.Write(data)
	if err==nil{err=backup.Sync()}
	closeErr:=backup.Close()
	if err!=nil{return err}
	if closeErr!=nil{return closeErr}
	if directory,err:=os.Open(root);err==nil{_ = directory.Sync();_ = directory.Close()}
	previous:=admin
	admin.PasswordHash=hash
	admin.Disabled=false
	auth.users[username]=admin
	if err:=auth.saveUsersLocked();err!=nil{
		auth.users[username]=previous
		_ = auth.audit.record("offline_admin_recovery","_service",username,"failed")
		return fmt.Errorf("cannot commit offline administrator recovery: %w",err)
	}
	auth.revokeUserSessionsLocked(username)
	if err:=auth.audit.record("offline_admin_recovery","_service",username,"ok");err!=nil{
		// A committed credential change cannot be rolled back based on a
		// separate audit write failure: tell the operator to inspect the log.
		return fmt.Errorf("administrator credentials updated but final audit record failed: %w",err)
	}
	return nil
}
