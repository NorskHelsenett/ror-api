package statuspage

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

//go:embed templates/index.html
var templateFS embed.FS

// Run starts the status page server.
func Run() {
	namespace := os.Getenv("STATUS_NAMESPACE")
	if namespace == "" {
		namespace = "nhn-ror"
	}
	port := os.Getenv("STATUS_PORT")
	if port == "" {
		port = "8080"
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	hub := NewSSEHub()

	watcher, err := NewWatcher(namespace, hub)
	if err != nil {
		log.Fatalf("failed to create watcher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go watcher.Start(ctx)

	// Routes
	router.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	router.GET("/events", hub.HandleSSE(watcher.CurrentSnapshot))

	router.GET("/", func(c *gin.Context) {
		data, err := templateFS.ReadFile("templates/index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "template error")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("statuspage: listening on :%s (namespace: %s)", port, namespace)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("statuspage: shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}
