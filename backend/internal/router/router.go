package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/controller"
)

type Dependencies struct {
	AuthController           *controller.AuthController
	SystemController         *controller.SystemController
	CatalogController        *controller.CatalogController
	AutomationController     *controller.AutomationController
	ExecutorController       *controller.ExecutorController
	TestCaseController       *controller.TestCaseController
	ExecutionController      *controller.ExecutionController
	ElementCaptureController *controller.ElementCaptureController
	NotificationController   *controller.NotificationController
	APIAutomationController  *controller.APIAutomationController
	DataFactoryController    *controller.DataFactoryController
	AIController             *controller.AIController
	PerformanceController    *controller.PerformanceController
	AuthMiddleware           gin.HandlerFunc
}

// RegisterRoutes 是唯一的路由注册入口。
func RegisterRoutes(engine *gin.Engine, deps Dependencies) {
	api := engine.Group("/api")
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": "ok"}})
	})
	api.POST("/executors/register", deps.ExecutorController.Register)
	api.POST("/executors/heartbeat", deps.ExecutorController.Heartbeat)
	executorCapture := api.Group("/executor/element-capture")
	executorCapture.POST("/:id/heartbeat", deps.ElementCaptureController.Heartbeat)
	executorCapture.POST("/:id/candidates", deps.ElementCaptureController.AddCandidate)
	executorCapture.POST("/:id/fail", deps.ElementCaptureController.FailSession)
	executorCapture.GET("/commands", deps.ElementCaptureController.ListCommands)
	executorCapture.POST("/commands/:id/ack", deps.ElementCaptureController.AckCommand)
	// 执行器回调使用任务 ID 作为一次性关联凭据，不依赖用户登录态。
	api.POST("/executions/tasks/:taskId/callback", deps.ExecutionController.Callback)
	api.POST("/api-automation/debug/:taskId/callback", deps.APIAutomationController.DebugCallback)
	api.POST("/api-automation/debug/:taskId/events/callback", deps.APIAutomationController.DebugEventCallback)
	api.POST("/api-automation/test-runs/tasks/:taskId/callback", deps.APIAutomationController.TestRunCallback)
	api.POST("/auth/login", deps.AuthController.Login)
	api.POST("/auth/register", deps.AuthController.Register)
	// 候选项响应始终先设置 no-store，连 JWT 认证失败响应也不例外。
	captureNoStoreAuthed := api.Group("", captureNoStore(), deps.AuthMiddleware)
	registerCaptureCandidateRoutes(captureNoStoreAuthed, deps)

	authed := api.Group("", deps.AuthMiddleware)
	authed.GET("/auth/me", deps.AuthController.Me)
	authed.POST("/auth/change-password", deps.AuthController.ChangePassword)
	authed.GET("/executors", deps.ExecutorController.List)
	authed.POST("/executors", deps.ExecutorController.Create)
	authed.POST("/executors/:executorId/token", deps.ExecutorController.GenerateToken)

	registerSystemRoutes(authed, deps)
	registerConfigRoutes(authed, deps)
	registerUIRoutes(authed, deps)
	registerTestCaseRoutes(authed, deps)
	registerExecutionRoutes(authed, deps)
	registerNotificationRoutes(authed, deps)
	registerAPIAutomationRoutes(authed, deps)
	registerDataFactoryRoutes(authed, deps)
	registerAIRoutes(authed, deps)
	registerPerformanceRoutes(authed, deps)
}

func registerDataFactoryRoutes(authed *gin.RouterGroup, deps Dependencies) {
	group := authed.Group("/data-factory")
	group.GET("/generators", deps.DataFactoryController.ListGenerators)
	group.POST("/generators/preview", deps.DataFactoryController.Preview)
}

