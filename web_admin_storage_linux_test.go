//go:build linux

package main

import (
 "encoding/json"
 "strings"
 "testing"
)

func TestAdminStorageCountersAreBoundedAndPathFree(t *testing.T) {
 root:=t.TempDir()
 storage:=readAdminStorage(root)
 if storage==nil||storage.TotalBytes==0{t.Fatal("expected readable Linux job-volume filesystem statistics")}
 if storage.UsedBytes>storage.TotalBytes||storage.AvailableBytes>storage.TotalBytes||
  storage.UsedPercent<0||storage.UsedPercent>100 {t.Fatalf("invalid filesystem counters: %+v",storage)}
 encoded,err:=json.Marshal(storage);if err!=nil{t.Fatal(err)}
 if strings.Contains(string(encoded),root)||strings.Contains(string(encoded),"path"){
  t.Fatal("storage summary leaked a private filesystem location")
 }
 if readAdminStorage(root+"/nonexistent-volume")==nil{
  // Fail closed to an unavailable location instead of returning stale data.
 }else{t.Fatal("unavailable storage location returned counters")}
}
