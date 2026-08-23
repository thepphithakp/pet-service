# vertex-pet-service — Review & Refactor Plan

> เอกสารนี้เขียนเพื่อใช้เป็น input ให้ AI coding agent ทำงานต่อ
> แต่ละ Phase ออกแบบให้ **ship แยกกันได้** และมี Acceptance Criteria ชัดเจน
> สถานะโค้ด ณ วันที่รีวิว: Go 1.25.1 / Fiber v2 / GORM v1.31 / ~1,939 LOC / **0 test files** / build ผ่าน

---

## 0. TL;DR — สรุปสำหรับผู้ตัดสินใจ

โครงสร้าง hexagonal (domain / port / application / adapter) **วางไว้ถูกแล้ว** — นี่คือจุดแข็งและไม่ต้องรื้อ
ปัญหาอยู่ที่ "เนื้อใน" ที่เขียนเร็วจนข้ามเรื่องพื้นฐาน โดยเรียงตามความเร่งด่วน:

| # | ปัญหา | ระดับ | Phase |
|---|-------|-------|-------|
| 1 | **ไม่มี authorization เลย** — user ใดก็ได้ที่ login แล้ว อ่าน/แก้/ลบสัตว์เลี้ยงของคนอื่นได้ทุกตัว (IDOR ทั้ง API) | 🔴 Critical | 1 |
| 2 | `GET /api/v1/admin/pets` ไม่เช็ค role — dump ข้อมูลทั้งระบบ | 🔴 Critical | 1 |
| 3 | Mass assignment — handler bind `domain.Pet` ตรงๆ จาก body (`petRequest` DTO ประกาศไว้แต่ไม่ได้ใช้) | 🔴 Critical | 1 |
| 4 | `UpdatePermissions` รับ full permission object จาก client แล้ว `Replace` → client แก้ master data ได้ | 🔴 Critical | 1 |
| 5 | `AutoMigrate` ตอน pod start ทุก replica — race, ลบ/rename column ไม่ได้, review ใน PR ไม่ได้ | 🟠 High | 2 |
| 6 | Master data hardcode ในโค้ด (`litter_service.go`) + seed ใน `main.go` | 🟠 High | 2–3 |
| 7 | ไม่มี validation ใดๆ, ไม่มี pagination, list API ส่ง avatar blob กลับทุกตัว | 🟠 High | 1, 5 |
| 8 | ไม่มี graceful shutdown / probes / connection pool / recover middleware | 🟠 High | 6 |
| 9 | Event publisher เป็น fire-and-forget goroutine ไม่มี timeout ไม่มี retry — หายได้ทุกเมื่อ | 🟡 Medium | 7 |
| 10 | 0 test, CI ไม่มี test/lint/scan | 🟡 Medium | 0, 8 |

**คำตอบเรื่อง Flyway: ควรทำครับ** และตรงจุดทั้งข้อ 5 และ 6 — รายละเอียดใน **§4 Phase 2** และ **§5 Flyway Deep Dive**

### 📌 สรุปสิ่งที่เปลี่ยนหลังการตัดสินใจ (2026-08-22) — อ่าน **§6** ก่อนเสมอ

| การตัดสินใจ | ผลต่อแผน |
|---|---|
| ทำ **RBAC เต็มรูปแบบ** + `thappithakpluemacting@gmail.com` = SUPER_ADMIN | เพิ่ม **Phase 1A** ที่ `vertex-auth-service` เป็น prerequisite (token ปัจจุบันไม่มี `roles` claim เลย) |
| **Master data แก้ผ่าน backoffice ได้** | 🔄 **Phase 3 เขียนใหม่ทั้งเฟส** — `R__` ใช้กับตารางที่ UI แก้ได้**ไม่ได้** (จะเขียนทับทุก deploy) ต้องแยกความเป็นเจ้าของเป็น 3 ชั้น + ทำ Admin CRUD API |
| **แยก schema** ต่อ service (pet → auth → event) | Phase 2 ขยายเป็น 3–5 วัน + เพิ่ม **Phase 9, 10** |
| **backup + ข้อมูลต้องครบ 100%** | เพิ่ม **§10 Runbook** พร้อม fingerprint verification และ gate ที่หยุดงานได้ |
| **ทำขนานได้ แต่ห้ามทำให้ app พัง** | เพิ่ม **§9 Work Streams** — 3 สาย + จุดชนไฟล์ + กฎ backward compatibility |
| PATCH ที่ล้างค่า null ได้ | Phase 4.4 ทำแบบ **additive** — เพิ่ม `PATCH` แต่ `PUT` เดิมยังทำงานเหมือนเดิม |
| **litter types / genders แก้ผ่าน UI ได้ด้วย** | ➕ **Phase 3.7 ใหม่** — ถอด CHECK constraint, ใช้ FK `NOT VALID` แทน, validation เช็คกับ master data ที่ cache ไม่ใช่ enum ในโค้ด |
| water log ผูก `MANAGE_WATER` | Phase 3.4 + **ต้อง backfill** ให้ caregiver ที่มี `MANAGE_TASKS` ไม่งั้นพฤติกรรมเปลี่ยน |

### ⚠️ กับดัก 3 อย่างที่พบระหว่างวางแผน — ห้ามพลาด

1. **`handleSignup` ไม่ยืนยันอีเมล** (`vertex-auth-service/main.go:183`) → ถ้า grant admin โดยดูแค่ email string **คนอื่นสมัครด้วยอีเมลนั้นชิงสิทธิ์ SUPER_ADMIN ได้** (Phase 1A.4)
2. **`vertex-backoffice` เรียก `/api/v1/admin` อยู่จริง** → ถ้า enforce RBAC ก่อน grant สิทธิ์ **จะล็อกตัวเองออกจากระบบ** (Phase 1A.6 ลำดับ deploy)
3. **`search_path` ต้องตั้งก่อนย้าย schema** ไม่งั้นได้ `relation "pets" does not exist` ทั้งระบบทันที (Phase 2.1 ขั้นที่ 1)
4. **`code` ของ master data ต้องตรงกับค่าที่เก็บใน prod เป๊ะ ไม่ใช่ค่าที่ดูสวย** — `litter.type` ถูกส่งกลับให้ client ทาง API ตรงๆ ถ้า seed เป็น `POOP` ทั้งที่ข้อมูลจริงคือ `Poop` จะพังทั้ง FK และ response (Phase 3.7 ขั้นที่ 2)

---

## 1. รายละเอียดสิ่งที่พบจากการรีวิว

### 1.1 Security (🔴 ต้องแก้ก่อนขึ้น production จริง)

**S-1 · ไม่มี ownership / authorization check ที่จุดใดเลย**
- `internal/adapter/handler/pet.go:60` `GetOne` — รับ `:id` จาก URL แล้วยิง `GetByID` ตรงๆ
- `pet.go:93` `Update`, `pet.go:110` `Delete` — เหมือนกัน
- `caregiver.go`, `litter.go`, `water.go` — ทุก endpoint ที่มี `:id` ของ pet ไม่เคยเทียบกับ `userId` จาก token
- ผลลัพธ์: token ที่ valid ตัวใดก็ได้ → `DELETE /api/v1/pets/<uuid ใครก็ได้>` สำเร็จ
- มีแค่ `GetAll` เดียวที่ scope ตาม user (`FindAllForUser`) ซึ่งพิสูจน์ว่า "ตั้งใจจะทำ" แต่ไม่ได้ทำต่อ

**S-2 · `GET /api/v1/admin/pets` ไม่มี role check** (`pet.go:53`, route `pet.go:126`)
อยู่ใน group เดียวกับ user API และ auth middleware ไม่ได้อ่าน role/claim ใดเลยนอกจาก `sub` กับ `name`

**S-3 · Mass assignment**
`pet.go:21` ประกาศ `petRequest` DTO ครบถ้วน **แต่ไม่มีที่ไหนเรียกใช้** — `Create`/`Update` ทำ `c.BodyParser(&pet)` ลงบน `domain.Pet` โดยตรง
client จึงส่ง `createdBy`, `updatedBy`, `caregivers`, `createdAt` มาเองได้ (`id`/`ownerId` ยังโดน service เขียนทับอยู่ จึงรอด — แต่เป็นการรอดโดยบังเอิญ)

**S-4 · Privilege escalation ผ่าน caregiver permissions**
`caregiver.go:56` รับ `[]domain.PetPermission` เต็มก้อนจาก body → `repository/caregiver.go:76` แปลงเป็น `PermissionModel` แล้ว `Association("Permissions").Replace(permModels)`
GORM จะ **upsert แถวใน `pet_permissions` (master table)** ให้ด้วย → client เปลี่ยนชื่อ/คำอธิบาย/สร้าง permission ID ใหม่ตามใจได้
ถูกต้องคือรับแค่ `[]string` ของ permission ID แล้ว validate กับ master

**S-5 · JWT validation หลวม** (`pkg/middleware/auth.go:23`)
- ไม่ verify `iss` / `aud`
- ไม่ใส่ `jwt.WithValidMethods([]string{"RS256"})` (มีเช็คใน keyfunc แล้ว แต่ควรใช้ option ด้วยเป็น defense-in-depth)
- prefix check `"Bearer "` case-sensitive (spec คือ case-insensitive)
- `log.Printf("[DEBUG] Auth middleware extracted sub: %s", ...)` ยิงทุก request → noise + log user id

**S-6 · Public key ฝังใน image** (`main.go:206` อ่าน `keys/public.pem`, `Dockerfile:16` `COPY keys ./keys`)
rotate key = ต้อง rebuild + redeploy ทุก service ควรมาจาก Secret/env หรือ JWKS endpoint ของ auth-service

**S-7 · Container run เป็น root**
`Dockerfile` ใช้ `FROM scratch` + `WORKDIR /root/` ไม่มี `USER` → UID 0
`helm/pet-service/values.yaml` `securityContext: {}` และ `podSecurityContext: {}` ว่างเปล่า — ไม่มี `runAsNonRoot`, `readOnlyRootFilesystem`, `drop ALL capabilities`

**S-8 · Log ทั้ง request body และ response body ทุก request** (`main.go:78-131`)
- `BodyLimit: 50MB` + avatar upload → log บวมมหาศาล
- mask regex ครอบแค่ `avatarData`/`token` — field อื่นที่ sensitive ในอนาคตจะหลุด
- `GET /pets` ที่คืน list พร้อม avatar → response body ทั้งก้อนเข้า log
- ใช้ `fmt.Println` ไม่ใช่ structured logger — ไม่มี level, ไม่มี sampling, ปิดไม่ได้ด้วย config

**S-9 · Credentials ใน manifest**
`k8s/01-postgres.yaml:9` `POSTGRES_PASSWORD: secretpassword` เป็น plaintext ใน git
`docker-compose.yml` ก็ hardcode `vertex_admin_password`

**S-10 · ไม่มี rate limit, ไม่มี CORS policy, ไม่มี request timeout, ไม่มี `recover` middleware**
ยืนยันแล้วว่าไม่มี `recover.New()` ที่ไหนเลย — panic ใน handler จะทำให้ request นั้นตายโดย logging middleware ไม่ทำงานต่อ

### 1.2 Correctness bugs

**C-1 · `pet.go:73` unchecked type assertion**
```go
userIDStr := c.Locals("userId").(string)   // panic ถ้า nil
```
handler อื่นใช้ `, ok` หมด — เฉพาะ `Create` ที่ไม่ได้ใช้ ตอนนี้ยังไม่ระเบิดเพราะ middleware set เสมอ แต่เป็นระเบิดเวลา

**C-2 · Audit trail ผิด** (`pet_service.go:100`, `pet_service.go:127`)
```go
actorID := updatedPet.OwnerUsername    // ใส่ "username ของเจ้าของ" ลงในช่อง ActorID
```
ควรเป็น user id ของ "คนที่กดแก้" ซึ่งตอนนี้ไม่ได้ส่งเข้ามาถึงชั้น service เลย
`ActorUsername` ก็ปล่อยว่าง ทำให้ event log ของ pet ต่างจาก litter/water ที่ทำถูก

**C-3 · PUT ทำตัวเป็น PATCH และล้างค่าไม่ได้** (`pet_service.go:60-95`)
เช็ค `if incoming.X != ""` / `!= nil` ทุกฟิลด์ → **ตั้ง `microchipId` เป็น null ไม่ได้, ล้าง `allergies` ไม่ได้**
ยกเว้น `IsSpayedNeutered` ที่เขียนทับไม่มีเงื่อนไข (ไม่ consistent กับตัวอื่น)
`UpdatedBy` ไม่เคยถูกเซ็ตเลยทั้งระบบ

**C-4 · Sentinel errors ประกาศแล้วไม่มีใครคืน**
`ErrCaregiverDuplicate`, `ErrLitterLogNotFound`, `ErrInvalidID` ถูก map ใน `apperror.FromDomain` แต่ **ไม่มี service/repo ไหน return** → error พวกนี้ตกไป `default:` เป็น 500 หมด
โดยเฉพาะ duplicate caregiver: `repository/caregiver.go:57` `Create` ชน unique index `idx_pet_user` → คืน pg error ดิบ → client เห็น 500 แทน 409/400

**C-5 · `CaregiverService.Add` กลืน error** (`caregiver_service.go:26`)
```go
existing, err := s.repo.FindDeletedByPetAndUser(...)
if err == nil && existing != nil { ... }   // error อื่นที่ไม่ใช่ not-found ก็เงียบ
```

**C-6 · `PermissionRepository.Seed` โยน error ทิ้งทั้งหมด** (`repository/caregiver.go:119`)
```go
r.db.WithContext(ctx).FirstOrCreate(&m, PermissionModel{ID: p.ID})   // ไม่รับ .Error
return nil                                                           // คืน nil เสมอ
```
และ `main.go:196` เรียกด้วย `permRepo.Seed(nil, ...)` — ส่ง `nil` เป็น context

**C-7 · `SaveBatch` กับ slice ว่าง** (`repository/litter.go:43`)
`db.Create(&models)` ที่ `len == 0` คืน `gorm.ErrEmptySlice` → 500 handler ไม่ guard

**C-8 · `apperror.IsAppError` ไม่ unwrap** (`pkg/apperror/error.go:64`)
เขียน type assertion เอง แทนที่จะใช้ `errors.As` → AppError ที่ถูก wrap ด้วย `fmt.Errorf("%w")` จะไม่ถูกจับ กลายเป็น 500

**C-9 · Delete semantics ไม่ตรงกันทั้งระบบ**
- `pets`, `pet_caregivers` = soft delete (มี `gorm.DeletedAt`)
- `litter_logs`, `water_logs` = **hard delete** (ไม่มี `DeletedAt`)
- `WaterRepository.Delete` (`repository/water.go:41`) ไม่เช็ค `RowsAffected` → ลบของที่ไม่มีอยู่ก็คืน 204 ส่วน `LitterRepository.Delete` เช็ค (แต่คืน `errors.New` ดิบ ไม่ใช่ sentinel — ดู C-4)

**C-10 · ฟิลด์ `IsActive` ตายสนิท**
`litter_logs.is_active` / `water_logs.is_active` มี default true แต่ **ไม่มี query ไหนกรองด้วยเลย** — grep แล้วไม่เจอ `is_active` ใน where clause ใดๆ

**C-11 · `LitterRepository.FindByPetID` ไม่มี `ORDER BY`** ส่วน water มี `Order("date desc")` — ผลลัพธ์ที่ client ได้ไม่ deterministic

**C-12 · ไม่มี foreign key จาก log tables ไปยัง pets**
`LitterModel` / `WaterModel` มีแค่ index บน `pet_id` ไม่มี FK → ลบ pet แล้ว log ค้างเป็น orphan ตลอดกาล
(`CaregiverModel` มี `constraint:OnDelete:CASCADE` ถูกแล้ว)

**C-13 · `PermissionRepository.FindAll` ไม่มีใครเรียก** — ไม่มี endpoint ให้ frontend ดึงรายการ permission ที่มีอยู่ ทั้งที่หน้าจอตั้ง permission ต้องใช้

**C-14 · `Dockerfile.dev` พัง** — `CMD ["go", "run", "main.go"]` แต่ไม่มี `main.go` ที่ root (อยู่ที่ `cmd/server/main.go`)
`docker-compose.yml` map port `8081:8081` แต่ default ของ app คือ `4001` และ helm set `PORT=8080` → **3 ค่าไม่ตรงกัน** และ compose ใช้งานจริงไม่ได้

### 1.3 Performance / Data model

**P-1 · ส่ง avatar blob ในทุก list response**
`GET /api/v1/pets` และ `/admin/pets` → `SELECT *` รวม `avatar_data` (`bytea`) ของทุกตัว serialize เป็น base64 ใน JSON
memory limit ใน helm คือ **128Mi** — avatar 2MB × 50 ตัว ก็ OOM แล้ว

**P-2 · ไม่มี pagination ที่ endpoint ไหนเลย** — `/pets`, `/admin/pets`, `/litter-logs`, `/water-logs` คืนทุกแถวเสมอ

**P-3 · Preload ซ้อน** `Preload("Caregivers.Permissions")` ใน list query — สาม query + in-memory join ทุกครั้ง แม้ caller ไม่ต้องการ caregivers

**P-4 · ไม่มี connection pool config** (`main.go:151`) — `SetMaxOpenConns` / `SetMaxIdleConns` / `SetConnMaxLifetime` ไม่ได้ตั้ง ใช้ default ของ database/sql (unlimited open conns) กับ Postgres ที่ default `max_connections=100` และมี 3 service แชร์

**P-5 · `sslmode=disable` hardcode** ใน DSN ไม่มี env ให้ override

**P-6 · `BirthDate` เป็น `time.Time`** → column เป็น `timestamptz` ทั้งที่ควรเป็น `date`

**P-7 · `default:gen_random_uuid()` เป็นของตาย** — app เซ็ต `uuid.New()` เองทุกครั้ง และ `gen_random_uuid()` ต้องการ PG13+ / pgcrypto

### 1.4 Architecture / Layering

**A-1 · `pkg/apperror` import ทั้ง `internal/domain` และ `gorm.io/gorm`**
package ที่อยู่นอก `internal/` ไม่ควรรู้จัก domain และ **ไม่ควรรู้จัก ORM เลย** — `errors.Is(err, gorm.ErrRecordNotFound)` ควรถูกแปลงเป็น sentinel ที่ชั้น repository แทน

**A-2 · `MasterDataService` และ `MasterDataHandler` ซ่อนอยู่ใน `litter_service.go` / `litter.go`** — คนละ bounded context กันสนิท

**A-3 · `internal/port/water.go` แหกแบบ** — port อื่นแยกเป็น `input.go` / `output.go` ตาม direction ส่วน water รวมทั้งสองไว้ไฟล์เดียวตาม feature

**A-4 · Wiring ทั้งหมดอยู่ใน `main.go` 213 บรรทัด** — ปนกันทั้ง DI, migration, seed, logging middleware, key loading, config

**A-5 · ไม่มี config struct** — `os.Getenv` กระจายอยู่ 3 ไฟล์ (`main.go`, `event/http_publisher.go`) ไม่มี validation ไม่ fail-fast ถ้า `DB_HOST` ว่าง

**A-6 · Actor (ผู้กระทำ) ไม่ถูกส่งลงไปถึงชั้น service** — handler อ่าน `c.Locals` แล้วยัดใส่ field ของ entity เอง ทำให้ pet ทำผิด (C-2) ส่วน litter/water ทำถูกโดยบังเอิญ

### 1.5 Ops / DX

**O-1 · ไม่มี liveness / readiness probe ใน helm deployment** ทั้งที่ `/health` มีอยู่
**O-2 · ไม่มี graceful shutdown** — `log.Fatal(app.Listen(...))` ไม่จับ SIGTERM → rolling update ตัด in-flight request ทิ้ง และ goroutine ที่ค้างส่ง event ตายหมด
**O-3 · `/health` ไม่เช็ค DB** — คืน `"OK"` เสมอ ใช้เป็น readiness ไม่ได้
**O-4 · ไม่มี test เลย 0 ไฟล์** — และไม่มี interface mock/fake สำหรับ port ทั้งที่ออกแบบมาเพื่อสิ่งนี้
**O-5 · CI ไม่มี `go test`, `go vet`, lint, `govulncheck`, image scan** — push main แล้ว deploy prod ทันทีไม่มี gate
**O-6 · CI เขียน kubeconfig ด้วย `echo "${{ secrets.KUBECONFIG_CONTENT }}"`** — `echo` แปลง backslash เพี้ยนได้ ควรใช้ base64 หรือ heredoc
**O-7 · Repo hygiene** — `patch_pet_service.py` ถูก track ใน git, `.idea/` มีอยู่ใน working tree, `pet-service` binary / `vertex-pet-service.tar` / `kubeconfig-vertex.yaml` อยู่ใน dir (มี gitignore แล้วแต่ไฟล์ยังอยู่)
**O-8 · ไม่มี Makefile / taskfile / README / API doc (OpenAPI)**
**O-9 · `go.mod` mark ทุก dependency เป็น `// indirect`** ทั้งที่ fiber/gorm/jwt เป็น direct — ไม่เคยรัน `go mod tidy`

