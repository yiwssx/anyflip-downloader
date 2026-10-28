package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"net/http"
	"net/http/httptest"
	"io"
)

func TestWebHistoryPersistsOnlyRedactedTerminalStatus(t *testing.T) {
	root := t.TempDir()
	s, err := newWebServer(testWebToken, root)
	if err != nil { t.Fatal(err) }
	id := strings.Repeat("a", 32)
	job := &webJob{id:id, stage:"downloading", total:4, completed:2, created:time.Now()}
	s.jobs[id] = job
	s.finishJob(job, "", os.ErrPermission)
	onDisk,err:=os.ReadFile(filepath.Join(root,webHistoryFilename))
	if err!=nil {t.Fatal(err)}
	for _, secret:=range []string{testWebToken,"anyflip.com","download title","/tmp/private"} {
		if bytes.Contains(onDisk,[]byte(secret)){t.Fatalf("history leaks %q",secret)}
	}
	info,err:=os.Stat(filepath.Join(root,webHistoryFilename))
	if err!=nil||info.Mode().Perm()!=0600 {t.Fatalf("wrong mode/metadata: %v %v",info,err)}
	restarted,err:=newWebServer(testWebToken,root)
	if err!=nil {t.Fatal(err)}
	if len(restarted.jobs)!=0 {t.Fatal("restored active queue on restart")}
	ts:=httptest.NewServer(restarted.handler())
	defer ts.Close()
	res:=webRequest(t,ts,"GET","/api/jobs/"+id,nil,true)
	if res.StatusCode!=http.StatusOK {t.Fatalf("archived GET=%d",res.StatusCode)}
	var status webJobResponse
	if err:=json.NewDecoder(res.Body).Decode(&status);err!=nil{t.Fatal(err)}
	res.Body.Close()
	if !status.Archived||status.DownloadReady||status.Stage!="failed"||status.ErrorCode!="storage_error" {
		t.Fatalf("bad archived status: %+v",status)
	}
	res=webRequest(t,ts,"GET","/api/jobs/"+id+"/download",nil,true)
	res.Body.Close()
	if res.StatusCode!=http.StatusNotFound {t.Fatalf("archived download must be denied: %d",res.StatusCode)}
	res=webRequest(t,ts,"POST","/api/jobs/"+id+"/retry",nil,true)
	res.Body.Close()
	if res.StatusCode!=http.StatusNotFound {t.Fatalf("archived retry must be denied: %d",res.StatusCode)}
	res=webRequest(t,ts,"GET","/api/jobs",nil,true)
	if res.StatusCode!=http.StatusOK {t.Fatalf("history status=%d",res.StatusCode)}
	var listing struct{Jobs []webJobResponse `json:"jobs"`}
	if err:=json.NewDecoder(res.Body).Decode(&listing);err!=nil{t.Fatal(err)}
	res.Body.Close()
	if len(listing.Jobs)!=1||!listing.Jobs[0].Archived {t.Fatalf("bad archive listing: %+v",listing.Jobs)}
	res=webRequest(t,ts,"GET","/api/jobs",nil,false)
	res.Body.Close()
	if res.StatusCode!=http.StatusUnauthorized {t.Fatalf("history is not authenticated: %d",res.StatusCode)}
}

func TestWebHistoryBoundedAndSurvivesRetentionCleanup(t *testing.T) {
	root:=t.TempDir()
	s,err:=newWebServer(testWebToken,root)
	if err!=nil {t.Fatal(err)}
	for i:=0;i<65;i++ {
		id:=strings.Repeat(string("0123456789abcdef"[i%16]),32)
		// Unique 32-character hex ID per entry, including across rotations.
		id=id[:30]+string("0123456789abcdef"[(i/16)%16])+string("0123456789abcdef"[i%16])
		job:=&webJob{id:id,stage:"failed",total:2,completed:1,created:time.Now()}
		s.archiveFinishedJobLockedWithTestLock(job)
	}
	if len(s.history)!=webHistoryMaxEntries {t.Fatalf("history length=%d",len(s.history))}
	restarted,err:=newWebServer(testWebToken,root)
	if err!=nil {t.Fatal(err)}
	if len(restarted.history)!=webHistoryMaxEntries {t.Fatalf("restored length=%d",len(restarted.history))}
	restarted.purgeOldJobs(time.Now().Add(8*24*time.Hour))
	if len(restarted.history)!=0 {t.Fatal("expired history remained in memory")}
	afterExpiry,err:=newWebServer(testWebToken,root)
	if err!=nil {t.Fatal(err)}
	if len(afterExpiry.history)!=0 {t.Fatal("expired history remained on disk")}
}

func (s *webServer) archiveFinishedJobLockedWithTestLock(job *webJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job.finished=time.Now()
	s.archiveFinishedJobLocked(job)
}

func TestWebHistoryRejectsSymlinkAndCorruption(t *testing.T) {
	for _,tc:=range []struct{name, content string; symlink bool}{
		{"corrupt","{not-json",false},
		{"old-version",`{"version":99,"entries":[]}`,false},
		{"link","",true},
	}{
		t.Run(tc.name,func(t *testing.T){
			root:=t.TempDir()
			target:=filepath.Join(root,webHistoryFilename)
			if tc.symlink {
				other:=filepath.Join(t.TempDir(),"outside")
				if err:=os.WriteFile(other,[]byte(`{"version":1,"entries":[]}`),0600);err!=nil{t.Fatal(err)}
				if err:=os.Symlink(other,target);err!=nil{t.Skip(err)}
			}else if err:=os.WriteFile(target,[]byte(tc.content),0600);err!=nil{t.Fatal(err)}
			if _,err:=newWebServer(testWebToken,root);err==nil{t.Fatal("accepted invalid history")}
		})
	}
}

func TestWebHistoryNoRawErrorOrSourceData(t *testing.T) {
	root:=t.TempDir()
	s,err:=newWebServer(testWebToken,root)
	if err!=nil{t.Fatal(err)}
	id:=strings.Repeat("e",32)
	job:=&webJob{id:id,stage:"downloading",created:time.Now(),spec:webJobSpec{URL:"https://anyflip.com/secret/book",Title:"secret title",Threads:2}}
	s.jobs[id]=job
	s.finishJob(job,"",os.ErrPermission)
	body,err:=os.ReadFile(filepath.Join(root,webHistoryFilename))
	if err!=nil{t.Fatal(err)}
	if bytes.Contains(body,[]byte("secret"))||bytes.Contains(body,[]byte("permission denied")){t.Fatal("leaked source or raw error")}
	resp:=httptest.NewRecorder()
	req:=httptest.NewRequest("GET","/api/jobs/"+id,nil)
	req.SetPathValue("id",id)
	req.Header.Set("Authorization","Bearer "+testWebToken)
	s.handler().ServeHTTP(resp,req)
	if resp.Code!=http.StatusOK{t.Fatalf("get status=%d",resp.Code)}
	_,_ = io.ReadAll(resp.Body)
}
