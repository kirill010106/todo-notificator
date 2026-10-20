# Воспроизведение проверки 20.10.2026

Это исторический сценарий для HEAD `9caed18`, до исправления P1. В текущей схеме уже нет открытого refresh-токена, поэтому старые ожидания `TestAuditRawRefresh` намеренно не подходят. Проверки исправлений находятся в `backend/internal/storage/postgres/refresh_tokens*_test.go` и `backend/internal/deployssh/host_key_test.go`; результаты — в [отчёте P1](../SECURITY-P1-FIXES-2026-10-20.md).

Проверен HEAD 9caed18. Сценарий ниже не изменяет исходники: он создаёт временные Go-тесты, подключает их через -overlay и запускает текущий api.ts в двух JS-контекстах с общим storage. Создаются только временные файлы и штатный кеш Go.

Требуются Python 3, Go, Node.js, установленные зависимости frontend (typescript) и зависимости Go-модулей. Запускать из корня репозитория. Нужны разрешённые локальные loopback-соединения для email-теста; Docker не требуется. SMTP, настоящие платежи и рабочие БД не используются. При холодном кеше первая компиляция может занять несколько минут.

Сохраните Python-блок во временный файл и выполните python с путём к нему. Скрипт напечатает каталог артефактов с overlay, исходниками проверок и журналами. Все 13 выбранных Go-тестов должны пройти; они подтверждают описанные в отчёте свойства, включая дефекты. Тесты не являются исправлением. JS-сценарии должны вывести:

```json
{"order":"loser-first","refresh_token":null,"events":["session-ended"]}
{"order":"winner-first","refresh_token":"new","events":[]}
```

№2 проверяется отдельно чтением .github/workflows/cd.yml: ssh-keyscan без заранее проверенного ключа и ANSIBLE_HOST_KEY_CHECKING: "False". Скрипт не подключается к SSH и не запускает workflow. SQL-проверки используют sqlmock, MongoDB-проверки — штатный mock deployment драйвера; это не интеграция с реальными БД. Email-тест извлекает литерал конфигурации сервера из main.go и использует настоящий webhook handler, не запуская main().

Первый пробный Go-прогон выявил ошибку самой фикстуры: пустая MongoDB-порция кодировалась как BSON null. Фикстура исправлена на пустой массив; этот сбой не считается дефектом приложения. Первый сетевой email-тест был ограничен sandbox; при разрешённом loopback он прошёл.