---

## 2. เป้าหมาย & ขอบเขต

### เป้าหมาย
1. **ปิดช่องโหว่ authorization ให้หมดก่อนอย่างอื่น** + ทำ **RBAC** เต็มรูปแบบ (ข้อ 1)
2. เปลี่ยน schema management จาก `AutoMigrate` → **Flyway** ที่ review ได้ / rollback ได้ / รันแยกจาก app
3. **แยก schema ต่อ service** (`pet` / `auth` / `event`) ทีละตัว เริ่มที่ pet (ข้อ 2, 3)
4. ย้าย master data จาก hardcode → ตารางใน DB + **เปิดให้ backoffice แก้ได้** (ข้อ 4)
5. ทำให้ service มี test, observability, และ lifecycle ที่ production-ready
6. **ไม่รื้อ hexagonal architecture** — ทำให้มันถูกต้องขึ้น ไม่ใช่เปลี่ยนแบบ
7. **ตลอดทาง: app ต้องทำงานได้เหมือนเดิม และข้อมูลต้องครบ 100%** (ข้อ 7, 8)

### Non-goals (จงใจไม่ทำในรอบนี้)
- **ไม่แยก database** ต่อ service — แยกแค่ระดับ **schema** ก่อน (แยก database เป็นก้าวถัดไปหลัง §Phase 10)
- ไม่เปลี่ยน framework (Fiber v2 อยู่ต่อ)
- ไม่ย้ายไป message broker จริง (Kafka/NATS) — ใช้ outbox pattern กับ HTTP ไปก่อน
- ไม่ทำ multi-tenancy
- ไม่ทำ refresh token / ลด token TTL (ควรมี ticket แยก — ดู Phase 1A.5)
- ไม่ย้าย avatar ไป object storage (ยังไม่มี MinIO/S3 ในคลัสเตอร์ — ดู §6 ข้อ 5)

---

## 3. Target structure

```
vertex-pet-service/
├── cmd/
│   ├── server/main.go              # เหลือแค่: load config → bootstrap → run → graceful shutdown
│   └── migrate/main.go             # (optional) CLI ตรวจ schema version / รัน flyway ผ่าน docker
├── internal/
│   ├── config/config.go            # Config struct + Load() + Validate()  ← ใหม่
│   ├── bootstrap/                  # DI wiring ทั้งหมดย้ายมาจาก main.go   ← ใหม่
│   │   ├── app.go
│   │   ├── db.go
│   │   └── routes.go
│   ├── domain/
│   │   ├── pet.go  caregiver.go  litter_log.go  water_log.go  permission.go
│   │   ├── masterdata.go           # ← ใหม่ (CatBreed, BloodType, LitterType)
│   │   ├── actor.go                # ← ใหม่ (Actor{UserID, Username, Roles})
│   │   └── errors.go
│   ├── port/
│   │   ├── input.go  output.go     # water ย้ายมารวมตาม direction (ลบ water.go)
│   │   └── event.go
│   ├── application/
│   │   ├── pet_service.go  caregiver_service.go  litter_service.go  water_service.go
│   │   ├── masterdata_service.go   # ← แยกออกจาก litter_service.go
│   │   └── authz.go                # ← ใหม่: PetAccessChecker (owner หรือ caregiver ที่มีสิทธิ์)
│   └── adapter/
│       ├── handler/
│       │   ├── dto/                # ← ใหม่: request/response DTO ทุกตัว + mapper
│       │   ├── pet.go  caregiver.go  litter.go  water.go  masterdata.go  health.go
│       ├── repository/
│       │   ├── model/              # ← แยก GORM model ออกจาก repo (model.go 170 บรรทัดกำลังบวม)
│       │   └── pet.go  caregiver.go  permission.go  litter.go  water.go  masterdata.go
│       └── event/http_publisher.go
├── pkg/
│   ├── apperror/error.go           # ตัด dependency ต่อ gorm + domain ออก
│   ├── middleware/
│   │   ├── auth.go  error_handler.go
│   │   ├── requestid.go  logging.go  recover.go  timeout.go   # ← แยกออกจาก main.go
│   └── logger/logger.go            # ← ใหม่: log/slog wrapper
├── db/                             # ← ใหม่ ทั้งหมด
│   ├── flyway.conf
│   ├── migration/                  # V__ schema (DDL)
│   ├── bootstrap/                  # one-off: ย้าย schema (ไม่อยู่ใน FLYWAY_LOCATIONS)
│   ├── codeowned/                  # R__ เฉพาะตารางที่ UI แก้ไม่ได้ (permissions, capabilities)
│   ├── verify/fingerprint.sql      # พิสูจน์ข้อมูลครบ (§10.3)
│   ├── rollback/                   # เอกสารประกอบ ไม่ถูกรันอัตโนมัติ
│   ├── seed/                       # เฉพาะ local/dev — ไม่รันบน prod
│   └── Dockerfile                  # FROM flyway/flyway + COPY sql
├── docs/
│   ├── REFACTOR_PLAN.md
│   ├── openapi.yaml                # ← ใหม่
│   └── MIGRATION_GUIDE.md          # ← ใหม่ (กติกาการเขียน migration)
├── Makefile                        # ← ใหม่
└── docker-compose.dev.yml          # ← ใหม่: postgres + flyway + app (แทนของเดิมที่พัง)
```

---

## 4. แผนงานรายเฟส

> **กติกาสำหรับ AI agent:** ทำทีละ Phase, ห้ามข้าม, จบแต่ละ Phase ต้อง `go build ./...` + `go vet ./...` + `go test ./...` ผ่าน และ commit แยก
> Phase 0 กับ Phase 1 ควรทำก่อนเสมอ ส่วน Phase 2 ทำคู่ขนานกับ Phase 1 ได้ถ้ามีคนสองคน

---

### Phase 0 — Safety net & repo hygiene (ครึ่งวัน)

ยังไม่แตะ logic ใดๆ — สร้างพื้นที่ให้ refactor ต่อได้อย่างปลอดภัย

**งาน**
1. `go mod tidy` (แก้ O-9 — ตอนนี้ทุก dep เป็น `// indirect` ผิดหมด)
2. ลบไฟล์ขยะ: `patch_pet_service.py` (ถูก track ใน git → `git rm`), `batch_endpoints.patch`, `pet-service` binary, `vertex-pet-service.tar`, `.idea/`
   เพิ่ม `.idea/`, `*.tar`, `kubeconfig*.yaml` ใน `.gitignore` (มีแล้วบางส่วน แต่ไฟล์ยังค้างอยู่ใน tree)
3. **หมุน credential ทุกตัวที่เคยอยู่ใน git** — `k8s/01-postgres.yaml` มี `POSTGRES_PASSWORD: secretpassword` แบบ plaintext ให้ย้ายไป sealed-secret / external-secret แล้วเปลี่ยนรหัสจริง
4. เพิ่ม `Makefile`: `build test lint run migrate migrate-info db-up db-down docker-dev`
5. เพิ่ม `.golangci.yml` (แนะนำ: `errcheck, govet, staticcheck, gosec, ineffassign, revive, bodyclose, sqlclosecheck`)
6. เขียน **characterization test** ครอบ endpoint ที่มีอยู่ทั้งหมดก่อน refactor — ใช้ `testcontainers-go` + postgres จริง หรือ `httptest` + fake repository
   จุดประสงค์คือ "ล็อกพฤติกรรมปัจจุบัน" ไม่ใช่ "พิสูจน์ว่าถูก" — ที่พฤติกรรมเป็น bug ให้เขียน test แล้วมาร์ค `t.Skip("known bug: S-1 IDOR, fixed in Phase 1")`
7. แก้ CI (`.github/workflows/deploy.yml`) เพิ่ม job `test` ที่รัน `go vet` + `go test ./...` + `golangci-lint` + `govulncheck` และ `needs: test` ที่ build job

**Acceptance**
- [ ] `make test` รันผ่านบนเครื่องเปล่าที่มีแค่ docker
- [ ] `git ls-files` ไม่มีไฟล์ binary / patch / kubeconfig / .idea
- [ ] CI fail ถ้า test fail (ทดสอบด้วยการทำให้ fail จริงหนึ่งครั้ง)
- [ ] `go mod tidy` แล้ว diff ของ `go.mod` แสดง direct dependency ถูกต้อง

---

### Phase 1A — RBAC ที่ `vertex-auth-service` 🔴 (1–2 วัน) — **prerequisite ของ Phase 1.2**

> ทำที่ repo `vertex-auth-service` ไม่ใช่ pet-service — แต่ต้องเสร็จก่อน pet-service จะ enforce role ได้
> ทำขนานกับ Phase 1.1 / 1.3–1.6 ของ pet-service ได้เต็มที่

**สภาพปัจจุบัน** (`vertex-auth-service/main.go`)
- `users` มีแค่ `id, email, password_hash, full_name, created_at, updated_at` — **ไม่มี role, ไม่มี email_verified**
- token ออกแค่ `sub`, `email`, `name`, `exp(72h)` — ไม่มี `roles`, `iss`, `aud`
- `handleSignup` (`main.go:183`) สมัครด้วย password ได้ทันทีโดยไม่ยืนยันอีเมล

**1A.1 · โมเดล RBAC**

แยกเป็น 2 ชั้นชัดเจน — **อย่าเอามาปนกัน**:

| ชั้น | เก็บที่ | ตอบคำถาม | ตัวอย่าง |
|---|---|---|---|
| **Global RBAC** | auth-service → `roles` claim ใน JWT | "คนนี้เป็นใครในระบบ" | `SUPER_ADMIN`, `PET_ADMIN`, `USER` |
| **Resource ACL** | pet-service → `pet_caregivers` + `caregiver_permissions` | "คนนี้ทำอะไรกับ **สัตว์เลี้ยงตัวนี้** ได้" | `EDIT_PROFILE`, `MANAGE_LITTER` |

**หลักการ:** JWT พก **`roles` เท่านั้น** ไม่พก permission list
แต่ละ service map `role → capability` ในตารางของตัวเอง (seed ด้วย migration)
→ token เล็ก, service ไม่ผูกกัน, เพิ่ม capability ใหม่ที่ pet-service ได้โดยไม่ต้องแตะ auth-service

**1A.2 · Schema (migration ที่ auth-service)**
```sql
CREATE TABLE roles (
    code        varchar(50)  PRIMARY KEY,
    name        varchar(200) NOT NULL,
    description text,
    is_system   boolean      NOT NULL DEFAULT false,  -- ห้ามลบ/แก้ผ่าน UI
    created_at  timestamptz  NOT NULL DEFAULT now()
);

CREATE TABLE user_roles (
    user_id    uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_code  varchar(50) NOT NULL REFERENCES roles(code),
    granted_at timestamptz NOT NULL DEFAULT now(),
    granted_by uuid,
    PRIMARY KEY (user_id, role_code)
);
CREATE INDEX idx_user_roles_user ON user_roles(user_id);

-- รายชื่อที่จะได้สิทธิ์อัตโนมัติ (แก้ผ่าน migration เท่านั้น ห้ามเปิดให้ UI แก้)
CREATE TABLE bootstrap_admins (
    email      varchar(320) PRIMARY KEY,
    role_code  varchar(50)  NOT NULL REFERENCES roles(code),
    note       text,
    granted_at timestamptz,                -- null = ยังไม่เคย grant
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE users ADD COLUMN email_verified boolean NOT NULL DEFAULT false;
-- บัญชีที่เคย login ผ่าน google ถือว่า verified แล้ว
UPDATE users SET email_verified = true
WHERE id IN (SELECT user_id FROM oauth_identities WHERE provider = 'google');
```

**1A.3 · Seed roles**
```sql
INSERT INTO roles (code, name, description, is_system) VALUES
    ('SUPER_ADMIN', 'Super Administrator', 'ทำได้ทุกอย่างในทุก service', true),
    ('PET_ADMIN',   'Pet Administrator',   'จัดการข้อมูลสัตว์เลี้ยงและ master data', true),
    ('USER',        'General User',        'ผู้ใช้ทั่วไป จัดการเฉพาะข้อมูลของตัวเอง', true)
ON CONFLICT (code) DO NOTHING;
```

**1A.4 · Bootstrap admin — `thappithakpluemacting@gmail.com`** 🔐

```sql
INSERT INTO bootstrap_admins (email, role_code, note) VALUES
    ('thappithakpluemacting@gmail.com', 'SUPER_ADMIN', 'initial system owner')
ON CONFLICT (email) DO NOTHING;

-- grant ทันที เฉพาะบัญชีที่ "มีอยู่แล้ว" และ "email ยืนยันแล้ว"
INSERT INTO user_roles (user_id, role_code)
SELECT u.id, b.role_code
FROM users u
JOIN bootstrap_admins b ON lower(u.email) = lower(b.email)
WHERE u.email_verified = true
ON CONFLICT DO NOTHING;

UPDATE bootstrap_admins b SET granted_at = now()
WHERE granted_at IS NULL
  AND EXISTS (SELECT 1 FROM users u WHERE lower(u.email) = lower(b.email) AND u.email_verified = true);
```

⚠️ **ข้อบังคับด้านความปลอดภัย — ห้ามข้าม**
`handleSignup` ไม่มีการยืนยันอีเมล ถ้าปล่อยให้ grant โดยดูแค่ email string **คนอื่นสมัครด้วยอีเมลนั้นชิงสิทธิ์ SUPER_ADMIN ไปได้ทันที**
กติกาใน application code:
```go
// เรียกหลัง login/signup สำเร็จทุกครั้ง
func reconcileBootstrapAdmin(ctx context.Context, u User, provider string) error {
    // grant อัตโนมัติได้เฉพาะเมื่อ email ถูกยืนยันแล้วเท่านั้น
    // (provider == "google" → ถือว่า verified, password signup → ไม่ให้)
    if !u.EmailVerified { return nil }
    // ... INSERT INTO user_roles ... ON CONFLICT DO NOTHING
}
```
- signup ด้วย password → `email_verified = false` เสมอ → **ไม่ได้สิทธิ์**
- login ด้วย Google → set `email_verified = true` → reconcile → ได้สิทธิ์
- 📌 **ถ้าบัญชีของคุณมีอยู่แล้วใน prod:** ตรวจก่อนว่าเคย login ด้วย Google ไหม
  ```sql
  SELECT u.id, u.email, oi.provider FROM users u
  LEFT JOIN oauth_identities oi ON oi.user_id = u.id
  WHERE lower(u.email) = 'thappithakpluemacting@gmail.com';
  ```
  ถ้าเป็นบัญชี password ล้วน → grant ด้วยมือครั้งเดียวโดยอ้าง **`user_id`** ไม่ใช่ email แล้วบันทึกไว้ใน migration เป็น one-off `V__`

**1A.5 · แก้ token**
```go
func generateToken(user User, roles []string) (string, error) {
    now := time.Now()
    return jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
        "iss":   cfg.JWTIssuer,      // "https://auth.vertex.local"
        "aud":   cfg.JWTAudience,    // "vertex-api"
        "sub":   user.ID.String(),
        "email": user.Email,
        "name":  user.FullName,
        "roles": roles,              // ← ใหม่
        "iat":   now.Unix(),
        "jti":   uuid.New().String(),
        "exp":   now.Add(cfg.AccessTokenTTL).Unix(),
    }).SignedString(parsedPrivateKey)
}
```
⚠️ **`iss`/`aud` ต้องปล่อยแบบ 2 เฟส** ไม่งั้น token ที่ user ถืออยู่ (อายุ 72 ชม.) จะใช้ไม่ได้ทันที:
1. auth-service เริ่ม**ออก** `iss`/`aud`/`roles` — pet-service **ยังไม่ validate** (ถ้าไม่มี `roles` → ถือเป็น `["USER"]`)
2. รออย่างน้อย 72 ชั่วโมง (เท่าอายุ token เดิม)
3. pet-service เปิด validate `iss`/`aud` เต็มรูปแบบ

🔸 **งานที่ควรทำแยก ticket (ไม่บล็อก):** ลด `exp` เหลือ 15–60 นาที + เพิ่ม refresh token — ตอนนี้ token ที่หลุดใช้ได้ 3 วันเต็ม

**1A.6 · ลำดับ deploy (สำคัญมาก — กัน backoffice ล็อกตัวเองออก)**
```
1. auth: รัน migration (roles, user_roles, bootstrap_admins, email_verified)
2. auth: ยืนยันด้วยตาว่า SELECT * FROM user_roles มีแถวของบัญชีคุณจริง   ← gate
3. auth: deploy code ที่ออก roles claim
4. login ใหม่ 1 ครั้ง แล้ว decode token ยืนยันว่ามี "roles": ["SUPER_ADMIN"]  ← gate
5. pet: ค่อย deploy โค้ดที่ enforce role
```
> **ห้ามสลับลำดับ 4 กับ 5 เด็ดขาด** — ถ้า enforce ก่อนที่ token จะมี roles จะเข้า backoffice ไม่ได้เลย

**1A.7 · Admin API สำหรับจัดการ role** (ใช้ที่ backoffice)
- `GET /api/v1/admin/users?q=&page=` — รายชื่อ user + roles
- `PUT /api/v1/admin/users/:id/roles` — body `{"roles":["PET_ADMIN"]}`
- `GET /api/v1/admin/roles` — รายการ role ทั้งหมด
- ทุก endpoint ต้องการ `SUPER_ADMIN`
- 🚫 **ห้ามให้ user ถอด `SUPER_ADMIN` ของตัวเอง** และ **ห้ามให้ระบบเหลือ SUPER_ADMIN 0 คน** (เช็คในระดับ service + `V__` เขียน constraint ไม่ได้ ต้องเช็คในโค้ด)
- ทุกการเปลี่ยน role ต้อง publish event ไป event-service (ใครให้สิทธิ์ใครเมื่อไหร่)

**Acceptance Phase 1A**
- [ ] `thappithakpluemacting@gmail.com` login แล้วได้ token ที่มี `"roles":["SUPER_ADMIN"]`
- [ ] สมัครบัญชีใหม่ด้วย password โดยใช้อีเมลที่อยู่ใน `bootstrap_admins` → **ไม่ได้** สิทธิ์ (มี test ครอบข้อนี้โดยเฉพาะ)
- [ ] user ปกติได้ `["USER"]`
- [ ] token เก่าที่ไม่มี `roles` ยังใช้งาน API เดิมได้ (ถือเป็น `USER`)
- [ ] ถอด SUPER_ADMIN คนสุดท้ายออก → ถูกปฏิเสธ

---

### Phase 1 — ปิดช่องโหว่ security 🔴 (1–2 วัน)

**งานที่ 1.1 — Authorization layer (แก้ S-1, S-2)**

สร้าง `internal/application/authz.go`:
```go
type PetAccessLevel int
const (
    AccessNone PetAccessLevel = iota
    AccessCaregiver           // เป็น caregiver ของ pet ตัวนี้
    AccessOwner               // เป็นเจ้าของ
)

type PetAccessChecker interface {
    // คืน access level ของ actor ต่อ pet ตัวนี้ + permission ที่มี
    Check(ctx context.Context, petID uuid.UUID, actor domain.Actor) (PetAccessLevel, []string, error)
}
```
- เพิ่ม `PetRepository.FindAccess(ctx, petID, userID) (ownerID uuid.UUID, permissions []string, err error)` — query เดียว join `pets` + `pet_caregivers` + `caregiver_permissions` ไม่ต้อง preload ทั้งก้อน
- **บังคับใช้ที่ชั้น application service ไม่ใช่ handler** — เพราะ handler จะโดนข้ามได้ถ้ามี caller ใหม่ (เช่น gRPC, cron)
- กติกาต่อ operation:

  | Operation | ต้องการ |
  |---|---|
  | `GET /pets/:id` | Owner หรือ Caregiver |
  | `PUT /pets/:id` | Owner หรือ Caregiver ที่มี `EDIT_PROFILE` |
  | `DELETE /pets/:id` | Owner เท่านั้น |
  | `GET/POST/DELETE /pets/:id/caregivers*` | Owner เท่านั้น |
  | `GET /pets/:id/litter-logs` | Owner หรือ Caregiver |
  | `POST/DELETE /pets/:id/litter-logs*` | Owner หรือ Caregiver ที่มี `MANAGE_LITTER` |
  | `GET/POST/DELETE /pets/:id/water-logs*` | Owner หรือ Caregiver ที่มี `MANAGE_WATER` (permission ใหม่ — ดู Phase 3.4) |
  | `GET /admin/pets` | capability `pet:read:any` — **ดูตารางฉบับสมบูรณ์ในงานที่ 1.2** |

