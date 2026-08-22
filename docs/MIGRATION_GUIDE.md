# คู่มือเขียน Database Migration — vertex-pet-service

> อ่านไฟล์นี้ก่อนแตะอะไรใน `db/` ทุกครั้ง
> รายละเอียดเบื้องหลังการตัดสินใจอยู่ใน [REFACTOR_PLAN.md](./REFACTOR_PLAN.md) §5 และ §10

---

## 1. ภาพรวม

`AutoMigrate` ถูกถอดออกแล้ว schema ทั้งหมดจัดการด้วย **Flyway**

```
db/
├── bootstrap/   รันด้วยมือครั้งเดียวตอนตั้งระบบ — ไม่อยู่ใน FLYWAY_LOCATIONS
├── migration/   V__  DDL + seed ครั้งแรก · รันครั้งเดียว · IMMUTABLE
├── codeowned/   R__  เฉพาะตารางที่ backoffice แก้ไม่ได้ · รันซ้ำเมื่อ checksum เปลี่ยน
├── seed/        R__  ข้อมูลตัวอย่าง local เท่านั้น · ไม่เข้า production image
├── verify/      SQL สำหรับพิสูจน์ว่าข้อมูลครบ
└── rollback/    เอกสารว่าถ้าต้องถอยจะรันอะไร · ไม่ถูกรันอัตโนมัติ
```

| ทำอะไร | ใช้คำสั่ง |
|---|---|
| ยก DB + รัน migration บนเครื่อง | `make db-up` |
| ล้างแล้วสร้างใหม่ | `make db-reset` |
| ดูสถานะ migration | `make migrate-info` |
| ตรวจ checksum | `make migrate-validate` |
| รัน integration test | `make test-integration` |

---

## 2. เลือกให้ถูกว่าจะเขียนแบบไหน

**นี่คือการตัดสินใจที่ผิดแล้วเจ็บที่สุดในระบบนี้** — เลือกจากคำถามเดียว:
**"ข้อมูลชุดนี้ให้ admin แก้ผ่าน backoffice ได้ไหม"**

| ชนิดข้อมูล | ใช้ | แก้ผ่าน UI | ตัวอย่างในโปรเจกต์นี้ |
|---|---|---|---|
| **ชั้น A · Code-owned** | `R__` + `ON CONFLICT DO UPDATE` | 🚫 ไม่ได้ | `pet_permissions`, `role_capabilities` |
| **ชั้น B · DB-owned** | `V__` seed ครั้งเดียว + Admin CRUD API | ✅ ได้ | `mst_species`, `mst_cat_breeds`, `mst_blood_types`, `mst_litter_types`, `mst_genders` |
| **One-off data fix** | `V__` | — | backfill `MANAGE_WATER` |
| **Transactional data** | 🚫 ห้ามใส่ใน Flyway | — | `pets`, `litter_logs`, `pet_caregivers` |

### 🚨 กับดักที่ต้องเข้าใจ

**อย่าใช้ `R__` กับตารางที่ backoffice แก้ได้**
`R__` รันใหม่ทุกครั้งที่ checksum เปลี่ยน → deploy ทีไรก็เขียนทับสิ่งที่ admin แก้ไว้ทุกที
นี่คือเหตุผลที่ `mst_*` ทั้งหมด seed ด้วย `V3__` ครั้งเดียวแล้วให้ database เป็น source of truth

**ชั้น A ใช้ `R__` ได้เพราะอะไร**
`pet_permissions` และ `role_capabilities` ผูกกับโค้ดที่บังคับใช้จริง
เพิ่ม permission ใหม่โดยไม่มีโค้ดรองรับ = permission ที่ไม่ทำอะไรเลย
จึงต้องมาคู่กับ code change เสมอ และไม่มีเหตุผลให้แก้ผ่าน UI

---

## 3. กฎเหล็ก

### 3.1 🚫 ห้ามแก้ไฟล์ `V__` ที่ apply บน environment ใดแล้ว
checksum จะเพี้ยน → `flyway validate` fail ทั้ง pipeline
เขียนผิดให้เขียน `V<ถัดไป>__fix_xxx.sql` ใหม่

### 3.2 ✅ Migration ต้อง backward compatible กับ app version ก่อนหน้า 1 release
Flyway Job รันก่อน pod ใหม่ขึ้น และระหว่าง rolling update จะมี pod เก่า+ใหม่พร้อมกัน
- เพิ่ม column → **ต้อง nullable หรือมี default**
- ลบ/rename column → **2 เฟส**: release N เลิกใช้ → release N+1 ค่อย drop

### 3.3 ✅ คิดถึงข้อมูลที่มีอยู่แล้วเสมอ
โค้ดเดิมไม่ validate อะไรเลย ข้อมูลจึงสกปรกได้ทุกรูปแบบ
ก่อนใส่ constraint ต้องทำความสะอาดก่อน และ**ห้าม `DELETE` ทิ้งเฉยๆ**
ให้เก็บเข้า `pet.orphaned_logs_quarantine` ก่อน (ดูตัวอย่างใน `V4__`)

