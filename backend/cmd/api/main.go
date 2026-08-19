package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"

	"synapseqa/backend/internal/controller"
	"synapseqa/backend/internal/repository"
	"synapseqa/backend/internal/router"
	"synapseqa/backend/internal/service"
)

// main 只负责应用装配：数据库连接、迁移、依赖注入、路由注册和服务启动。
func main() {
	appCtx, stopApp := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopApp()
	dsn := env("DATABASE_URL", "postgres://postgres:postgres@127.0.0.1:5432/synapse_qa?sslmode=disable")
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("连接 PostgreSQL 失败：%v\n请设置 DATABASE_URL，例如 postgres://用户名:密码@127.0.0.1:5432/synapse_qa?sslmode=disable", err)
	}

	bootstrapApp := &app{db: db, jwtSecret: []byte(env("JWT_SECRET", "synapse-local-dev-secret"))}
	if err := bootstrapApp.migrate(ctx); err != nil {
		log.Fatal(err)
	}
	if err := bootstrapApp.seed(ctx); err != nil {
		log.Fatal(err)
	}
	if err := bootstrapApp.migrateLegacyAPIData(ctx); err != nil {
		log.Fatal(err)
	}

	systemRepo := repository.NewSystemRepository(db)
	catalogRepo := repository.NewCatalogRepository(db)
	automationRepo := repository.NewAutomationRepository(db)
	executorRepo := repository.NewExecutorRepository(db)
	testCaseRepo := repository.NewTestCaseRepository(db)
	executionRepo := repository.NewExecutionRepository(db)
	elementCaptureRepo := repository.NewElementCaptureRepository(db)
	notificationRepo := repository.NewNotificationRepository(db)
	apiAutomationRepo := repository.NewAPIAutomationRepository(db)
	aiRepo := repository.NewAIRepository(db)
	performanceRepo := repository.NewPerformanceRepository(db)
	dashboardRepo := repository.NewDashboardRepository(db)

	systemService := service.NewSystemService(systemRepo, bootstrapApp.jwtSecret)
	catalogService := service.NewCatalogService(catalogRepo, systemRepo)
	automationService := service.NewAutomationService(automationRepo, systemRepo)
	executorService := service.NewExecutorService(executorRepo, env("EXECUTOR_SHARED_TOKEN", "synapse-local-executor-token"))
	testCaseService := service.NewTestCaseService(testCaseRepo, systemRepo)
	executionService := service.NewExecutionService(executionRepo, executorRepo, testCaseRepo, systemRepo, env("EXECUTION_CALLBACK_BASE", "http://127.0.0.1:8080"))
	elementCaptureService := service.NewElementCaptureService(elementCaptureRepo, executorRepo, []byte(env("EXECUTOR_SHARED_TOKEN", "synapse-local-executor-token")))
	notificationService := service.NewNotificationService(notificationRepo)
	apiAutomationService := service.NewAPIAutomationService(apiAutomationRepo, systemRepo, bootstrapApp.jwtSecret)
	apiAutomationService.ConfigureDebug(executorRepo, env("EXECUTOR_CALLBACK_BASE", "http://127.0.0.1:8080"))
	dataFactoryService := service.NewDataFactoryService()
	aiToolExecutor := service.NewAIToolExecutor(apiAutomationService, catalogService, testCaseService, executionService, automationService)
	aiService := service.NewAIService(aiRepo, aiToolExecutor)
	performanceService := service.NewPerformanceService(performanceRepo, executorRepo, systemRepo, env("EXECUTION_CALLBACK_BASE", "http://127.0.0.1:8080"))
	dashboardService := service.NewDashboardService(dashboardRepo)
	executionService.SetNotifier(notificationService)
	executorService.SetNotifier(notificationService)
	executionService.StartScheduler(appCtx)
	elementCaptureService.StartScheduler(appCtx)
	performanceService.SetNotifier(notificationService)
	performanceService.StartScheduler(appCtx)

	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery(), ginCORS())
	router.RegisterRoutes(engine, router.Dependencies{
		AuthController:           controller.NewAuthController(systemService),
		SystemController:         controller.NewSystemController(systemService),
		CatalogController:        controller.NewCatalogController(catalogService),
		AutomationController:     controller.NewAutomationController(automationService),
		ExecutorController:       controller.NewExecutorController(executorService),
		TestCaseController:       controller.NewTestCaseController(testCaseService),
		ExecutionController:      controller.NewExecutionController(executionService),
		ElementCaptureController: controller.NewElementCaptureController(elementCaptureService),
		NotificationController:   controller.NewNotificationController(notificationService),
		APIAutomationController:  controller.NewAPIAutomationController(apiAutomationService),
		DataFactoryController:    controller.NewDataFactoryController(dataFactoryService),
		AIController:             controller.NewAIController(aiService),
		PerformanceController:    controller.NewPerformanceController(performanceService),
		DashboardController:      controller.NewDashboardController(dashboardService),
		AuthMiddleware:           controller.AuthMiddleware(systemService),
	})

	addr := env("API_ADDR", "127.0.0.1:8080")
	log.Printf("Synapse QA API 已启动：http://%s", addr)
	server := &http.Server{Addr: addr, Handler: engine}
	go func() {
		<-appCtx.Done()
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancelShutdown()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("API 优雅关闭失败：%v", err)
		}
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// ginCORS 是 Gin 版本的跨域中间件，供 React 开发服务器调用后端 API。
func ginCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, If-Match, X-Request-ID, X-Executor-ID, X-Executor-Token")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