- **สำคัญ:** เมื่อไม่มีสิทธิ์ ให้คืน **404** (ไม่ใช่ 403) สำหรับ resource ที่ไม่ใช่ของตัวเอง เพื่อไม่ให้ enumerate UUID ได้ ยกเว้นกรณีเป็น caregiver แล้วสิทธิ์ไม่พอ ให้คืน 403
- `DELETE /pets/:id/litter-logs/:logId` และ water เดียวกัน: ต้อง verify ว่า `logId` นั้นอยู่ใต้ `petID` จริง (ตอนนี้ลบ log ของ pet อื่นผ่าน URL ของ pet ตัวเองได้)

**งานที่ 1.2 — เชื่อม RBAC เข้ากับ pet-service (แก้ S-2) — ต้องรอ Phase 1A**

**Actor จาก token**
```go
type Actor struct {
    UserID   uuid.UUID
    Username string
    Email    string
    Roles    []string
}
func (a Actor) HasRole(r string) bool
```
middleware อ่าน claim `roles` (ถ้าไม่มี → `["USER"]` เพื่อ backward compat ระหว่างเฟสเปลี่ยนผ่าน) แล้วใส่ลง `context.Context`

**ตาราง capability ของ pet-service** (migration `V__` + seed ด้วย `R__` เพราะเป็น **code-owned** — capability ผูกกับโค้ดที่บังคับใช้ ไม่ควรแก้ผ่าน UI)
```sql
CREATE TABLE role_capabilities (
    role_code  varchar(50)  NOT NULL,
    capability varchar(100) NOT NULL,
    PRIMARY KEY (role_code, capability)
);
```
`R__0005_role_capabilities.sql`:
```sql
INSERT INTO role_capabilities (role_code, capability) VALUES
    ('SUPER_ADMIN', 'pet:read:any'),      ('SUPER_ADMIN', 'pet:write:any'),
    ('SUPER_ADMIN', 'pet:delete:any'),    ('SUPER_ADMIN', 'caregiver:manage:any'),
    ('SUPER_ADMIN', 'masterdata:write'),  ('SUPER_ADMIN', 'log:write:any'),
    ('PET_ADMIN',   'pet:read:any'),      ('PET_ADMIN',   'pet:write:any'),
    ('PET_ADMIN',   'masterdata:write')
ON CONFLICT DO NOTHING;

DELETE FROM role_capabilities
WHERE (role_code, capability) NOT IN (
    ('SUPER_ADMIN','pet:read:any'),('SUPER_ADMIN','pet:write:any'),
    ('SUPER_ADMIN','pet:delete:any'),('SUPER_ADMIN','caregiver:manage:any'),
    ('SUPER_ADMIN','masterdata:write'),('SUPER_ADMIN','log:write:any'),
    ('PET_ADMIN','pet:read:any'),('PET_ADMIN','pet:write:any'),
    ('PET_ADMIN','masterdata:write')
);
```
> ตารางนี้เป็นข้อยกเว้นที่ยังใช้ `R__` ได้ เพราะ **ไม่เปิดให้ backoffice แก้** (ต่างจาก master data ทั่วไปใน Phase 3)

**กติกาการตัดสินใจ — ชั้นเดียวกันทั้งระบบ**
```go
func (a *Authorizer) Allow(ctx, actor, petID, need Requirement) error {
    // 1) global capability (admin path) — ผ่านได้เลย ไม่ต้องดู ownership
    if a.caps.HasAny(actor.Roles, need.Capabilities...) { return nil }
    // 2) resource ACL (owner / caregiver path)
    lvl, perms, err := a.pets.FindAccess(ctx, petID, actor.UserID)
    ...
}
```

**ตารางสิทธิ์ฉบับสมบูรณ์** (แทนตารางในงานที่ 1.1)

| Endpoint | Global capability ที่ผ่านได้ | หรือ Resource ACL |
|---|---|---|
| `GET /pets` | — | scope เฉพาะของตัวเอง (เหมือนเดิม) |
| `GET /admin/pets` | `pet:read:any` | ❌ ไม่มีทางอื่น |
| `GET /pets/:id` | `pet:read:any` | Owner หรือ Caregiver |
| `PATCH /pets/:id` | `pet:write:any` | Owner หรือ Caregiver + `EDIT_PROFILE` |
| `DELETE /pets/:id` | `pet:delete:any` | Owner เท่านั้น |
| `*/caregivers*` | `caregiver:manage:any` | Owner เท่านั้น |
| `GET /pets/:id/litter-logs` | `pet:read:any` | Owner หรือ Caregiver |
| `POST|DELETE /pets/:id/litter-logs*` | `log:write:any` | Owner หรือ Caregiver + `MANAGE_LITTER` |
| `GET /pets/:id/water-logs` | `pet:read:any` | Owner หรือ Caregiver |
| `POST|DELETE /pets/:id/water-logs*` | `log:write:any` | Owner หรือ Caregiver + `MANAGE_WATER` ⬅️ permission ใหม่ |
| `*/admin/master-data/*` | `masterdata:write` | ❌ ไม่มีทางอื่น |

**เพิ่ม `MANAGE_WATER` เข้า master permission** (ตามการตัดสินใจข้อ 5) — ดู Phase 3.4 เรื่องการ backfill ให้ caregiver เดิม

**ย้าย route**
```go
api   := app.Group("/api/v1",       authMW)                          // ผู้ใช้ทั่วไป
admin := app.Group("/api/v1/admin", authMW, requireAnyCapability(...)) // แยก group ชัดเจน
```
⚠️ **ห้าม deploy ก่อน Phase 1A.6 ขั้นที่ 4 ผ่าน** — ไม่งั้น backoffice เข้าไม่ได้

**Backward compatibility (ตามการตัดสินใจข้อ 7)**
- token เก่าที่ไม่มี `roles` → ถือเป็น `["USER"]` ใช้ API ของตัวเองได้ตามปกติ ไม่พัง
- `/admin/pets` จะเข้มขึ้นทันที — **นี่คือการเปลี่ยนพฤติกรรมที่ตั้งใจ** และเป็นการอุดช่องโหว่ ต้องยืนยันว่า backoffice login ด้วยบัญชี admin ได้ก่อน

**งานที่ 1.3 — DTO + validation (แก้ S-3, C-1)**
- สร้าง `internal/adapter/handler/dto/` แยก `CreatePetRequest`, `UpdatePetRequest`, `PetResponse`, `PetListItemResponse` (ไม่มี avatar), `CreateLitterLogRequest`, ฯลฯ
- handler bind DTO เท่านั้น **ห้าม bind `domain.*` ตรงๆ** แล้ว map เป็น domain object
- ลบ `petRequest` เดิมที่ dead code
- ใช้ `go-playground/validator/v10` + middleware ที่แปลง validation error → 400 พร้อม field-level detail
- กติกา validation ขั้นต่ำ:
  - **structural** (ทำด้วย tag ได้): `name` required 1–100 ตัวอักษร, `birthDate` ไม่อยู่อนาคต, `amount` > 0 และ ≤ ค่า max ที่สมเหตุสมผล, `avatarData` ≤ 2MB
  - **master-data-backed** — 🚫 **ห้าม hardcode enum ในโค้ด** เพราะ admin เพิ่มค่าใหม่ผ่าน backoffice ได้: `species`, `gender`, `breed`, `bloodType`, `litter.type` ต้องเช็คกับ **master data ที่ cache ไว้** (Phase 3.5)
    ```go
    // ❌ ผิด — เพิ่มชนิดใหม่ใน UI แล้วใช้ไม่ได้
    Type string `validate:"oneof=POOP PEE"`
    // ✅ ถูก — เช็คตอน runtime กับ master data ที่ active อยู่
    if !h.master.IsValid(ctx, masterdata.LitterType, req.Type) { return apperror.BadRequest(...) }
    ```
  - ⚠️ **ข้อควรระวัง:** ระหว่าง Phase 1 ที่ master data ยังไม่ย้ายเข้า DB (Phase 3) ให้ validate แบบ **soft** ไปก่อน (เช็คความยาว + ไม่ว่าง) **อย่าเพิ่งใส่ enum ตายตัว** ไม่งั้นจะต้องกลับมารื้อ และอาจ reject ค่าที่มีอยู่จริงใน prod
- แก้ `pet.go:73` ให้ใช้ `, ok` แล้วคืน 401

**งานที่ 1.4 — Caregiver permission (แก้ S-4)**
- เปลี่ยน request body เป็น `{"permissionIds": ["EDIT_PROFILE", ...]}`
- service validate ทุก ID กับ `PermissionRepository.FindAll()` ก่อน ถ้ามีตัวไหนไม่รู้จัก → 400
- repository เปลี่ยนจาก `Replace(permModels)` เป็นการเขียนตาราง join `caregiver_permissions` ตรงๆ (delete + insert ใน transaction) เพื่อไม่ให้แตะ master table
- เพิ่ม endpoint `GET /api/v1/master-data/pet-permissions` (แก้ C-13)

**งานที่ 1.5 — JWT & runtime hardening (แก้ S-5, S-7, S-10)**
- `jwt.Parse(..., jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(cfg.JWTIssuer), jwt.WithAudience(cfg.JWTAudience), jwt.WithLeeway(30*time.Second))`
- prefix check เป็น case-insensitive
- ลบ `log.Printf("[DEBUG] ...")` ออก
- เพิ่ม `recover.New()`, `limiter.New()`, `timeout` middleware
- Dockerfile: เพิ่ม non-root user, `WORKDIR /app`
  ```dockerfile
  FROM scratch
  COPY --from=builder /etc/passwd /etc/passwd   # หรือใช้ gcr.io/distroless/static:nonroot
  USER 65532:65532
  ```
  **แนะนำเปลี่ยนเป็น `gcr.io/distroless/static:nonroot`** ตรงไปตรงมากว่า scratch
- helm values: `podSecurityContext: {runAsNonRoot: true, runAsUser: 65532, fsGroup: 65532, seccompProfile: {type: RuntimeDefault}}`, `securityContext: {allowPrivilegeEscalation: false, readOnlyRootFilesystem: true, capabilities: {drop: [ALL]}}`

**งานที่ 1.6 — Logging (แก้ S-8)**
- ย้าย logging middleware จาก `main.go` ไป `pkg/middleware/logging.go` ใช้ `log/slog` (stdlib)
- **default: ไม่ log body** เปิดได้ด้วย `LOG_BODY=true` เฉพาะ non-prod
- ถ้าเปิด: จำกัดขนาดที่ 4KB, ใช้ allowlist ของ path แทน blanket, และใช้ **field-name denylist ที่ maintain รวมศูนย์** แทน regex ตัวเดียว
- ไม่ log response body ของ list endpoint เลย

**Acceptance**
- [ ] มี test ที่พิสูจน์ว่า user A เข้าถึง pet ของ user B ไม่ได้ ทุก endpoint (ทั้ง 4xx และไม่มี data leak)
- [ ] `GET /admin/pets` ด้วย token ธรรมดา → 403/404
- [ ] ส่ง `{"createdBy":"hacker"}` ใน body ของ `POST /pets` แล้วค่านั้นไม่ถูกเก็บ
- [ ] `PUT /caregivers/:id` ที่ส่ง permission ID มั่ว → 400 และ `pet_permissions` ไม่เปลี่ยน
- [ ] container รันด้วย non-root (ยืนยันด้วย `kubectl exec -- id`)
- [ ] test ที่ `t.Skip` ไว้ใน Phase 0 ปลด skip แล้วผ่านหมด

---

### Phase 2 — Flyway + แยก schema `pet` 🟠 (3–5 วัน)

> อ่าน **§5 (Flyway Deep Dive)** และ **§10 (Backup & Data Verification Runbook)** ให้จบก่อนลงมือ
> เฟสนี้แตะข้อมูล production → **ทุกขั้นตอนต้องมี backup และการพิสูจน์จำนวนแถว**

**2.0 · Gate ก่อนเริ่ม** — รันคำสั่งนับจำนวนแถวใน §6 ข้อ A ก่อน แล้วเลือกเส้นทางตาม §10.1
เอกสารส่วนที่เหลือเขียนตาม **เส้นทาง A (มีข้อมูลจริง / ยอม downtime ไม่ได้)** ซึ่งเป็นเส้นทางที่ปลอดภัยที่สุด

**2.1 · แยก schema `pet` — ทำ *ก่อน* baseline**

ทำก่อนเพราะถ้าใส่ Flyway ที่ `public` แล้วค่อยย้าย จะต้องย้าย `flyway_schema_history` ตามไปด้วยและแก้ config กลางทาง — ยุ่งกว่าโดยไม่จำเป็น

> 💡 **ข่าวดีเรื่อง "ต้องย้ายข้อมูลมาให้ครบ":** `ALTER TABLE ... SET SCHEMA` ใน PostgreSQL เป็น **metadata-only operation — ไม่มีการ copy ข้อมูล ไม่มีการเขียนไฟล์ใหม่ ทำเสร็จในเสี้ยววินาที** และเนื่องจาก DDL ของ PostgreSQL เป็น transactional จึงห่อทั้งหมดไว้ใน transaction เดียวได้ — สำเร็จทั้งหมด หรือ rollback ทั้งหมด **ไม่มีสถานะครึ่งๆ กลางๆ**
> ความเสี่ยงที่แท้จริงจึงไม่ใช่ "ข้อมูลหาย" แต่คือ "**app หาตารางไม่เจอ**" ระหว่างเปลี่ยนผ่าน — ซึ่งแก้ด้วย `search_path` (ขั้นที่ 1 ข้างล่าง)

`db/bootstrap/001_move_to_pet_schema.sql` (รันครั้งเดียว **ก่อน** Flyway):
```sql
BEGIN;
SET lock_timeout = '5s';

CREATE SCHEMA IF NOT EXISTS pet;

ALTER TABLE public.pets                  SET SCHEMA pet;
ALTER TABLE public.pet_permissions       SET SCHEMA pet;
ALTER TABLE public.pet_caregivers        SET SCHEMA pet;
ALTER TABLE public.caregiver_permissions SET SCHEMA pet;
ALTER TABLE public.litter_logs           SET SCHEMA pet;
ALTER TABLE public.water_logs            SET SCHEMA pet;

-- sequence / type ที่ผูกกับตารางย้ายตามอัตโนมัติ แต่ตรวจให้แน่:
-- SELECT sequencename, schemaname FROM pg_sequences WHERE schemaname='public';

GRANT USAGE ON SCHEMA pet TO pet_app, pet_migrator;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA pet TO pet_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA pet GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO pet_app;
GRANT ALL ON SCHEMA pet TO pet_migrator;

COMMIT;
```

**ลำดับการรันแบบไม่มี downtime:**
```
ขั้นที่ 1  deploy app เวอร์ชันที่ DSN มี  search_path=pet,public
           → ตอนนี้ pet schema ยังว่าง app ยังเจอตารางที่ public ตามปกติ ✅ ไม่พัง
ขั้นที่ 2  §10.2 — backup + บันทึกจำนวนแถวก่อนย้าย
ขั้นที่ 3  รัน 001_move_to_pet_schema.sql
           → app หาตารางเจอทันทีผ่าน search_path ✅ ไม่ต้อง restart
ขั้นที่ 4  §10.3 — พิสูจน์จำนวนแถวหลังย้าย = ก่อนย้าย   ← GATE ห้ามผ่านถ้าไม่ตรง
ขั้นที่ 5  flyway baseline (2.2)
ขั้นที่ 6  ภายหลัง เมื่อ auth/event ย้ายเสร็จแล้ว → เปลี่ยน search_path เหลือ pet อย่างเดียว
```
⚠️ **ห้ามลืม `search_path` ในขั้นที่ 1** — ถ้าย้าย schema ก่อนที่ app จะรู้จัก `pet` จะได้ `relation "pets" does not exist` ทั้งระบบทันที

**DB user แยกสิทธิ์** (สร้างพร้อมกัน):
```sql
CREATE ROLE pet_migrator LOGIN PASSWORD :'migrator_pw';  -- DDL ได้ ใช้เฉพาะ Flyway Job
CREATE ROLE pet_app      LOGIN PASSWORD :'app_pw';       -- DML เท่านั้น ใช้ใน runtime
```
> `pet_app` ไม่มีสิทธิ์ DDL → **AutoMigrate กลับมาเองไม่ได้อีกตลอดกาล** และ SQL injection ก็ `DROP TABLE` ไม่ได้

**2.2 · Baseline schema ปัจจุบัน**
```bash
pg_dump --schema-only --no-owner --no-privileges --no-comments \
  --schema=pet "$PROD_DSN" > db/migration/V1__baseline_pet_schema.sql
```
- ทำความสะอาด: ลบ `SET`/`SELECT pg_catalog.set_config`/`OWNER TO`
- เติมบรรทัดบนสุด: `CREATE SCHEMA IF NOT EXISTS pet;` และ `CREATE EXTENSION IF NOT EXISTS pgcrypto;`
- 🚫 **ห้ามแก้ไฟล์นี้อีกเลยหลัง apply แล้ว** — checksum จะเพี้ยน

**2.3 · โครงไดเรกทอรี**
```
db/
├── bootstrap/                      # one-off runbook script — ไม่อยู่ใน FLYWAY_LOCATIONS
│   └── 001_move_to_pet_schema.sql
├── flyway.conf
├── Dockerfile
├── migration/                      # V__ : DDL + seed master data ครั้งแรก, IMMUTABLE
│   ├── V1__baseline_pet_schema.sql
│   ├── V2__add_masterdata_tables.sql
│   ├── V3__seed_masterdata_initial.sql       # ← seed ครั้งเดียว (ดู Phase 3.1)
│   ├── V4__add_fk_and_soft_delete_to_logs.sql
│   ├── V5__add_constraints_and_indexes.sql
│   ├── V6__add_role_capabilities.sql
│   └── V7__add_manage_water_permission.sql
├── codeowned/                      # R__ : เฉพาะตารางที่ UI แก้ไม่ได้
│   ├── R__0005_role_capabilities.sql
│   └── R__0010_pet_permissions.sql
├── rollback/                       # เอกสารประกอบ ไม่ถูกรันอัตโนมัติ
└── seed/                           # local/dev เท่านั้น
    └── R__9000_dev_sample_pets.sql
```
> ⚠️ **สังเกตว่าไม่มีโฟลเดอร์ `masterdata/` แบบ `R__` แล้ว** — เพราะการตัดสินใจข้อ 4 (แก้ผ่าน backoffice ได้) ทำให้ `R__` ใช้กับ master data ไม่ได้อีกต่อไป ดูเหตุผลเต็มใน Phase 3.1 และ §5.4

**Flyway config**
```
FLYWAY_SCHEMAS=pet
FLYWAY_DEFAULT_SCHEMA=pet
FLYWAY_TABLE=flyway_schema_history
FLYWAY_LOCATIONS=filesystem:/flyway/sql/migration,filesystem:/flyway/sql/codeowned
FLYWAY_BASELINE_ON_MIGRATE=true
FLYWAY_BASELINE_VERSION=1
FLYWAY_VALIDATE_ON_MIGRATE=true
FLYWAY_OUT_OF_ORDER=false
FLYWAY_CLEAN_DISABLED=true
```

**2.4 · Migration ใหม่ที่ต้องเขียน (แก้หนี้ที่ AutoMigrate ทำไม่ได้)**

`V4__add_fk_and_soft_delete_to_logs.sql` (แก้ C-9, C-12):
```sql
-- ⚠️ ตรวจ orphan ก่อน แล้ว "ย้ายไปตารางกักกัน" ไม่ใช่ DELETE ทิ้ง (ข้อ 8: ข้อมูลต้องครบ)
CREATE TABLE IF NOT EXISTS pet.orphaned_logs_quarantine (
    source_table text, row_data jsonb, quarantined_at timestamptz DEFAULT now()
);
INSERT INTO pet.orphaned_logs_quarantine (source_table, row_data)
SELECT 'litter_logs', to_jsonb(l) FROM pet.litter_logs l
WHERE NOT EXISTS (SELECT 1 FROM pet.pets p WHERE p.id = l.pet_id);
INSERT INTO pet.orphaned_logs_quarantine (source_table, row_data)
SELECT 'water_logs', to_jsonb(w) FROM pet.water_logs w
WHERE NOT EXISTS (SELECT 1 FROM pet.pets p WHERE p.id = w.pet_id);

DELETE FROM pet.litter_logs l WHERE NOT EXISTS (SELECT 1 FROM pet.pets p WHERE p.id = l.pet_id);
DELETE FROM pet.water_logs  w WHERE NOT EXISTS (SELECT 1 FROM pet.pets p WHERE p.id = w.pet_id);

ALTER TABLE pet.litter_logs ADD COLUMN deleted_at timestamptz;
ALTER TABLE pet.water_logs  ADD COLUMN deleted_at timestamptz;
CREATE INDEX idx_litter_logs_deleted_at ON pet.litter_logs(deleted_at);
CREATE INDEX idx_water_logs_deleted_at  ON pet.water_logs(deleted_at);

ALTER TABLE pet.litter_logs ADD CONSTRAINT fk_litter_logs_pet
    FOREIGN KEY (pet_id) REFERENCES pet.pets(id) ON DELETE CASCADE;
ALTER TABLE pet.water_logs ADD CONSTRAINT fk_water_logs_pet
    FOREIGN KEY (pet_id) REFERENCES pet.pets(id) ON DELETE CASCADE;
```
> ตาราง quarantine ทำให้ "ไม่มีข้อมูลหายแบบเงียบๆ" — ถ้าหลัง migrate มันว่างก็ drop ทิ้งได้ ถ้าไม่ว่างแปลว่าเจอปัญหาที่ต้องคุยกัน

