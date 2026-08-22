-- =============================================================================
-- V1 — Baseline: schema ที่ GORM AutoMigrate สร้างไว้ ณ วันที่เริ่มใช้ Flyway
-- =============================================================================
-- ⚠️ ห้ามแก้ไฟล์นี้หลังจากถูก apply บน environment ใดแล้ว — checksum จะเพี้ยน
--    ถ้าเขียนผิด ให้เขียน V<ถัดไป>__fix_xxx.sql ใหม่แทน
--
-- บน database ที่มีข้อมูลอยู่แล้ว (prod): FLYWAY_BASELINE_ON_MIGRATE=true จะ mark
-- migration นี้ว่า applied โดยไม่รัน → schema เดิมไม่ถูกแตะเลย
-- บน database เปล่า (local/CI): migration นี้จะรันจริงและสร้าง schema ให้เหมือน prod
--
-- ชนิดข้อมูลด้านล่างอ้างอิงจาก gorm.io/driver/postgres v1.6.2 getSchemaBaseType():
--   string ไม่ระบุ size → text        string size:100 → varchar(100)
--   int (64-bit)        → bigint      float64         → decimal
--   time.Time           → timestamptz []byte          → bytea
--   bool                → boolean     uuid.UUID+tag   → uuid
-- =============================================================================

CREATE SCHEMA IF NOT EXISTS pet;

-- ต้องการสำหรับ gen_random_uuid() บน PostgreSQL < 13
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- --- pets ---------------------------------------------------------------
CREATE TABLE IF NOT EXISTS pet.pets (
    id                 uuid NOT NULL DEFAULT gen_random_uuid(),
    owner_id           uuid NOT NULL,
    owner_username     varchar(100),
    name               text,
    species            text,
    breed              text,
    color_code         text,
    birth_date         timestamptz,
    gender             text,
    avatar_data        bytea,
    current_weight     decimal,
    microchip_id       text,
    is_spayed_neutered boolean,
    blood_type         text,
    allergies          text,
    personality        text,
    created_at         timestamptz,
    updated_at         timestamptz,
    deleted_at         timestamptz,
    created_by         text,
    updated_by         text,
    CONSTRAINT pets_pkey PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_pets_owner_id       ON pet.pets (owner_id);
CREATE INDEX IF NOT EXISTS idx_pets_owner_username ON pet.pets (owner_username);
CREATE INDEX IF NOT EXISTS idx_pets_deleted_at     ON pet.pets (deleted_at);

-- --- pet_permissions (master data ของสิทธิ์ caregiver) ------------------
CREATE TABLE IF NOT EXISTS pet.pet_permissions (
    id          text NOT NULL,
    name        text,
    description text,
    is_active   boolean DEFAULT true,
    CONSTRAINT pet_permissions_pkey PRIMARY KEY (id)
);

-- --- pet_caregivers -----------------------------------------------------
CREATE TABLE IF NOT EXISTS pet.pet_caregivers (
    id         uuid NOT NULL DEFAULT gen_random_uuid(),
    pet_id     uuid NOT NULL,
    user_id    uuid NOT NULL,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    created_by text,
    updated_by text,
    CONSTRAINT pet_caregivers_pkey PRIMARY KEY (id),
    CONSTRAINT fk_pets_caregivers FOREIGN KEY (pet_id)
        REFERENCES pet.pets (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_pet_caregivers_user_id    ON pet.pet_caregivers (user_id);
CREATE INDEX IF NOT EXISTS idx_pet_caregivers_deleted_at ON pet.pet_caregivers (deleted_at);

-- ⚠️ unique index นี้ไม่ได้กรอง deleted_at ทำให้แถวที่ soft delete แล้วยังกินที่อยู่
--    เป็นเหตุผลที่ CaregiverService.Add ต้องมี logic Restore
--    V5 จะเปลี่ยนเป็น partial unique index แล้วลบ logic นั้นทิ้งได้
CREATE UNIQUE INDEX IF NOT EXISTS idx_pet_user ON pet.pet_caregivers (pet_id, user_id);

-- --- caregiver_permissions (join table ของ many2many) -------------------
-- ⚠️ ชื่อ column มาจาก "ชื่อ struct" ใน GORM (CaregiverModel / PermissionModel)
--    ไม่ใช่ชื่อตาราง — ห้ามเปลี่ยน มี model/schema_test.go เฝ้าอยู่
CREATE TABLE IF NOT EXISTS pet.caregiver_permissions (
    -- DEFAULT gen_random_uuid() ตรงนี้ไม่มีประโยชน์ (insert ระบุค่าเสมอ) แต่มีอยู่จริงใน prod
    -- เพราะ GORM ก็อปนิยาม field มาจาก Caregiver.ID ที่มี tag default:gen_random_uuid()
    -- baseline ต้องสร้าง schema ให้เหมือน prod เป๊ะ จึงคงไว้ — ถ้าจะเอาออกให้ทำใน V ถัดไป
    caregiver_model_id  uuid NOT NULL DEFAULT gen_random_uuid(),
    permission_model_id text NOT NULL,
    CONSTRAINT caregiver_permissions_pkey PRIMARY KEY (caregiver_model_id, permission_model_id),
    CONSTRAINT fk_caregiver_permissions_caregiver FOREIGN KEY (caregiver_model_id)
        REFERENCES pet.pet_caregivers (id) ON DELETE CASCADE,
    CONSTRAINT fk_caregiver_permissions_permission FOREIGN KEY (permission_model_id)
        REFERENCES pet.pet_permissions (id) ON DELETE CASCADE
);

-- --- litter_logs --------------------------------------------------------
CREATE TABLE IF NOT EXISTS pet.litter_logs (
    id         uuid NOT NULL DEFAULT gen_random_uuid(),
    pet_id     uuid NOT NULL,
    date       timestamptz,
    type       text,
    amount     bigint,
    created_at timestamptz,
    updated_at timestamptz,
    created_by text,
    is_active  boolean DEFAULT true,
    CONSTRAINT litter_logs_pkey PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_litter_pet_date ON pet.litter_logs (pet_id, date DESC);

-- --- water_logs ---------------------------------------------------------
CREATE TABLE IF NOT EXISTS pet.water_logs (
    id         uuid NOT NULL DEFAULT gen_random_uuid(),
    pet_id     uuid NOT NULL,
    date       timestamptz,
    amount     bigint,
    created_at timestamptz,
    updated_at timestamptz,
    created_by text,
    is_active  boolean DEFAULT true,
    CONSTRAINT water_logs_pkey PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_water_pet_date ON pet.water_logs (pet_id, date DESC);
