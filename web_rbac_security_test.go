package main

import (
 "bytes"
 "encoding/json"
 "os"
 "path/filepath"
 "sync"
 "testing"
 "time"
)

func TestRBACConcurrentLastSuperAdminRevocation(t *testing.T) {
 root:=rbacTestRoot(t)
 first:=rbacTestIdentity("first-owner")
 second:=rbacTestIdentity("second-owner")
 if err:=bootstrapRBACStore(root,first);err!=nil{t.Fatal(err)}
 s,err:=openRBACStore(root);if err!=nil{t.Fatal(err)}
 if err:=s.registerVerifiedUser(googleUser{Owner:second,Email:"second@rcat.ac.th"});err!=nil{t.Fatal(err)}
 if err:=s.changeRole(first,second,roleSuperAdmin);err!=nil{t.Fatal(err)}
 var wg sync.WaitGroup
 results:=make(chan error,2)
 for _,p:=range [][2]string{{first,second},{second,first}} {
  wg.Add(1)
  go func(actor,target string) {
   defer wg.Done()
   results<-s.changeRole(actor,target,roleUser)
  }(p[0],p[1])
 }
 wg.Wait();close(results)
 success:=0
 for e:=range results{if e==nil{success++}}
 if success!=1{t.Fatalf("expected exactly one concurrent demotion, got %d",success)}
 persisted,err:=readRBACDocument(root);if err!=nil{t.Fatal(err)}
 active:=0
 for _,m:=range persisted.Members{if m.Role==roleSuperAdmin&&!m.Disabled{active++}}
 if active!=1{t.Fatalf("lost protected last super admin: %d",active)}
}

func TestRBACAuditRotationAndConcurrentAppend(t *testing.T) {
 root:=rbacTestRoot(t)
 owner:=rbacTestIdentity("audit-owner")
 if err:=bootstrapRBACStore(root,owner);err!=nil{t.Fatal(err)}
 store,err:=openRBACStore(root);if err!=nil{t.Fatal(err)}
 event:=rbacAuditEvent{At:time.Now().UTC(),Event:"privileged_denied",Actor:owner}
 encoded,err:=json.Marshal(event);if err!=nil{t.Fatal(err)}
 line:=append(encoded,'\n')
 copies:=rbacAuditMaxBytes/len(line)
 if copies<2{t.Fatal("unexpected audit event size")}
 path:=filepath.Join(root,rbacAuditFilename)
 if err:=os.WriteFile(path,bytes.Repeat(line,copies),0600);err!=nil{t.Fatal(err)}
 // A same-size record must trigger rotation because the prefilled file
 // cannot fit another complete line.
 if err:=store.audit("privileged_denied",owner,"","");err!=nil{t.Fatal(err)}
 prev,err:=readRBACAuditFile(path+".1");if err!=nil{t.Fatal(err)}
 current,err:=readRBACAuditFile(path);if err!=nil{t.Fatal(err)}
 if len(prev)!=copies||len(current)!=1{t.Fatalf("rotation lost events: prior=%d current=%d",len(prev),len(current))}
 for _,filename:=range []string{path,path+".1"} {
  stat,err:=os.Lstat(filename);if err!=nil{t.Fatal(err)}
  if stat.Mode().Perm()!=0600||!stat.Mode().IsRegular(){t.Fatal("unsafe rotated audit file:",filename)}
 }
 const workers=8
 const writesPerWorker=5
 var wg sync.WaitGroup
 for i:=0;i<workers;i++{
  wg.Add(1)
  go func(){
   defer wg.Done()
   for j:=0;j<writesPerWorker;j++ {
    if err:=store.audit("privileged_denied",owner,"","");err!=nil{t.Errorf("concurrent audit append: %v",err)}
   }
  }()
 }
 wg.Wait()
 current,err=readRBACAuditFile(path);if err!=nil{t.Fatal(err)}
 if len(current)!=1+workers*writesPerWorker{t.Fatalf("missing concurrent audit entries: %d",len(current))}
 if _,err:=openRBACStore(root);err!=nil{t.Fatalf("audit could not survive restart: %v",err)}
}
