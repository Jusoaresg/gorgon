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
	"github.com/jusoaresg/gorgon/internal/scheduler/cron"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// @title           Gongon
// @version         0.2
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
	cron.StartDailyUpdate(scheduler.UpdateAllShows)

	// Initialize Crons, Schedulers and Listeners
	scheduler.Start()
	cron.StartVerifyEpisodeWasDeleted(scheduler.VerifyEpisodeWasDeleted)
	cron.StartDailySummaryCron(dependencies.DB)
	telegramService.StartTelegramBotListener(dependencies.DB)

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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(ctx); err != nil {
		log.Printf("error shutting down Echo server: %v", err)
	}

	if err := dependencies.DB.Close(); err != nil {
		log.Printf("error closing database: %v", err)
	} else {
		log.Printf("database closed cleanly")
	}
	log.Println("shutdown complete")
}
