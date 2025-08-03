package support

import (
	"fmt"
	"net/http"
	"time"

	"encoding/json"
	"net"

	"github.com/gin-gonic/gin"
)

// Import net

func GinConstruct() *GinSupport {
	ginSupport := GinSupport{}
	ginSupport.StartBaseRoute()
	// Wait briefly to ensure the goroutine has started and port is set
	time.Sleep(100 * time.Millisecond)
	Helper.PrintGroupName("Web server started localhost with port: " + fmt.Sprintf("%d", ginSupport.GetPort()))
	return &ginSupport
}

type GinSupport struct {
	router *gin.Engine
	Port   int
	// ...existing code...
}

// GetPort returns the port Gin is running on
func (g *GinSupport) GetPort() int {
	return g.Port
}

func (c *GinSupport) GetObject() any {
	return c
}

func (g *GinSupport) StartBaseRoute() {

	// Set Gin to release mode for production
	gin.SetMode(gin.ReleaseMode)

	// Initialize Gin's default router.
	g.router = gin.Default()

	// Display the gin logger.
	g.router.Use(gin.Logger())

	// Define a route handler.
	g.router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Hello, Gin!",
		})
	})

	// Post data message handler
	type MsgAddRequest struct {
		Msg string `json:"msg" binding:"required"`
	}

	g.router.POST("/msg/notif/:task_id", func(c *gin.Context) {
		task_id := c.Param("task_id")
		var req MsgAddRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if len(req.Msg) > 102400 { // Check if the message length exceeds 100KB
			c.JSON(http.StatusBadRequest, gin.H{"error": "msg exceeds 100KB limit"})
			return
		}

		currentConnection := Helper.ConfigYaml.ConfigData.Broker_connection
		conn := Helper.BrokerConnection.GetConnection(currentConnection["key"].(string))
		if conn == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get broker connection"})
			return
		}

		payload := map[string]string{
			"msg":        req.Msg,
			"created_at": time.Now().Format(time.RFC3339),
			"status":     GetStatus().STATUS_PROCESS,
		}
		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to marshal payload"})
			return
		}
		conn.Pub(fmt.Sprint(task_id, ".", "notif_add"), string(jsonBytes))

		// Process the validated data
		c.JSON(http.StatusOK, gin.H{"status": "success", "return": req})
	})

	go func() {
		// Create a TCP listener on port 0 (random free port)
		listener, err := net.Listen("tcp", ":0") // Bind to localhost only
		if err != nil {
			panic(err)
		}
		// Get and store the actual port in GinSupport struct
		g.Port = listener.Addr().(*net.TCPAddr).Port

		// Start Gin using the listener
		if err := g.router.RunListener(listener); err != nil {
			panic(err)
		}
	}()

}