func registerPerformanceRoutes(authed *gin.RouterGroup, deps Dependencies) {
	authed.GET("/perf/plans", deps.PerformanceController.ListPlans)
	authed.GET("/perf/plans/:id", deps.PerformanceController.GetPlan)
	authed.POST("/perf/plans", deps.PerformanceController.CreatePlan)
	authed.PATCH("/perf/plans/:id", deps.PerformanceController.UpdatePlan)
	authed.DELETE("/perf/plans/:id", deps.PerformanceController.DeletePlan)
	authed.POST("/perf/plans/:id/run", deps.PerformanceController.RunPlan)
	authed.GET("/perf/runs", deps.PerformanceController.ListRuns)
	authed.GET("/perf/runs/:id", deps.PerformanceController.GetRun)
}

func registerAIRoutes(authed *gin.RouterGroup, deps Dependencies) {
	group := authed.Group("/ai")
	group.GET("/config", deps.AIController.GetConfig)
	group.POST("/models", deps.AIController.SaveModel)
	group.DELETE("/models/:id", deps.AIController.DeleteModel)
	group.POST("/test-connection", deps.AIController.TestConnection)
	group.POST("/preferences", deps.AIController.SavePreferences)
	group.POST("/chat", deps.AIController.Chat)
}

func registerAPIAutomationRoutes(authed *gin.RouterGroup, deps Dependencies) {
	group := authed.Group("/api-automation")
	group.GET("/interfaces", deps.APIAutomationController.ListInterfaces)
	group.POST("/interfaces", deps.APIAutomationController.CreateInterface)
	group.GET("/interfaces/:id", deps.APIAutomationController.GetInterface)
	group.PATCH("/interfaces/:id", deps.APIAutomationController.UpdateInterface)
	group.PATCH("/interfaces/:id/configuration", deps.APIAutomationController.UpdateInterfaceConfiguration)
	group.DELETE("/interfaces/:id", deps.APIAutomationController.DeleteInterface)
	group.POST("/interfaces/:id/restore", deps.APIAutomationController.RestoreInterface)
	group.POST("/interfaces/batch-delete", deps.APIAutomationController.BatchDeleteInterfaces)
	group.POST("/interfaces/batch-status", deps.APIAutomationController.BatchUpdateInterfaceStatus)
	group.POST("/interfaces/batch-move", deps.APIAutomationController.BatchMoveInterfaces)
	group.POST("/interfaces/:id/preview", deps.APIAutomationController.PreviewRequest)
	group.POST("/interfaces/:id/debug", deps.APIAutomationController.StartDebug)
	group.GET("/interfaces/:id/debug-runs", deps.APIAutomationController.ListInterfaceDebugRuns)
	group.GET("/interfaces/:id/versions", deps.APIAutomationController.ListInterfaceVersions)
	group.GET("/interfaces/:id/versions/:version", deps.APIAutomationController.GetInterfaceVersion)
	group.GET("/interfaces/:id/versions/:version/diff", deps.APIAutomationController.DiffInterfaceVersions)
	group.POST("/interfaces/:id/versions/:version/restore", deps.APIAutomationController.RestoreInterfaceVersion)
	group.POST("/interfaces/:id/curl", deps.APIAutomationController.ExportCurl)
	group.POST("/curl/parse", deps.APIAutomationController.ParseCurl)
	group.POST("/temp-files", deps.APIAutomationController.UploadTempFile)
	group.DELETE("/temp-files/:id", deps.APIAutomationController.DeleteTempFile)
	group.GET("/debug/:taskId", deps.APIAutomationController.GetDebug)
	group.GET("/debug/:taskId/events", deps.APIAutomationController.ListDebugEvents)
	group.GET("/debug/:taskId/events/stream", deps.APIAutomationController.StreamDebugEvents)
	group.POST("/debug/:taskId/cancel", deps.APIAutomationController.CancelDebug)
	group.GET("/debug-runs/:id", deps.APIAutomationController.GetDebugRunDetail)
	group.GET("/project-headers", deps.APIAutomationController.ListProjectHeaders)
	group.POST("/project-headers", deps.APIAutomationController.CreateProjectHeader)
	group.PATCH("/project-headers/:id", deps.APIAutomationController.UpdateProjectHeader)
	group.DELETE("/project-headers/:id", deps.APIAutomationController.DeleteProjectHeader)
	group.GET("/global-variables", deps.APIAutomationController.ListGlobalVariables)
	group.POST("/global-variables", deps.APIAutomationController.CreateGlobalVariable)
	group.PATCH("/global-variables/:id", deps.APIAutomationController.UpdateGlobalVariable)
	group.DELETE("/global-variables/:id", deps.APIAutomationController.DeleteGlobalVariable)
	group.GET("/test-cases", deps.APIAutomationController.ListTestCases)
	group.POST("/test-cases", deps.APIAutomationController.CreateTestCase)
	group.GET("/test-cases/:id", deps.APIAutomationController.GetTestCase)
	group.PATCH("/test-cases/:id", deps.APIAutomationController.UpdateTestCase)
	group.DELETE("/test-cases/:id", deps.APIAutomationController.DeleteTestCase)
	group.POST("/test-cases/:id/validate", deps.APIAutomationController.ValidateTestCase)
	group.POST("/test-cases/:id/publish", deps.APIAutomationController.PublishTestCase)
	group.GET("/test-cases/:id/versions", deps.APIAutomationController.ListTestCaseVersions)
	group.GET("/test-cases/:id/versions/:version", deps.APIAutomationController.GetTestCaseVersion)
	group.POST("/test-runs", deps.APIAutomationController.StartTestRun)
	group.GET("/test-runs/:batchId", deps.APIAutomationController.GetTestRun)
}

