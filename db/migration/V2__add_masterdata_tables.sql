-- =============================================================================
-- V2 — ตาราง master data
-- =============================================================================
-- ทุกตารางในไฟล์นี้เป็น "ชั้น B" ตาม docs/REFACTOR_PLAN.md §Phase 3.1
-- คือ database เป็น source of truth และแก้ผ่าน backoffice UI ได้
--
-- ⚠️ ห้าม seed ตารางเหล่านี้ด้วย repeatable migration (R__)
--    เพราะ R__ จะรันใหม่ทุกครั้งที่ checksum เปลี่ยน แล้วเขียนทับสิ่งที่ admin แก้ผ่าน UI
--    การ seed ครั้งแรกอยู่ที่ V3 (รันครั้งเดียวแล้วจบ)
-- =============================================================================

-- created_by / updated_by / version จำเป็นเพราะเปิดให้แก้ผ่าน UI:
--   ต้องรู้ว่าใครแก้ และต้องกัน admin สองคนแก้ชนกัน (optimistic locking)

CREATE TABLE IF NOT EXISTS pet.mst_species (
    code       varchar(50)  NOT NULL,
    name_en    varchar(200) NOT NULL,
    name_th    varchar(200),
    sort_order int          NOT NULL DEFAULT 0,
    is_active  boolean      NOT NULL DEFAULT true,
    version    int          NOT NULL DEFAULT 1,
    created_at timestamptz  NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz  NOT NULL DEFAULT now(),
    updated_by uuid,
    CONSTRAINT mst_species_pkey PRIMARY KEY (code)
);

CREATE TABLE IF NOT EXISTS pet.mst_cat_breeds (
    code         varchar(50)  NOT NULL,
    species_code varchar(50)  NOT NULL,
    name_en      varchar(200) NOT NULL,
    name_th      varchar(200),
    -- legacy_label เก็บสตริงที่ API v1 เคยคืนแบบตรงตัวอักษร เช่น 'Scottish Fold (หูพับ)'
    -- ทำให้ย้าย master data เข้า DB ได้โดย response ของ v1 ไม่เปลี่ยน (Phase 3.6)
    legacy_label varchar(300),
    sort_order   int          NOT NULL DEFAULT 0,
    is_active    boolean      NOT NULL DEFAULT true,
    version      int          NOT NULL DEFAULT 1,
    created_at   timestamptz  NOT NULL DEFAULT now(),
    created_by   uuid,
    updated_at   timestamptz  NOT NULL DEFAULT now(),
    updated_by   uuid,
    CONSTRAINT mst_cat_breeds_pkey PRIMARY KEY (code),
    CONSTRAINT fk_mst_cat_breeds_species FOREIGN KEY (species_code)
        REFERENCES pet.mst_species (code)
);

CREATE INDEX IF NOT EXISTS idx_mst_cat_breeds_active
    ON pet.mst_cat_breeds (species_code, sort_order) WHERE is_active;

CREATE TABLE IF NOT EXISTS pet.mst_blood_types (
    code       varchar(50)  NOT NULL,
    name_en    varchar(200) NOT NULL,
    name_th    varchar(200),
    legacy_label varchar(300),
    sort_order int          NOT NULL DEFAULT 0,
    is_active  boolean      NOT NULL DEFAULT true,
    version    int          NOT NULL DEFAULT 1,
    created_at timestamptz  NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz  NOT NULL DEFAULT now(),
    updated_by uuid,
    CONSTRAINT mst_blood_types_pkey PRIMARY KEY (code)
);

-- ⚠️ mst_litter_types และ mst_genders เดิมออกแบบเป็น "ชั้น C" ที่บังคับด้วย
--    CHECK constraint และแก้ผ่าน UI ไม่ได้
--    แต่ตามการตัดสินใจล่าสุด (แก้ผ่าน backoffice ได้) จึงย้ายมาเป็นชั้น B:
--      - ห้ามใส่ CHECK (type IN (...)) เพราะ admin เพิ่มชนิดใหม่แล้วจะบันทึก log ไม่ได้
--      - ใช้ FOREIGN KEY แบบ NOT VALID แทน (V8)
CREATE TABLE IF NOT EXISTS pet.mst_litter_types (
    code       varchar(50)  NOT NULL,
    name_en    varchar(200) NOT NULL,
    name_th    varchar(200),
    sort_order int          NOT NULL DEFAULT 0,
    is_active  boolean      NOT NULL DEFAULT true,
    version    int          NOT NULL DEFAULT 1,
    created_at timestamptz  NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz  NOT NULL DEFAULT now(),
    updated_by uuid,
    CONSTRAINT mst_litter_types_pkey PRIMARY KEY (code)
);

CREATE TABLE IF NOT EXISTS pet.mst_genders (
    code       varchar(50)  NOT NULL,
    name_en    varchar(200) NOT NULL,
    name_th    varchar(200),
    sort_order int          NOT NULL DEFAULT 0,
    is_active  boolean      NOT NULL DEFAULT true,
    version    int          NOT NULL DEFAULT 1,
    created_at timestamptz  NOT NULL DEFAULT now(),
    created_by uuid,
    updated_at timestamptz  NOT NULL DEFAULT now(),
    updated_by uuid,
    CONSTRAINT mst_genders_pkey PRIMARY KEY (code)
);

-- trigger กลาง: อัปเดต updated_at ทุกครั้งที่มีการแก้ผ่าน UI
CREATE OR REPLACE FUNCTION pet.touch_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['mst_species','mst_cat_breeds','mst_blood_types','mst_litter_types','mst_genders']
    LOOP
        EXECUTE format(
            'CREATE TRIGGER trg_%1$s_touch BEFORE UPDATE ON pet.%1$s
             FOR EACH ROW EXECUTE FUNCTION pet.touch_updated_at()', t);
    END LOOP;
END $$;
