package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/vertex/pet-service/internal/adapter/handler/dto"
	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/internal/port"
	"github.com/vertex/pet-service/pkg/apperror"
)

type MasterDataHandler struct {
	useCase port.MasterDataUseCase
	admin   port.MasterDataAdminUseCase
}

func NewMasterDataHandler(uc port.MasterDataUseCase, admin port.MasterDataAdminUseCase) *MasterDataHandler {
	return &MasterDataHandler{useCase: uc, admin: admin}
}

// --- API v1 — ห้ามเปลี่ยน response shape ---------------------------------
//
// สอง endpoint นี้เคยคืน array ของ string ธรรมดา และ client ที่ใช้อยู่คาดหวังแบบนั้น
// การย้าย master data เข้าฐานข้อมูลต้องไม่ทำให้ค่าที่คืนเปลี่ยนแม้แต่ตัวอักษรเดียว
// (มี golden test เฝ้าอยู่)

func (h *MasterDataHandler) GetCatBreeds(c *fiber.Ctx) error {
	setMasterDataCacheHeaders(c)
	return c.JSON(h.useCase.GetCatBreeds(c.UserContext()))
}

func (h *MasterDataHandler) GetBloodTypes(c *fiber.Ctx) error {
	setMasterDataCacheHeaders(c)
	return c.JSON(h.useCase.GetBloodTypes(c.UserContext()))
}

// GetPermissions คืน master permission ของ caregiver
//
// endpoint ใหม่ — เดิม PermissionRepository.FindAll มีอยู่แต่ไม่มีใครเรียก
// ทำให้หน้าตั้งสิทธิ์ที่ backoffice ไม่มีทางรู้ว่ามี permission อะไรให้เลือกบ้าง
func (h *MasterDataHandler) GetPermissions(c *fiber.Ctx) error {
	perms, err := h.useCase.Permissions(c.UserContext())
	if err != nil {
		return apperror.FromDomain(err)
	}
	setMasterDataCacheHeaders(c)
	return c.JSON(perms)
}

// --- API v2 — รูปแบบมีโครงสร้าง -------------------------------------------

func (h *MasterDataHandler) ListV2(c *fiber.Ctx) error {
	t := domain.MasterDataType(c.Params("type"))
	items, err := h.useCase.List(c.UserContext(), t)
	if err != nil {
		return apperror.FromDomain(err)
	}
	setMasterDataCacheHeaders(c)
	return c.JSON(dto.ToMasterDataResponses(items))
}

// --- Admin ----------------------------------------------------------------

func (h *MasterDataHandler) AdminList(c *fiber.Ctx) error {
	t := domain.MasterDataType(c.Params("type"))
	items, err := h.admin.ListAll(c.UserContext(), t)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(dto.ToMasterDataResponses(items))
}

func (h *MasterDataHandler) AdminCreate(c *fiber.Ctx) error {
	t := domain.MasterDataType(c.Params("type"))

	var req dto.CreateMasterDataRequest
	if err := c.BodyParser(&req); err != nil {
		return apperror.BadRequest("Invalid request body", err)
	}
	if err := req.Validate(t); err != nil {
		return apperror.FromDomain(err)
	}

	created, err := h.admin.Create(c.UserContext(), t, req.ToDomain())
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.Status(fiber.StatusCreated).JSON(dto.ToMasterDataResponse(*created))
}

func (h *MasterDataHandler) AdminUpdate(c *fiber.Ctx) error {
	t := domain.MasterDataType(c.Params("type"))

	var req dto.UpdateMasterDataRequest
	if err := c.BodyParser(&req); err != nil {
		return apperror.BadRequest("Invalid request body", err)
	}
	if err := req.Validate(t); err != nil {
		return apperror.FromDomain(err)
	}

	// code มาจาก path เสมอ — เปลี่ยนไม่ได้เพราะข้อมูลอื่นอ้างอยู่
	item := req.ToDomain()
	item.Code = c.Params("code")

	updated, err := h.admin.Update(c.UserContext(), t, item)
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(dto.ToMasterDataResponse(*updated))
}

// AdminDeactivate ปิดการใช้งาน ไม่ลบทิ้ง
//
// ใช้ method DELETE เพื่อให้เข้ากับที่ client คาดหวัง แต่พฤติกรรมคือ soft deactivate
// response บอกจำนวนข้อมูลที่ยังอ้างถึง เพื่อให้ UI แจ้งผู้ใช้ได้
func (h *MasterDataHandler) AdminDeactivate(c *fiber.Ctx) error {
	t := domain.MasterDataType(c.Params("type"))

	usage, err := h.admin.Deactivate(c.UserContext(), t, c.Params("code"))
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(fiber.Map{
		"code":         c.Params("code"),
		"isActive":     false,
		"affectedRows": usage,
		"message":      "ปิดการใช้งานแล้ว ข้อมูลเดิมที่ใช้ค่านี้ยังแสดงผลได้ตามปกติ",
	})
}

// AdminUsage บอกจำนวนข้อมูลที่อ้างถึง — ให้ UI เตือนก่อนกดปิด
func (h *MasterDataHandler) AdminUsage(c *fiber.Ctx) error {
	t := domain.MasterDataType(c.Params("type"))

	n, err := h.admin.UsageCount(c.UserContext(), t, c.Params("code"))
	if err != nil {
		return apperror.FromDomain(err)
	}
	return c.JSON(fiber.Map{"code": c.Params("code"), "usageCount": n})
}

// --- routes ---------------------------------------------------------------

func (h *MasterDataHandler) RegisterRoutes(r fiber.Router) {
	// v1 — รูปแบบเดิม ห้ามเปลี่ยน
	r.Get("/master-data/cat-breeds", h.GetCatBreeds)
	r.Get("/master-data/blood-types", h.GetBloodTypes)
	r.Get("/master-data/pet-permissions", h.GetPermissions)

	// v2 — รูปแบบมีโครงสร้าง ครอบทุกชนิด
	r.Get("/v2/master-data/:type", h.ListV2)
}

func (h *MasterDataHandler) RegisterAdminRoutes(r fiber.Router) {
	r.Get("/master-data/:type", h.AdminList)
	r.Post("/master-data/:type", h.AdminCreate)
	r.Put("/master-data/:type/:code", h.AdminUpdate)
	r.Delete("/master-data/:type/:code", h.AdminDeactivate)
	r.Get("/master-data/:type/:code/usage", h.AdminUsage)
}

// setMasterDataCacheHeaders — master data เปลี่ยนไม่บ่อย ให้ client cache ได้
// ตั้งสั้นพอที่ค่าที่ admin แก้จะไปถึงผู้ใช้ในเวลาที่ยอมรับได้
func setMasterDataCacheHeaders(c *fiber.Ctx) {
	c.Set("Cache-Control", "private, max-age="+strconv.Itoa(60))
}
