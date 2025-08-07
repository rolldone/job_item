package jobmanager

import (
	"encoding/json"
	"fmt"
	"job_item/support"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type MsgNotifController struct {
	// Add fields as necessary for the controller
}

// NewMsgNotifController creates a new instance of MsgNotifController.
func NewMsgNotifController() *MsgNotifController {
	return &MsgNotifController{}
}

// AddNotif adds a message to the notification system.
func (c *MsgNotifController) AddNotif(ctx *gin.Context) {
	task_id := ctx.Param("task_id")
	var req support.MsgAddRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Msg) > 102400 { // Check if the message length exceeds 100KB
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "msg exceeds 100KB limit"})
		return
	}

	currentConnection := support.Helper.ConfigYaml.ConfigData.Broker_connection
	conn := support.Helper.BrokerConnection.GetConnection(currentConnection["key"].(string))
	if conn == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get broker connection"})
		return
	}

	payload := map[string]string{
		"msg":        req.Msg,
		"created_at": time.Now().Format(time.RFC3339),
		"status":     support.GetStatus().STATUS_PROCESS,
	}
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to marshal payload"})
		return
	}
	conn.Pub(fmt.Sprint(task_id, ".", "notif_add"), string(jsonBytes))

	// Process the validated data
	ctx.JSON(http.StatusOK, gin.H{"status": "success", "return": req})
}
