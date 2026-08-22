package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/vertex/pet-service/internal/domain"
)

// masterDataTable อธิบายว่าแต่ละชนิดผูกกับตารางไหนและถูกอ้างจากที่ใด
//
// 🔐 นี่คือ allowlist — ชื่อตารางและ column มาจากตรงนี้เท่านั้น
//
//	ค่าจาก client ใช้ได้แค่เป็น key ในการค้นหา map นี้
//	จึงไม่มีทางที่ input จะไหลลงไปเป็นชื่อตารางใน SQL
type masterDataTable struct {
	table string
	// hasSpecies / hasLegacy บอกว่าตารางนี้มี column พิเศษหรือไม่
	hasSpecies bool
	hasLegacy  bool

	// usageTable / usageColumn คือที่ที่ข้อมูลจริงอ้างถึง master ตัวนี้
	usageTable  string
	usageColumn string
	// usageMatchesLegacy = true แปลว่าข้อมูลจริงเก็บเป็น label ไม่ใช่ code
	//
	// pets.breed เก็บสตริงอย่าง "Scottish Fold (หูพับ)" ไม่ใช่ "SCOTTISH_FOLD"
	// การนับจึงต้องเทียบกับ legacy_label
	// (การย้ายไปเก็บ code เป็น column migration 3 เฟส — งานของ Phase 5)
	usageMatchesLegacy bool
}

var masterDataTables = map[domain.MasterDataType]masterDataTable{
	domain.MasterSpecies: {
		table: "mst_species", usageTable: "pets", usageColumn: "species",
	},
	domain.MasterCatBreeds: {
		table: "mst_cat_breeds", hasSpecies: true, hasLegacy: true,
		usageTable: "pets", usageColumn: "breed", usageMatchesLegacy: true,
	},
	domain.MasterBloodTypes: {
		table: "mst_blood_types", hasLegacy: true,
		usageTable: "pets", usageColumn: "blood_type", usageMatchesLegacy: true,
	},
	domain.MasterLitterTypes: {
		table: "mst_litter_types", usageTable: "litter_logs", usageColumn: "type",
	},
	domain.MasterGenders: {
		table: "mst_genders", usageTable: "pets", usageColumn: "gender",
	},
}

type GORMMasterDataRepository struct {
	db *gorm.DB
}

func NewGORMMasterDataRepository(db *gorm.DB) *GORMMasterDataRepository {
	return &GORMMasterDataRepository{db: db}
}

// masterDataRow รับผลจาก query — ใช้ pointer กับ column ที่บางตารางไม่มี
type masterDataRow struct {
	Code        string
	NameEn      string
	NameTh      *string
	LegacyLabel *string
	SpeciesCode *string
	SortOrder   int
	IsActive    bool
	Version     int
	CreatedAt   any
	CreatedBy   *string
	UpdatedAt   any
	UpdatedBy   *string
}

func (r *GORMMasterDataRepository) meta(t domain.MasterDataType) (masterDataTable, error) {
	m, ok := masterDataTables[t]
	if !ok {
		return masterDataTable{}, fmt.Errorf("%w: ไม่รู้จัก master data ชนิด %q", domain.ErrValidation, t)
	}
	return m, nil
}

// selectColumns สร้างรายการ column ให้ตรงกับตารางนั้นๆ
// ตารางที่ไม่มี legacy_label / species_code จะได้ NULL แทน
func (m masterDataTable) selectColumns() string {
	legacy, species := "NULL AS legacy_label", "NULL AS species_code"
	if m.hasLegacy {
		legacy = "legacy_label"
	}
	if m.hasSpecies {
		species = "species_code"
	}
	return fmt.Sprintf(
		"code, name_en, name_th, %s, %s, sort_order, is_active, version, created_at, created_by, updated_at, updated_by",
		legacy, species)
}

func (r *GORMMasterDataRepository) FindAll(ctx context.Context, t domain.MasterDataType, includeInactive bool) ([]domain.MasterDataItem, error) {
	m, err := r.meta(t)
	if err != nil {
		return nil, err
	}

	q := r.db.WithContext(ctx).Table(m.table).Select(m.selectColumns())
	if !includeInactive {
		q = q.Where("is_active")
	}

	var rows []masterDataRow
	if err := q.Order("sort_order, code").Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]domain.MasterDataItem, len(rows))
	for i, row := range rows {
		out[i] = row.toDomain()
	}
	return out, nil
}

func (r *GORMMasterDataRepository) FindByCode(ctx context.Context, t domain.MasterDataType, code string) (*domain.MasterDataItem, error) {
	m, err := r.meta(t)
	if err != nil {
		return nil, err
	}

	var row masterDataRow
	err = r.db.WithContext(ctx).Table(m.table).Select(m.selectColumns()).
		Where("code = ?", code).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrMasterDataNotFound
		}
		return nil, err
	}
	item := row.toDomain()
	return &item, nil
}

