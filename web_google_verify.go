package main

import (
 "context"
 "crypto"
 "crypto/rsa"
 "crypto/sha256"
 "encoding/base64"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "math/big"
 "net/http"
 "strconv"
 "strings"
 "time"
)

// Verify Google's RS256 signature with the public keys from Google's official
// JWKS endpoint. Never trust unsigned/unverified JWT payload claims.
func (g *googleAuth) verifyToken(ctx context.Context,raw,nonce string)(googleUser,error){
 parts:=strings.Split(raw,".")
 if len(parts)!=3{return googleUser{},errors.New("invalid JWT")}
 var header struct{Alg string `json:"alg"`;Kid string `json:"kid"`}
 head,err:=base64.RawURLEncoding.DecodeString(parts[0])
 if err!=nil || len(head)>2048{return googleUser{},errors.New("invalid JWT header")}
 if err:=json.Unmarshal(head,&header);err!=nil||header.Alg!="RS256"||header.Kid==""{return googleUser{},errors.New("unsupported JWT header")}
 sig,err:=base64.RawURLEncoding.DecodeString(parts[2])
 if err!=nil || len(sig)>1024{return googleUser{},errors.New("invalid JWT signature")}
 key,err:=g.publicKey(ctx,header.Kid)
 if err!=nil{return googleUser{},err}
 hash:=sha256.Sum256([]byte(parts[0]+"."+parts[1]))
 if err:=rsa.VerifyPKCS1v15(key,crypto.SHA256,hash[:],sig);err!=nil{return googleUser{},errors.New("Google ID token signature invalid")}
 body,err:=base64.RawURLEncoding.DecodeString(parts[1])
 if err!=nil||len(body)>8192{return googleUser{},errors.New("invalid JWT claims")}
 var claims struct{
  Iss string `json:"iss"`
  Sub string `json:"sub"`
  Aud string `json:"aud"`
  Azp string `json:"azp"`
  Nonce string `json:"nonce"`
  Email string `json:"email"`
  EmailVerified json.RawMessage `json:"email_verified"`
  HD string `json:"hd"`
  Exp int64 `json:"exp"`
  Iat int64 `json:"iat"`
 }
 if err:=json.Unmarshal(body,&claims);err!=nil{return googleUser{},err}
 verified:=string(claims.EmailVerified)=="true"||string(claims.EmailVerified)==`"true"`
 if claims.Azp!=""&&claims.Azp!=g.clientID{return googleUser{},errors.New("invalid authorized party")}
 return googleAllowedIdentity(claims.Iss,claims.Sub,claims.Aud,claims.Nonce,claims.Email,claims.HD,verified,claims.Exp,claims.Iat,g.clientID,nonce)
}

func (g *googleAuth) publicKey(ctx context.Context,kid string)(*rsa.PublicKey,error){
 g.mu.Lock()
 key,ok:=g.keys[kid]
 if time.Now().Before(g.keysExpiry) {
  g.mu.Unlock()
  if ok{return key,nil}
  return nil,errors.New("unknown Google JWKS key")
 }
 g.mu.Unlock()
 req,err:=http.NewRequestWithContext(ctx,"GET",g.jwksURL,nil)
 if err!=nil{return nil,err}
 resp,err:=g.httpClient.Do(req);if err!=nil{return nil,err}
 defer resp.Body.Close()
 if resp.StatusCode!=200{return nil,errors.New("JWKS unavailable")}
 var doc struct{ Keys []struct{
  Kty string `json:"kty"`;Kid string `json:"kid"`;Alg string `json:"alg"`
  Use string `json:"use"`;N string `json:"n"`;E string `json:"e"`
 } `json:"keys"`}
 if err:=json.NewDecoder(io.LimitReader(resp.Body,256<<10)).Decode(&doc);err!=nil{return nil,err}
 keys:=make(map[string]*rsa.PublicKey)
 for _,entry:=range doc.Keys {
  if entry.Kty!="RSA"||entry.Kid==""||(entry.Alg!=""&&entry.Alg!="RS256")||(entry.Use!=""&&entry.Use!="sig"){continue}
  n,err:=base64.RawURLEncoding.DecodeString(entry.N)
  if err!=nil||len(n)<256||len(n)>1024{continue}
  exponent,err:=base64.RawURLEncoding.DecodeString(entry.E)
  if err!=nil||len(exponent)==0||len(exponent)>4{continue}
  e:=0
  for _,v:=range exponent{e=e*256+int(v)}
  if e<3||e%2==0||e>1<<20{continue}
  keys[entry.Kid]=&rsa.PublicKey{N:new(big.Int).SetBytes(n),E:e}
 }
 if len(keys)==0{return nil,errors.New("Google JWKS missing signing keys")}
 // Bound caching; fetch new certificates as Google rotates signing keys.
 ttl:=5*time.Minute
 if cc:=resp.Header.Get("Cache-Control");cc!=""{
  for _,part:=range strings.Split(cc,","){
   part=strings.TrimSpace(part)
   if strings.HasPrefix(part,"max-age="){
    value,err:=strconv.Atoi(strings.TrimPrefix(part,"max-age="))
    if err==nil&&value>=30&&value<=3600{ttl=time.Duration(value)*time.Second}
   }
  }
 }
 g.mu.Lock()
 g.keys=keys
 g.keysExpiry=time.Now().Add(ttl)
 result,ok:=keys[kid]
 g.mu.Unlock()
 if !ok{return nil,fmt.Errorf("Google signing key unavailable")}
 return result,nil
}