`V5__add_constraints_and_indexes.sql`:
```sql
-- 🚫 ห้ามใส่ CHECK (type IN (...)) — litter type แก้ผ่าน UI ได้ (การตัดสินใจล่าสุด)
--    ถ้าใส่ CHECK ไว้ admin เพิ่มชนิดใหม่ผ่าน backoffice แล้วจะบันทึก log ไม่ได้
--    ใช้ FK ไป mst_litter_types แทน → ดู Phase 3.7
-- 🚫 ห้าม UPDATE ... upper(type) ตรงนี้เช่นกัน — จะเปลี่ยน response ที่ client เห็น (ผิดข้อ 7)
--    การ normalize ค่า ให้ทำใน Phase 3.7 พร้อมการ map เข้ากับ master data

ALTER TABLE pet.litter_logs ADD CONSTRAINT chk_litter_amount CHECK (amount > 0);
ALTER TABLE pet.water_logs  ADD CONSTRAINT chk_water_amount  CHECK (amount > 0);

-- partial unique index → เชิญ caregiver ที่เคยถูกลบซ้ำได้ (แก้ C-4/C-5 ที่ต้นเหตุ)
DROP INDEX IF EXISTS pet.idx_pet_user;
CREATE UNIQUE INDEX idx_pet_user_active ON pet.pet_caregivers(pet_id, user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_pet_caregivers_user_active ON pet.pet_caregivers(user_id) WHERE deleted_at IS NULL;
```
> ⚠️ `chk_litter_amount` / `chk_water_amount` อาจติดตั้งไม่ได้ถ้าข้อมูลเดิมมี `amount = 0` หรือติดลบ (โค้ดปัจจุบันไม่ validate เลย) — **ต้องตรวจก่อน**:
> ```sql
> SELECT 'litter' src, count(*) FROM pet.litter_logs WHERE amount <= 0
> UNION ALL SELECT 'water', count(*) FROM pet.water_logs WHERE amount <= 0;
> ```
> ถ้าเจอ ต้องตัดสินใจร่วมกันว่าจะแก้เป็นค่าอะไร หรือจะใช้ `NOT VALID` — **ห้ามให้ agent เดาเอง**

`V7__add_manage_water_permission.sql` — ดู Phase 3.4

**2.5 · ถอด AutoMigrate ออก**
- ลบ `migrateAndSeed()` ทั้งฟังก์ชันจาก `cmd/server/main.go` และลบ `PermissionRepository.Seed` + method ใน port
- เพิ่ม startup guard แทน:
```go
const RequiredSchemaVersion = 7

func AssertSchemaVersion(ctx context.Context, db *gorm.DB) error {
    var v string
    err := db.WithContext(ctx).Raw(
        `SELECT version FROM pet.flyway_schema_history
         WHERE success AND version IS NOT NULL
         ORDER BY installed_rank DESC LIMIT 1`).Scan(&v).Error
    if err != nil { return fmt.Errorf("cannot read schema history: %w", err) }
    if versionLess(v, RequiredSchemaVersion) {
        return fmt.Errorf("schema version %s < required %d — run migrations first", v, RequiredSchemaVersion)
    }
    return nil
}
```
- เพิ่ม `TestSchemaMatchesModels` — boot DB ที่ migrate แล้ว ใช้ `db.Migrator().HasTable/HasColumn` ยืนยันว่าทุก field ใน GORM model มี column จริง
  **นี่คือ safety net ทดแทน AutoMigrate** เพราะจากนี้ไป model กับ SQL จะไม่ sync กันเองอีกแล้ว

**2.6 · Local dev**
`docker-compose.dev.yml` (แทนของเดิมที่พัง — ดู C-14):
```yaml
services:
  postgres:
    image: postgres:15-alpine
    environment: { POSTGRES_USER: vertex, POSTGRES_PASSWORD: vertex, POSTGRES_DB: vertex }
    ports: ["5432:5432"]
    healthcheck: { test: ["CMD-SHELL","pg_isready -U vertex"], interval: 2s, retries: 15 }

  flyway:
    image: flyway/flyway:11-alpine
    command: -connectRetries=20 migrate
    volumes:
      - ./db/migration:/flyway/sql/migration
      - ./db/codeowned:/flyway/sql/codeowned
      - ./db/seed:/flyway/sql/seed
    environment:
      FLYWAY_URL: jdbc:postgresql://postgres:5432/vertex
      FLYWAY_USER: vertex
      FLYWAY_PASSWORD: vertex
      FLYWAY_SCHEMAS: pet
      FLYWAY_DEFAULT_SCHEMA: pet
      FLYWAY_LOCATIONS: filesystem:/flyway/sql/migration,filesystem:/flyway/sql/codeowned,filesystem:/flyway/sql/seed
    depends_on: { postgres: { condition: service_healthy } }

  pet-service:
    build: { context: ., dockerfile: Dockerfile.dev }
    ports: ["4001:4001"]
    environment:
      PORT: 4001
      DB_HOST: postgres
      DB_SEARCH_PATH: pet
    depends_on: { flyway: { condition: service_completed_successfully } }
```
- แก้ `Dockerfile.dev` ให้ชี้ `./cmd/server` (ตอนนี้ชี้ `main.go` ที่ root ซึ่งไม่มีอยู่)
- **แก้ปัญหา port 3 ค่าไม่ตรงกัน** — เลือก `4001` ใช้ให้เหมือนกันทั้ง compose / helm / default ในโค้ด

**2.7 · Deploy pipeline**
- `db/Dockerfile` → push image `pet-service-migrations` tag เดียวกับ app (`sha-<commit>`)
- Helm pre-upgrade hook Job (ดู §5.5)
- CI job `migration-check`: spin postgres → `flyway migrate` → `flyway validate` → `flyway info` → `TestSchemaMatchesModels`
- CI job `migration-on-prod-copy` (แนะนำมาก): restore dump ของ prod → รัน migration → เทียบจำนวนแถว → **นี่คือ gate ที่จับปัญหาแบบ constraint ติดตั้งไม่ได้เพราะข้อมูลเดิมสกปรก ได้ก่อนถึง prod**

**Acceptance Phase 2**
- [ ] `make db-up` บนเครื่องเปล่า → ได้ schema `pet` ครบโดยไม่ต้องรัน app
- [ ] `flyway info` บน prod: V1 = `Baseline`, V2–V7 = `Success`
- [ ] `grep -rn AutoMigrate internal/ cmd/` ไม่เจอผลลัพธ์
- [ ] จำนวนแถวทุกตาราง**ก่อนและหลัง**ย้าย schema ตรงกัน 100% (§10.3)
- [ ] `pet.orphaned_logs_quarantine` ว่าง หรือได้รับการยืนยันจากทีมแล้ว
- [ ] `pet_app` รัน `CREATE TABLE` ไม่ได้ (พิสูจน์ด้วยการลองจริง)
- [ ] app ที่ start ก่อน migration เสร็จ → fail fast พร้อม error ชัดเจน
- [ ] **app ทำงานได้เหมือนเดิมทุกอย่าง** — characterization test จาก Phase 0 ผ่านทั้งหมด

---

### Phase 3 — Master data + Backoffice CRUD (2–3 วัน)

> **เฟสนี้เขียนใหม่ทั้งหมดจากการตัดสินใจข้อ 4** — เดิมวางไว้ใช้ `R__` แต่ในเมื่อ backoffice ต้องแก้ได้ `R__` จะเขียนทับสิ่งที่ user แก้ทุกครั้งที่ deploy จึงใช้ไม่ได้

**3.1 · แยกความเป็นเจ้าของข้อมูลให้ชัด — นี่คือหัวใจของเฟสนี้**

| ชั้น | Source of truth | วิธี seed | แก้ผ่าน backoffice | ตาราง |
|---|---|---|---|---|
| **A · Code-owned** | git | `R__` (idempotent upsert) | 🚫 ไม่ได้ | `role_capabilities`, `pet_permissions` |
| **B · DB-owned** | database | `V__` seed **ครั้งเดียว** | ✅ ได้ | `mst_species`, `mst_cat_breeds`, `mst_blood_types` |
| **B · DB-owned** (ต่อ) | database | `V__` seed **ครั้งเดียว** | ✅ ได้ | `mst_litter_types`, `mst_genders` ⬅️ ย้ายมาจากชั้น C ตามการตัดสินใจ |

**เหตุผลของการแยก:**
- **ชั้น A** ผูกกับโค้ดที่บังคับใช้จริง — เพิ่ม `pet_permissions` ใหม่โดยไม่มีโค้ดรองรับ = permission ที่ไม่ทำอะไรเลย จึงต้องมาคู่กับ code change เสมอ → `R__` เหมาะที่สุด
- **ชั้น B** เป็นข้อมูลธุรกิจล้วน เพิ่มสายพันธุ์แมวใหม่ไม่ต้องแก้โค้ด → ให้ DB เป็นเจ้าของ + UI แก้ได้
- **เดิมมีชั้น C (code-constrained)** สำหรับ `mst_litter_types` / `mst_genders` เพราะจะใช้ `CHECK (type IN ('POOP','PEE'))`
  ✅ **การตัดสินใจ: ให้แก้ผ่าน UI ได้ → ชั้น C ถูกยุบเข้าชั้น B** ผลที่ตามมา 3 อย่าง (ต้องทำครบทั้งสาม):

  | # | เปลี่ยนอะไร | ที่ไหน |
  |---|---|---|
  | 1 | 🚫 **ถอด `CHECK (type IN (...))` ออก** — ถ้าคงไว้ admin เพิ่มชนิดใหม่ผ่าน UI แล้ว **บันทึก log ด้วยชนิดนั้นไม่ได้** (constraint ปฏิเสธ) = ฟีเจอร์พังทันที | Phase 2.4 `V5__` |
  | 2 | ✅ ใช้ **FK ไป `mst_litter_types(code)` แทน** — ได้ referential integrity โดยที่ค่าใหม่จาก UI ใช้ได้ทันทีไม่ต้อง deploy | Phase 3.7 |
  | 3 | ✅ **validation ในโค้ดห้าม hardcode enum** ต้องเช็คกับ master data ที่ cache ไว้ | Phase 1.3 |

  > 💡 ข้อ 2 คือกำไรที่ได้: FK ให้ความปลอดภัยพอๆ กับ CHECK แต่ **ขยายค่าใหม่ได้โดยไม่ต้อง migration** — ตรงกับสิ่งที่คุณต้องการพอดี

**3.2 · Schema master data**
```sql
-- V2__add_masterdata_tables.sql
CREATE TABLE pet.mst_species (
    code       varchar(50)  PRIMARY KEY,
    name_en    varchar(200) NOT NULL,
    name_th    varchar(200),
    sort_order int          NOT NULL DEFAULT 0,
    is_active  boolean      NOT NULL DEFAULT true,
    created_at timestamptz  NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz  NOT NULL DEFAULT now(),
    updated_by uuid,
    version    int          NOT NULL DEFAULT 1   -- optimistic locking สำหรับ UI
);
-- mst_cat_breeds, mst_blood_types โครงเดียวกัน (breeds มี species_code REFERENCES mst_species)
-- mst_litter_types, mst_genders, mst_blood_types โครงเดียวกัน — เปิด CRUD ทั้งหมด
```
- `created_by` / `updated_by` **บังคับต้องมี** — เพราะเปิดให้แก้ผ่าน UI แล้วต้องรู้ว่าใครแก้
- `version` สำหรับ optimistic locking กัน 2 admin แก้ชนกัน

**3.3 · Admin CRUD API** (ต้องการ capability `masterdata:write`)
```
GET    /api/v1/admin/master-data/{type}           # รวม inactive ด้วย
POST   /api/v1/admin/master-data/{type}
PUT    /api/v1/admin/master-data/{type}/{code}    # ต้องส่ง version มาด้วย
DELETE /api/v1/admin/master-data/{type}/{code}    # → soft deactivate เท่านั้น
```
`{type}` ∈ `species | cat-breeds | blood-types | litter-types | genders` (ชั้น B ทั้งหมด)
🚫 ไม่เปิด CRUD ให้ชั้น A (`pet_permissions`, `role_capabilities`) — สองตัวนี้ผูกกับโค้ดที่บังคับใช้ เพิ่มผ่าน UI แล้วจะไม่มีผลอะไร

**กติกาบังคับ:**
1. 🚫 **`DELETE` ต้องเป็น soft deactivate เสมอ (`is_active=false`)** ห้าม hard delete — ถ้ามี pet อ้าง `breed` นั้นอยู่ ข้อมูลจะพัง
2. ก่อน deactivate ให้เตือนจำนวนที่ใช้อยู่: `SELECT count(*) FROM pet.pets WHERE breed = $1`
3. `code` แก้ไม่ได้หลังสร้าง (เป็น PK ที่ข้อมูลอื่นอ้างอยู่) — แก้ได้แค่ `name_*`, `sort_order`, `is_active`
4. ทุกการเปลี่ยนแปลง publish event ไป event-service (`EventType: "MasterData"`)
5. `version` ไม่ตรง → 409 Conflict

**3.4 · เพิ่ม `MANAGE_WATER` permission** (การตัดสินใจข้อ 5)
`V7__add_manage_water_permission.sql`:
```sql
INSERT INTO pet.pet_permissions (id, name, description, is_active) VALUES
    ('MANAGE_WATER', 'Record Water Intake', 'Can record water intake logs', true)
ON CONFLICT (id) DO NOTHING;

-- ❗ backfill: caregiver ที่เคยมี MANAGE_TASKS ต้องได้ MANAGE_WATER ด้วย
--    ไม่งั้นคนที่เคยบันทึกน้ำได้ จะบันทึกไม่ได้ทันทีหลัง deploy = พฤติกรรมเปลี่ยน (ผิดข้อ 7)
INSERT INTO pet.caregiver_permissions (pet_caregiver_id, permission_model_id)
SELECT cp.pet_caregiver_id, 'MANAGE_WATER'
FROM pet.caregiver_permissions cp
WHERE cp.permission_model_id = 'MANAGE_TASKS'
ON CONFLICT DO NOTHING;
```
> ⚠️ ตรวจชื่อ column ของตาราง join จริงก่อน — GORM ตั้งชื่อเองจาก `many2many:caregiver_permissions` ให้ยืนยันด้วย `\d pet.caregiver_permissions`
> 🔸 ตัดสินใจร่วมกันด้วยว่า `MANAGE_MEDICAL` / `MANAGE_WEIGHT` ที่ยังไม่มี API รองรับ จะเก็บไว้หรือ deactivate

**3.5 · Service layer**
1. ลบ `MasterDataService` ที่ hardcode ออกจาก `internal/application/litter_service.go` (แก้ A-2) → ไฟล์ใหม่ `masterdata_service.go`
2. repository อ่านจาก DB, filter `is_active = true` สำหรับ public API
3. **Cache แบบ invalidate ได้** — ไม่ใช่ TTL อย่างเดียว เพราะ admin แก้แล้วต้องเห็นผลทันที
   - cache ใน memory + TTL 5 นาที เป็น safety net
   - admin CRUD สำเร็จ → invalidate ทันที
   - ⚠️ **หลาย replica:** invalidate ที่ replica เดียวไม่พอ → ใช้ Postgres `LISTEN/NOTIFY` หรือลด TTL เหลือ 30 วินาที (ง่ายกว่า และ master data ไม่ได้ต้อง real-time ขนาดนั้น) — **แนะนำ TTL 30 วินาที**
4. เพิ่ม `GET /api/v1/master-data/pet-permissions` (แก้ C-13 — หน้าตั้ง permission ที่ backoffice ต้องใช้)

**3.6 · Backward compatibility (ข้อ 7 — ห้ามทำให้ app พัง)**
- `GET /api/v1/master-data/cat-breeds` → **คง response shape เดิมเป๊ะ** `["Scottish Fold (หูพับ)", ...]` โดย service ประกอบสตริงจาก `name_en (name_th)` ให้เหมือนเดิม
- `GET /api/v2/master-data/cat-breeds` → shape ใหม่
  ```json
  [{"code":"SCOTTISH_FOLD","nameEn":"Scottish Fold","nameTh":"สก็อตติชโฟลด์ (หูพับ)","sortOrder":10}]
  ```
- v1 ประกาศ deprecated ใน OpenAPI + ใส่ header `Deprecation: true` และ `Sunset: <date>`
- ลบ v1 ได้ก็ต่อเมื่อยืนยันแล้วว่าไม่มี client เรียก (ดูจาก access log)
- ⚠️ **ต้องมี test ที่ยืนยันว่า v1 คืนค่าเหมือนก่อน refactor แบบตัวต่อตัว** — เอา output ของโค้ดปัจจุบันมาเป็น golden file

**3.7 · เชื่อม transactional data เข้ากับ master data** ⬅️ ส่วนที่มาแทน CHECK constraint

เมื่อ master data แก้ผ่าน UI ได้ การผูกความถูกต้องต้องยืดหยุ่นตาม ใช้ **3 ระดับ** ไล่จากเบาไปหนัก:

| ระดับ | กลไก | ค่าใหม่จาก UI ใช้ได้ทันที? | ต้องมีข้อมูลสะอาดก่อน? |
|---|---|---|---|
| **L1 · Soft validation** | app เช็คกับ master data ที่ cache ไว้ | ✅ | ❌ ไม่ต้อง |
| **L2 · FK `NOT VALID`** | DB บังคับกับ**แถวใหม่**เท่านั้น แถวเก่าปล่อยผ่าน | ✅ | ❌ ไม่ต้อง |
| **L3 · FK validated** | DB บังคับทุกแถว | ✅ | ✅ ต้องสะอาดครบ |

> 💡 **`NOT VALID` คือพระเอกของงานนี้** — PostgreSQL รองรับ FK ที่ "บังคับกับ INSERT/UPDATE ใหม่ แต่ไม่ตรวจแถวเดิม"
> ทำให้เพิ่ม referential integrity ได้ทันทีโดย **ไม่ต้องแตะข้อมูลเดิมเลย** (ตรงกับข้อ 7 และ 8) แล้วค่อย `VALIDATE CONSTRAINT` ทีหลังเมื่อข้อมูลสะอาด

**ขั้นที่ 1 — audit ข้อมูลจริงก่อน (บังคับ ห้ามข้าม)**
```sql
SELECT 'litter.type'  AS col, type      AS val, count(*) FROM pet.litter_logs GROUP BY 2
UNION ALL SELECT 'pets.gender',     gender,     count(*) FROM pet.pets GROUP BY 2
UNION ALL SELECT 'pets.species',    species,    count(*) FROM pet.pets GROUP BY 2
UNION ALL SELECT 'pets.blood_type', blood_type, count(*) FROM pet.pets GROUP BY 2
UNION ALL SELECT 'pets.breed',      breed,      count(*) FROM pet.pets GROUP BY 2
ORDER BY 1, 3 DESC;
```
> โค้ดปัจจุบัน **ไม่ validate อะไรเลย** ทุก column พวกนี้จึงเป็น free text — คาดหวังว่าจะเจอค่าแปลกๆ
> `internal/domain/litter_log.go:13` เขียนคอมเมนต์ไว้ว่า `// "Poop" or "Pee"` → ค่าจริงน่าจะเป็น **ตัวพิมพ์ผสม ไม่ใช่ `POOP`/`PEE`**

**ขั้นที่ 2 — seed master data ให้ครอบคลุมค่าที่มีอยู่จริง**

🔑 **กฎสำคัญ: `code` ที่ seed ต้องตรงกับค่าที่เก็บอยู่ใน prod เป๊ะๆ ไม่ใช่ค่าที่เราคิดว่าสวย**
เพราะ `litter.type` ถูกส่งกลับให้ client ทาง API ตรงๆ — เปลี่ยนค่าในตาราง = เปลี่ยน response = **app พัง** (ผิดข้อ 7)

```sql
-- V3__seed_masterdata_initial.sql (ตัวอย่าง — ปรับ code ตามผล audit ขั้นที่ 1)
INSERT INTO pet.mst_litter_types (code, name_en, name_th, sort_order, is_active) VALUES
    ('Poop', 'Poop', 'อุจจาระ', 10, true),   -- ⬅️ ใช้ค่าตามที่มีอยู่จริง ไม่ใช่ 'POOP'
    ('Pee',  'Pee',  'ปัสสาวะ', 20, true)
ON CONFLICT (code) DO NOTHING;
```
> ถ้า audit เจอค่าที่ไม่อยากเก็บไว้ (พิมพ์ผิด, ค่าว่าง) → **อย่าลบทิ้ง** ให้ seed เข้าไปด้วยแล้วตั้ง `is_active = false`
> ผลคือ: log เก่ายังแสดงผลได้ปกติ แต่ dropdown ในแอปไม่แสดงตัวเลือกนั้นอีก ✅ ข้อมูลครบ ✅ app ไม่พัง

