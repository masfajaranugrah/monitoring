package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"monitoring/internal/config"
	"monitoring/internal/crypto"
	"monitoring/internal/database"
	"monitoring/internal/handlers"
	"monitoring/internal/middleware"
	"monitoring/internal/ping"
	"monitoring/internal/sse"
	"monitoring/internal/store"
	"monitoring/internal/vpn"
)

func main() {
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- Infrastructure setup ---
	if err := database.Connect(cfg.DatabaseURL); err != nil {
		log.Fatalf("[fatal] database connection failed: %v", err)
	}
	defer database.Close()

	if err := database.RunMigrations(ctx); err != nil {
		log.Fatalf("[fatal] migrations failed: %v", err)
	}
	if err := database.SeedAdmin(ctx); err != nil {
		log.Fatalf("[fatal] admin seed failed: %v", err)
	}

	if err := crypto.Init(); err != nil {
		log.Fatalf("[fatal] crypto init failed: %v", err)
	}
	middleware.InitJWT(cfg.JWTSecret)

	// --- Realtime hub ---
	hub := sse.NewHub()

	// --- Monitoring engine ---
	storer := store.New()
	vpnManager := vpn.NewManager()
	engine := ping.NewMonitorEngine(storer, hub, cfg.PingConcurrency, vpnManager.EnsureRoute)
	engine.Start(ctx)

	// Handlers use the engine to nudge reloads after customer mutations.
	handlers.MonitorEngine = engine

	// --- Background retention ---
	go store.RetentionWorker(ctx)

	// --- VPN manager ---
	vpnHandler := handlers.NewVPNHandler(vpnManager)

	// --- Auto-connect semua VPN aktif saat server menyala ---
	go vpnHandler.AutoConnectActiveVPNs(ctx)

	// --- Gin router ---
	gin.SetMode(releaseMode())
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"time":    time.Now(),
			"monitor": engine.IsRunning(),
		})
	})

	r.Static("/assets", "./web/assets")
	r.StaticFile("/favicon.svg", "./web/favicon.svg")
	r.NoRoute(func(c *gin.Context) {
		// SPA fallback for the built frontend.
		if c.Request.Method != "GET" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if c.Request.URL.Path == "/api/events" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		path := "./web/index.html"
		c.File(path)
	})

	api := r.Group("/api")
	{
		// Public
		api.POST("/auth/login", handlers.Login)
		api.GET("/events", hub.Stream)

		auth := api.Group("", middleware.AuthMiddleware())
		{
			auth.GET("/auth/me", handlers.Me)
			auth.POST("/auth/change-password", handlers.ChangePassword)

			auth.GET("/dashboard/stats", handlers.GetDashboardStats)
			auth.GET("/map/customers", handlers.MapCustomers)
			auth.GET("/system/stats", handlers.GetSystemStats)

			// Customers
			auth.GET("/customers", handlers.ListCustomers)
			auth.POST("/customers", handlers.CreateCustomer)
			auth.GET("/customers/:id", handlers.GetCustomer)
			auth.PUT("/customers/:id", handlers.UpdateCustomer)
			auth.DELETE("/customers/:id", handlers.DeleteCustomer)
			auth.PATCH("/customers/:id/monitoring", handlers.ToggleMonitoring)

			// Ping history & status logs
			auth.GET("/customers/:id/ping-history", handlers.PingHistory)
			auth.GET("/customers/:id/status-logs", handlers.StatusLogs)

			// VPNs
			auth.GET("/vpn", vpnHandler.List)
			auth.GET("/vpn/:id", vpnHandler.Get)
			auth.POST("/vpn", middleware.AdminOnly(), vpnHandler.Create)
			auth.PUT("/vpn/:id", middleware.AdminOnly(), vpnHandler.Update)
			auth.DELETE("/vpn/:id", middleware.AdminOnly(), vpnHandler.Delete)
			auth.PATCH("/vpn/:id/active", middleware.AdminOnly(), vpnHandler.SetActive)
			auth.POST("/vpn/:id/test", vpnHandler.TestConnection)
			auth.POST("/vpn/:id/connect", middleware.AdminOnly(), vpnHandler.ConnectVPN)
			auth.POST("/vpn/:id/disconnect", middleware.AdminOnly(), vpnHandler.DisconnectVPN)
			auth.POST("/vpn/refresh", middleware.AdminOnly(), vpnHandler.RefreshStatus)

			// Alerts
			auth.GET("/alerts", handlers.AlertsList)
			auth.PATCH("/alerts/:id/read", handlers.AlertMarkRead)

			// Web terminal (admin only)
			auth.GET("/terminal/ws", middleware.AdminOnly(), handlers.TerminalWS)
		}
	}

	// Ensure the monitoring engine is exercised even if the VPN manager is idle.

	srv := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	go func() {
		log.Printf("monitoring API listening on :%s", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[fatal] server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	engine.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}

func releaseMode() string {
	if os.Getenv("GIN_MODE") == "release" {
		return gin.ReleaseMode
	}
	return gin.DebugMode
}
