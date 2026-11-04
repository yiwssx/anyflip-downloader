package main

import (
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
)

func rbacTestRoot(t *testing.T)string{
 t.Helper()
 root:=t.TempDir()
 if err:=os.Chmod(root,0700);err!=nil{t.Fatal(err)}
 return root
}
func rbacTestIdentity(s string)string{return googleOwner("https://accounts.google.com",s)}

func TestRBACBootstrapExplicitPrivateDurable(t *testing.T){
 root:=rbacTestRoot(t)
 owner:=rbacTestIdentity("operator")
 if _,err:=openRBACStore(root);err==nil{t.Fatal("missing role store accepted")}
 if err:=bootstrapRBACStore(root,"teacher@rcat.ac.th");err==nil{t.Fatal("email bootstrap accepted")}
 if err:=bootstrapRBACStore(root,owner);err!=nil{t.Fatal(err)}
 if err:=bootstrapRBACStore(root,rbacTestIdentity("attacker"));err==nil{t.Fatal("second bootstrap overwrote roles")}
 stat,err:=os.Lstat(filepath.Join(root,rbacFilename))
 if err!=nil{t.Fatal(err)}
 if !stat.Mode().IsRegular() || stat.Mode().Perm()!=0600{t.Fatal("unsafe store mode")}
 s,err:=openRBACStore(root);if err!=nil{t.Fatal(err)}
 if role,ok:=s.currentRole(owner);!ok||role!=roleSuperAdmin{t.Fatal("bootstrap role missing")}
 if err:=s.registerVerifiedUser(googleUser{Owner:rbacTestIdentity("teacher"),Email:"teacher@rcat.ac.th"});err!=nil{t.Fatal(err)}
 again,err:=openRBACStore(root);if err!=nil{t.Fatal(err)}
 if role,ok:=again.currentRole(rbacTestIdentity("teacher"));!ok||role!=roleUser{t.Fatalf("not default user: %s %v",role,ok)}
 if _,err:=os.Stat(filepath.Join(root,rbacFilename+".bak"));err!=nil{t.Fatal("previous backup missing",err)}
}

func TestRBACStoreMalformedOrUnsafeFailsClosed(t *testing.T){
 for _,tc:=range []struct{name string;breakStore func(string)error}{
  {"corrupt",func(p string)error{return os.WriteFile(p,[]byte("bad"),0600)}},
  {"mode",func(p string)error{return os.Chmod(p,0644)}},
  {"symlink",func(p string)error{if err:=os.Remove(p);err!=nil{return err};return os.Symlink("/dev/null",p)}},
  {"unknown role",func(p string)error{
   b,err:=os.ReadFile(p);if err!=nil{return err}
   return os.WriteFile(p,[]byte(strings.Replace(string(b),`"super_admin"`,`"god"`,1)),0600)
  }},
 }{
  t.Run(tc.name,func(t *testing.T){
   root:=rbacTestRoot(t)
   if err:=bootstrapRBACStore(root,rbacTestIdentity("operator"));err!=nil{t.Fatal(err)}
   if err:=tc.breakStore(filepath.Join(root,rbacFilename));err!=nil{t.Fatal(err)}
   if _,err:=openRBACStore(root);err==nil{t.Fatal("unsafe RBAC store accepted")}
  })
 }
 root:=rbacTestRoot(t)
 if err:=bootstrapRBACStore(root,rbacTestIdentity("operator"));err!=nil{t.Fatal(err)}
 if err:=os.Chmod(root,0755);err!=nil{t.Fatal(err)}
 if _,err:=openRBACStore(root);err==nil{t.Fatal("world-readable directory accepted")}
}

func TestRBACRoleGrantRevokeAndLastSuperAdmin(t *testing.T){
 root:=rbacTestRoot(t)
 owner:=rbacTestIdentity("operator")
 target:=rbacTestIdentity("teacher")
 if err:=bootstrapRBACStore(root,owner);err!=nil{t.Fatal(err)}
 s,err:=openRBACStore(root);if err!=nil{t.Fatal(err)}
 if err:=s.registerVerifiedUser(googleUser{Owner:target,Email:"teacher@rcat.ac.th"});err!=nil{t.Fatal(err)}
 if err:=s.changeRole(target,target,roleSuperAdmin);err==nil{t.Fatal("user promoted self")}
 if err:=s.changeRole(owner,owner,roleUser);err==nil{t.Fatal("last admin demoted")}
 if err:=s.changeRole(owner,target,roleAdmin);err!=nil{t.Fatal(err)}
 if role,_:=s.currentRole(target);role!=roleAdmin{t.Fatal("grant did not apply")}
 if err:=s.changeRole(target,owner,roleUser);err==nil{t.Fatal("admin changed super admin")}
 if err:=s.changeRole(owner,target,roleUser);err!=nil{t.Fatal(err)}
 if role,_:=s.currentRole(target);role!=roleUser{t.Fatal("revocation not immediate")}
 persisted,err:=openRBACStore(root);if err!=nil{t.Fatal(err)}
 if role,_:=persisted.currentRole(target);role!=roleUser{t.Fatal("revocation did not survive restart")}
}

func TestRBACConcurrentRegistrations(t *testing.T){
 root:=rbacTestRoot(t)
 if err:=bootstrapRBACStore(root,rbacTestIdentity("operator"));err!=nil{t.Fatal(err)}
 s,err:=openRBACStore(root);if err!=nil{t.Fatal(err)}
 var wg sync.WaitGroup
 for i:=0;i<20;i++{
  wg.Add(1)
  go func(n int){
   defer wg.Done()
   owner:=rbacTestIdentity(string(rune('A'+n)))
   if err:=s.registerVerifiedUser(googleUser{Owner:owner,Email:"test@rcat.ac.th"});err!=nil{t.Errorf("register: %v",err)}
  }(i)
 }
 wg.Wait()
 again,err:=openRBACStore(root);if err!=nil{t.Fatal(err)}
 if len(again.members)!=21{t.Fatalf("concurrent updates lost: %d",len(again.members))}
}