**ขั้นที่ 3 — ใส่ FK แบบ `NOT VALID`**
```sql
-- V8__link_transactional_to_masterdata.sql
ALTER TABLE pet.litter_logs
    ADD CONSTRAINT fk_litter_logs_type FOREIGN KEY (type)
    REFERENCES pet.mst_litter_types(code) NOT VALID;

ALTER TABLE pet.pets
    ADD CONSTRAINT fk_pets_gender FOREIGN KEY (gender)
    REFERENCES pet.mst_genders(code) NOT VALID;

ALTER TABLE pet.pets
    ADD CONSTRAINT fk_pets_species FOREIGN KEY (species)
    REFERENCES pet.mst_species(code) NOT VALID;
```
⚠️ **ข้อจำกัดที่ต้องรู้:** FK ไม่ยอมให้ค่าเป็น `''` (สตริงว่าง) ที่ไม่มีใน master — ถ้า audit เจอสตริงว่าง ต้อง seed `''` เข้าไปด้วย หรือเปลี่ยนเป็น `NULL` ก่อน (แต่การเปลี่ยนเป็น NULL = เปลี่ยน response → ต้องยืนยันกับทีมก่อน)

**ขั้นที่ 4 — `VALIDATE` ทีหลัง (แยก PR)**
```sql
ALTER TABLE pet.litter_logs VALIDATE CONSTRAINT fk_litter_logs_type;  -- ล็อกเบา ไม่บล็อก read/write
```

**สรุปแผนต่อ column**

| Column | ระดับที่ทำใน Phase 3 | หมายเหตุ |
|---|---|---|
| `litter_logs.type` | **L2** (FK NOT VALID) | ค่าน้อย seed ครอบคลุมง่าย |
| `pets.gender` | **L2** | ตรวจ audit ก่อน อาจมีค่าว่าง |
| `pets.species` | **L2** | น่าจะมีแค่ `Cat` |
| `pets.blood_type` | **L1** เท่านั้น | เป็น nullable + ค่าอิสระ ให้ soft validate ก่อน |
| `pets.breed` | **L1** เท่านั้น | ⬇️ ดูด้านล่าง |

**⚠️ `pets.breed` เป็นเคสพิเศษ — อย่าใส่ FK ในรอบนี้**
ตอนนี้เก็บ **display string** เช่น `"Scottish Fold (หูพับ)"` ส่วน `mst_cat_breeds.code` ที่ออกแบบไว้คือ `SCOTTISH_FOLD` → ใส่ FK ตรงๆ ไม่ได้
ต้องทำ **column migration 3 เฟส** ซึ่งควรแยกไป **Phase 5** (data model):
```
1. เพิ่ม pets.breed_code (nullable) + FK NOT VALID ไป mst_cat_breeds(code)
2. backfill โดย map จาก breed → code (ค่าที่ map ไม่ได้ ปล่อย NULL แล้วรายงานออกมา)
3. app dual-write ทั้ง breed และ breed_code
4. เมื่อ client ทุกตัวอ่าน breed_code แล้ว → drop breed  (release ถัดไป ไม่ใช่รอบนี้)
```

**การ deactivate master data ที่ถูกใช้อยู่**
- FK ป้องกัน **hard delete** ให้อัตโนมัติแล้ว (DB จะปฏิเสธเอง) — เป็นชั้นป้องกันที่ CHECK ให้ไม่ได้
- `is_active = false` ยังทำได้เสมอ และ **ไม่กระทบข้อมูลเดิม** เพราะแถวใน master ยังอยู่ FK ยังชี้ได้
- Admin API ต้องเตือนจำนวนที่ใช้อยู่ก่อน deactivate:
  ```sql
  SELECT count(*) FROM pet.litter_logs WHERE type = $1;
  ```

**Acceptance Phase 3**
- [ ] เพิ่มสายพันธุ์แมวใหม่ผ่าน backoffice ได้ และ **deploy รอบถัดไปไม่เขียนทับ** (ทดสอบจริง: แก้ผ่าน API → รัน `flyway migrate` ซ้ำ → ข้อมูลยังอยู่)
- [ ] `grep -rn "Scottish Fold" internal/` ไม่เจอผลลัพธ์
- [ ] `GET /api/v1/master-data/cat-breeds` คืนค่า**เหมือนเดิมทุกตัวอักษร** (golden test)
- [ ] caregiver ที่เคยมี `MANAGE_TASKS` ยังบันทึก water log ได้หลัง deploy
- [ ] deactivate สายพันธุ์ที่มี pet ใช้อยู่ → ได้คำเตือน และ pet เดิมยังแสดงผลปกติ
- [ ] user ธรรมดายิง `POST /admin/master-data/cat-breeds` → 403
- [ ] **เพิ่ม litter type ใหม่ผ่าน backoffice → บันทึก litter log ด้วยชนิดนั้นได้ทันทีโดยไม่ต้อง deploy** ⬅️ ข้อพิสูจน์ว่าถอด CHECK ถูกแล้ว
- [ ] `grep -rn "oneof=POOP\|oneof=MALE" internal/` ไม่เจอผลลัพธ์ (ไม่มี enum ตายตัวหลงเหลือ)
- [ ] `GET /pets/:id/litter-logs` คืนค่า `type` เหมือนเดิมทุกตัวอักษร (golden test)
- [ ] ลอง hard delete master data ที่มีคนใช้อยู่ → DB ปฏิเสธ (FK ทำงาน)

---

### Phase 4 — Layering & code cleanup (1–2 วัน)

1. **`pkg/apperror` ตัด dependency** (แก้ A-1)
   - ย้าย `FromDomain` ไปเป็น `internal/adapter/handler/errmap.go`
   - repository เป็นคนแปลง `gorm.ErrRecordNotFound` → `domain.ErrXxxNotFound` (บางที่ทำแล้ว บางที่ไม่ทำ ให้ทำให้ครบ)
   - `IsAppError` → เปลี่ยนเป็น `errors.As` (แก้ C-8)
2. **Sentinel errors ใช้จริง** (แก้ C-4)
   - repository จับ pg error code `23505` → `domain.ErrCaregiverDuplicate`
   - `LitterRepository.Delete` คืน `domain.ErrLitterLogNotFound` แทน `errors.New` ดิบ
   - `WaterRepository.Delete` เช็ค `RowsAffected` ให้เหมือนกัน (แก้ C-9)
3. **Actor context** (แก้ A-6, C-2)
   - `domain.Actor{UserID uuid.UUID, Username string, Roles []string}`
   - middleware ใส่ลง `context.Context` (ไม่ใช่ `c.Locals` อย่างเดียว) ผ่าน typed key
   - ทุก use case method รับ actor เป็น parameter หรืออ่านจาก ctx ผ่าน helper `domain.ActorFromContext(ctx)`
   - `EventLog.ActorID` เป็น user id เสมอ, `ActorUsername` เป็น username เสมอ — ไม่ใช่เอา username ยัดใส่ ActorID
   - เซ็ต `CreatedBy` / `UpdatedBy` จาก actor ที่ชั้น service เท่านั้น (ห้ามรับจาก body)
4. **เพิ่ม `PATCH` ที่ล้างค่า null ได้ — โดยไม่ทำให้ `PUT` เดิมพัง** (แก้ C-3, การตัดสินใจข้อ 6+7)
   - เพิ่ม `PATCH /pets/:id` ใหม่ ที่ทุกฟิลด์เป็น `Optional[T]` แยก 3 สถานะได้:
     ```go
     type Optional[T any] struct { Defined bool; Value *T }  // UnmarshalJSON set Defined=true
     // ไม่ส่ง field มา      → Defined=false        → ไม่แก้
     // ส่ง "field": null    → Defined=true, Value=nil → ล้างเป็น NULL  ⬅️ สิ่งที่ทำไม่ได้ตอนนี้
     // ส่ง "field": "abc"   → Defined=true, Value=&"abc" → เซ็ตค่า
     ```
   - **`PUT /pets/:id` เดิมยังอยู่และพฤติกรรมเหมือนเดิมเป๊ะ** (ข้อ 7) แค่ mark deprecated + `Sunset` header
   - implement `PUT` เป็น adapter บางๆ ที่แปลงเป็น PATCH request ภายใน เพื่อไม่ให้มี logic ซ้ำสองชุด
   - ลบ `PUT` ได้เมื่อยืนยันจาก access log ว่าไม่มี client เรียกแล้ว
   - ⚠️ `IsSpayedNeutered` ปัจจุบันเขียนทับไม่มีเงื่อนไข (ต่างจากฟิลด์อื่น) — ใน `PUT` ต้อง**คงพฤติกรรมนี้ไว้** ส่วน `PATCH` ให้เป็น optional เหมือนฟิลด์อื่น
5. **แยก `main.go`** (แก้ A-4) → `internal/config`, `internal/bootstrap`, `pkg/middleware/*`
   เป้าหมาย: `main.go` ≤ 40 บรรทัด
6. **Config struct** (แก้ A-5) — ใช้ `env` tag + validate + fail fast ตอน boot
   ```go
   type Config struct {
       Port            int    `env:"PORT" envDefault:"4001"`
       DBHost          string `env:"DB_HOST,required"`
       DBSSLMode       string `env:"DB_SSLMODE" envDefault:"require"`
       DBMaxOpenConns  int    `env:"DB_MAX_OPEN_CONNS" envDefault:"20"`
       JWTPublicKeyPEM string `env:"JWT_PUBLIC_KEY"`          // แทนการอ่านไฟล์ (แก้ S-6)
       EventServiceURL string `env:"EVENT_SERVICE_URL,required"`
       LogLevel        string `env:"LOG_LEVEL" envDefault:"info"`
   }
   ```
7. **`internal/port/water.go` → ยุบเข้า `input.go`/`output.go`** (แก้ A-3)
8. **แยก `repository/model.go` (170 บรรทัด)** เป็น `model/` package หนึ่งไฟล์ต่อ entity
9. ลบ logic `Restore` ใน `CaregiverService.Add` หลัง Phase 2 ใส่ partial unique index แล้ว (แก้ C-5)
10. `SaveBatch` guard slice ว่าง (แก้ C-7) + ห่อ `SaveBatch` + publish events ใน transaction เดียว

**Acceptance**
- [ ] `main.go` ≤ 40 บรรทัด
- [ ] `pkg/` ไม่ import `internal/` และไม่ import `gorm` เลย (ตรวจด้วย `go list -deps` ใน CI)
- [ ] มี test ที่ล้าง `microchipId` เป็น null ได้สำเร็จ
- [ ] เพิ่ม caregiver ซ้ำ → 409 ไม่ใช่ 500
- [ ] ลบ litter log ที่ไม่มีอยู่ → 404 ไม่ใช่ 204 (ทั้ง litter และ water)

---

### Phase 5 — Data model & performance (1–2 วัน)

1. **Avatar ออกจาก list response** (แก้ P-1) — **ทำก่อนข้ออื่นเพราะกระทบ prod ทันที**
   - `PetListItemResponse` ไม่มี `avatarData`
   - เพิ่ม `GET /api/v1/pets/:id/avatar` คืน binary พร้อม `ETag` + `Cache-Control`
   - repository เพิ่ม `FindAllForUserSummary` ที่ `Select` เฉพาะ column ที่ต้องใช้ (ไม่ `SELECT *`)
2. **Avatar ออกจาก Postgres** (แนะนำ แต่แยก PR ได้)
   - ย้ายไป MinIO/S3 เก็บแค่ `avatar_url` + `avatar_updated_at`
   - migration `V5__` เพิ่ม column, backfill ด้วย script, `V6__` drop column เก่าหลัง verify
   - ถ้ายังไม่พร้อม: อย่างน้อยแยกไปตาราง `pet_avatars` (pet_id PK, data bytea) เพื่อไม่ให้ `SELECT * FROM pets` ลากไปด้วย
3. **Pagination** (แก้ P-2) — cursor-based บน `(date DESC, id DESC)` สำหรับ log endpoints, offset ก็พอสำหรับ `/pets`
   response envelope: `{"data": [...], "nextCursor": "...", "hasMore": true}`
4. **`Preload` แบบเลือกได้** (แก้ P-3) — `?include=caregivers` แทนการ preload เสมอ
5. **Connection pool** (แก้ P-4) ตั้งจาก config: `MaxOpenConns=20, MaxIdleConns=10, ConnMaxLifetime=30m, ConnMaxIdleTime=5m`
   (3 services × 20 = 60 < `max_connections` 100 ของ postgres — คำนวณให้พอดี)
6. **`sslmode` จาก env** (แก้ P-5)
7. **`BirthDate` → `date`** (แก้ P-6) — migration `ALTER TABLE pets ALTER COLUMN birth_date TYPE date`
8. **ใช้ `is_active` จริง หรือลบทิ้ง** (แก้ C-10) — ตัดสินใจกับ PO ว่าต้องการ soft-hide log ไหม ถ้าไม่ ให้ drop column
9. **`Order` ที่ litter** (แก้ C-11) ให้เหมือน water
10. ลบ `default:gen_random_uuid()` หรือลบ `uuid.New()` ในโค้ด — เลือกทางเดียว (แก้ P-7) แนะนำให้ app generate เพื่อให้ได้ ID ก่อน insert สำหรับ outbox/event

**Acceptance**
- [ ] `GET /pets` ของ user ที่มี 50 ตัว → response < 100KB และ p95 < 200ms
- [ ] Load test 100 concurrent ไม่ทำให้ Postgres connection เต็ม
- [ ] EXPLAIN ยืนยันว่า authz query ใช้ index

---

### Phase 6 — Observability & lifecycle (1 วัน)

1. **Graceful shutdown** (แก้ O-2)
   ```go
   go func() { if err := app.Listen(addr); err != nil { log.Error(...) } }()
   sig := make(chan os.Signal, 1); signal.Notify(sig, os.Interrupt, syscall.SIGTERM); <-sig
   ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second); defer cancel()
   _ = app.ShutdownWithContext(ctx)
   eventPublisher.Close(ctx)   // รอ in-flight events (ดู Phase 7)
   sqlDB.Close()
   ```
2. **Health endpoints แยกกัน** (แก้ O-3)
   - `GET /livez` → 200 เสมอถ้า process ยังอยู่
   - `GET /readyz` → ping DB (timeout 2s) + ตรวจ schema version
   - `GET /health` เดิม → alias ของ `/livez` เพื่อ backward compat
3. **Helm probes** (แก้ O-1)
   ```yaml
   livenessProbe:  { httpGet: {path: /livez,  port: http}, initialDelaySeconds: 5,  periodSeconds: 15 }
   readinessProbe: { httpGet: {path: /readyz, port: http}, initialDelaySeconds: 3,  periodSeconds: 5, failureThreshold: 3 }
   startupProbe:   { httpGet: {path: /livez,  port: http}, failureThreshold: 30, periodSeconds: 2 }
   ```
   เพิ่ม `terminationGracePeriodSeconds: 30`, PodDisruptionBudget, และปรับ resource limit (128Mi ต่ำเกินไปถ้ายังมี avatar — ตั้ง `requests: 128Mi / limits: 512Mi`)
4. **Structured logging ทั้งระบบ** — `log/slog` JSON handler, ใส่ `request_id` ผ่าน context ทุก log line, ลบ `fmt.Println` / `log.Printf` ทั้งหมด
5. **OpenTelemetry (optional แต่แนะนำ)** — trace + metric ผ่าน otel-go, instrument fiber + gorm
   ถ้ายังไม่พร้อม: อย่างน้อย expose `/metrics` (Prometheus) นับ request count / duration / error rate ต่อ route
6. เพิ่ม HPA ใน helm (ถ้า metrics-server พร้อม)

**Acceptance** — ทำเสร็จและตรวจบนคลัสเตอร์จริงแล้ว 2026-08-23
- [x] `kubectl rollout restart` ระหว่างยิง load → 0 dropped request
      ยิงผ่าน ingress 4,662 request คร่อม rollout 2 รอบ ได้ 401/429 เท่านั้น **ไม่มี 5xx เลย**
      (429 คือ rate limiter ทำงาน ไม่ใช่ request ตก รอบที่คุมอัตราให้ต่ำกว่า limit ได้ 401 ครบ 90/90)
      log ของ pod เก่ายืนยันลำดับ: รับ SIGTERM → readyz ตอบ 503 → รอ drain 5s →
      "request ที่ค้างอยู่ทำงานจนจบแล้ว" → "ปิดตัวเรียบร้อย" รวม ~5s (grace 45s)
- [x] pod ที่ DB ล่ม → `readyz` fail แต่ `livez` ผ่าน (ไม่โดน kill ทิ้งวนลูป)
      คุมด้วย `TestLiveness_IgnoresDependencies` + `TestReadiness_DBUnavailable`
- [x] ทุก log line มี `request_id` และ parse เป็น JSON ได้
      ตรวจ 130 บรรทัดจาก pod จริง: parse JSON ได้ทุกบรรทัด, access log มี `request_id` ครบ
      (กล่อง ASCII ตอน start ของ Fiber ปิดด้วย `DisableStartupMessage`)

**สิ่งที่ทำเพิ่มนอกเหนือจากแผน (เจอระหว่างทำ)**
- ไม่ log `/livez` `/readyz` `/health` `/metrics` — probe ยิงทุก 2–5 วินาทีจนกลบ access log
  ของ request จริง (เจอตอนไล่ incident วันที่ 2026-08-23)
- probe/scrape ไม่ถูกนับใน rate limiter — ไม่งั้นตอน traffic สูง probe จะโดน 429 แล้ว k8s ฆ่า pod
- `openTestDB` ปฏิเสธฐานข้อมูลที่ไม่มี dev seed — กัน integration test ยิงใส่ production
  ตอนมี port-forward ค้างที่ localhost:5432
- CD เดิม `helm upgrade | tee` กลืน exit code (เป็นของ `tee` = 0 เสมอ) job จึงขึ้นเขียว
  ทั้งที่ deploy ไม่ขึ้น → ใส่ `set -euo pipefail` + step `Verify rollout` เทียบ image กับ sha
  (แก้ทั้ง pet-service และ auth-service)

**ยังไม่ทำ (ตั้งใจปิดไว้)**
- HPA และ PodDisruptionBudget เขียน template ไว้แล้วแต่ `enabled: false`
  คลัสเตอร์มี node เดียวและ replica เดียว PDB `minAvailable: 1` จะทำให้ `kubectl drain`
  ค้างตลอดไป กลายเป็นขัดขวาง maintenance แทนที่จะช่วย
  เปิดเมื่อ `replicaCount >= 2` และตรวจก่อนว่า rate limiter ซึ่งเก็บ state ในหน่วยความจำ
  ของแต่ละ pod ยังให้ผลที่ยอมรับได้เมื่อกระจายหลาย pod
- OpenTelemetry (trace) — ทำแค่ Prometheus metrics ตามทางเลือกที่แผนเปิดไว้

---

### Phase 7 — Event publishing reliability (1–2 วัน)

ปัจจุบัน (`internal/adapter/event/http_publisher.go`): `go func(){ http.Post(...) }()` — ไม่มี timeout, ไม่มี retry, ไม่มี backpressure, ctx ที่รับมาไม่ได้ใช้, pod restart = event หาย

**งาน**
1. **แก้เร่งด่วนก่อน (30 นาที):** ใส่ `http.Client{Timeout: 5*time.Second}` + `http.NewRequestWithContext` + `context.WithoutCancel` (ไม่งั้น ctx ของ request ถูก cancel ก่อน goroutine ยิงเสร็จ) + limit จำนวน goroutine ด้วย buffered channel worker pool
2. **แก้ให้ถูก (Transactional Outbox):**
   - `V__` เพิ่มตาราง `event_outbox(id, aggregate_type, aggregate_id, event_type, payload jsonb, created_at, published_at, attempts, last_error)`
   - service เขียน event ลง outbox **ใน transaction เดียวกับ business data**
   - background worker poll `published_at IS NULL` → POST ไป event-service → mark published (มี exponential backoff, max attempts, index `WHERE published_at IS NULL`)
   - ใช้ `FOR UPDATE SKIP LOCKED` เพื่อให้หลาย replica ทำงานพร้อมกันได้
   - เพิ่ม metric `outbox_pending_count` + alert
3. `Publish` เปลี่ยน signature ให้คืน `error` (ตอนนี้กลืน error หมด)
4. Idempotency key ที่ event-service เพื่อกัน duplicate จาก retry

**Acceptance**
- [ ] kill event-service แล้วสร้าง pet → business data commit สำเร็จ, event ค้างใน outbox
- [ ] เปิด event-service กลับมา → event ถูกส่งภายใน 30 วินาทีโดยไม่ต้อง restart pet-service
- [ ] `SIGTERM` ระหว่างมี event ค้าง → ไม่มี event หาย