func registerNotificationRoutes(authed *gin.RouterGroup, deps Dependencies) {
	authed.GET("/notifications", deps.NotificationController.List)
	authed.GET("/notifications/unread-count", deps.NotificationController.Count)
	authed.GET("/notifications/stream", deps.NotificationController.Stream)
	authed.PATCH("/notifications/:id/read", deps.NotificationController.Read)
	authed.POST("/notifications/read-all", deps.NotificationController.ReadAll)
	authed.DELETE("/notifications/:id", deps.NotificationController.Delete)
	authed.GET("/notifications/preferences", deps.NotificationController.Preferences)
	authed.PATCH("/notifications/preferences", deps.NotificationController.UpdatePreferences)
	authed.POST("/notifications/system", deps.NotificationController.System)
}

func registerExecutionRoutes(authed *gin.RouterGroup, deps Dependencies) {
	authed.GET("/executions", deps.ExecutionController.List)
	authed.GET("/executions/statistics", deps.ExecutionController.Statistics)
	authed.POST("/executions", deps.ExecutionController.Create)
	authed.POST("/executions/debug", deps.ExecutionController.StartDebug)
	authed.GET("/executions/debug/:taskId", deps.ExecutionController.GetDebug)
	authed.GET("/executions/:id", deps.ExecutionController.Get)
	authed.POST("/executions/:id/cancel", deps.ExecutionController.Cancel)
	authed.GET("/executions/tasks/:taskId/logs", deps.ExecutionController.ListTaskLogs)
}

