package handler

import (
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/services"
	"fmt"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"resty.dev/v3"
)

type CronHandler struct {
	App            *core.AppServer
	DB             *gorm.DB
	Task           *cron.Cron
	Client         *resty.Client
	SaveUrlService *services.TbVideoService
}

func NewCronHandler(app *core.AppServer, db *gorm.DB, task *cron.Cron) *CronHandler {
	return &CronHandler{
		App:            app,
		DB:             db,
		Task:           task,
		Client:         resty.New().SetHeader("TransVideoId", "9836C8E8C2EC4F7792345DA661529292"),
		SaveUrlService: services.NewVideoService(db),
	}

}

func (h *CronHandler) runTask() {
	fmt.Println("定时任务执行中....")

}
func (h *CronHandler) SetUp() {
	_, err := h.Task.AddFunc("0/3 * * * * *", h.runTask)
	if err != nil {
		return
	}

	h.Task.Start() // 启动定时任务

}
