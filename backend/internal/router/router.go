package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/controller"
)

type Dependencies struct {
	AuthController       *controller.AuthController
	SystemController     *controller.SystemController
	CatalogController    *controller.CatalogController
	AutomationController *controller.AutomationController
	ExecutorController   *controller.ExecutorController
	TestCaseController   *controller.TestCaseController
	AuthMiddleware       gin.HandlerFunc
}

// RegisterRoutes 是唯一的路由注册入口。
func RegisterRoutes(engine *gin.Engine, deps Dependencies) {
	api := engine.Group("/api")
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": "ok"}})
	})
	api.POST("/executors/register", deps.ExecutorController.Register)
	api.POST("/executors/heartbeat", deps.ExecutorController.Heartbeat)
	api.POST("/auth/login", deps.AuthController.Login)
	api.POST("/auth/register", deps.AuthController.Register)

	authed := api.Group("", deps.AuthMiddleware)
	authed.GET("/auth/me", deps.AuthController.Me)
	authed.GET("/executors", deps.ExecutorController.List)

	registerSystemRoutes(authed, deps)
	registerConfigRoutes(authed, deps)
	registerUIRoutes(authed, deps)
	registerTestCaseRoutes(authed, deps)
}

func registerSystemRoutes(authed *gin.RouterGroup, deps Dependencies) {
	authed.GET("/system/overview", deps.SystemController.Overview)
	authed.GET("/system/users", deps.SystemController.ListUsers)
	authed.PATCH("/system/users/:id", deps.SystemController.UpdateUser)
	authed.DELETE("/system/users/:id", deps.SystemController.DeleteUser)
	authed.GET("/system/roles", deps.SystemController.ListRoles)
	authed.POST("/system/roles", deps.SystemController.CreateRole)
	authed.PATCH("/system/roles/:id", deps.SystemController.UpdateRole)
	authed.DELETE("/system/roles/:id", deps.SystemController.DeleteRole)
	authed.GET("/system/menus", deps.CatalogController.ListMenus)
	authed.GET("/system/dictionaries", deps.CatalogController.ListDictionaries)
	authed.GET("/system/logs", deps.CatalogController.ListLogs)
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
