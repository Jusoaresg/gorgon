package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jusoaresg/gorgon/config"
	telegramService "github.com/jusoaresg/gorgon/external/telegram/service"
	"github.com/jusoaresg/gorgon/internal/app"
	"github.com/jusoaresg/gorgon/internal/routes"
	"github.com/jusoaresg/gorgon/internal/scheduler"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// @title           Gongon
// @version         0.3
// @description     Show Download Manager API
// @BasePath /api/v1

// @contact.name   Jusoares
// @contact.email  julianosgreg@gmail.com

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html
func main() {
	if err := config.Init(); err != nil {
		panic(fmt.Errorf("failed to initialize config: %w", err))
	}

	e := echo.New()

	cors := middleware.CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}
	e.Use(middleware.CORSWithConfig(cors))
	e.Use(middleware.RequestLogger())

	dependencies := app.NewDependencies()

	routes.InitializeRoutes(e, dependencies)

	// Root context for background processes and workers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize and start unified scheduler manager
	schedManager := scheduler.NewManager(dependencies.DB)
	schedManager.Start(ctx)

	// Start Telegram bot listener with context
	telegramService.StartTelegramBotListener(ctx, dependencies.DB)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		err := e.Start(fmt.Sprintf(":%s", config.Port))
		if err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal("shutting down the server due to error: ", err)
		}
	}()

	sig := <-sigs
	log.Printf("signal received: %s, shutting down gorgon", sig)

	// Step 1: Gracefully stop Echo server (stop accepting new requests)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Printf("error shutting down Echo server: %v", err)
	}

	// Step 2: Signal background workers to stop and wait for them to finish
	log.Println("waiting for background workers to finish in-flight jobs...")
	cancel()
	schedManager.StopWithTimeout(10 * time.Second)

	// Step 3: Cleanly close database connection
	if err := dependencies.DB.Close(); err != nil {
		log.Printf("error closing database: %v", err)
	} else {
		log.Printf("database closed cleanly")
	}
	log.Println("shutdown complete")
}