### 3.4 🚫 ห้ามใช้ CHECK constraint กับค่าที่มาจาก master data
```sql
-- ❌ ผิด — admin เพิ่มชนิดใหม่ผ่าน UI แล้วบันทึก log ด้วยชนิดนั้นไม่ได้
ALTER TABLE pet.litter_logs ADD CONSTRAINT chk_type CHECK (type IN ('POOP','PEE'));

-- ✅ ถูก — ขยายค่าใหม่ได้โดยไม่ต้อง migration
ALTER TABLE pet.litter_logs ADD CONSTRAINT fk_litter_logs_type
    FOREIGN KEY (type) REFERENCES pet.mst_litter_types(code) NOT VALID;
```
`NOT VALID` = บังคับกับแถวใหม่ แต่ไม่ตรวจแถวเดิม → เพิ่มความปลอดภัยได้โดยไม่แตะข้อมูลเดิม
เมื่อข้อมูลสะอาดแล้วค่อย `VALIDATE CONSTRAINT` แยก PR

### 3.5 ✅ ตารางใหญ่ต้องระวัง lock
```sql
-- บรรทัดแรกของไฟล์ ถ้าใช้ CREATE INDEX CONCURRENTLY (รันใน transaction ไม่ได้)
-- flyway:executeInTransaction=false

SET lock_timeout = '5s';
SET statement_timeout = '5min';
```

### 3.6 🚫 `FLYWAY_CLEAN_DISABLED=true` เสมอ ทุก environment ไม่มีข้อยกเว้น

### 3.7 ✅ `R__` ต้องมีเลข 4 หลักนำหน้า
Flyway รัน `R__` ตาม**ลำดับตัวอักษรของ description** ไม่ใช่ตามเวลา
`R__0005_role_capabilities.sql` ก่อน `R__0010_pet_permissions.sql`
ทุก `R__` รัน**หลัง** `V__` ทั้งหมดเสมอ

---

## 4. Checklist ก่อนเปิด PR

```
[ ] เลือกชั้นข้อมูลถูก (§2) — ตารางที่ backoffice แก้ได้ต้องไม่ใช้ R__
[ ] ไม่ได้แก้ไฟล์ V__ เดิมที่ apply แล้ว
[ ] column ใหม่เป็น nullable หรือมี default
[ ] ถ้าใส่ constraint → ทำความสะอาดข้อมูลเดิมก่อน และเก็บของที่ต้องเอาออกไว้ในตารางกักกัน
[ ] ไม่มี CHECK constraint กับค่าที่มาจาก master data
[ ] make db-reset ผ่าน (รันบน DB เปล่าได้)
[ ] make migrate ซ้ำอีกรอบผ่าน (idempotent)
[ ] make test-integration ผ่าน (schema ตรงกับ GORM model)
[ ] ถ้าเพิ่ม field ใน GORM model → เพิ่ม RequiredSchemaVersion ใน internal/bootstrap/schema.go
[ ] ถ้า migration มีความเสี่ยง → เขียน db/rollback/V<n>__rollback.sql ไว้ด้วย
```

---

## 5. Rollback

Flyway Community **ไม่มี `undo`** ใช้ forward-fix เป็นหลัก

| สถานการณ์ | ทำอย่างไร |
|---|---|
| migration ยังไม่ commit | DDL ของ PostgreSQL เป็น transactional → rollback อัตโนมัติ |
| migration commit ไปแล้ว | เขียน `V<ถัดไป>__revert_xxx.sql` |
| migration fail กลางทาง | Flyway mark เป็น `failed` → แก้ไฟล์แล้ว `flyway repair` ⚠️ `repair` แก้แค่ history ไม่ย้อนข้อมูล |
| ย้าย schema แล้วต้องถอย | `db/bootstrap/999_rollback_move_to_public.sql` |
| ท่าสุดท้าย | `pg_restore` จาก dump — เสียข้อมูลหลังเวลา backup ต้องประกาศ maintenance window |

**migration ที่ทำลายข้อมูล** (drop column, delete แถว) ต้อง backup ตารางนั้นไว้ใน `_backup_*`
ในไฟล์ migration เดียวกันก่อน แล้วค่อย drop ทีหลังเมื่อมั่นใจ

---

## 6. Runbook สำหรับ production

ดู [REFACTOR_PLAN.md §10](./REFACTOR_PLAN.md) ฉบับเต็ม — สรุปลำดับที่ห้ามสลับ:

```
1. deploy app ที่มี DB_SEARCH_PATH=pet,public ก่อน        ← ห้ามข้าม
2. backup (2 รูปแบบ) + restore ทดสอบบน staging
3. บันทึก fingerprint "before"  (db/verify/fingerprint.sql)
4. รัน db/bootstrap/000_create_roles.sql
5. รัน db/bootstrap/001_move_to_pet_schema.sql
6. fingerprint "after" → diff → ต้องตรงทั้ง 3 ค่า          ← GATE
7. db/verify/post_move_checks.sql → ทุกข้อผ่าน             ← GATE
8. helm upgrade (Flyway Job รันอัตโนมัติเป็น pre-upgrade hook)
9. ตรวจ pet.orphaned_logs_quarantine ต้องว่าง               ← GATE
10. smoke test + characterization test ทั้งชุด
11. หลัง baseline สำเร็จ → ตั้ง migration.baselineOnMigrate=false
```

⚠️ **ขั้นที่ 1 ห้ามข้าม** — ถ้าย้าย schema ก่อนที่ app จะรู้จัก `pet`
จะได้ `relation "pets" does not exist` ทั้งระบบทันที