func registerSystemRoutes(authed *gin.RouterGroup, deps Dependencies) {
	authed.GET("/system/overview", controller.RequirePermission("system.overview.read"), deps.SystemController.Overview)
	authed.GET("/system/users", controller.RequirePermission("system.user.read"), deps.SystemController.ListUsers)
	authed.POST("/system/users", controller.RequirePermission("system.user.manage"), deps.SystemController.CreateUser)
	authed.PATCH("/system/users/:id", controller.RequirePermission("system.user.manage"), deps.SystemController.UpdateUser)
	authed.POST("/system/users/:id/unlock", controller.RequirePermission("system.user.manage"), deps.SystemController.UnlockUser)
	authed.POST("/system/users/:id/reset-password", controller.RequirePermission("system.user.manage"), deps.SystemController.ResetUserPassword)
	authed.DELETE("/system/users/:id", controller.RequirePermission("system.user.manage"), deps.SystemController.DeleteUser)
	authed.GET("/system/roles", controller.RequirePermission("system.role.read"), deps.SystemController.ListRoles)
	authed.GET("/system/permissions", controller.RequirePermission("system.role.read"), deps.SystemController.ListPermissions)
	authed.POST("/system/roles", controller.RequirePermission("system.role.manage"), deps.SystemController.CreateRole)
	authed.PATCH("/system/roles/:id", controller.RequirePermission("system.role.manage"), deps.SystemController.UpdateRole)
	authed.DELETE("/system/roles/:id", controller.RequirePermission("system.role.manage"), deps.SystemController.DeleteRole)
	authed.GET("/system/menus", controller.RequirePermission("system.role.read"), deps.CatalogController.ListMenus)
	authed.GET("/system/dictionaries", controller.RequirePermission("system.settings.read"), deps.CatalogController.ListDictionaries)
	authed.GET("/system/logs", controller.RequirePermission("system.audit.read"), deps.SystemController.ListOperationLogs)
	authed.GET("/system/logs/export", controller.RequirePermission("system.audit.export"), deps.SystemController.ExportOperationLogs)
	authed.GET("/system/settings", controller.RequirePermission("system.settings.read"), deps.SystemController.ListSettings)
	authed.PATCH("/system/settings/:groupKey", controller.RequirePermission("system.settings.manage"), deps.SystemController.UpdateSettings)
	authed.GET("/system/settings/:groupKey/history", controller.RequirePermission("system.settings.read"), deps.SystemController.ListSettingHistory)
	authed.POST("/system/settings/:groupKey/rollback", controller.RequirePermission("system.settings.manage"), deps.SystemController.RollbackSettings)
}

func registerConfigRoutes(authed *gin.RouterGroup, deps Dependencies) {
	authed.GET("/config/executor-token", deps.CatalogController.GetExecutorToken)
	authed.POST("/config/executor-token/generate", deps.CatalogController.GenerateExecutorToken)
	authed.GET("/config/projects", deps.CatalogController.ListProjects)
	authed.POST("/config/projects", deps.CatalogController.CreateProject)
	authed.PATCH("/config/projects/:id", deps.CatalogController.UpdateProject)
	authed.DELETE("/config/projects/:id", deps.CatalogController.DeleteProject)
	authed.GET("/config/products", deps.CatalogController.ListProducts)
	authed.POST("/config/products", deps.CatalogController.CreateProduct)
	authed.PATCH("/config/products/:id", deps.CatalogController.UpdateProduct)
	authed.DELETE("/config/products/:id", deps.CatalogController.DeleteProduct)
	authed.GET("/config/products/:id/stats", deps.CatalogController.ProductStats)
	authed.POST("/config/products/:id/copy", deps.CatalogController.CopyProduct)
	authed.GET("/config/product-modules", deps.CatalogController.ListProductModules)
	authed.POST("/config/product-modules", deps.CatalogController.CreateProductModule)
	authed.PATCH("/config/product-modules/:id", deps.CatalogController.UpdateProductModule)
	authed.DELETE("/config/product-modules/:id", deps.CatalogController.DeleteProductModule)
	authed.GET("/config/test-objects", deps.AutomationController.ListTestObjects)
	authed.POST("/config/test-objects", deps.AutomationController.CreateTestObject)
	authed.PATCH("/config/test-objects/:id", deps.AutomationController.UpdateTestObject)
	authed.DELETE("/config/test-objects/:id", deps.AutomationController.DeleteTestObject)
}

