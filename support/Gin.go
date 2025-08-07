package support

import (
	"fmt"
	"time"

	"net"

	"github.com/gin-gonic/gin"
	"github.com/zishang520/engine.io/v2/log"
	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/socket.io/v2/socket"
)

// Import net

// Post data message handler
type MsgAddRequest struct {
	Msg string `json:"msg" binding:"required"`
}

func GinConstruct() *GinSupport {
	ginSupport := GinSupport{}
	ginSupport.StartBaseRoute()
	// Wait briefly to ensure the goroutine has started and port is set
	time.Sleep(100 * time.Millisecond)
	Helper.PrintGroupName("Web server started localhost with port: " + fmt.Sprintf("%d", ginSupport.GetPort()))
	return &ginSupport
}

type GinSupport struct {
	Router *gin.Engine
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

func (g *GinSupport) StartSocket() {
	log.DEBUG = true
	c := socket.DefaultServerOptions()
	c.SetServeClient(true)
	// c.SetConnectionStateRecovery(&socket.ConnectionStateRecovery{})
	// c.SetAllowEIO3(true)
	c.SetPingInterval(300 * time.Millisecond)
	c.SetPingTimeout(200 * time.Millisecond)
	c.SetMaxHttpBufferSize(1000000)
	c.SetConnectTimeout(1000 * time.Millisecond)
	c.SetTransports(types.NewSet("polling", "webtransport"))
	c.SetCors(&types.Cors{
		Origin:      "*",
		Credentials: true,
	})
	socketio := socket.NewServer(nil, nil)
	socketio.On("connection", func(clients ...interface{}) {
		client := clients[0].(*socket.Socket)

		client.On("message", func(args ...interface{}) {
			client.Emit("message-back", args...)
		})
		client.Emit("auth", client.Handshake().Auth)

		client.On("message-with-ack", func(args ...interface{}) {
			ack := args[len(args)-1].(socket.Ack)
			ack(args[:len(args)-1], nil)
		})
	})

	socketio.Of("/custom", nil).On("connection", func(clients ...interface{}) {
		client := clients[0].(*socket.Socket)
		client.Emit("auth", client.Handshake().Auth)
	})

}

func (g *GinSupport) StartBaseRoute() {

	// Set Gin to release mode for production
	gin.SetMode(gin.ReleaseMode)

	// Initialize Gin's default router.
	g.Router = gin.Default()

	// Display the gin logger.
	g.Router.Use(gin.Logger())

	// Define a route handler.
	g.Router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Hello, Gin!",
		})
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
		if err := g.Router.RunListener(listener); err != nil {
			panic(err)
		}
	}()
}
