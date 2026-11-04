//go:build linux

package main

import (
 "math"
 "golang.org/x/sys/unix"
)

// Statfs measures the filesystem containing the job volume. In production
// that should be the dedicated quota-backed mount; never expose its path.
func readAdminStorage(root string) *adminStorageRecord {
 var fs unix.Statfs_t
 if err:=unix.Statfs(root,&fs);err!=nil{return nil}
 if fs.Bsize<=0||fs.Blocks==0||fs.Bfree>fs.Blocks||fs.Bavail>fs.Blocks{return nil}
 size:=uint64(fs.Bsize)
 if fs.Blocks>math.MaxUint64/size{return nil}
 total:=fs.Blocks*size
 used:=(fs.Blocks-fs.Bfree)*size
 available:=fs.Bavail*size
 percent:=int(float64(used)/float64(total)*100+0.5)
 if percent>100{percent=100}
 return &adminStorageRecord{TotalBytes:total,UsedBytes:used,AvailableBytes:available,UsedPercent:percent}
}
