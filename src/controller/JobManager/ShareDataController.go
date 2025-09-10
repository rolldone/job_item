package jobmanager

import (
	"fmt"
	"job_item/src/helper"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ShareDataController is a thin HTTP wrapper that delegates to helper share-data store.
type ShareDataController struct{}

func NewShareDataController() *ShareDataController {
	return &ShareDataController{}
}

type AddShareDataRequest struct {
	Key  string `json:"key" binding:"required"`
	Data string `json:"data" binding:"required"`
	TTL  int    `json:"ttl_seconds,omitempty"`
}

// POST /msg/share/data/:task_id
func (c *ShareDataController) AddShareData(ctx *gin.Context) {
	taskID := ctx.Param("task_id")
	var req AddShareDataRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// append to helper store (helper will handle ttl semantics)
	helper.ShareDataAdd(taskID, req.Key, req.Data, req.TTL)

	ctx.JSON(http.StatusOK, gin.H{"status": "ok", "key": req.Key, "task_id": taskID})
}

// GET /msg/share/data/:task_id/:key
func (c *ShareDataController) GetShareData(ctx *gin.Context) {
	taskID := ctx.Param("task_id")
	key := ctx.Param("key")
	fmt.Println("GetShareData called with taskID:", taskID, "key:", key)
	data, err := helper.ShareDataGet(taskID, key)
	if err != nil || len(data) == 0 {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"key":     key,
		"task_id": taskID,
		"data":    data,
	})
}
