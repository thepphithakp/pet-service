package application

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
)

// masterDataCacheTTL — master data เปลี่ยนไม่บ่อยแต่ admin แก้ผ่าน UI ได้
//
// 30 วินาทีเป็นการแลกระหว่าง "ไม่ยิง DB ทุก request" กับ "แก้แล้วเห็นผลเร็วพอ"
//
// จงใจไม่ทำ invalidate ข้ามเครื่อง เพราะ service รันหลาย replica
// การล้าง cache ที่ replica เดียวไม่ช่วยอะไร และการทำให้ครบทุกตัวต้องใช้
// LISTEN/NOTIFY หรือ message bus ซึ่งไม่คุ้มกับข้อมูลที่ทนหน่วง 30 วินาทีได้
const masterDataCacheTTL = 30 * time.Second

type cacheEntry struct {
	items    []domain.MasterDataItem
	loadedAt time.Time
}

// MasterDataService implements port.MasterDataUseCase และ port.MasterDataAdminUseCase
type MasterDataService struct {
	repo  port.MasterDataRepository
	perms port.PermissionRepository
	authz *Authorizer

	mu    sync.RWMutex
	cache map[domain.MasterDataType]cacheEntry
}

func NewMasterDataService(
	repo port.MasterDataRepository,
	perms port.PermissionRepository,
	authz *Authorizer,
) *MasterDataService {
	return &MasterDataService{
		repo:  repo,
		perms: perms,
		authz: authz,
		cache: make(map[domain.MasterDataType]cacheEntry),
	}
}

// --- ฝั่งผู้ใช้ทั่วไป ------------------------------------------------------

// GetCatBreeds คืนรูปแบบเดิมของ API v1 — array ของ string ธรรมดา
//
// ⚠️ ต้องคืนค่าเหมือนเดิมทุกตัวอักษร ("Scottish Fold (หูพับ)" ไม่ใช่ "SCOTTISH_FOLD")
//
//	ค่ามาจาก column legacy_label ที่ V3 seed ไว้ให้ตรงกับที่เคย hardcode
func (s *MasterDataService) GetCatBreeds(ctx context.Context) []string {
	return s.labels(ctx, domain.MasterCatBreeds)
}

func (s *MasterDataService) GetBloodTypes(ctx context.Context) []string {
	return s.labels(ctx, domain.MasterBloodTypes)
}

func (s *MasterDataService) labels(ctx context.Context, t domain.MasterDataType) []string {
	items, err := s.active(ctx, t)
	if err != nil {
		// คืน slice ว่างแทนการพัง — dropdown ว่างดีกว่าหน้าจอ error
		slog.ErrorContext(ctx, "อ่าน master data ไม่สำเร็จ", "type", t, "error", err)
		return []string{}
	}
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.DisplayLabel()
	}
	return out
}

// List คืนรูปแบบมีโครงสร้างสำหรับ API v2
func (s *MasterDataService) List(ctx context.Context, t domain.MasterDataType) ([]domain.MasterDataItem, error) {
	if !t.Valid() {
		return nil, fmt.Errorf("%w: ไม่รู้จัก master data ชนิด %q", domain.ErrValidation, t)
	}
	return s.active(ctx, t)
}

func (s *MasterDataService) Permissions(ctx context.Context) ([]domain.PetPermission, error) {
	all, err := s.perms.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PetPermission, 0, len(all))
	for _, p := range all {
		if p.IsActive {
			out = append(out, p)
		}
	}
	return out, nil
}

// IsValid ตรวจว่าค่าที่ client ส่งมามีอยู่จริงและยัง active
//
// ใช้แทนการ hardcode enum ในโค้ด เพราะ admin เพิ่มค่าใหม่ผ่าน backoffice ได้
// ค่าที่เพิ่มใหม่ต้องใช้งานได้ทันทีโดยไม่ต้อง deploy
func (s *MasterDataService) IsValid(ctx context.Context, t domain.MasterDataType, code string) bool {
	if strings.TrimSpace(code) == "" {
		return false
	}
	items, err := s.active(ctx, t)
	if err != nil {
		// อ่าน master data ไม่ได้ → ปล่อยผ่าน
		//
		// เลือกทางนี้เพราะ DB สะดุดชั่วคราวไม่ควรทำให้ผู้ใช้บันทึกข้อมูลไม่ได้
		// ความถูกต้องระดับฐานข้อมูลยังมี FK จาก V8 คุมอยู่อีกชั้น
		slog.ErrorContext(ctx, "ตรวจ master data ไม่สำเร็จ ปล่อยผ่าน", "type", t, "error", err)
		return true
	}
	for _, it := range items {
		if it.Code == code || it.DisplayLabel() == code {
			return true
		}
	}
	return false
}

// --- ฝั่ง admin -----------------------------------------------------------

