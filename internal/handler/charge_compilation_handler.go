package handler

import (
	"net/http"
	"strconv"

	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/services"
	"github.com/difyz9/ytb2bili/pkg/store/model"
	"github.com/gin-gonic/gin"
)

type ChargeCompilationHandler struct {
	BaseHandler
	Service *services.ChargeCompilationService
}

func NewChargeCompilationHandler(app *core.AppServer, service *services.ChargeCompilationService) *ChargeCompilationHandler {
	return &ChargeCompilationHandler{
		BaseHandler: BaseHandler{App: app},
		Service:     service,
	}
}

func (h *ChargeCompilationHandler) RegisterRoutes(api *gin.RouterGroup) {
	charge := api.Group("/charge")
	charge.GET("/pools/summary", h.poolSummary)
	charge.GET("/pools", h.listPool)
	charge.GET("/batches", h.listBatches)
	charge.GET("/batches/:id", h.getBatch)
	charge.POST("/batches/draft", h.createDraft)
	charge.POST("/batches/:id/reroll", h.rerollDraft)
	charge.PUT("/batches/:id/order", h.reorderDraft)
	charge.POST("/batches/:id/start", h.startBatch)
	charge.POST("/batches/:id/cancel", h.cancelBatch)
	charge.POST("/batches/:id/retry", h.retryBatch)
}

func (h *ChargeCompilationHandler) poolSummary(c *gin.Context) {
	summary, err := h.Service.PoolSummary()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "读取充电素材池统计失败", err)
		return
	}
	h.respondOK(c, summary)
}

func (h *ChargeCompilationHandler) listPool(c *gin.Context) {
	tier, err := strconv.Atoi(c.Query("tier"))
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "档位只能是30或50", err)
		return
	}
	items, err := h.Service.ListPool(tier, c.Query("state"))
	if err != nil {
		h.respondError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	h.respondOK(c, items)
}

func (h *ChargeCompilationHandler) listBatches(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	batches, err := h.Service.ListBatches(limit)
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "读取拼接批次失败", err)
		return
	}
	h.respondOK(c, batches)
}

func (h *ChargeCompilationHandler) getBatch(c *gin.Context) {
	id, ok := h.batchID(c)
	if !ok {
		return
	}
	batch, err := h.Service.GetBatch(id)
	if err != nil {
		h.respondError(c, http.StatusNotFound, "拼接批次不存在", err)
		return
	}
	h.respondOK(c, batch)
}

func (h *ChargeCompilationHandler) createDraft(c *gin.Context) {
	var request struct {
		Tier int `json:"tier" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		h.respondError(c, http.StatusBadRequest, "请选择30元或50元素材池", err)
		return
	}
	batch, err := h.Service.CreateDraft(request.Tier)
	if err != nil {
		h.respondError(c, http.StatusConflict, err.Error(), err)
		return
	}
	h.respondCreated(c, batch)
}

func (h *ChargeCompilationHandler) rerollDraft(c *gin.Context) {
	id, ok := h.batchID(c)
	if !ok {
		return
	}
	batch, err := h.Service.GetBatch(id)
	if err != nil {
		h.respondError(c, http.StatusNotFound, "拼接批次不存在", err)
		return
	}
	if batch.State != model.CompilationStateDraft {
		h.respondError(c, http.StatusConflict, "只有草稿批次可以重新随机", nil)
		return
	}
	if err := h.Service.CancelBatch(batch.ID); err != nil {
		h.respondError(c, http.StatusConflict, err.Error(), err)
		return
	}
	next, err := h.Service.CreateDraft(batch.Tier)
	if err != nil {
		h.respondError(c, http.StatusConflict, err.Error(), err)
		return
	}
	h.respondOK(c, next)
}

func (h *ChargeCompilationHandler) reorderDraft(c *gin.Context) {
	id, ok := h.batchID(c)
	if !ok {
		return
	}
	var request struct {
		SourceVideoIDs []uint `json:"source_video_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		h.respondError(c, http.StatusBadRequest, "请提交完整素材顺序", err)
		return
	}
	batch, err := h.Service.ReorderDraft(id, request.SourceVideoIDs)
	if err != nil {
		h.respondError(c, http.StatusConflict, err.Error(), err)
		return
	}
	h.respondOK(c, batch)
}

func (h *ChargeCompilationHandler) startBatch(c *gin.Context) {
	h.startOrRetry(c, false)
}

func (h *ChargeCompilationHandler) retryBatch(c *gin.Context) {
	h.startOrRetry(c, true)
}

func (h *ChargeCompilationHandler) startOrRetry(c *gin.Context, retry bool) {
	id, ok := h.batchID(c)
	if !ok {
		return
	}
	var request struct {
		PreviewSeconds int    `json:"preview_seconds"`
		UploadPolicy   string `json:"upload_policy"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		h.respondError(c, http.StatusBadRequest, "请设置试看时间和上传策略", err)
		return
	}
	if request.UploadPolicy == "" {
		request.UploadPolicy = model.UploadPolicyImmediate
	}
	batch, err := h.Service.StartBatch(id, request.PreviewSeconds, request.UploadPolicy)
	if err != nil {
		h.respondError(c, http.StatusConflict, err.Error(), err)
		return
	}
	message := "拼接处理任务已进入队列"
	if retry {
		message = "批次重试任务已进入队列"
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": message, "data": batch})
}

func (h *ChargeCompilationHandler) cancelBatch(c *gin.Context) {
	id, ok := h.batchID(c)
	if !ok {
		return
	}
	if err := h.Service.CancelBatch(id); err != nil {
		h.respondError(c, http.StatusConflict, err.Error(), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "批次已取消，素材已释放"})
}

func (h *ChargeCompilationHandler) batchID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "无效的批次ID", err)
		return 0, false
	}
	return uint(value), true
}

func (h *ChargeCompilationHandler) respondOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": data})
}

func (h *ChargeCompilationHandler) respondCreated(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, gin.H{"code": 201, "message": "拼接草稿已创建", "data": data})
}

func (h *ChargeCompilationHandler) respondError(c *gin.Context, status int, message string, err error) {
	if err != nil {
		h.App.Logger.Warnf("%s: %v", message, err)
	}
	c.JSON(status, gin.H{"code": status, "message": message})
}