func (r *GORMMasterDataRepository) Create(ctx context.Context, t domain.MasterDataType, item domain.MasterDataItem) (*domain.MasterDataItem, error) {
	m, err := r.meta(t)
	if err != nil {
		return nil, err
	}

	values := map[string]any{
		"code": item.Code, "name_en": item.NameEn, "name_th": item.NameTh,
		"sort_order": item.SortOrder, "is_active": item.IsActive,
		"version": 1, "created_by": item.CreatedBy, "updated_by": item.CreatedBy,
	}
	if m.hasLegacy {
		values["legacy_label"] = item.LegacyLabel
	}
	if m.hasSpecies {
		values["species_code"] = item.SpeciesCode
	}

	if err := r.db.WithContext(ctx).Table(m.table).Create(values).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, domain.ErrMasterDataDuplicate
		}
		if isForeignKeyViolation(err) {
			return nil, fmt.Errorf("%w: speciesCode ที่อ้างถึงไม่มีอยู่จริง", domain.ErrValidation)
		}
		return nil, err
	}
	return r.FindByCode(ctx, t, item.Code)
}

// Update ใช้ optimistic locking ผ่าน column version
//
// WHERE version = ? ทำให้การแก้พร้อมกันสองคน มีคนเดียวที่สำเร็จ
// อีกคนได้ RowsAffected = 0 แล้วเรารู้ว่าต้องแจ้งให้โหลดใหม่
func (r *GORMMasterDataRepository) Update(ctx context.Context, t domain.MasterDataType, item domain.MasterDataItem) (*domain.MasterDataItem, error) {
	m, err := r.meta(t)
	if err != nil {
		return nil, err
	}

	values := map[string]any{
		"name_en": item.NameEn, "name_th": item.NameTh,
		"sort_order": item.SortOrder, "is_active": item.IsActive,
		"updated_by": item.UpdatedBy,
		"version":    gorm.Expr("version + 1"),
	}
	if m.hasLegacy {
		values["legacy_label"] = item.LegacyLabel
	}
	if m.hasSpecies {
		values["species_code"] = item.SpeciesCode
	}

	// 🚫 จงใจไม่อัปเดต code — เป็น primary key ที่ข้อมูลอื่นอ้างอยู่
	//    การเปลี่ยน code จะทำให้ข้อมูลเดิมชี้ไปที่ไม่มีอยู่
	res := r.db.WithContext(ctx).Table(m.table).
		Where("code = ? AND version = ?", item.Code, item.Version).
		Updates(values)
	if res.Error != nil {
		if isForeignKeyViolation(res.Error) {
			return nil, fmt.Errorf("%w: speciesCode ที่อ้างถึงไม่มีอยู่จริง", domain.ErrValidation)
		}
		return nil, res.Error
	}

	if res.RowsAffected == 0 {
		// แยกให้ออกว่า "ไม่มีรายการนี้" กับ "มีคนแก้ไปก่อน"
		if _, err := r.FindByCode(ctx, t, item.Code); err != nil {
			return nil, err
		}
		return nil, domain.ErrVersionConflict
	}
	return r.FindByCode(ctx, t, item.Code)
}

// CountUsage นับข้อมูลจริงที่อ้างถึง master ตัวนี้
func (r *GORMMasterDataRepository) CountUsage(ctx context.Context, t domain.MasterDataType, code string) (int64, error) {
	m, err := r.meta(t)
	if err != nil {
		return 0, err
	}

	match := code
	if m.usageMatchesLegacy {
		// ข้อมูลจริงเก็บเป็น label ไม่ใช่ code จึงต้องแปลงก่อนนับ
		item, err := r.FindByCode(ctx, t, code)
		if err != nil {
			return 0, err
		}
		match = item.DisplayLabel()
	}

	var n int64
	q := r.db.WithContext(ctx).Table(m.usageTable).Where(m.usageColumn+" = ?", match)
	if m.usageTable == "pets" || m.usageTable == "litter_logs" {
		q = q.Where("deleted_at IS NULL")
	}
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (row masterDataRow) toDomain() domain.MasterDataItem {
	item := domain.MasterDataItem{
		Code: row.Code, NameEn: row.NameEn, NameTh: row.NameTh,
		LegacyLabel: row.LegacyLabel, SpeciesCode: row.SpeciesCode,
		SortOrder: row.SortOrder, IsActive: row.IsActive, Version: row.Version,
		CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy,
	}
	item.CreatedAt = asTime(row.CreatedAt)
	item.UpdatedAt = asTime(row.UpdatedAt)
	return item
}

// isForeignKeyViolation ตรวจ SQLSTATE 23503
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23503"
	}
	return false
}

func asTime(v any) time.Time {
	if t, ok := v.(time.Time); ok {
		return t
	}
	return time.Time{}
}