func (s *MasterDataService) ListAll(ctx context.Context, t domain.MasterDataType) ([]domain.MasterDataItem, error) {
	if err := s.authorizeWrite(ctx, t); err != nil {
		return nil, err
	}
	return s.repo.FindAll(ctx, t, true)
}

func (s *MasterDataService) Create(ctx context.Context, t domain.MasterDataType, item domain.MasterDataItem) (*domain.MasterDataItem, error) {
	if err := s.authorizeWrite(ctx, t); err != nil {
		return nil, err
	}
	if actor, ok := domain.ActorFromContext(ctx); ok {
		uid := actor.UserID.String()
		item.CreatedBy = &uid
	}

	created, err := s.repo.Create(ctx, t, item)
	if err != nil {
		return nil, err
	}
	s.invalidate(t)
	slog.InfoContext(ctx, "เพิ่ม master data", "type", t, "code", created.Code)
	return created, nil
}

func (s *MasterDataService) Update(ctx context.Context, t domain.MasterDataType, item domain.MasterDataItem) (*domain.MasterDataItem, error) {
	if err := s.authorizeWrite(ctx, t); err != nil {
		return nil, err
	}
	if actor, ok := domain.ActorFromContext(ctx); ok {
		uid := actor.UserID.String()
		item.UpdatedBy = &uid
	}

	updated, err := s.repo.Update(ctx, t, item)
	if err != nil {
		return nil, err
	}
	s.invalidate(t)
	slog.InfoContext(ctx, "แก้ไข master data", "type", t, "code", updated.Code)
	return updated, nil
}

// Deactivate ปิดการใช้งาน ไม่ลบทิ้ง
//
// 🚫 ไม่มี hard delete โดยตั้งใจ
//
//	ข้อมูลจริงอ้างถึง master ตัวนี้อยู่ (pets.breed, litter_logs.type)
//	การลบจะทำให้ข้อมูลเดิมแสดงผลไม่ได้ และ FK จาก V8 ก็จะปฏิเสธอยู่แล้ว
//
// คืนจำนวนข้อมูลที่ยังอ้างถึง เพื่อให้ UI แจ้งผู้ใช้ได้ว่ากระทบกี่รายการ
func (s *MasterDataService) Deactivate(ctx context.Context, t domain.MasterDataType, code string) (int64, error) {
	if err := s.authorizeWrite(ctx, t); err != nil {
		return 0, err
	}

	current, err := s.repo.FindByCode(ctx, t, code)
	if err != nil {
		return 0, err
	}
	usage, err := s.repo.CountUsage(ctx, t, code)
	if err != nil {
		return 0, err
	}

	current.IsActive = false
	if actor, ok := domain.ActorFromContext(ctx); ok {
		uid := actor.UserID.String()
		current.UpdatedBy = &uid
	}
	if _, err := s.repo.Update(ctx, t, *current); err != nil {
		return 0, err
	}
	s.invalidate(t)
	slog.InfoContext(ctx, "ปิดการใช้งาน master data", "type", t, "code", code, "usage", usage)
	return usage, nil
}

func (s *MasterDataService) UsageCount(ctx context.Context, t domain.MasterDataType, code string) (int64, error) {
	if err := s.authorizeWrite(ctx, t); err != nil {
		return 0, err
	}
	return s.repo.CountUsage(ctx, t, code)
}

func (s *MasterDataService) authorizeWrite(ctx context.Context, t domain.MasterDataType) error {
	if !t.Valid() {
		return fmt.Errorf("%w: ไม่รู้จัก master data ชนิด %q", domain.ErrValidation, t)
	}
	return s.authz.AuthorizeGlobal(ctx, domain.CapMasterDataWrite)
}

// --- cache ---------------------------------------------------------------

func (s *MasterDataService) active(ctx context.Context, t domain.MasterDataType) ([]domain.MasterDataItem, error) {
	s.mu.RLock()
	entry, ok := s.cache[t]
	s.mu.RUnlock()
	if ok && time.Since(entry.loadedAt) < masterDataCacheTTL {
		return entry.items, nil
	}

	items, err := s.repo.FindAll(ctx, t, false)
	if err != nil {
		// ถ้าโหลดไม่ได้แต่มีของเก่าอยู่ ใช้ของเก่าไปก่อน
		// ดีกว่าทำให้ dropdown ว่างเพราะ DB สะดุดชั่วคราว
		if ok {
			return entry.items, nil
		}
		return nil, err
	}

	s.mu.Lock()
	s.cache[t] = cacheEntry{items: items, loadedAt: time.Now()}
	s.mu.Unlock()
	return items, nil
}

// invalidate ล้าง cache ของ replica นี้ทันทีหลัง admin แก้
// replica อื่นจะตามมาภายใน TTL
func (s *MasterDataService) invalidate(t domain.MasterDataType) {
	s.mu.Lock()
	delete(s.cache, t)
	s.mu.Unlock()
}