func registerUIRoutes(authed *gin.RouterGroup, deps Dependencies) {
	registerUIAssetRoutes(authed, "/ui/elements", "page_element", deps)
	registerUIAssetRoutes(authed, "/ui/steps", "page_step", deps)
	registerUIAssetRoutes(authed, "/ui/cases", "test_case", deps)
	registerUIAssetRoutes(authed, "/ui/variables", "global_variable", deps)
	authed.GET("/ui/page-elements", deps.AutomationController.ListPageElements)
	authed.POST("/ui/page-elements", deps.AutomationController.CreatePageElement)
	authed.PATCH("/ui/page-elements/:id", deps.AutomationController.UpdatePageElement)
	authed.DELETE("/ui/page-elements/:id", deps.AutomationController.DeletePageElement)
	capture := authed.Group("/ui/page-elements")
	capture.POST("/capture-sessions", controller.RequirePermission("ui.element.capture"), deps.ElementCaptureController.CreateSession)
	capture.GET("/capture-sessions/:id", controller.RequirePermission("ui.element.read"), deps.ElementCaptureController.GetSession)
	capture.PATCH("/capture-sessions/:id/mode", controller.RequirePermission("ui.element.manage"), deps.ElementCaptureController.SetMode)
	capture.POST("/capture-sessions/:id/stop", controller.RequirePermission("ui.element.capture"), deps.ElementCaptureController.StopSession)
	capture.GET("/:id/versions", controller.RequirePermission("ui.element.read"), deps.ElementCaptureController.ListVersions)
	capture.POST("/:id/versions/:version/rollback", controller.RequirePermission("ui.element.rollback"), deps.ElementCaptureController.RollbackVersion)
}

func registerCaptureCandidateRoutes(authed *gin.RouterGroup, deps Dependencies) {
	capture := authed.Group("/ui/page-elements")
	capture.GET("/capture-sessions/:id/candidates", controller.RequirePermission("ui.element.read"), deps.ElementCaptureController.ListCandidates)
	capture.PATCH("/capture-sessions/:id/candidates/:candidateId", controller.RequirePermission("ui.element.manage"), deps.ElementCaptureController.UpdateCandidate)
	capture.DELETE("/capture-sessions/:id/candidates/:candidateId", controller.RequirePermission("ui.element.manage"), deps.ElementCaptureController.DeleteCandidate)
	capture.POST("/capture-sessions/:id/save", controller.RequirePermission("ui.element.manage"), deps.ElementCaptureController.SaveCandidates)
}

func captureNoStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

func registerUIAssetRoutes(authed *gin.RouterGroup, path string, assetType string, deps Dependencies) {
	authed.GET(path, deps.AutomationController.ListUIAssets(assetType))
	authed.POST(path, deps.AutomationController.CreateUIAsset(assetType))
	authed.PATCH(path+"/:id", deps.AutomationController.UpdateUIAsset(assetType))
	authed.DELETE(path+"/:id", deps.AutomationController.DeleteUIAsset(assetType))
}

func registerTestCaseRoutes(authed *gin.RouterGroup, deps Dependencies) {
	authed.GET("/test-cases", deps.TestCaseController.List)
	authed.POST("/test-cases", deps.TestCaseController.Create)
	authed.POST("/test-cases/import", deps.TestCaseController.Import)
	authed.GET("/test-cases/export", deps.TestCaseController.Export)
	authed.GET("/test-cases/:id", deps.TestCaseController.Get)
	authed.PATCH("/test-cases/:id", deps.TestCaseController.Update)
	authed.DELETE("/test-cases/:id", deps.TestCaseController.Delete)
	authed.GET("/test-cases/:id/datasets", deps.TestCaseController.ListDatasets)
	authed.POST("/test-cases/:id/datasets", deps.TestCaseController.CreateDataset)
	authed.PATCH("/test-cases/:id/datasets/:datasetId", deps.TestCaseController.UpdateDataset)
	authed.DELETE("/test-cases/:id/datasets/:datasetId", deps.TestCaseController.DeleteDataset)
}