```python
from pathlib import Path
import json, subprocess, tempfile, re

root = Path.cwd()
assert (root / 'go.work').is_file(), 'Run from repository root'
tmp = Path(tempfile.mkdtemp(prefix='todo-audit-20261020-'))
cases = {
    "backend/internal/http-server/router/audit_repro_test.go": r'''
package router
import(
 "context"
 "io"
 "log/slog"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
 "github.com/kirill010106/todo-notificator/internal/config"
 "github.com/kirill010106/todo-notificator/internal/http-server/handlers/account"
 "github.com/kirill010106/todo-notificator/internal/domain"
 "github.com/kirill010106/todo-notificator/internal/storage"
 "github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/login"
 "github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/refresh"
 logsget "github.com/kirill010106/todo-notificator/internal/http-server/handlers/logs/get"
 "github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
 "github.com/kirill010106/todo-notificator/internal/lib/jwt"
)
type auditUser struct{}
func (*auditUser) User(context.Context,string)(domain.User,error){return domain.User{},storage.ErrUserNotFound}
func (*auditUser) SaveRefreshToken(context.Context,int64,string,time.Time)error{return nil}
type auditRefresh struct{length int}
func(s *auditRefresh) RotateRefreshToken(_ context.Context,old,new string,_ time.Time)(*domain.User,error){s.length=len(old);return nil,storage.ErrRefreshTokenInvalid}
type auditReader struct{r io.Reader;n int}
func(r *auditReader)Read(p []byte)(int,error){n,e:=r.r.Read(p);r.n+=n;return n,e}
func(*auditReader)Close()error{return nil}
func TestAuditAuthBody(t *testing.T){
 log:=slog.New(slog.NewTextHandler(io.Discard,nil));cfg:=&config.Config{AppSecret:"audit",RefreshTokenTTL:time.Hour}
 payload:=strings.Repeat("x",8<<20)
 for _,route:=range []string{"login","refresh"}{
 s:=&auditRefresh{};body:="{\"email\":\"audit@example.com\",\"password\":\""+payload+"\"}";h:=login.New(log,&auditUser{},cfg,nil)
 if route=="refresh"{body="{\"refresh_token\":\""+payload+"\"}";h=refresh.New(log,s,cfg)}
 reader:=&auditReader{r:strings.NewReader(body)};r:=httptest.NewRequest("POST","/"+route,reader);w:=httptest.NewRecorder();h(w,r)
 if reader.n!=len(body)||w.Code!=401{t.Fatalf("%s bytes=%d status=%d",route,reader.n,w.Code)}
 if route=="refresh"&&s.length!=len(payload){t.Fatal("not passed to storage")}
 t.Logf("%s consumed %d bytes; HTTP %d",route,reader.n,w.Code)
 }}
type auditLogs struct{limit int32}
func(s *auditLogs)GetLogs(_ context.Context,_ int64,limit,offset int32)([]domain.ActivityLog,error){s.limit=limit;return nil,nil}
func TestAuditPremiumLimit(t *testing.T){
 s:=&auditLogs{};token,e:=jwt.NewAccessToken(domain.User{ID:1,IsPremium:true},"audit",time.Hour);if e!=nil{t.Fatal(e)}
 r:=httptest.NewRequest("GET","/me/logs?limit=2147483647",nil);r.Header.Set("Authorization","Bearer "+token);w:=httptest.NewRecorder()
 auth.New("audit")(logsget.New(slog.New(slog.NewTextHandler(io.Discard,nil)),s)).ServeHTTP(w,r)
 if w.Code!=200||s.limit!=2147483647{t.Fatalf("status %d limit %d",w.Code,s.limit)};t.Logf("Premium forwards limit=%d",s.limit)
}

func TestAuditRegisterCap(t *testing.T){
 body:="{\"email\":\"audit@example.com\",\"password\":\""+strings.Repeat("x",8<<20)+"\"}"
 reader:=&auditReader{r:strings.NewReader(body)};r:=httptest.NewRequest("POST","/register",reader);w:=httptest.NewRecorder()
 (account.Handler{}).Register(w,r)
 if w.Code!=400||reader.n>4097{t.Fatalf("status=%d consumed=%d",w.Code,reader.n)}
 t.Logf("active registration rejected oversized JSON with HTTP %d after reading %d bytes",w.Code,reader.n)
}
''',
    "backend/clients/activitylogger/audit_repro_test.go": r'''
package activitylogger
import("context";"io";"log/slog";"testing";"time";pb "github.com/kirill010106/todo-notificator/root/pkg/activity_logger/v1";"google.golang.org/grpc")
type auditBlocked struct{pb.ActivityLoggerClient;started chan struct{};release chan struct{}}
func(s *auditBlocked)LogEvent(ctx context.Context,_ *pb.LogRequest,_ ...grpc.CallOption)(*pb.LogResponse,error){s.started<-struct{}{};select{case <-s.release:case <-ctx.Done():};return &pb.LogResponse{Success:true},nil}
func TestAuditConcurrentEvents(t *testing.T){
 const n=256;s:=&auditBlocked{started:make(chan struct{},n),release:make(chan struct{})}
 c:=&Client{api:s,log:slog.New(slog.NewTextHandler(io.Discard,nil)),timeout:5*time.Second};defer close(s.release)
 for i:=0;i<n;i++{c.LogEvent(int64(i),"AUDIT",1,nil)}
 timer:=time.NewTimer(3*time.Second);defer timer.Stop()
 for i:=0;i<n;i++{select{case <-s.started:case <-timer.C:t.Fatalf("only %d calls admitted",i)}}
 t.Logf("all %d RPCs active simultaneously without global cap",n)
}
''',
    "backend/internal/storage/postgres/audit_repro_test.go": r'''
package postgres
import("context";"regexp";"testing";"time";"github.com/DATA-DOG/go-sqlmock")
func TestAuditRawRefresh(t *testing.T){
 db,m,e:=sqlmock.New();if e!=nil{t.Fatal(e)};defer db.Close();s:=&Storage{DB:db};ctx:=context.Background()
 raw:="audit-plaintext-refresh";next:="audit-next-refresh";expiry:=time.Now().Add(time.Hour)
 row:=func()*sqlmock.Rows{return sqlmock.NewRows([]string{"id","email","password_hash","is_verified","is_premium","display_name","auth_version"}).AddRow(7,"audit@example.com",[]byte("hash"),true,false,"",0)}
 m.ExpectBegin();m.ExpectQuery("SELECT id,email,password_hash").WithArgs(int64(7)).WillReturnRows(row())
 m.ExpectExec(regexp.QuoteMeta("INSERT INTO refresh_tokens(user_id,token,expires_at) VALUES($1,$2,$3)")).WithArgs(int64(7),raw,expiry.UTC()).WillReturnResult(sqlmock.NewResult(1,1));m.ExpectCommit()
 if e=s.SaveRefreshTokenVersion(ctx,7,0,raw,expiry);e!=nil{t.Fatal(e)}
 m.ExpectBegin();m.ExpectQuery(regexp.QuoteMeta("SELECT user_id FROM refresh_tokens WHERE token=$1")).WithArgs(raw).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(7))
 m.ExpectQuery("SELECT id,email,password_hash").WithArgs(int64(7)).WillReturnRows(row())
 m.ExpectExec("UPDATE refresh_tokens SET").WithArgs(raw,next,expiry.UTC(),int64(7)).WillReturnResult(sqlmock.NewResult(0,1));m.ExpectCommit()
 if _,e=s.RotateRefreshToken(ctx,raw,next,expiry);e!=nil{t.Fatal(e)}
 if e=m.ExpectationsWereMet();e!=nil{t.Fatal(e)};t.Log("raw bearer value unchanged in INSERT, SELECT and UPDATE (SQL boundary)")
}

func TestAuditCancellationSQL(t *testing.T){
 db,m,e:=sqlmock.New();if e!=nil{t.Fatal(e)};defer db.Close()
 query:="UPDATE payments SET status = $1, updated_at = CURRENT_TIMESTAMP WHERE yookassa_payment_id = $2 RETURNING user_id"
 m.ExpectQuery(regexp.QuoteMeta(query)).WithArgs("canceled","audit").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(7))
 if _,e=(&Storage{DB:db}).UpdatePaymentStatus(context.Background(),"audit","canceled");e!=nil{t.Fatal(e)}
 if e=m.ExpectationsWereMet();e!=nil{t.Fatal(e)}
 t.Log("actual SQL WHERE filters only payment id, no current-status guard")
}
''',
    "activity-logger/cmd/audit_repro_test.go": r'''
package main
import("context";"testing";"time"
 pb "github.com/kirill010106/todo-notificator/root/pkg/activity_logger/v1"
 "go.mongodb.org/mongo-driver/v2/bson";"go.mongodb.org/mongo-driver/v2/mongo";"go.mongodb.org/mongo-driver/v2/mongo/options"
 "go.mongodb.org/mongo-driver/v2/event";"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest")
func auditServer(t *testing.T,responses ...bson.D)*server{
 t.Helper();o:=options.Client();o.Deployment=drivertest.NewMockDeployment(responses...);o.SetRetryReads(false)
 c,e:=mongo.Connect(o);if e!=nil{t.Fatal(e)};t.Cleanup(func(){_=c.Disconnect(context.Background())});return &server{collection:c.Database("audit").Collection("activities")}}
func auditBatch(id int64,docs ...any)bson.D{if docs==nil{docs=[]any{}};return bson.D{{"ok",1},{"cursor",bson.D{{"id",id},{"ns","audit.activities"},{"firstBatch",bson.A(docs)}}}}}
func TestAuditCursorFailure(t *testing.T){
 doc:=bson.D{{"_id",bson.NewObjectID()},{"user_id",int64(1)},{"action","first"},{"created_at",time.Now()}}
 s:=auditServer(t,auditBatch(123,doc),bson.D{{"ok",0},{"code",int32(43)},{"errmsg","injected cursor lost"}},bson.D{{"ok",1}})
 r,e:=s.GetLogs(context.Background(),&pb.GetLogsRequest{UserId:1,Limit:50});if e!=nil||len(r.Logs)!=1{t.Fatalf("resp=%v err=%v",r,e)}
 t.Log("getMore error 43 swallowed; successful partial response with 1 log")
}
func TestAuditDecodeFailure(t *testing.T){
 s:=auditServer(t,auditBatch(0,bson.D{{"user_id","not-an-integer"}}))
 r,e:=s.GetLogs(context.Background(),&pb.GetLogsRequest{UserId:1});if e!=nil||len(r.Logs)!=0{t.Fatalf("resp=%v err=%v",r,e)}
 t.Log("invalid BSON record omitted with nil RPC error")
}
func TestAuditCanceledRPC(t *testing.T){
 for _,method:=range []string{"GetLogs","LogEvent"}{
  ctx,cancel:=context.WithCancel(context.Background());cancel()
  if method=="GetLogs"{s:=auditServer(t,auditBatch(0));_,e:=s.GetLogs(ctx,&pb.GetLogsRequest{UserId:1});if e!=nil{t.Fatal(e)}}else{
   s:=auditServer(t,bson.D{{"ok",1},{"n",1}});r,e:=s.LogEvent(ctx,&pb.LogRequest{UserId:1});if e!=nil||!r.Success{t.Fatalf("%v %v",r,e)}}
  t.Logf("%s executed MongoDB operation despite canceled incoming context",method)
 }}
func TestAuditMongoLimit(t *testing.T){
 var limit int64;o:=options.Client();o.Deployment=drivertest.NewMockDeployment(auditBatch(0))
 o.SetMonitor(&event.CommandMonitor{Started:func(_ context.Context,e *event.CommandStartedEvent){if e.CommandName=="find"{limit=e.Command.Lookup("limit").AsInt64()}}})
 c,e:=mongo.Connect(o);if e!=nil{t.Fatal(e)};defer c.Disconnect(context.Background())
 _,e=(&server{collection:c.Database("audit").Collection("activities")}).GetLogs(context.Background(),&pb.GetLogsRequest{UserId:1,Limit:2147483647})
 if e!=nil||limit!=2147483647{t.Fatalf("limit=%d err=%v",limit,e)};t.Logf("MongoDB find.limit=%d",limit)
}
''',
    "backend/internal/http-server/handlers/payments/webhook/audit_repro_test.go": r'''
package webhook
import("context";"io";"log/slog";"net/http/httptest";"strings";"testing";yoopayment "github.com/rvinnie/yookassa-sdk-go/yookassa/payment")
type auditPayment struct{status string;premium bool;calls int}
func(p *auditPayment)CompletePaymentAndGrantPremium(context.Context,string)(int64,error){p.premium=true;p.status="succeeded";return 7,nil}
func(p *auditPayment)UpdatePaymentStatus(_ context.Context,_ string,status string)(int64,error){p.status=status;return 7,nil}
func(p *auditPayment)FindPayment(context.Context,string)(*yoopayment.Payment,error){p.calls++;return &yoopayment.Payment{ID:"audit",Status:yoopayment.Succeeded},nil}
func TestAuditCancellationNoVerification(t *testing.T){
 p:=&auditPayment{status:"succeeded",premium:true}
 r:=httptest.NewRequest("POST","/webhooks/yookassa",strings.NewReader("{\"type\":\"notification\",\"event\":\"payment.canceled\",\"object\":{\"id\":\"audit\",\"status\":\"canceled\"}}"));r.RemoteAddr="185.71.76.1:1234"
 w:=httptest.NewRecorder();New(slog.New(slog.NewTextHandler(io.Discard,nil)),p,p)(w,r)
 if w.Code!=200||p.calls!=0||p.status!="canceled"||!p.premium{t.Fatalf("HTTP %d state=%+v",w.Code,p)}
 t.Log("allowed IP: canceled accepted, FindPayment calls=0; updater called despite succeeded local status")
}
''',
    "notifiers/email/cmd/audit_repro_test.go": r'''
package main
import("context";"io";"log/slog";"net";"net/http";"testing";"time"
 "github.com/kirill010106/todo-notificator/notifiers/email/internal/webhook")
type auditSched struct{}
func(auditSched)Reschedule(){}
type auditSender struct{}
func(auditSender)SendVerificationEmail(string,string)error{return nil}
func TestAuditEmailTimeouts(t *testing.T){
 entered:=make(chan struct{},1)
 h:=webhook.New(slog.New(slog.NewTextHandler(io.Discard,nil)),auditSched{},auditSender{},"audit")
 mux:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){entered<-struct{}{};h.ServeHTTP(w,r)})
 srv:=&http.Server{__EMAIL_SERVER_FIELDS__}
 l,e:=net.Listen("tcp","127.0.0.1:0");if e!=nil{t.Fatal(e)}
 defer srv.Close();go srv.Serve(l)
 c,e:=net.Dial("tcp",l.Addr().String());if e!=nil{t.Fatal(e)}
 _,e=io.WriteString(c,"POST / HTTP/1.1\r\nHost: audit\r\nX-Webhook-Secret: audit\r\nContent-Length: 100\r\n\r\n{");if e!=nil{t.Fatal(e)}
 select{case <-entered:case <-time.After(time.Second):t.Fatal("handler not reached")}
 done:=make(chan error,1);go func(){done<-srv.Shutdown(context.Background())}()
 select{case e:=<-done:t.Fatalf("unexpectedly shut down before body completion: %v",e);case <-time.After(150*time.Millisecond):}
 if srv.ReadTimeout!=0||srv.ReadHeaderTimeout!=0||srv.WriteTimeout!=0||srv.IdleTimeout!=0{t.Fatal("timeout configured")}
 t.Log("actual server literal has all timeouts=0; Shutdown(background) remains blocked by unfinished body")
 c.Close();select{case <-done:case <-time.After(2*time.Second):t.Fatal("shutdown did not complete after peer closed")}
}
''',
}

email = root / "notifiers/email/cmd/main.go"
literal = re.search(r"srv := &http.Server\{(.*?)\n\t\}", email.read_text(), re.S).group(1)
literal = literal.replace("cfg.Webhook.Address", '"127.0.0.1:0"')
replacements = {}
for rel, code in cases.items():
    target = tmp / rel.replace("/", "_")
    target.write_text(code.replace("__EMAIL_SERVER_FIELDS__", literal), encoding="utf-8")
    replacements[str(root / rel)] = str(target)
overlay = tmp / "overlay.json"
overlay.write_text(json.dumps({"Replace": replacements}), encoding="utf-8")
frontend = r'''import fs from "node:fs";
import vm from "node:vm";
import assert from "node:assert/strict";
import {createRequire} from "node:module";
import path from "node:path";
const ts=createRequire(path.resolve("frontend/package.json"))("typescript");
const source=fs.readFileSync("frontend/src/lib/api.ts","utf8").replace("import.meta.env.VITE_API_BASE_URL",'""');
const code=ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.CommonJS}}).outputText;
for(const order of ["loser-first","winner-first"]){
 const values=new Map([["access_token","x."+btoa(JSON.stringify({uid:7,n:1}))+".x"],["refresh_token","old"]]);
 const storage={getItem:k=>values.get(k)??null,setItem:(k,v)=>values.set(k,v),removeItem:k=>values.delete(k)};
 const pending=[];const events=[];
 function tab(){
  const exports={};vm.runInNewContext(code,{exports,location:{hostname:"localhost"},localStorage:storage,
   window:{dispatchEvent:e=>events.push(e.type)},Event,atob,fetch:(_url,init)=>new Promise(resolve=>pending.push({resolve,body:JSON.parse(init.body)}))});
  return exports;
 }
 const a=tab(),b=tab();
 const pa=a.refreshAccess().then(()=>true,()=>false),pb=b.refreshAccess().then(()=>true,()=>false);
 assert.equal(pending.length,2);assert.equal(pending[0].body.refresh_token,"old");assert.equal(pending[1].body.refresh_token,"old");
 const success={ok:true,status:200,json:async()=>({access_token:"x."+btoa(JSON.stringify({uid:7,n:2}))+".x",refresh_token:"new"})};
 const failure={ok:false,status:401,json:async()=>({error:"invalid"})};
 if(order==="loser-first"){
  pending[1].resolve(failure);assert.equal(await pb,false);
  pending[0].resolve(success);assert.equal(await pa,false);
  assert.equal(storage.getItem("refresh_token"),null);
 }else{
  pending[0].resolve(success);assert.equal(await pa,true);
  pending[1].resolve(failure);assert.equal(await pb,false);
  assert.equal(storage.getItem("refresh_token"),"new");
 }
 console.log(JSON.stringify({order,refresh_token:storage.getItem("refresh_token"),events}));
}
'''

js = tmp / "frontend-race.mjs"
js.write_text(frontend, encoding="utf-8")
command = [
    "go", "test", f"-overlay={overlay}",
    "./backend/internal/http-server/router",
    "./backend/clients/activitylogger",
    "./backend/internal/storage/postgres",
    "./backend/internal/http-server/handlers/payments/webhook",
    "./activity-logger/cmd", "./notifiers/email/cmd",
    "-run", "^(TestAudit|TestRegistrationRollsBackWhenDeliveryCannotBeQueued)",
    "-count=1", "-v", "-p", "1",
]
print(f"Evidence directory: {tmp}", flush=True)
for label, command in [("go", command), ("frontend", ["node", str(js)])]:
    result = subprocess.run(command, cwd=root, text=True, encoding="utf-8", errors="replace",
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    (tmp / f"{label}-results.log").write_text(result.stdout, encoding="utf-8")
    print(result.stdout, flush=True)
    if result.returncode:
        raise SystemExit(result.returncode)

```