---

### Phase 8 — Testing & CI gate (ต่อเนื่อง)

**เป้าหมาย coverage**
| Layer | เป้า | วิธี |
|---|---|---|
| `internal/domain` | 90% | unit test ล้วน ไม่มี dependency |
| `internal/application` | 85% | fake ที่ implement `port.*` (เขียนมือ ไม่ต้อง mockgen ก็ได้ เพราะ interface เล็ก) |
| `internal/adapter/repository` | 70% | integration test กับ postgres จริงผ่าน `testcontainers-go` (รัน Flyway migration ก่อนทุกครั้ง) |
| `internal/adapter/handler` | 75% | `app.Test(req)` ของ Fiber + fake use case |
| E2E | happy path + authz | testcontainers: postgres + flyway + app |

**สิ่งที่ต้องมีแน่ๆ**
- Authorization matrix test — ตารางเต็มของ (role × endpoint × ownership) ทุกช่อง
- `TestSchemaMatchesModels` (จาก Phase 2)
- Migration test: รัน migration บน DB เปล่า และบน DB ที่ baseline จาก dump prod — ต้องได้ schema เหมือนกัน
- Golden test ของ error response shape

**CI gates ที่ต้องผ่านก่อน merge**
`go vet` · `golangci-lint` · `go test -race ./...` · `govulncheck` · `flyway validate` · Trivy scan บน image · coverage ไม่ลดลง

---

### Phase 9 — Flyway + schema `auth` ที่ vertex-auth-service (2 วัน)

> ทำ**หลัง** Phase 2 เสร็จและอยู่ตัวแล้ว — ใช้ pet-service เป็นต้นแบบ (การตัดสินใจข้อ 2: ทีละอัน เริ่มที่ pet)

**สภาพปัจจุบัน:** `vertex-auth-service/main.go` ทั้ง service อยู่ใน **ไฟล์เดียว 400+ บรรทัด** และ `AutoMigrate(&User{}, &OAuthIdentity{})` ที่บรรทัด 121

**งาน**
1. `db/bootstrap/001_move_to_auth_schema.sql` — `CREATE SCHEMA auth;` + `SET SCHEMA` ตาราง `users`, `oauth_identities` และตาราง RBAC จาก Phase 1A
   > ⚠️ **`auth` เป็นชื่อที่ปลอดภัย** แต่ระวังอย่าใช้ `user` เป็นชื่อ schema (เป็น reserved word ใน PostgreSQL)
2. `search_path=auth,public` ที่ DSN **ก่อน** ย้าย (เหมือน Phase 2.1 ขั้นที่ 1)
3. baseline + `FLYWAY_SCHEMAS=auth` + `FLYWAY_TABLE=flyway_schema_history`
4. ถอด `AutoMigrate` + สร้าง user `auth_app` / `auth_migrator`
5. **แยกไฟล์** — `main.go` 400+ บรรทัดควรแยกเป็นชั้นเหมือน pet-service อย่างน้อย `handler / service / repository / config`
   (ทำแยก PR ได้ ไม่ต้องรวมกับ Flyway)

⚠️ **ประเด็นสำคัญ:** ตาราง RBAC (Phase 1A) จะถูกสร้างใน `public` ก่อน แล้วค่อยย้ายมา `auth` ที่เฟสนี้
ถ้าอยากประหยัดขั้นตอน ให้ Phase 1A **สร้างใน `auth` schema ตั้งแต่แรกเลย** แล้วเฟสนี้ย้ายแค่ `users` / `oauth_identities`
→ **แนะนำแบบหลัง** ถ้าตัดสินใจตอนนี้ได้

**Acceptance:** เหมือน Phase 2 · จำนวนแถว `users` / `oauth_identities` / `user_roles` ก่อน-หลังตรงกัน 100% · login ได้ปกติ

---

### Phase 10 — Flyway + schema `event` ที่ vertex-event-service (1 วัน)

> ง่ายที่สุดในสามตัว — มีตารางเดียว (`event_logs`) และเป็น append-only

**งาน**
1. `CREATE SCHEMA event;` + `ALTER TABLE public.event_logs SET SCHEMA event;`
2. baseline + `FLYWAY_SCHEMAS=event`
3. ถอด `AutoMigrate` ที่ `internal/adapter/repository/event_repo.go:15`
4. **เพิ่มสิ่งที่ควรมีตั้งแต่แรก:**
   - partition ตาม `created_at` รายเดือน (event log โตเร็วและไม่มีใครลบ)
   - retention policy — เก็บกี่เดือน
   - index บน `(entity_type, entity_id, created_at DESC)` สำหรับหน้า timeline
   - idempotency key เพื่อกัน duplicate จาก retry ของ outbox (Phase 7)

**หลังเฟสนี้** — ทั้ง 3 service แยก schema ครบ:
```
vertex
├── pet    (pets, pet_caregivers, litter_logs, water_logs, mst_*, flyway_schema_history)
├── auth   (users, oauth_identities, roles, user_roles, flyway_schema_history)
├── event  (event_logs, flyway_schema_history)
└── public (ควรเหลือแค่ extension — ถ้าว่างแล้วคือทำถูก)
```
- [x] ตรวจขั้นสุดท้าย: `SELECT tablename FROM pg_tables WHERE schemaname='public';` → **ว่างแล้ว**
      (ไม่มีทั้งตาราง extension และ function เหลือใน public เลย)
- [x] แต่ละ service `SELECT` ตารางของ service อื่นไม่ได้ — พิสูจน์ด้วยการลองจริง 2026-08-23

      | user | pet.pets | auth.users | event.event_logs |
      |---|---|---|---|
      | `pet_app`   | อ่านได้ | 🔒 denied | 🔒 denied |
      | `auth_app`  | 🔒 denied | อ่านได้ | 🔒 denied |
      | `event_app` | 🔒 denied | 🔒 denied | อ่านได้ |

- [ ] เอา `public` ออกจาก `search_path` ของทุก service
      ปลอดภัยที่จะทำแล้วเพราะ public ว่างสนิท แต่ต้อง redeploy ทั้งสาม service
      เก็บไว้ทำรวมกับรอบ deploy ถัดไป — grant เป็นตัวคุมจริงอยู่แล้ว (ตารางข้างบน)
      ส่วนนี้เป็น defense-in-depth

**สิ่งที่ทำเพิ่มนอกเหนือจากแผน — auth ของ event-service**

แผนเดิมของเฟสนี้พูดถึงแค่ schema กับ Flyway แต่ตอนลงมือพบว่า
`GET /api/v1/admin/events` **ไม่มีการตรวจสิทธิ์อะไรเลย** ใครก็ตามบนอินเทอร์เน็ต
ดึง event log ทั้งหมดได้ ซึ่งมี user id และ pet id ของผู้ใช้จริง
(ยืนยันด้วยการยิงจริงผ่าน ingress) ส่วน `POST /api/v1/events` ก็เปิดโล่งเช่นกัน
คือยิง event ปลอมเข้าระบบได้ ทำให้ audit log ที่มีไว้ตรวจสอบเชื่อถือไม่ได้

- `GET /api/v1/admin/events` → JWT (คีย์ชุดเดียวกับ auth-service) + ต้องเป็น SUPER_ADMIN
- `POST /api/v1/events` → service token เทียบแบบ constant-time
  ไม่ใช้ JWT ของผู้ใช้เพราะการส่ง event เป็น fire-and-forget และต่อไปจะเป็น
  outbox ที่ส่งทีหลัง token ของผู้ใช้อาจหมดอายุไปแล้วตอนส่งจริง
- `event_app` แทน `vertex_admin` — เดิม service ที่แค่ต้องเขียน log
  ต่อ database ด้วย superuser จึงอ่านและลบข้อมูลของ pet และ auth ได้หมด

⏳ **ยังไม่ได้ deploy** — ยังไม่มี repo `vertex-event-service` บน GitHub
   จึงยังไม่มี CI/CD ที่จะ build image ขึ้น ghcr.io ให้คลัสเตอร์ดึงไปใช้
   โค้ดพร้อมแล้วและ commit ไว้ในเครื่องแล้ว

---

## 5. Flyway Deep Dive

### 5.1 ทำไม Flyway (และทางเลือกอื่นที่ควรรู้)

| เครื่องมือ | ข้อดี | ข้อเสีย |
|---|---|---|
| **Flyway** (ที่คุณถาม) | มาตรฐานอุตสาหกรรม, `R__` repeatable เหมาะกับ master data มาก, `validate`/`repair`/`info` ครบ, ใช้ SQL ล้วนทำให้ DBA ช่วย review ได้, ใช้ tool เดียวกันได้ทุก service ไม่ว่าเขียนภาษาอะไร | image เป็น JVM (~180MB), ต้อง run แยกจาก Go binary, `undo` เป็นฟีเจอร์เสียเงิน (Teams) |
| golang-migrate | binary เล็ก, embed ใน Go ได้ | **ไม่มี repeatable migration** → ทำ master data ลำบาก, ต้องเขียน `V__` ใหม่ทุกครั้งที่แก้ข้อมูล |
| goose | เล็ก, embed ได้, เขียน migration เป็น Go ได้ (ดีสำหรับ data backfill ซับซ้อน) | ไม่มี repeatable แท้ๆ, ecosystem เล็กกว่า |
| Atlas | declarative + diff จาก GORM model ได้อัตโนมัติ, versioned ก็ได้ | learning curve, บาง feature เสียเงิน, declarative mode อันตรายถ้าไม่ระวัง |

**ข้อสรุป: ไปกับ Flyway ครับ** เหตุผลหลักคือ `R__` repeatable migration ตอบโจทย์ "script insert/update master data" ที่คุณถามได้ตรงที่สุด — แก้ไฟล์ SQL แล้ว migrate ซ้ำ Flyway จะ re-apply ให้เองเมื่อ checksum เปลี่ยน โดยไม่ต้องสร้างไฟล์ version ใหม่ทุกครั้ง
เรื่อง JVM image ไม่ใช่ปัญหาจริง เพราะมันรันเป็น **Job แยก ไม่ได้อยู่ใน runtime path ของ app**

> ถ้าอนาคตรำคาญ JVM จริงๆ **goose** เป็นทางถอยที่ย้ายง่ายที่สุด (naming convention คล้ายกัน) แต่ต้องยอมเสีย repeatable migration ไป

### 5.2 ⚠️ ประเด็นสำคัญ: ตอนนี้ 3 service แชร์ database เดียวกัน

ยืนยันจากโค้ด:
- `docker-compose.yml` — auth-service ใช้ `dbname=vertex` เดียวกับ pet-service
- `k8s/01-postgres.yaml` — `POSTGRES_DB: vertex` ตัวเดียว
- `vertex-auth-service/main.go:121` — `dbConn.AutoMigrate(&User{}, &OAuthIdentity{})`
- `vertex-event-service/.../event_repo.go:15` — `db.AutoMigrate(&domain.EventLog{})`

ทั้ง 3 service AutoMigrate ลง schema `public` เดียวกัน ถ้าใส่ Flyway โดยไม่คิดเรื่องนี้ จะเจอ:
- Flyway ของ pet-service เห็นตารางของ auth (`users`, `oauth_identities`) แล้วคิดว่าเป็นของตัวเอง
- `flyway_schema_history` ตัวเดียวถูกแย่งกันเขียนถ้า service อื่นทำตามในอนาคต
- `flyway clean` (ถ้าใครเผลอรัน) ลบตารางของ service อื่นเรียบ

> ✅ **ตัดสินใจแล้ว (ข้อ 3): ไปทาง Option B — แยก schema** ส่วน Option A เก็บไว้เป็นบันทึกว่าทำไมถึงไม่เลือก

**Option A — history table แยกต่อ service แต่ยังอยู่ใน `public` (ไม่เลือก)**
```
FLYWAY_TABLE=flyway_schema_history_pet
FLYWAY_SCHEMAS=public
FLYWAY_CLEAN_DISABLED=true
```
- ✅ ไม่ต้องย้ายตาราง ไม่ต้องประสานกับ auth/event ทำวันนี้ได้เลย
- ❌ ไม่มีขอบเขตความเป็นเจ้าของจริง ต้องอาศัยวินัยล้วนๆ
- ❌ **ถ้าทำ A ก่อนแล้วค่อยย้าย จะต้องย้าย `flyway_schema_history_pet` ตามไปด้วยและแก้ `FLYWAY_TABLE` กลางทาง** — ยุ่งกว่าทำ B ตั้งแต่แรก

**Option B — schema-per-service (`pet`, `auth`, `event`) ← เลือกอันนี้**
```sql
CREATE SCHEMA IF NOT EXISTS pet;
ALTER TABLE public.pets SET SCHEMA pet;   -- metadata-only ไม่ copy ข้อมูล
-- DSN: search_path=pet,public
```
- ✅ ขอบเขตชัด, `flyway clean` ปลอดภัย, grant สิทธิ์ต่อ service ได้จริง
- ✅ **ตรวจแล้วว่าไม่มี cross-service FK** (pet เก็บ `user_id` เป็น UUID เปล่าๆ ไม่มี FK ไป `users`) → ย้ายได้โดยไม่กระทบ auth
- ✅ `SET SCHEMA` เป็น metadata-only + DDL ของ PostgreSQL เป็น transactional → **ไม่มีความเสี่ยงข้อมูลหาย**
- ⚠️ ต้องตั้ง `search_path` **ก่อน** ย้าย ไม่งั้น app หาตารางไม่เจอ (Phase 2.1)

**ลำดับ:** pet (Phase 2) → auth (Phase 9) → event (Phase 10) — **ทีละตัว ห้ามทำพร้อมกัน**
ปลายทางที่ดีที่สุดจริงๆ คือแยก **database** ต่อ service แต่นั่นเป็นก้าวถัดไป (non-goal ของรอบนี้)

**อีกเรื่องที่ต้องทำ:** สร้าง DB user แยกต่อ service
- `pet_migrator` — มีสิทธิ์ DDL ใช้เฉพาะ Flyway Job
- `pet_app` — มีแค่ `SELECT/INSERT/UPDATE/DELETE` ใช้ใน runtime **ไม่มีสิทธิ์ DDL**
  → แม้ app จะโดน SQL injection ก็ `DROP TABLE` ไม่ได้ และเป็นการบังคับโดยธรรมชาติว่า AutoMigrate จะกลับมาไม่ได้อีก

### 5.3 กติกาการเขียน migration (ใส่ใน `docs/MIGRATION_GUIDE.md`)

**Naming**
```
V<version>__<snake_case_description>.sql     # DDL, รันครั้งเดียว, IMMUTABLE
R__<NNNN>_<snake_case_description>.sql       # master data, idempotent, รันซ้ำเมื่อ checksum เปลี่ยน
```
- `V` ใช้เลขจำนวนเต็มเรียง (`V1`, `V2`, ...) **ห้ามใช้ timestamp** — เพราะ repo นี้มีคน commit น้อย ไม่มีปัญหา merge conflict มากพอที่จะคุ้มกับความสับสน
- `R__` ไม่มี version — Flyway รันตาม **ลำดับตัวอักษรของ description** จึงต้อง **ใส่เลข 4 หลักนำหน้าเสมอ** (`R__0010_`, `R__0020_`) ไม่งั้นลำดับจะเดาไม่ได้เมื่อมี FK ระหว่าง master table
- `R__` ทุกไฟล์รัน **หลัง** `V__` ทั้งหมดเสมอ — ปลอดภัยที่จะอ้างตารางที่สร้างใน `V__` ล่าสุด

**กฎเหล็ก**
1. 🚫 **ห้ามแก้ไฟล์ `V__` ที่ apply บน env ใด env หนึ่งแล้ว** — checksum เพี้ยน `validate` fail ทั้ง pipeline
   ถ้าเขียนผิด → เขียน `V<next>__fix_xxx.sql` ใหม่
2. ✅ Migration ต้อง **backward compatible** กับ app version ก่อนหน้าอย่างน้อย 1 release (เพราะ Job รันก่อน pod ใหม่ขึ้น และระหว่าง rolling update จะมี pod เก่า+ใหม่พร้อมกัน)
   - เพิ่ม column → ต้อง nullable หรือมี default
   - **ลบ/rename column ต้องทำ 2 เฟส:** release N เลิกใช้ column → release N+1 ค่อย drop
3. ✅ ทุก `V__` ต้องคิดถึงข้อมูลที่มีอยู่แล้ว — ใส่ `DELETE`/`UPDATE` ทำความสะอาดก่อน `ADD CONSTRAINT` (เหมือนใน `V3__`)
4. ✅ ตารางใหญ่: ใช้ `CREATE INDEX CONCURRENTLY` — แต่มันรันใน transaction ไม่ได้ ต้องใส่ `-- flyway:executeInTransaction=false` บรรทัดแรกของไฟล์
5. ✅ ตั้ง `SET lock_timeout = '5s'; SET statement_timeout = '5min';` ต้นไฟล์ที่ ALTER ตารางใหญ่ ป้องกัน lock ค้างทั้ง DB
6. 🚫 `FLYWAY_CLEAN_DISABLED=true` เสมอทุก environment ไม่มีข้อยกเว้น
7. ✅ `FLYWAY_VALIDATE_ON_MIGRATE=true`, `FLYWAY_OUT_OF_ORDER=false`
8. ✅ Migration ที่แตะข้อมูล production ต้อง **ทดสอบบน copy ของ prod data ก่อน** ไม่ใช่แค่ DB เปล่า

**Rollback policy**
Flyway Community ไม่มี `undo` — ให้ใช้แนวทางนี้แทน:
- **Forward-fix เป็นหลัก** — เขียน `V<next>__revert_xxx.sql`
- ทุก migration ที่มีความเสี่ยง ให้เขียนไฟล์ `db/rollback/V<n>__rollback.sql` เก็บไว้ (ไม่อยู่ใน `FLYWAY_LOCATIONS`) เป็นเอกสารว่าถ้าต้องถอยจะรันอะไร
- Backup ก่อน migrate เสมอบน prod (ใส่เป็น step ใน CI ก่อน hook Job หรือใช้ PITR)

### 5.4 ตัวอย่าง master data migration

`V2__add_masterdata_tables.sql`:
```sql
-- ตารางกลางสำหรับ master data ทุกชนิดใช้โครงเดียวกัน
CREATE TABLE mst_species (
    code        varchar(50)  PRIMARY KEY,
    name_en     varchar(200) NOT NULL,
    name_th     varchar(200),
    sort_order  int          NOT NULL DEFAULT 0,
    is_active   boolean      NOT NULL DEFAULT true,
    created_at  timestamptz  NOT NULL DEFAULT now(),
    updated_at  timestamptz  NOT NULL DEFAULT now()
);

CREATE TABLE mst_cat_breeds (
    code         varchar(50)  PRIMARY KEY,
    species_code varchar(50)  NOT NULL REFERENCES mst_species(code),
    name_en      varchar(200) NOT NULL,
    name_th      varchar(200),
    sort_order   int          NOT NULL DEFAULT 0,
    is_active    boolean      NOT NULL DEFAULT true,
    created_at   timestamptz  NOT NULL DEFAULT now(),
    updated_at   timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX idx_mst_cat_breeds_active ON mst_cat_breeds(species_code, sort_order) WHERE is_active;

CREATE TABLE mst_blood_types  (code varchar(20) PRIMARY KEY, name_en varchar(100) NOT NULL, name_th varchar(100), sort_order int NOT NULL DEFAULT 0, is_active boolean NOT NULL DEFAULT true);
CREATE TABLE mst_litter_types (code varchar(20) PRIMARY KEY, name_en varchar(100) NOT NULL, name_th varchar(100), sort_order int NOT NULL DEFAULT 0, is_active boolean NOT NULL DEFAULT true);
CREATE TABLE mst_genders      (code varchar(20) PRIMARY KEY, name_en varchar(100) NOT NULL, name_th varchar(100), sort_order int NOT NULL DEFAULT 0, is_active boolean NOT NULL DEFAULT true);
-- ⚠️ ทุกตารางข้างบนเป็นชั้น B (แก้ผ่าน backoffice ได้) → ต้องมี created_by/updated_by/version ครบตาม Phase 3.2
```

> 🔴 **อัปเดตหลังการตัดสินใจข้อ 4 (2026-08-22):** เดิมส่วนนี้แนะนำให้ seed สายพันธุ์แมวด้วย `R__`
> **แต่เมื่อ backoffice ต้องแก้ master data ได้ `R__` ใช้กับตารางเหล่านั้นไม่ได้แล้ว** — ทุกครั้งที่ deploy มันจะเขียนทับสิ่งที่ admin แก้ผ่าน UI
> ตารางชั้น B (species / cat_breeds / blood_types) จึงย้ายไป seed ด้วย `V3__seed_masterdata_initial.sql` **ครั้งเดียว** แล้วให้ DB เป็นเจ้าของ (ดู Phase 3.1)
> ตัวอย่าง `R__` ข้างล่างยังใช้ได้กับ **ชั้น A (code-owned)** เท่านั้น เช่น `pet_permissions`, `role_capabilities`

