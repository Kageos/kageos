package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kageos/kageos/core/app-server/service"
	"github.com/kageos/kageos/pkg/contextx"
	"github.com/kageos/kageos/pkg/ginx/response"
)

type LogArchive struct{ service *service.LogArchiveService }

func NewLogArchive(archiveService *service.LogArchiveService) *LogArchive {
	return &LogArchive{service: archiveService}
}

func (h *LogArchive) List(c *gin.Context) {
	if contextx.GetRequestUser(c) != service.SystemUsername {
		response.FailWithMessage(c, "仅 system 超管可查看日志归档")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	rows, total, err := h.service.List(contextx.ToContext(c), page, pageSize)
	if err != nil {
		response.FailWithMessage(c, "查询日志归档失败: "+err.Error())
		return
	}
	cfg := h.service.Config()
	response.OkWithData(c, gin.H{"list": rows, "total": total, "retention_days": cfg.RetentionDays, "cron_expr": cfg.CronExpr, "timezone": cfg.Timezone})
}

func (h *LogArchive) Retry(c *gin.Context) {
	if contextx.GetRequestUser(c) != service.SystemUsername {
		response.FailWithMessage(c, "仅 system 超管可重试日志归档")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.FailWithMessage(c, "无效的归档批次")
		return
	}
	if err := h.service.Retry(contextx.ToContext(c), id); err != nil {
		response.FailWithMessage(c, "重试归档未完成，可从已保存的阶段继续: "+err.Error())
		return
	}
	response.OkWithData(c, gin.H{"id": id})
}
