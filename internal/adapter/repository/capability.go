package repository

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"
)

// capabilityCacheTTL — role_capabilities เปลี่ยนเฉพาะตอน deploy (seed ด้วย R__)
// จึง cache ได้นาน แต่ไม่ถาวร เผื่อมีการแก้ระหว่าง migration
const capabilityCacheTTL = 5 * time.Minute

// GORMCapabilityRepository อ่าน role → capability พร้อม cache ในหน่วยความจำ
//
// ตารางนี้ถูกอ่านทุก request ที่ตรวจสิทธิ์ แต่มีไม่กี่สิบแถวและแทบไม่เปลี่ยน
// การยิง DB ทุกครั้งจึงเป็นการเปลืองเปล่า
type GORMCapabilityRepository struct {
	db *gorm.DB

	mu       sync.RWMutex
	cache    map[string]map[string]bool // role → set ของ capability
	loadedAt time.Time
}

func NewGORMCapabilityRepository(db *gorm.DB) *GORMCapabilityRepository {
	return &GORMCapabilityRepository{db: db}
}

func (r *GORMCapabilityRepository) HasAny(ctx context.Context, roles []string, capabilities ...string) (bool, error) {
	if len(roles) == 0 || len(capabilities) == 0 {
		return false, nil
	}

	table, err := r.load(ctx)
	if err != nil {
		return false, err
	}

	for _, role := range roles {
		caps, ok := table[role]
		if !ok {
			continue
		}
		for _, c := range capabilities {
			if caps[c] {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *GORMCapabilityRepository) load(ctx context.Context) (map[string]map[string]bool, error) {
	r.mu.RLock()
	if r.cache != nil && time.Since(r.loadedAt) < capabilityCacheTTL {
		defer r.mu.RUnlock()
		return r.cache, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	// เช็คซ้ำ เผื่อมี goroutine อื่นโหลดไปแล้วระหว่างรอ lock
	if r.cache != nil && time.Since(r.loadedAt) < capabilityCacheTTL {
		return r.cache, nil
	}

	var rows []struct {
		RoleCode   string
		Capability string
	}
	if err := dbFrom(ctx, r.db).
		Table("role_capabilities").
		Select("role_code", "capability").
		Scan(&rows).Error; err != nil {
		// ถ้าโหลดไม่ได้แต่มี cache เก่าอยู่ ให้ใช้ของเก่าไปก่อน
		// ดีกว่าปฏิเสธทุก request เพราะ DB สะดุดชั่วคราว
		if r.cache != nil {
			return r.cache, nil
		}
		return nil, err
	}

	table := make(map[string]map[string]bool, len(rows))
	for _, row := range rows {
		if table[row.RoleCode] == nil {
			table[row.RoleCode] = make(map[string]bool)
		}
		table[row.RoleCode][row.Capability] = true
	}

	r.cache, r.loadedAt = table, time.Now()
	return table, nil
}

// Invalidate ล้าง cache — ใช้ในเทสต์ และเผื่อมี admin endpoint ในอนาคต
func (r *GORMCapabilityRepository) Invalidate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = nil
}