`R__0010_pet_permissions.sql` — ตัวอย่างการใช้ `R__` ที่ยัง**ถูกต้อง** (code-owned, UI แก้ไม่ได้):
```sql
INSERT INTO pet.pet_permissions (id, name, description, is_active) VALUES
    ('EDIT_PROFILE',   'Edit Profile',            'แก้ไขข้อมูลโปรไฟล์สัตว์เลี้ยง', true),
    ('MANAGE_MEDICAL', 'Manage Medical Records',  'ดูและเพิ่มประวัติการรักษา/วัคซีน', true),
    ('MANAGE_WEIGHT',  'Update Weight Log',       'บันทึกน้ำหนัก',                  true),
    ('MANAGE_TASKS',   'Manage Daily Tasks',      'ดูและติ๊กงานประจำวัน',           true),
    ('MANAGE_LITTER',  'Record Litter Box',       'บันทึกการขับถ่าย',               true),
    ('MANAGE_WATER',   'Record Water Intake',     'บันทึกการดื่มน้ำ',               true)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name, description = EXCLUDED.description,
    is_active = EXCLUDED.is_active;

-- permission ที่ถูกถอดออกจากไฟล์นี้ → ปิดการใช้งาน (ไม่ DELETE เพราะ caregiver_permissions อ้างอยู่)
UPDATE pet.pet_permissions SET is_active = false
WHERE is_active = true
  AND id NOT IN ('EDIT_PROFILE','MANAGE_MEDICAL','MANAGE_WEIGHT','MANAGE_TASKS','MANAGE_LITTER','MANAGE_WATER');
```

**สามแพตเทิร์นสำหรับข้อมูลสามชนิด — แยกให้ชัด อย่าปนกัน**

| ชนิดข้อมูล | ใช้ | ตัวอย่างในโปรเจกต์นี้ |
|---|---|---|
| **Code-owned catalog** — source of truth คือ git, UI แก้ไม่ได้ | `R__` + `ON CONFLICT DO UPDATE` | `pet_permissions`, `role_capabilities` |
| **DB-owned master data** — UI แก้ได้, DB เป็น source of truth | `V__` seed **ครั้งเดียว** + Admin CRUD API | `mst_cat_breeds`, `mst_species`, `mst_blood_types` |
| **One-off data fix** รันครั้งเดียวแล้วจบ | `V__` | backfill `MANAGE_WATER`, normalize `litter.type` เป็นตัวใหญ่ |
| **Transactional data** ที่ user สร้าง | 🚫 **ห้ามใส่ใน Flyway** | pets, logs, caregivers |

**ข้อควรระวังของ `R__`:** ถ้ามีคนแก้ master data ตรงใน DB (เช่น backoffice) การ migrate รอบถัดไปจะเขียนทับ
→ ต้องตัดสินใจ: **ถ้า master data ต้องแก้ผ่าน UI ได้ ก็ห้ามใช้ `R__` กับตารางนั้น** ใช้ `V__` seed ครั้งเดียวแทน แล้วให้ backoffice เป็นเจ้าของ
✅ **การตัดสินใจข้อ 4 บอกว่า backoffice ต้องแก้ได้** → ตารางชั้น B ใช้ `V__` seed ครั้งเดียว + Admin CRUD API (Phase 3)
เหลือ `R__` ไว้ใช้กับชั้น A (`pet_permissions`, `role_capabilities`) เท่านั้น — เขียนกฎข้อนี้ลง `docs/MIGRATION_GUIDE.md` ให้ชัด

### 5.5 Helm hook Job

`helm/pet-service/templates/migration-job.yaml`:
```yaml
{{- if .Values.migration.enabled }}
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ include "pet-service.fullname" . }}-migrate-{{ .Release.Revision }}
  labels: {{- include "pet-service.labels" . | nindent 4 }}
  annotations:
    "helm.sh/hook": pre-install,pre-upgrade
    "helm.sh/hook-weight": "-5"
    "helm.sh/hook-delete-policy": before-hook-creation
spec:
  backoffLimit: {{ .Values.migration.backoffLimit | default 1 }}
  activeDeadlineSeconds: 600
  ttlSecondsAfterFinished: 3600
  template:
    spec:
      restartPolicy: Never
      securityContext: { runAsNonRoot: true, runAsUser: 1000 }
      containers:
        - name: flyway
          image: "{{ .Values.migration.image.repository }}:{{ .Values.migration.image.tag }}"
          args: ["-connectRetries=20", "migrate"]
          env:
            - name: FLYWAY_URL
              value: "jdbc:postgresql://{{ .Values.db.host }}:{{ .Values.db.port }}/{{ .Values.db.name }}?sslmode={{ .Values.db.sslMode }}"
            - name: FLYWAY_USER
              valueFrom: { secretKeyRef: { name: pet-db-migrator, key: username } }
            - name: FLYWAY_PASSWORD
              valueFrom: { secretKeyRef: { name: pet-db-migrator, key: password } }
            - name: FLYWAY_TABLE
              value: "flyway_schema_history_pet"
            - name: FLYWAY_LOCATIONS
              value: "filesystem:/flyway/sql/migration,filesystem:/flyway/sql/masterdata"
            - name: FLYWAY_BASELINE_ON_MIGRATE
              value: "true"
            - name: FLYWAY_BASELINE_VERSION
              value: "1"
            - name: FLYWAY_VALIDATE_ON_MIGRATE
              value: "true"
            - name: FLYWAY_CLEAN_DISABLED
              value: "true"
            - name: FLYWAY_OUT_OF_ORDER
              value: "false"
          resources:
            requests: { cpu: 100m, memory: 256Mi }
            limits:   { cpu: 500m, memory: 512Mi }
{{- end }}
```

**หมายเหตุสำคัญเรื่อง `BASELINE_ON_MIGRATE`**
- บน **DB ที่มีข้อมูลอยู่แล้ว (prod ปัจจุบัน)** และยังไม่มี history table → Flyway จะ baseline ที่ version 1 แล้ว **ข้าม `V1__`** (mark เป็น applied โดยไม่รัน) จากนั้นรัน `V2__` ขึ้นไป ✅ ตรงตามที่ต้องการพอดี
- บน **DB เปล่า (local/CI)** → ไม่มีอะไรให้ baseline, `V1__` รันปกติ ✅
- ⚠️ หลังจาก prod baseline สำเร็จแล้ว ควรพิจารณาตั้ง `FLYWAY_BASELINE_ON_MIGRATE=false` เพื่อไม่ให้มันเผลอ baseline DB ผิดตัวในอนาคต
- ตรวจสอบทุกครั้งด้วย `flyway info` ก่อน แล้วค่อยปล่อยจริง — **รันแบบ dry-run บน staging ที่ restore มาจาก prod dump ก่อนเสมอ**

**Concurrency:** Flyway ใช้ PostgreSQL advisory lock ทำให้รันหลาย instance พร้อมกันปลอดภัย (ตัวหลังจะรอ) — แต่ Helm hook รันตัวเดียวอยู่แล้ว

### 5.6 Migration image

`db/Dockerfile`:
```dockerfile
FROM flyway/flyway:11-alpine
COPY migration  /flyway/sql/migration
COPY masterdata /flyway/sql/masterdata
```
- **pin version ให้แน่น** (`11-alpine` ไม่ใช่ `latest`) — Flyway major upgrade เปลี่ยนพฤติกรรม checksum ได้
- push image พร้อม tag เดียวกับ app (`sha-<commit>`) เพื่อให้ app version กับ schema version สอดคล้องกันเสมอ
- CI: เพิ่ม job `build-migrations` ที่ `needs`-ed โดย `deploy` และ build จาก `./db`

**ทางเลือกที่ง่ายกว่า (ถ้าอยากเริ่มเร็ว):** ใช้ `flyway/flyway:11-alpine` ตรงๆ แล้ว mount SQL จาก ConfigMap ที่ generate ด้วย `.Files.Glob "db/**"` — แต่ ConfigMap จำกัด 1MB และ helm `.Files` ไม่อ่านนอก chart directory ต้อง symlink SQL เข้าไปใน chart
→ **แนะนำ custom image** ตั้งแต่แรก จะได้ไม่ต้องย้ายทีหลัง

### 5.7 ลำดับการ rollout จริงบน production

```
1. [staging] restore dump จาก prod → รัน flyway info (ต้องเห็น "Schema is not empty, no history table")
2. [staging] flyway migrate → flyway info → flyway validate → smoke test
3. [prod]    backup / ยืนยัน PITR window
4. [prod]    deploy app version ที่ยัง "ไม่พึ่ง" schema ใหม่ และเอา AutoMigrate ออกแล้ว   ← สำคัญ
5. [prod]    รัน Flyway Job (baseline + V2..V4)
6. [prod]    ยืนยัน flyway info + spot check ข้อมูล master data
7. [prod]    deploy app version ที่ใช้ schema ใหม่
8. เพิกถอนสิทธิ์ DDL ของ user ที่ app ใช้ → AutoMigrate กลับมาไม่ได้อีกตลอดกาล
```
> ขั้นที่ 4 กับ 7 แยกกันคือหัวใจ — **อย่า deploy app ใหม่พร้อม migration ครั้งแรก** ถ้า migration พังจะได้ rollback ง่าย

---

## 6. การตัดสินใจที่ยืนยันแล้ว (2026-08-22)

| # | หัวข้อ | การตัดสินใจ | กระทบเฟสไหน |
|---|---|---|---|
| 1 | RBAC | **ทำ RBAC เต็มรูปแบบ** และ set `thappithakpluemacting@gmail.com` เป็น **SUPER_ADMIN** (ทำได้ทุกอย่าง) | Phase 1A (ใหม่), 1.2 |
| 2 | ขอบเขต service | **แตะทั้ง 3 service** แต่ทำทีละตัว: pet → auth → event | Phase 2, 9, 10 (ใหม่) |
| 3 | Schema separation | **ทำ** แยก schema ต่อ service | Phase 2, 9, 10 |
| 4 | Master data | **ต้องแก้ผ่าน backoffice UI ได้** → `R__` ใช้กับตารางที่แก้ได้ไม่ได้ ต้องเปลี่ยนโมเดลความเป็นเจ้าของ + ทำ admin CRUD API | Phase 3 (เขียนใหม่ทั้งเฟส) |
| 5 | Water log permission | เพิ่ม `MANAGE_WATER` ใหม่ (ตามที่แนะนำ) | Phase 1.4, 3 |
| 6 | Update semantics | ใช้ **PATCH ที่ล้างค่าเป็น null ได้** | Phase 4.4 |
| 7 | การทำงานขนาน | **ทำขนานได้** แต่ **ห้ามทำให้ app พังหรือพฤติกรรมเปลี่ยน** | §9 (ใหม่) |
| 8 | ข้อมูลเดิม | **backup ก่อนเสมอ + ต้องย้ายมาครบ 100%** ทั้งตอน migrate และตอนย้าย schema พร้อมการพิสูจน์ | §10 (ใหม่) |
| 9 | ปริมาณข้อมูล prod | **ยังไม่ทราบ** → plan ยึดเส้นทางที่ปลอดภัยที่สุด (สมมติว่ามีข้อมูลจริง) | §10.1 |
| 10 | `mst_litter_types` / `mst_genders` | **แก้ผ่าน backoffice UI ได้ด้วย** → ยุบชั้น C เข้าชั้น B, 🚫 **ถอด `CHECK (type IN (...))` ออก**, ใช้ **FK `NOT VALID`** แทน, และ **validation ในโค้ดห้าม hardcode enum** | Phase 1.3, 2.4, 3.1, **3.7 (ใหม่)** |

### ⚠️ สิ่งที่พบเพิ่มจากการตัดสินใจข้างต้น — ต้องอ่านก่อนเริ่ม

**(ก) การให้สิทธิ์ admin ด้วย email มีช่องโหว่ ถ้าไม่ระวัง**
`vertex-auth-service/main.go:183` `handleSignup` สมัครด้วย email + password ได้ **โดยไม่มีการยืนยันอีเมลใดๆ** (`User` struct ไม่มี field `EmailVerified` เลย)
→ ถ้าเขียน migration แบบ "ใครก็ตามที่มี email นี้ = SUPER_ADMIN" แล้วบัญชียังไม่ถูกสร้าง **คนอื่นสมัครด้วยอีเมลนั้นชิงไปก่อนแล้วได้สิทธิ์สูงสุดทันที**

**กติกาบังคับ** (รายละเอียดใน Phase 1A.4):
1. ให้สิทธิ์เฉพาะบัญชีที่ **มีอยู่แล้ว** เท่านั้น หรือ
2. ถ้ายังไม่มีบัญชี → ให้สิทธิ์อัตโนมัติได้เฉพาะเมื่อสมัครผ่าน **Google OAuth** (ซึ่ง verify email ให้แล้ว) เท่านั้น **ห้าม** ให้กับ signup แบบ password
3. และควรเพิ่ม `email_verified` เข้า `users` เป็นงานแยก

**(ข) `vertex-backoffice` เรียก `/api/v1/admin` อยู่จริง** → ตอนใส่ RBAC ต้องแน่ใจว่าบัญชี admin ถูก grant **ก่อน** enforce ไม่งั้น backoffice ล็อกตัวเองออก (ดู Phase 1A.6 — ลำดับ deploy)

**(ค) master data ที่แก้ผ่าน UI ได้ กับที่แก้ไม่ได้ ต้องแยกกันให้ชัด** — ไม่ใช่ทุกตารางควรเปิดให้แก้ (ดู Phase 3.1)

### ❓ ยังเหลืออีก 2 ข้อที่ต้องเช็ค (ไม่บล็อกการเริ่มงาน)

**ข้อ A — ปริมาณข้อมูลใน prod** ยิงคำสั่งนี้แล้วส่งผลลัพธ์มา จะช่วยตัดงานได้เยอะ:
```bash
kubectl exec -n vertex deploy/postgres -- psql -U vertex_admin -d vertex -c "
SELECT 'pets' t, count(*) FROM pets
UNION ALL SELECT 'pet_caregivers', count(*) FROM pet_caregivers
UNION ALL SELECT 'litter_logs', count(*) FROM litter_logs
UNION ALL SELECT 'water_logs', count(*) FROM water_logs
UNION ALL SELECT 'users', count(*) FROM users
UNION ALL SELECT 'event_logs', count(*) FROM event_logs;"
```
- **ถ้าทุกตาราง < ~100 แถว และยอมรับ downtime สั้นๆ ได้** → ข้าม baseline ทั้งหมด สร้าง schema ใหม่ใน `pet` แล้ว import ข้อมูลกลับ **ง่ายกว่ามากและได้ schema ที่สะอาดกว่า** (§10.1 เส้นทาง B)
- **ถ้ามีข้อมูลเยอะ / ยอมรับ downtime ไม่ได้** → ใช้ §10.1 เส้นทาง A (baseline + `SET SCHEMA`)
- 📌 **plan ปัจจุบันเขียนตามเส้นทาง A ไว้ก่อน** เพราะปลอดภัยกว่าเมื่อยังไม่รู้

**ข้อ B — mobile app** เรียก `/api/v1/master-data/cat-breeds` และ `/blood-types` อยู่ไหม และ deploy version ใหม่ได้เร็วแค่ไหน
- ตอบไม่ได้ตอนนี้ก็ไม่เป็นไร — Phase 3 ออกแบบให้ **คง response shape เดิมของ v1 ไว้** และเพิ่ม `/api/v2/master-data/*` แทน ตามหลัก "ห้ามทำให้ app พัง" (ข้อ 7)

---

## 7. ลำดับการลงมือ

> ปรับตามการตัดสินใจข้อ 7 (ทำขนานได้) — ดูแผนภาพเต็มใน **§9.1**

### สัปดาห์ที่ 1 — วางฐาน (ทำเรียงกัน ห้ามขนาน)
1. **Phase 0** — safety net, characterization test, CI gate, repo hygiene
2. **Phase 4.5 + 4.8** — แยก `main.go` และ `model.go` (pure refactor, merge ทันที)
   → ตัดจุดชนไฟล์หลักออกก่อนแยกสาย (§9.2)

### สัปดาห์ที่ 2–3 — แยก 3 สายขนานกัน
| สาย | งาน | เจ้าของ |
|---|---|---|
| **A** | Phase 1.1, 1.3, 1.4, 1.5, 1.6 — security ที่ pet-service | |
| **B** | Phase 1A — RBAC ที่ auth-service | |
| **C** | Phase 2 — Flyway + แยก schema `pet` (ตาม §10 runbook) | |

**🔗 MERGE 1:** Phase 1.2 (เชื่อม RBAC) — ต้องรอสาย A + B และ **ต้องผ่าน Phase 1A.6 ขั้นที่ 4 ก่อน**

### สัปดาห์ที่ 4 — Master data
**🔗 MERGE 2:** Phase 3 — ต้องรอ RBAC (สำหรับ `masterdata:write`) และตาราง `mst_*` (จากสาย C)

### สัปดาห์ที่ 5+ — ขนานได้อีกครั้ง
Phase 5 (perf) ‖ Phase 6 (ops) ‖ Phase 7 (outbox) → แล้วค่อย Phase 9 (auth schema) → Phase 10 (event schema)

---

### ถ้าเวลาน้อยกว่านั้นมาก — ทำ 5 อย่างนี้ก่อน
1. **Phase 0** — ถ้าไม่มี characterization test จะพิสูจน์ไม่ได้ว่า "app ทำงานได้เหมือนเดิม" (เงื่อนไขข้อ 7)
2. **Phase 1A + 1.1 + 1.2** — authorization ตอนนี้ข้อมูลลูกค้าเปิดโล่งทั้งหมด นี่คือเรื่องที่เร่งด่วนจริง
3. **Phase 5.1** — เอา avatar ออกจาก list response (memory limit 128Mi + blob ทุกตัว = OOM รอเกิด)
4. **Phase 6.1–6.3** — graceful shutdown + probes (rolling update ตอนนี้ตัด request ทิ้ง)
5. **Phase 7.1** — ใส่ timeout ให้ event publisher (30 นาที คุ้มที่สุดในเอกสารนี้)

**Phase 2 (Flyway) ยิ่งเลื่อนยิ่งแพง** — ทุกวันที่ `AutoMigrate` ยังอยู่ schema ยิ่งห่างจากสิ่งที่ควบคุมได้
📌 **ถ้าผลนับแถวใน §6 ข้อ A ออกมาน้อย → ทำ Phase 2 เดี๋ยวนี้เลยด้วยเส้นทาง B (§10.1)** เพราะตอนนี้คือช่วงที่ถูกที่สุดและได้ schema สะอาดที่สุด

---

## 8. ภาคผนวก — ตารางอ้างอิงจุดในโค้ด

| ID | ไฟล์:บรรทัด | ปัญหา |
|---|---|---|
| S-1 | `internal/adapter/handler/pet.go:60,93,110` + caregiver/litter/water ทุกไฟล์ | ไม่มี ownership check |
| S-2 | `internal/adapter/handler/pet.go:53,126` | admin route ไม่มี role check |
| S-3 | `internal/adapter/handler/pet.go:21,80,99` | `petRequest` dead code, bind domain ตรงๆ |
| S-4 | `internal/adapter/handler/caregiver.go:56` → `repository/caregiver.go:76` | client แก้ master permission ได้ |
| S-5 | `pkg/middleware/auth.go:23,45` | JWT ไม่ verify iss/aud, มี DEBUG log |
| S-6 | `cmd/server/main.go:206`, `Dockerfile:16` | public key ฝังใน image |
| S-7 | `Dockerfile:11-13`, `helm/.../values.yaml:20,23` | run เป็น root, securityContext ว่าง |
| S-8 | `cmd/server/main.go:78-131` | log body ทั้ง req/res, ใช้ `fmt.Println` |
| S-9 | `k8s/01-postgres.yaml:9`, `docker-compose.yml:8` | plaintext password ใน git |
| S-10 | `cmd/server/main.go:65` | ไม่มี recover/limiter/timeout middleware |
| C-1 | `internal/adapter/handler/pet.go:73` | unchecked type assertion |
| C-2 | `internal/application/pet_service.go:100,127` | `ActorID` = username ของเจ้าของ |
| C-3 | `internal/application/pet_service.go:60-95` | PUT ทำตัวเป็น PATCH, ล้างค่า null ไม่ได้ |
| C-4 | `internal/domain/errors.go:8-11` | sentinel error ไม่มีใคร return |
| C-5 | `internal/application/caregiver_service.go:26` | กลืน error |
| C-6 | `internal/adapter/repository/caregiver.go:119-125`, `cmd/server/main.go:196` | Seed ทิ้ง error, ส่ง nil ctx |
| C-7 | `internal/adapter/repository/litter.go:43` | `Create` slice ว่าง → 500 |
| C-8 | `pkg/apperror/error.go:64` | `IsAppError` ไม่ unwrap |
| C-9 | `internal/adapter/repository/water.go:41` vs `litter.go:59` | delete semantics ไม่ตรงกัน |
| C-10 | `internal/adapter/repository/model.go:137,161` | `is_active` ไม่มีใคร query |
| C-11 | `internal/adapter/repository/litter.go:21` | ไม่มี ORDER BY |
| C-12 | `internal/adapter/repository/model.go:127-136,151-160` | ไม่มี FK ไป pets |
| C-13 | `internal/adapter/repository/caregiver.go:107` | `FindAll` ไม่มีใครเรียก, ไม่มี endpoint |
| C-14 | `Dockerfile.dev:7`, `docker-compose.yml:19` | dev image พัง, port ไม่ตรงกัน 3 ที่ |
| P-1 | `internal/adapter/repository/pet.go:21-50` | list ส่ง avatar blob |
| P-2 | ทุก list endpoint | ไม่มี pagination |
| P-4 | `cmd/server/main.go:151-172` | ไม่มี connection pool config |
| A-1 | `pkg/apperror/error.go:6-7` | pkg import internal + gorm |
| A-2 | `internal/application/litter_service.go:95-112` | MasterData ปนใน litter |
| A-4 | `cmd/server/main.go` ทั้งไฟล์ | 213 บรรทัดปนทุกอย่าง |
| A-5 | `cmd/server/main.go:143`, `internal/adapter/event/http_publisher.go:19` | os.Getenv กระจาย |
| O-1 | `helm/pet-service/templates/deployment.yaml:35` | ไม่มี probes |
| O-2 | `cmd/server/main.go:147` | `log.Fatal(app.Listen())` |
| O-9 | `go.mod:6-30` | ทุก dep เป็น `// indirect` |

