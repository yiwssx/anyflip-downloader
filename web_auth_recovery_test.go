package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineAdminRecoveryRequiresExistingAdminAndPrivateBackup(t *testing.T) {
	dir:=t.TempDir()
	hash,err:=hashAccountPassword("original-admin-very-long-password")
	if err!=nil{t.Fatal(err)}
	accounts,err:=loadAccountAuth(dir,"admin",hash,false)
	if err!=nil{t.Fatal(err)}
	if err:=accounts.addUser("alice","original-alice-very-long-password","user");err!=nil{t.Fatal(err)}
	if err:=accounts.addUser("otheradmin","original-otheradmin-password","admin");err!=nil{t.Fatal(err)}
	if err:=accounts.setAccountDisabled("admin","otheradmin",true);err!=nil{t.Fatal(err)}

	if err:=recoverAdminOffline(dir,"alice","new-secret-password-012345");err==nil{t.Fatal("promoted regular user to admin")}
	if _,err:=os.Stat(filepath.Join(dir,recoveryBackupFilename));!errors.Is(err,os.ErrNotExist){t.Fatal("rejected recovery unexpectedly created a backup")}
	if err:=recoverAdminOffline(dir,"otheradmin","new-admin-secret-password-01234");err!=nil{t.Fatal(err)}
	refreshed,err:=loadAccountAuth(dir,"","",false)
	if err!=nil{t.Fatal(err)}
	if refreshed.users["otheradmin"].Disabled{t.Fatal("recovered administrator remained disabled")}
	if _,ok:=refreshed.authenticate("otheradmin","original-otheradmin-password");ok{t.Fatal("old password still valid")}
	if _,ok:=refreshed.authenticate("otheradmin","new-admin-secret-password-01234");!ok{t.Fatal("recovered password invalid")}
	backup:=filepath.Join(dir,recoveryBackupFilename)
	info,err:=os.Stat(backup)
	if err!=nil{t.Fatal(err)}
	if info.Mode().Perm()!=0600{t.Fatalf("recovery snapshot permissions = %v",info.Mode().Perm())}
	raw,err:=os.ReadFile(backup);if err!=nil{t.Fatal(err)}
	if !strings.Contains(string(raw), `"disabled":true`){t.Fatal("backup did not preserve prior suspended state")}
	if err:=recoverAdminOffline(dir,"otheradmin","yet-another-password-012345");err==nil{
		t.Fatal("second recovery overwrote existing snapshot")
	}
	events,err:=refreshed.audit.readForTest()
	if err!=nil{t.Fatal(err)}
	found:=false
	for _,e:=range events{
		if e.Event=="offline_admin_recovery" && e.Target=="otheradmin" && e.Result=="ok"{found=true}
	}
	if !found{t.Fatal("offline recovery success not audited")}
	if _,err:=os.Stat(filepath.Join(dir,accountsFile));err!=nil{t.Fatal(err)}
}

func TestOfflineAdminRecoveryRejectsMissingOrUnsafeStore(t *testing.T) {
	if err:=recoverAdminOffline("", "admin","strong-password-123456");err==nil{t.Fatal("accepted unspecified root")}
	dir:=t.TempDir()
	if err:=recoverAdminOffline(dir,"admin","strong-password-123456");err==nil{t.Fatal("bootstrapped new admin when store missing")}
	hash,_:=hashAccountPassword("original-admin-very-long-password")
	a,err:=loadAccountAuth(dir,"admin",hash,false);if err!=nil{t.Fatal(err)}
	target:=filepath.Join(dir,accountsFile)
	if err:=os.Chmod(target,0644);err!=nil{t.Fatal(err)}
	if err:=recoverAdminOffline(dir,"admin","strong-password-123456");err==nil{t.Fatal("recovered from unsafe store")}
	if err:=os.Chmod(target,0600);err!=nil{t.Fatal(err)}
	// Audit must be writable before any recovery snapshot/account mutation.
	if err:=os.Chmod(filepath.Join(dir,authAuditFilename),0644);err!=nil{t.Fatal(err)}
	if err:=recoverAdminOffline(dir,"admin","strong-password-123456");err==nil{t.Fatal("recovery ignored unsafe audit")}
	_ = a
}
