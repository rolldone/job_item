package jobitem

import (
	"encoding/json"
	"fmt"
	"job_item/src/helper"
	"job_item/support"
	"net/http"

	"github.com/gin-gonic/gin"
)

func CreateJobHandler(c *gin.Context) {
	var jobRequest struct {
		AppId    string                 `json:"app_id" binding:"required"`
		Event    string                 `json:"event" binding:"required"`
		FormBody map[string]interface{} `json:"form_body" binding:"required"`
	}

	if err := c.ShouldBindJSON(&jobRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}

	conn := support.Helper.BrokerConnection.GetConnection(support.Helper.ConfigYaml.ConfigData.Broker_connection["key"].(string))
	if conn == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "failed to get broker connection"})
		return
	}

	// Generate a UUID for the task_id
	// This should be replaced with your actual UUID generation logic
	uuid, err := helper.GenerateUUIDv7()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "failed to generate UUID"})
		return
	}

	payload := map[string]interface{}{
		"task_id": uuid, // This should be generated or passed in
		"data":    jobRequest.FormBody,
	}

	formBodyString, err := json.Marshal(payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "failed to marshal form body"})
		return
	}

	conn.Pub(fmt.Sprintf("%s.%s", jobRequest.AppId, jobRequest.Event), string(formBodyString))

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Job created successfully!",
		"job":     jobRequest,
		"app_id":  support.Helper.ConfigYaml.ConfigData.Uuid,
	})
}