---
*จบเอกสาร — ถ้าอนุมัติแล้ว แนะนำให้ส่งให้ agent ทีละ Phase พร้อมบอกให้อ่านไฟล์นี้ทั้งไฟล์ก่อนเริ่มทุกครั้ง*

---

## 9. การทำงานขนาน — Work Streams (การตัดสินใจข้อ 7)

> เงื่อนไขบังคับ: **"อย่าทำให้ app พัง และทำงานได้เช่นเดิม"**
> ทุกกฎในหัวข้อนี้มีไว้เพื่อรักษาเงื่อนไขนั้นขณะที่มีหลายสายทำงานพร้อมกัน

### 9.1 แบ่งเป็น 3 สาย

```
                    Phase 0 (Safety net) ← ทุกสายต้องรอตัวนี้เสร็จก่อน
                            │
        ┌───────────────────┼───────────────────┐
        ▼                   ▼                   ▼
   สาย A: pet security   สาย B: auth RBAC   สาย C: database
   1.1 authz layer       1A.1–1A.5          2.1 แยก schema pet
   1.3 DTO+validation    1A.7 admin API     2.2 baseline
   1.4 caregiver perm                       2.4 migrations V2–V7
   1.5 JWT+hardening                        2.5 ถอด AutoMigrate
   1.6 logging                              2.6 local dev
        │                   │                   │
        └─────────┬─────────┘                   │
                  ▼                             │
          🔗 MERGE 1: Phase 1.2                 │
          (ต้องมีทั้ง authz layer + roles claim) │
                  │                             │
                  └──────────┬──────────────────┘
                             ▼
                    🔗 MERGE 2: Phase 3 (master data)
                    (ต้องมีทั้ง RBAC + ตาราง mst_*)
                             │
        ┌────────────────────┼────────────────────┐
        ▼                    ▼                    ▼
   Phase 4 (layering)   Phase 5 (perf)      Phase 6 (ops)
        └────────────────────┼────────────────────┘
                             ▼
              Phase 7 (outbox) → Phase 9 (auth) → Phase 10 (event)
```

**สาย A และ B ทำขนานได้เต็มที่** — คนละ repo คนละไฟล์ ไม่ชนกันเลย
**สาย C ทำขนานกับ A ได้** แต่ต้องระวังจุดชนใน §9.2
**Phase 4/5/6 ทำขนานกันได้** หลัง MERGE 2

### 9.2 จุดที่ไฟล์ชนกัน (conflict hotspots)

| ไฟล์ | สายที่แตะ | วิธีเลี่ยง |
|---|---|---|
| `cmd/server/main.go` | A (middleware), C (ถอด AutoMigrate) | ⚠️ **ชนแน่นอน** — ให้**สาย C ทำ Phase 4.5 (แยก `main.go` ออกเป็น `internal/bootstrap`) เป็นงานแรกสุด** แล้ว merge ทันที จากนั้นทั้งสองสายแตะคนละไฟล์ |
| `internal/port/output.go` | A (`FindAccess`), C (ไม่ค่อยแตะ) | สาย A เป็นเจ้าของ สาย C ขอผ่าน A |
| `internal/adapter/repository/model.go` | A (ไม่แตะ), C (แตะเยอะ) | สาย C เป็นเจ้าของ + ทำ Phase 4.8 (แยกเป็น `model/` package) ก่อน |
| `internal/domain/permission.go` | A (`MANAGE_WATER`), C (migration) | ประสานกันครั้งเดียวตอน Phase 3.4 |
| `helm/pet-service/**` | C (migration Job), Phase 6 (probes) | คนละ template file — ไม่ชน |
| `.github/workflows/deploy.yml` | Phase 0 (test job), C (migration job) | ⚠️ ให้ Phase 0 วาง skeleton ของ job ทั้งหมดไว้ก่อน สายอื่นแค่เติมเนื้อใน |

> 📌 **กฎข้อแรกก่อนเริ่มงานขนาน: ทำ Phase 4.5 (แยก `main.go`) + Phase 4.8 (แยก `model.go`) ก่อน แล้ว merge เข้า main ทันที**
> สองอันนี้เป็น pure refactor ไม่เปลี่ยนพฤติกรรม แต่ตัดจุดชนหลักออกไปได้ทั้งหมด

### 9.3 กฎบังคับทุก PR — เพื่อไม่ให้ app พัง

1. ✅ **Characterization test จาก Phase 0 ต้องเขียวทั้งหมด** ทุก PR ไม่มีข้อยกเว้น
   ถ้า test ตัวไหนต้องเปลี่ยน = **พฤติกรรมเปลี่ยน** ต้องอธิบายใน PR description และได้รับการอนุมัติก่อน
2. ✅ **API เดิมต้องใช้งานได้เหมือนเดิม** — endpoint / method / request shape / response shape / status code
   ที่ตกลงกันแล้วว่าเปลี่ยนได้มีแค่ 2 อย่าง:
   - `GET /admin/pets` เข้มขึ้น (ตั้งใจ — เป็นการอุดช่องโหว่)
   - เพิ่ม endpoint/field ใหม่ (additive ไม่กระทบของเดิม)
3. ✅ **Migration ต้อง backward compatible กับ app version ก่อนหน้า 1 release** — เพราะ Flyway Job รันก่อน pod ใหม่ขึ้น ระหว่าง rolling update จะมี pod เก่า+ใหม่พร้อมกัน
   - เพิ่ม column → nullable หรือมี default
   - ลบ/rename column → 2 เฟส (release N เลิกใช้ → release N+1 ค่อย drop)
4. ✅ **PR ละหนึ่งเรื่อง** — ห้ามรวม refactor เข้ากับ behavior change ใน PR เดียว review ไม่ออก
5. ✅ **rebase จาก main ทุกเช้า** — สาย A/B/C แยกกันนานเกิน 2-3 วันจะ merge ยาก
6. ✅ ฟีเจอร์ที่ยังไม่พร้อม → ซ่อนหลัง env flag ที่ default = ปิด แทนการค้างบน branch ยาวๆ
7. 🚫 **ห้าม force push บน branch ที่มีคนอื่นใช้อยู่**

### 9.4 ลำดับ deploy ที่ปลอดภัย (ห้ามสลับ)

```
1. Phase 0            → deploy ได้ทันที (ไม่เปลี่ยนพฤติกรรม)
2. Phase 4.5 + 4.8    → deploy ได้ทันที (pure refactor)
3. Phase 1A (auth)    → deploy + ยืนยันว่า token มี roles  ← GATE
4. Phase 2 (database) → §10 runbook เต็มรูปแบบ             ← GATE
5. Phase 1.2 (enforce RBAC)  ← ห้ามก่อนขั้นที่ 3 ผ่าน
6. Phase 1.1/1.3–1.6  → deploy ได้ (แต่แนะนำรวมกับขั้นที่ 5)
7. Phase 3 ขึ้นไป
```
> ⚠️ ขั้นที่ 3 ต้องมาก่อนขั้นที่ 5 เสมอ ไม่งั้น **backoffice เข้าไม่ได้** (Phase 1A.6)

---

## 10. Backup & Data Verification Runbook (การตัดสินใจข้อ 8)

> **"ข้อมูลเดิมต้อง backup และย้ายมาให้ครบเมื่อ migrate และย้าย schema"**
> หัวข้อนี้คือขั้นตอนที่ทำให้พิสูจน์ได้ว่าครบจริง ไม่ใช่แค่เชื่อว่าครบ

### 10.1 Gate แรก — เลือกเส้นทางจากปริมาณข้อมูล

รันคำสั่งใน §6 ข้อ A ก่อนตัดสินใจ

**เส้นทาง A — มีข้อมูลจริง / ยอมรับ downtime ไม่ได้** *(plan เขียนตามเส้นทางนี้)*
- ย้าย schema ด้วย `ALTER TABLE ... SET SCHEMA` — metadata-only ไม่มีการ copy ข้อมูล
- baseline ด้วย `pg_dump --schema-only` แล้ว `baselineOnMigrate=true`
- downtime ≈ 0 (แค่ lock สั้นๆ ตอน `SET SCHEMA`)
- ✅ ปลอดภัยที่สุด ❌ schema เดิมที่ AutoMigrate สร้างไว้ไม่สวยก็ต้องอยู่กับมันไป

**เส้นทาง B — ข้อมูลน้อยมาก (< ~100 แถว/ตาราง) และยอม downtime สั้นๆ ได้**
```
1. pg_dump --data-only ตารางทั้งหมดเก็บไว้
2. drop ตารางเดิมทิ้ง
3. เขียน V1__ เป็น schema ใหม่ที่ออกแบบดีๆ ใน pet schema
   (แก้ชื่อ column, เปลี่ยน birth_date เป็น date, ตัด default gen_random_uuid() ที่ไม่ได้ใช้,
    ใส่ NOT NULL / CHECK / FK ครบตั้งแต่แรก)
4. flyway migrate บน DB เปล่า
5. เขียน script import ข้อมูลกลับ + พิสูจน์จำนวนแถวตาม §10.3
```
- ✅ ได้ schema สะอาดกว่ามาก ประหยัดเวลา Phase 2 ไปเกือบครึ่ง ✅ ไม่ต้องทำ V4/V5 (constraint อยู่ใน V1 เลย)
- ❌ ต้องเขียน import script และมี downtime
- 📌 **ถ้าผลนับออกมาน้อยจริง แนะนำเส้นทาง B อย่างยิ่ง** — นี่คือช่วงเวลาที่ถูกที่สุดในการแก้ schema ให้ถูกต้อง

### 10.2 ก่อนแตะข้อมูล — Backup (บังคับทุกครั้ง)

```bash
STAMP=$(date +%Y%m%d-%H%M%S)

# 1. full logical backup (custom format — restore ทีละตารางได้)
kubectl exec -n vertex deploy/postgres -- \
  pg_dump -U vertex_admin -d vertex -Fc --no-owner \
  > backup/vertex-${STAMP}.dump

# 2. plain SQL อีกชุด (อ่านด้วยตาได้ / grep ได้ตอนฉุกเฉิน)
kubectl exec -n vertex deploy/postgres -- \
  pg_dump -U vertex_admin -d vertex --no-owner \
  | gzip > backup/vertex-${STAMP}.sql.gz

# 3. บันทึก fingerprint ของข้อมูล (§10.3)
psql "$PROD_DSN" -f db/verify/fingerprint.sql > backup/fingerprint-before-${STAMP}.txt
```

**กฎ:**
- 🚫 **backup ที่ยังไม่ได้ทดสอบ restore = ไม่นับว่ามี backup** — ต้อง restore ลง staging แล้ว query ดูจริงอย่างน้อย 1 ครั้ง
- 📦 เก็บไฟล์ backup **นอกคลัสเตอร์** (เครื่องตัวเอง / object storage / drive) ไม่ใช่ใน pod
- ⏱️ ยืนยัน PITR window ของ Postgres ก่อนเริ่ม (ถ้าไม่มี → ตั้งก่อน หรือยอมรับว่ามีแค่ logical backup)
- 📝 บันทึกเวลาที่ backup และเวลาที่เริ่ม migrate — ถ้าต้อง restore จะรู้ว่าเสียข้อมูลช่วงไหนไป

### 10.3 พิสูจน์ว่าข้อมูลครบ — Fingerprint

`db/verify/fingerprint.sql` — รัน**ก่อน**และ**หลัง**ทุกครั้งที่แตะข้อมูล แล้ว diff กัน:
```sql
\pset format unaligned
\pset fieldsep '|'

WITH t AS (
  SELECT 'pets' AS tbl, count(*) AS n,
         md5(string_agg(id::text, ',' ORDER BY id)) AS id_hash,
         md5(string_agg(md5(p.*::text), ',' ORDER BY p.id)) AS row_hash
  FROM pets p
  UNION ALL SELECT 'pet_caregivers', count(*),
         md5(string_agg(id::text, ',' ORDER BY id)),
         md5(string_agg(md5(c.*::text), ',' ORDER BY c.id)) FROM pet_caregivers c
  UNION ALL SELECT 'litter_logs', count(*),
         md5(string_agg(id::text, ',' ORDER BY id)),
         md5(string_agg(md5(l.*::text), ',' ORDER BY l.id)) FROM litter_logs l
  UNION ALL SELECT 'water_logs', count(*),
         md5(string_agg(id::text, ',' ORDER BY id)),
         md5(string_agg(md5(w.*::text), ',' ORDER BY w.id)) FROM water_logs w
  UNION ALL SELECT 'pet_permissions', count(*),
         md5(string_agg(id::text, ',' ORDER BY id)), NULL FROM pet_permissions
  UNION ALL SELECT 'users', count(*),
         md5(string_agg(id::text, ',' ORDER BY id)),
         md5(string_agg(md5(u.*::text), ',' ORDER BY u.id)) FROM users u
)
SELECT tbl, n, id_hash, row_hash FROM t ORDER BY tbl;

-- ตาราง join ที่ไม่มี PK เดี่ยว นับแยก
SELECT 'caregiver_permissions' tbl, count(*) n FROM caregiver_permissions;
```

**การตีความผลลัพธ์:**

| ผล | หมายความว่า | ทำอย่างไร |
|---|---|---|
| `n` + `id_hash` + `row_hash` ตรงกันหมด | ข้อมูลเหมือนเดิมเป๊ะ ✅ | ผ่าน |
| `n` + `id_hash` ตรง แต่ `row_hash` ต่าง | จำนวนและตัวตนครบ แต่**ค่าในบางแถวเปลี่ยน** | ⚠️ ถูกต้องถ้าเป็น migration ที่ตั้งใจแปลงข้อมูล (เช่น `upper(type)`) — **ต้องอธิบายได้ว่าแถวไหนเปลี่ยนเพราะอะไร** |
| `n` ตรง แต่ `id_hash` ต่าง | จำนวนเท่ากันแต่**คนละแถว** 🚨 | **หยุดทันที rollback** |
| `n` ต่าง | ข้อมูลหายหรือเกิน 🚨 | **หยุดทันที rollback** |

> 📌 สำหรับ `ALTER TABLE ... SET SCHEMA` **ทั้งสามค่าต้องตรงกัน 100%** เพราะไม่มีการแตะข้อมูลเลย
> ถ้าไม่ตรง = มีคนเขียนข้อมูลเข้ามาระหว่างนั้น หรือย้ายตารางไม่ครบ

**เช็คเพิ่มหลังย้าย schema:**
```sql
-- ต้องไม่มีตารางของ pet-service ค้างอยู่ที่ public
SELECT schemaname, tablename FROM pg_tables
WHERE tablename IN ('pets','pet_caregivers','pet_permissions',
                    'caregiver_permissions','litter_logs','water_logs')
ORDER BY schemaname;
-- คาดหวัง: schemaname = 'pet' ทุกแถว ไม่มี 'public'

-- FK ทั้งหมดยังชี้ถูกที่
SELECT conname, conrelid::regclass AS from_tbl, confrelid::regclass AS to_tbl
FROM pg_constraint WHERE contype = 'f'
  AND connamespace = 'pet'::regnamespace;

-- ไม่มี sequence / index ค้างที่ public
SELECT sequencename, schemaname FROM pg_sequences WHERE schemaname = 'public';
```

### 10.4 Rollback

**ระหว่าง `SET SCHEMA` (ขั้นที่ 3 ของ Phase 2.1)**
DDL ของ PostgreSQL เป็น transactional — ถ้าอยู่ใน `BEGIN...COMMIT` เดียวกัน error = rollback อัตโนมัติ ไม่มีสถานะครึ่งๆ
ถ้า commit ไปแล้วแต่ต้องถอย:
```sql
BEGIN;
ALTER TABLE pet.pets SET SCHEMA public;
-- ... ทุกตาราง
COMMIT;
```
app ที่มี `search_path=pet,public` จะยังทำงานได้ทั้งสองทาง ✅

**ระหว่าง Flyway migration**
- Flyway Community **ไม่มี `undo`** → ใช้ forward-fix เป็นหลัก เขียน `V<next>__revert_xxx.sql`
- ทุก migration ที่มีความเสี่ยง เขียนไฟล์คู่ไว้ที่ `db/rollback/V<n>__rollback.sql` (ไม่อยู่ใน `FLYWAY_LOCATIONS`) เป็นเอกสารว่าถ้าต้องถอยจะรันอะไร
- migration ที่ทำลายข้อมูล (drop column, delete แถว) → **ต้อง backup ตารางนั้นไว้ในตาราง `_backup_*` ก่อน** ในไฟล์ migration เดียวกัน แล้วค่อย drop ทีหลังเมื่อมั่นใจ
- ถ้า migration fail กลางทาง: Flyway จะ mark เป็น `failed` ใน history → ต้อง `flyway repair` หลังแก้ไฟล์ (⚠️ `repair` แก้แค่ history table ไม่ได้ย้อนข้อมูล)

**Rollback ท่าสุดท้าย — restore จาก dump**
```bash
kubectl exec -i -n vertex deploy/postgres -- \
  pg_restore -U vertex_admin -d vertex --clean --if-exists < backup/vertex-${STAMP}.dump
```
🚨 จะเสียข้อมูลที่เขียนเข้ามาหลังเวลา backup — ประกาศ maintenance window ก่อนเสมอ

### 10.5 Checklist สรุป — ติดไว้ข้างจอตอนทำจริง

```
ก่อนเริ่ม
[ ] ประกาศ maintenance window (ถึงแม้คาดว่า downtime ≈ 0)
[ ] pg_dump 2 รูปแบบ + เก็บนอกคลัสเตอร์
[ ] restore dump ลง staging แล้ว query ยืนยันว่าใช้ได้จริง
[ ] รัน fingerprint.sql → เก็บผล "before"
[ ] ยืนยัน PITR window
[ ] ซ้อม migration ทั้งชุดบน staging ที่ restore จาก prod dump   ← จับปัญหา constraint ติดตั้งไม่ได้ ที่นี่
[ ] SELECT DISTINCT type FROM litter_logs;  ตรวจค่าแปลกก่อนใส่ CHECK

ระหว่างทำ
[ ] deploy app ที่มี search_path=pet,public ก่อน (ยืนยัน /readyz เขียว)
[ ] รัน 001_move_to_pet_schema.sql
[ ] fingerprint "after" → diff กับ "before" → ต้องตรงทั้ง 3 ค่า      ← GATE
[ ] เช็คว่าไม่มีตารางค้างที่ public (§10.3)                          ← GATE
[ ] flyway baseline → flyway info (V1 ต้องเป็น Baseline)
[ ] flyway migrate → flyway info → flyway validate
[ ] fingerprint อีกรอบ → อธิบาย row_hash ที่เปลี่ยนได้ทุกตาราง       ← GATE
[ ] SELECT * FROM pet.orphaned_logs_quarantine → ต้องว่าง            ← GATE

หลังเสร็จ
[ ] smoke test: login → GET /pets → POST litter-log → GET อีกครั้ง
[ ] characterization test suite ทั้งชุดเขียว
[ ] ยืนยัน pet_app รัน CREATE TABLE ไม่ได้
[ ] เก็บ backup + fingerprint ไว้อย่างน้อย 30 วัน
[ ] บันทึกสิ่งที่เจอลง docs/MIGRATION_GUIDE.md
```
