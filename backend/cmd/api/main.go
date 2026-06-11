package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
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

	systemRepo := repository.NewSystemRepository(db)
	catalogRepo := repository.NewCatalogRepository(db)
	automationRepo := repository.NewAutomationRepository(db)
	executorRepo := repository.NewExecutorRepository(db)

	systemService := service.NewSystemService(systemRepo, bootstrapApp.jwtSecret)
	catalogService := service.NewCatalogService(catalogRepo, systemRepo)
	automationService := service.NewAutomationService(automationRepo, systemRepo)
	executorService := service.NewExecutorService(executorRepo, env("EXECUTOR_SHARED_TOKEN", "synapse-local-executor-token"))

	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery(), ginCORS())
	router.RegisterRoutes(engine, router.Dependencies{
		AuthController:       controller.NewAuthController(systemService),
		SystemController:     controller.NewSystemController(systemService),
		CatalogController:    controller.NewCatalogController(catalogService),
		AutomationController: controller.NewAutomationController(automationService),
		ExecutorController:   controller.NewExecutorController(executorService),
		AuthMiddleware:       controller.AuthMiddleware(systemService),
	})

	addr := env("API_ADDR", "127.0.0.1:8080")
	log.Printf("Synapse QA API 已启动：http://%s", addr)
	log.Fatal(engine.Run(addr))
}

// ginCORS 是 Gin 版本的跨域中间件，供 React 开发服务器调用后端 API。
func ginCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
