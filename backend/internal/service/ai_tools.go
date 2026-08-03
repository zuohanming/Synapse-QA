package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"synapseqa/backend/internal/model"
)

// AIToolExecutor holds references to services that AI tools can call.
type AIToolExecutor struct {
	apiSvc  *APIAutomationService
	catSvc  *CatalogService
	tcSvc   *TestCaseService
	execSvc *ExecutionService
	autoSvc *AutomationService
}

// NewAIToolExecutor creates a tool executor wired with business services.
func NewAIToolExecutor(apiSvc *APIAutomationService, catSvc *CatalogService, tcSvc *TestCaseService, execSvc *ExecutionService, autoSvc *AutomationService) *AIToolExecutor {
	return &AIToolExecutor{apiSvc: apiSvc, catSvc: catSvc, tcSvc: tcSvc, execSvc: execSvc, autoSvc: autoSvc}
}

// ToolDefinitions returns OpenAI-compatible tool definitions for function calling.
func (e *AIToolExecutor) ToolDefinitions() []map[string]any {
	return []map[string]any{
		// ========== 接口管理 ==========
		{
			"type": "function",
			"function": map[string]any{
				"name":        "search_interfaces",
				"description": "搜索接口管理中的 API 接口列表。可按关键词、HTTP 方法筛选。返回匹配的接口 ID、名称、方法和路径。",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"keyword": map[string]any{"type": "string", "description": "搜索关键词，匹配接口名称或路径"},
						"method":  map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "PATCH", "DELETE"}, "description": "HTTP 方法筛选"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "create_interface",
				"description": "在接口管理中创建一条新的 API 接口记录。需要名称、HTTP 方法和请求路径。productId 可选，不填时自动使用第一个产品。",
				"parameters": map[string]any{
					"type":     "object",
					"required": []string{"name", "method", "path"},
					"properties": map[string]any{
						"name":      map[string]any{"type": "string", "description": "接口名称"},
						"method":    map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "PATCH", "DELETE"}, "description": "HTTP 请求方法"},
						"path":      map[string]any{"type": "string", "description": "接口请求路径，如 /api/users"},
						"productId": map[string]any{"type": "integer", "description": "产品 ID，可选"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "get_interface",
				"description": "根据接口 ID 获取接口的详细信息。",
				"parameters": map[string]any{
					"type": "object", "required": []string{"id"},
					"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "接口 ID"}},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "delete_interface",
				"description": "软删除指定 ID 的接口，可恢复。",
				"parameters": map[string]any{
					"type": "object", "required": []string{"id"},
					"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "接口 ID"}},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "update_interface",
				"description": "更新指定 ID 的接口信息。只需传要修改的字段，未传字段保持不变。",
				"parameters": map[string]any{
					"type":     "object",
					"required": []string{"id"},
					"properties": map[string]any{
						"id":     map[string]any{"type": "integer", "description": "接口 ID（必填）"},
						"name":   map[string]any{"type": "string", "description": "新接口名称"},
						"method": map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "PATCH", "DELETE"}},
						"path":   map[string]any{"type": "string", "description": "新接口路径"},
					},
				},
			},
		},

		// ========== 产品/项目/模块 ==========
		{
			"type": "function",
			"function": map[string]any{
				"name":        "list_products",
				"description": "列出系统中的所有产品。用于了解系统中有哪些产品。",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"keyword": map[string]any{"type": "string", "description": "搜索关键词，匹配产品名称"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "list_modules",
				"description": "列出指定产品下的所有功能模块。productId 可选，不填时列出第一个产品的模块。",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"productId": map[string]any{"type": "integer", "description": "产品 ID，可选"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "create_product",
				"description": "创建一个新的产品。需要产品名称。projectId 可选，不填时自动使用第一个项目。",
				"parameters": map[string]any{
					"type":     "object",
					"required": []string{"name"},
					"properties": map[string]any{
						"name":      map[string]any{"type": "string", "description": "产品名称"},
						"projectId": map[string]any{"type": "integer", "description": "所属项目 ID，可选"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "update_product",
				"description": "更新指定 ID 的产品名称。",
				"parameters": map[string]any{
					"type":     "object",
					"required": []string{"id", "name"},
					"properties": map[string]any{
						"id":   map[string]any{"type": "integer", "description": "产品 ID"},
						"name": map[string]any{"type": "string", "description": "新名称"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "delete_product",
				"description": "删除指定 ID 的产品。",
				"parameters": map[string]any{
					"type": "object", "required": []string{"id"},
					"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "产品 ID"}},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "create_module",
				"description": "在产品下创建一个功能模块。productId 可选，不填时使用第一个产品。",
				"parameters": map[string]any{
					"type":     "object",
					"required": []string{"name"},
					"properties": map[string]any{
						"name":      map[string]any{"type": "string", "description": "模块名称"},
						"productId": map[string]any{"type": "integer", "description": "所属产品 ID，可选"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "update_module",
				"description": "更新指定 ID 的模块名称。需要提供模块所属的产品 ID。",
				"parameters": map[string]any{
					"type":     "object",
					"required": []string{"id", "name", "productId"},
					"properties": map[string]any{
						"id":        map[string]any{"type": "integer", "description": "模块 ID"},
						"name":      map[string]any{"type": "string", "description": "新名称"},
						"productId": map[string]any{"type": "integer", "description": "所属产品 ID（必填）"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "delete_module",
				"description": "删除指定 ID 的模块。",
				"parameters": map[string]any{
					"type": "object", "required": []string{"id"},
					"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "模块 ID"}},
				},
			},
		},

		// ========== 项目 ==========
		{
			"type": "function",
			"function": map[string]any{
				"name":        "list_projects",
				"description": "列出系统中的所有项目。可按名称搜索。",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"keyword": map[string]any{"type": "string", "description": "搜索关键词，匹配项目名称"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "create_project",
				"description": "创建一个新项目。只需提供项目名称。",
				"parameters": map[string]any{
					"type":     "object",
					"required": []string{"name"},
					"properties": map[string]any{
						"name": map[string]any{"type": "string", "description": "项目名称"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "update_project",
				"description": "更新指定 ID 的项目名称。",
				"parameters": map[string]any{
					"type":     "object",
					"required": []string{"id", "name"},
					"properties": map[string]any{
						"id":   map[string]any{"type": "integer", "description": "项目 ID"},
						"name": map[string]any{"type": "string", "description": "新名称"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "delete_project",
				"description": "删除指定 ID 的项目。",
				"parameters": map[string]any{
					"type": "object", "required": []string{"id"},
					"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "项目 ID"}},
				},
			},
		},

		// ========== 测试用例 ==========
		{
			"type": "function",
			"function": map[string]any{
				"name":        "search_test_cases",
				"description": "搜索测试用例库中的用例。可按名称、优先级、状态、类型筛选。返回用例 ID、名称、类型、优先级、状态。",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"keyword":  map[string]any{"type": "string", "description": "搜索关键词，匹配用例名称"},
						"priority": map[string]any{"type": "string", "enum": []string{"P0", "P1", "P2", "P3"}, "description": "优先级筛选"},
						"status":   map[string]any{"type": "string", "enum": []string{"draft", "active", "deprecated"}, "description": "状态筛选"},
						"caseType": map[string]any{"type": "string", "enum": []string{"api", "ui", "unit", "script"}, "description": "用例类型筛选"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "get_test_case",
				"description": "根据用例 ID 获取测试用例的详细信息，包括步骤、描述、预期结果等。",
				"parameters": map[string]any{
					"type": "object", "required": []string{"id"},
					"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "用例 ID"}},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "create_test_case",
				"description": "在测试用例库中创建一个新的测试用例。需要名称和用例类型。productId 可选。",
				"parameters": map[string]any{
					"type":     "object",
					"required": []string{"name", "caseType"},
					"properties": map[string]any{
						"name":        map[string]any{"type": "string", "description": "用例名称"},
						"caseType":    map[string]any{"type": "string", "enum": []string{"api", "ui", "unit", "script"}, "description": "用例类型"},
						"priority":    map[string]any{"type": "string", "enum": []string{"P0", "P1", "P2", "P3"}, "description": "优先级，默认 P2"},
						"description": map[string]any{"type": "string", "description": "用例描述或测试目的"},
						"productId":   map[string]any{"type": "integer", "description": "产品 ID，可选"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "delete_test_case",
				"description": "删除指定 ID 的测试用例。",
				"parameters": map[string]any{
					"type": "object", "required": []string{"id"},
					"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "用例 ID"}},
				},
			},
		},

		// ========== 执行任务 ==========
		{
			"type": "function",
			"function": map[string]any{
				"name":        "list_execution_runs",
				"description": "列出最近的测试执行记录（执行批次）。可按状态筛选。返回执行 ID、类型、状态、创建时间。",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"status": map[string]any{"type": "string", "enum": []string{"pending", "running", "passed", "failed", "cancelled"}, "description": "按状态筛选"},
						"limit":  map[string]any{"type": "integer", "description": "返回条数，默认 10"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "get_execution_run",
				"description": "根据执行 ID 获取某次测试执行的详细信息，包括各个任务的执行状态。",
				"parameters": map[string]any{
					"type": "object", "required": []string{"id"},
					"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "执行批次 ID"}},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "cancel_execution_run",
				"description": "取消一个正在执行或等待中的执行批次。",
				"parameters": map[string]any{
					"type": "object", "required": []string{"id"},
					"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "执行批次 ID"}},
				},
			},
		},

		// ========== 测试环境 ==========
		{
			"type": "function",
			"function": map[string]any{
				"name":        "list_test_objects",
				"description": "列出测试环境/测试对象列表。可按环境名称或产品筛选。",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"envName":   map[string]any{"type": "string", "description": "环境名称关键词"},
						"productId": map[string]any{"type": "integer", "description": "产品 ID，可选"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "create_test_object",
				"description": "创建一个新的测试环境/测试对象。productId 可选，不填时使用第一个产品。",
				"parameters": map[string]any{
					"type":     "object",
					"required": []string{"envName", "target"},
					"properties": map[string]any{
						"envName":   map[string]any{"type": "string", "description": "环境名称"},
						"target":    map[string]any{"type": "string", "description": "测试对象/目标地址"},
						"productId": map[string]any{"type": "integer", "description": "产品 ID，可选"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "update_test_object",
				"description": "更新指定 ID 的测试环境信息。",
				"parameters": map[string]any{
					"type":     "object",
					"required": []string{"id"},
					"properties": map[string]any{
						"id":      map[string]any{"type": "integer", "description": "测试环境 ID"},
						"envName": map[string]any{"type": "string", "description": "新环境名称"},
						"target":  map[string]any{"type": "string", "description": "新测试对象"},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "delete_test_object",
				"description": "删除指定 ID 的测试环境。",
				"parameters": map[string]any{
					"type": "object", "required": []string{"id"},
					"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "测试环境 ID"}},
				},
			},
		},
	}
}

// Execute runs the named tool with the given JSON arguments and returns a human-readable result.
func (e *AIToolExecutor) Execute(ctx context.Context, userID int64, toolName string, args json.RawMessage) (string, error) {
	switch toolName {
	// 接口管理
	case "search_interfaces":
		return e.searchInterfaces(ctx, userID, args)
	case "create_interface":
		return e.createInterface(ctx, userID, args)
	case "get_interface":
		return e.getInterface(ctx, userID, args)
	case "delete_interface":
		return e.deleteInterface(ctx, userID, args)
	case "update_interface":
		return e.updateInterface(ctx, userID, args)
	// 产品/模块
	case "list_products":
		return e.listProducts(ctx, args)
	case "list_modules":
		return e.listModules(ctx, args)
	case "create_product":
		return e.createProduct(ctx, args)
	case "update_product":
		return e.updateProduct(ctx, args)
	case "delete_product":
		return e.deleteProduct(ctx, args)
	case "create_module":
		return e.createModule(ctx, args)
	case "update_module":
		return e.updateModule(ctx, args)
	case "delete_module":
		return e.deleteModule(ctx, args)
	// 项目
	case "list_projects":
		return e.listProjects(ctx, args)
	case "create_project":
		return e.createProject(ctx, args)
	case "update_project":
		return e.updateProject(ctx, args)
	case "delete_project":
		return e.deleteProject(ctx, args)
	// 测试用例
	case "search_test_cases":
		return e.searchTestCases(ctx, args)
	case "get_test_case":
		return e.getTestCase(ctx, args)
	case "create_test_case":
		return e.createTestCase(ctx, args)
	case "delete_test_case":
		return e.deleteTestCase(ctx, args)
	// 执行任务
	case "list_execution_runs":
		return e.listExecutionRuns(ctx, args)
	case "get_execution_run":
		return e.getExecutionRun(ctx, args)
	case "cancel_execution_run":
		return e.cancelExecutionRun(ctx, args)
	// 测试环境
	case "list_test_objects":
		return e.listTestObjects(ctx, args)
	case "create_test_object":
		return e.createTestObject(ctx, args)
	case "update_test_object":
		return e.updateTestObject(ctx, args)
	case "delete_test_object":
		return e.deleteTestObject(ctx, args)
	default:
		return "", fmt.Errorf("未知工具: %s", toolName)
	}
}

// ==================== 接口管理 ====================

func (e *AIToolExecutor) searchInterfaces(ctx context.Context, userID int64, args json.RawMessage) (string, error) {
	var params struct {
		Keyword string `json:"keyword"`
		Method  string `json:"method"`
	}
	json.Unmarshal(args, &params)

	page, err := e.apiSvc.ListInterfaces(ctx, userID, model.APIInterfaceFilter{Keyword: params.Keyword, Method: params.Method}, 1, 20)
	if err != nil {
		return "", fmt.Errorf("查询接口失败: %w", err)
	}
	if page.Total == 0 {
		return "没有找到匹配的接口。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共找到 %d 个接口：\n\n| ID | 名称 | 方法 | 路径 | 状态 |\n|----|------|------|------|------|\n"))

	items := toMapSlice(page.Items)
	for i, m := range items {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("\n... 还有 %d 条结果未显示", page.Total-20))
			break
		}
		sb.WriteString(fmt.Sprintf("| %v | %v | %v | %v | %v |\n", m["id"], m["name"], m["method"], m["path"], m["lifecycleStatus"]))
	}
	return sb.String(), nil
}

func (e *AIToolExecutor) createInterface(ctx context.Context, userID int64, args json.RawMessage) (string, error) {
	var params struct {
		Name      string  `json:"name"`
		Method    string  `json:"method"`
		Path      string  `json:"path"`
		ProductID float64 `json:"productId"`
	}
	json.Unmarshal(args, &params)

	productID := int64(params.ProductID)
	if productID <= 0 {
		productID = e.autoDiscoverProduct(ctx)
		if productID <= 0 {
			return "创建失败：无法自动获取产品 ID，请手动指定 productId。", nil
		}
	}

	req := model.APIInterfaceRequest{
		ProductID: productID, Name: params.Name, Method: params.Method, Path: params.Path,
		Protocol: "HTTP", EndpointType: "rest", LifecycleStatus: "draft", TimeoutSeconds: 30,
	}
	id, err := e.apiSvc.CreateInterface(ctx, userID, "AI助手", req)
	if err != nil {
		return "", fmt.Errorf("创建接口失败: %w", err)
	}
	return fmt.Sprintf("创建成功！接口 ID 为 %d，名称「%s」，方法 %s，路径 %s。", id, params.Name, params.Method, params.Path), nil
}

func (e *AIToolExecutor) getInterface(ctx context.Context, userID int64, args json.RawMessage) (string, error) {
	var params struct {
		ID json.RawMessage `json:"id"`
	}
	json.Unmarshal(args, &params)
	id, err := parseIntFromJSON(params.ID)
	if err != nil {
		return "", fmt.Errorf("无效的 ID: %w", err)
	}
	iface, err := e.apiSvc.GetInterface(ctx, userID, id)
	if err != nil {
		return "", fmt.Errorf("获取接口失败: %w", err)
	}
	return fmt.Sprintf("接口详情：\n- ID: %d\n- 名称: %s\n- 方法: %s\n- 路径: %s\n- 状态: %s\n- 协议: %s\n- 超时: %ds",
		iface.ID, iface.Name, iface.Method, iface.Path, iface.LifecycleStatus, iface.Protocol, iface.TimeoutSeconds), nil
}

func (e *AIToolExecutor) deleteInterface(ctx context.Context, userID int64, args json.RawMessage) (string, error) {
	var params struct {
		ID json.RawMessage `json:"id"`
	}
	json.Unmarshal(args, &params)
	id, err := parseIntFromJSON(params.ID)
	if err != nil {
		return "", fmt.Errorf("无效的 ID: %w", err)
	}
	if err := e.apiSvc.DeleteInterface(ctx, userID, "AI助手", id); err != nil {
		return "", fmt.Errorf("删除接口失败: %w", err)
	}
	return fmt.Sprintf("接口 ID %d 已成功删除。", id), nil
}

// ==================== 产品/模块 ====================

func (e *AIToolExecutor) listProducts(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Keyword string `json:"keyword"`
	}
	json.Unmarshal(args, &params)

	page, err := e.catSvc.ListProducts(ctx, "", params.Keyword, 1, 20)
	if err != nil {
		return "", fmt.Errorf("查询产品失败: %w", err)
	}
	if page.Total == 0 {
		return "系统中暂无产品。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 个产品：\n\n| ID | 产品名称 |\n|----|----------|\n", page.Total))
	for _, m := range toMapSlice(page.Items) {
		sb.WriteString(fmt.Sprintf("| %v | %v |\n", m["id"], m["name"]))
	}
	return sb.String(), nil
}

func (e *AIToolExecutor) listModules(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		ProductID float64 `json:"productId"`
	}
	json.Unmarshal(args, &params)
	productID := int64(params.ProductID)
	if productID <= 0 {
		productID = e.autoDiscoverProduct(ctx)
	}

	page, err := e.catSvc.ListProductModules(ctx, productID, "", "", "", 1, 50)
	if err != nil {
		return "", fmt.Errorf("查询模块失败: %w", err)
	}
	if page.Total == 0 {
		return "该产品下暂无模块。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("产品 ID %d 下共 %d 个模块：\n\n| ID | 模块名称 |\n|----|----------|\n", productID, page.Total))
	for _, m := range toMapSlice(page.Items) {
		sb.WriteString(fmt.Sprintf("| %v | %v |\n", m["id"], m["name"]))
	}
	return sb.String(), nil
}

// ==================== 测试用例 ====================

func (e *AIToolExecutor) searchTestCases(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Keyword  string `json:"keyword"`
		Priority string `json:"priority"`
		Status   string `json:"status"`
		CaseType string `json:"caseType"`
	}
	json.Unmarshal(args, &params)

	page, err := e.tcSvc.List(ctx, model.TestCaseFilter{
		Name: params.Keyword, Priority: params.Priority, Status: params.Status, CaseType: params.CaseType,
	}, 1, 20)
	if err != nil {
		return "", fmt.Errorf("查询用例失败: %w", err)
	}
	if page.Total == 0 {
		return "没有找到匹配的测试用例。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共找到 %d 个用例：\n\n| ID | 名称 | 类型 | 优先级 | 状态 |\n|----|------|------|--------|------|\n", page.Total))
	for _, m := range toMapSlice(page.Items) {
		sb.WriteString(fmt.Sprintf("| %v | %v | %v | %v | %v |\n", m["id"], m["name"], m["caseType"], m["priority"], m["status"]))
	}
	return sb.String(), nil
}

func (e *AIToolExecutor) getTestCase(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		ID json.RawMessage `json:"id"`
	}
	json.Unmarshal(args, &params)
	id, err := parseIntFromJSON(params.ID)
	if err != nil {
		return "", fmt.Errorf("无效的 ID: %w", err)
	}
	tc, err := e.tcSvc.Get(ctx, id)
	if err != nil {
		return "", fmt.Errorf("获取用例失败: %w", err)
	}
	return fmt.Sprintf("用例详情：\n- ID: %d\n- 名称: %s\n- 类型: %s\n- 优先级: %s\n- 状态: %s\n- 拥有者: %s\n- 描述: %s",
		tc.ID, tc.Name, tc.CaseType, tc.Priority, tc.Status, tc.Owner, truncate(tc.Description, 200)), nil
}

func (e *AIToolExecutor) createTestCase(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Name        string  `json:"name"`
		CaseType    string  `json:"caseType"`
		Priority    string  `json:"priority"`
		Description string  `json:"description"`
		ProductID   float64 `json:"productId"`
	}
	json.Unmarshal(args, &params)
	if params.Priority == "" {
		params.Priority = "P2"
	}

	productID := int64(params.ProductID)
	if productID <= 0 {
		productID = e.autoDiscoverProduct(ctx)
		if productID <= 0 {
			return "创建失败：无法自动获取产品 ID，请手动指定 productId。", nil
		}
	}

	req := model.TestCaseRequest{
		ProductID: productID, Name: params.Name, CaseType: params.CaseType,
		Priority: params.Priority, Description: params.Description, Status: "draft",
	}
	id, err := e.tcSvc.Create(ctx, "AI助手", req)
	if err != nil {
		return "", fmt.Errorf("创建用例失败: %w", err)
	}
	return fmt.Sprintf("创建成功！用例 ID 为 %d，名称「%s」，类型 %s，优先级 %s。", id, params.Name, params.CaseType, params.Priority), nil
}

// ==================== 执行任务 ====================

func (e *AIToolExecutor) listExecutionRuns(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Status string  `json:"status"`
		Limit  float64 `json:"limit"`
	}
	json.Unmarshal(args, &params)
	limit := int(params.Limit)
	if limit <= 0 {
		limit = 10
	}

	page, err := e.execSvc.ListRuns(ctx, model.ExecutionRunFilter{Status: params.Status}, 1, limit)
	if err != nil {
		return "", fmt.Errorf("查询执行记录失败: %w", err)
	}
	if page.Total == 0 {
		return "没有找到执行记录。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 条执行记录，最近 %d 条：\n\n| ID | 类型 | 状态 | 创建时间 |\n|----|------|------|----------|\n", page.Total, limit))
	for _, m := range toMapSlice(page.Items) {
		sb.WriteString(fmt.Sprintf("| %v | %v | %v | %v |\n", m["id"], m["runType"], m["status"], m["createdAt"]))
	}
	return sb.String(), nil
}

func (e *AIToolExecutor) getExecutionRun(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		ID json.RawMessage `json:"id"`
	}
	json.Unmarshal(args, &params)
	id, err := parseIntFromJSON(params.ID)
	if err != nil {
		return "", fmt.Errorf("无效的 ID: %w", err)
	}
	run, err := e.execSvc.GetRun(ctx, id)
	if err != nil {
		return "", fmt.Errorf("获取执行详情失败: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("执行 #%d 详情：\n- 类型: %s\n- 状态: %s\n- 触发者: %s\n- 创建时间: %s\n\n",
		run.ID, run.RunType, run.Status, run.TriggeredBy, run.CreatedAt.Format("2006-01-02 15:04")))
	if len(run.Tasks) > 0 {
		sb.WriteString("| 任务 | 状态 | 创建时间 |\n|------|------|----------|\n")
		for _, t := range run.Tasks {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", t.TaskID, t.Status, t.CreatedAt.Format("15:04")))
		}
	}
	return sb.String(), nil
}

// ==================== 测试环境 ====================

func (e *AIToolExecutor) listTestObjects(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		EnvName   string  `json:"envName"`
		ProductID float64 `json:"productId"`
	}
	json.Unmarshal(args, &params)

	page, err := e.autoSvc.ListTestObjects(ctx, "", params.EnvName, fmt.Sprintf("%d", int64(params.ProductID)), 1, 20)
	if err != nil {
		return "", fmt.Errorf("查询测试环境失败: %w", err)
	}
	if page.Total == 0 {
		return "没有找到测试环境。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 个测试环境：\n\n| ID | 环境名称 | 测试对象 |\n|----|----------|----------|\n", page.Total))
	for _, m := range toMapSlice(page.Items) {
		sb.WriteString(fmt.Sprintf("| %v | %v | %v |\n", m["id"], m["envName"], m["target"]))
	}
	return sb.String(), nil
}

// ==================== 接口管理（补充） ====================

func (e *AIToolExecutor) updateInterface(ctx context.Context, userID int64, args json.RawMessage) (string, error) {
	var params struct {
		ID     float64 `json:"id"`
		Name   string  `json:"name"`
		Method string  `json:"method"`
		Path   string  `json:"path"`
	}
	json.Unmarshal(args, &params)
	id := int64(params.ID)
	if id <= 0 {
		return "", fmt.Errorf("接口 ID 不能为空")
	}

	// Get existing interface to preserve fields
	existing, err := e.apiSvc.GetInterface(ctx, userID, id)
	if err != nil {
		return "", fmt.Errorf("获取接口失败: %w", err)
	}

	req := model.APIInterfaceRequest{
		ProductID: existing.ProductID, ModuleID: existing.ModuleID,
		Name: existing.Name, Method: existing.Method, Path: existing.Path,
		Protocol: existing.Protocol, EndpointType: existing.EndpointType,
		LifecycleStatus: existing.LifecycleStatus, TimeoutSeconds: existing.TimeoutSeconds,
		Revision: existing.Revision,
	}
	if params.Name != "" {
		req.Name = params.Name
	}
	if params.Method != "" {
		req.Method = params.Method
	}
	if params.Path != "" {
		req.Path = params.Path
	}

	if err := e.apiSvc.UpdateInterface(ctx, userID, "AI助手", id, req); err != nil {
		return "", fmt.Errorf("更新接口失败: %w", err)
	}
	return fmt.Sprintf("接口 ID %d 已更新：名称「%s」，方法 %s，路径 %s。", id, req.Name, req.Method, req.Path), nil
}

// ==================== 产品/模块（补充） ====================

func (e *AIToolExecutor) createProduct(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Name      string  `json:"name"`
		ProjectID float64 `json:"projectId"`
	}
	json.Unmarshal(args, &params)
	projectID := int64(params.ProjectID)
	if projectID <= 0 {
		projectID = e.autoDiscoverProject(ctx)
		if projectID <= 0 {
			return "创建失败：无法自动获取项目 ID，请手动指定 projectId。", nil
		}
	}

	req := model.ProductRequest{ProjectID: projectID, Name: params.Name}
	if err := e.catSvc.CreateProduct(ctx, "AI助手", req); err != nil {
		return "", fmt.Errorf("创建产品失败: %w", err)
	}
	return fmt.Sprintf("创建成功！产品「%s」已创建。", params.Name), nil
}

func (e *AIToolExecutor) updateProduct(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		ID   float64 `json:"id"`
		Name string  `json:"name"`
	}
	json.Unmarshal(args, &params)
	id := int64(params.ID)

	// 先查找已有产品，获取必填字段（ProjectID、UIType 等）
	page, err := e.catSvc.ListProducts(ctx, fmt.Sprintf("%d", id), "", 1, 1)
	if err != nil || page.Total == 0 {
		return "", fmt.Errorf("产品 ID %d 不存在", id)
	}
	existing := toMapSlice(page.Items)[0]
	projectID := int64(existing["projectId"].(float64))
	uiType := existing["uiType"].(string)
	apiType := existing["apiType"].(string)
	if params.Name == "" {
		params.Name = existing["name"].(string)
	}

	req := model.ProductRequest{ProjectID: projectID, Name: params.Name, UIType: uiType, APIType: apiType}
	if err := e.catSvc.UpdateProduct(ctx, "AI助手", id, req); err != nil {
		return "", fmt.Errorf("更新产品失败: %w", err)
	}
	return fmt.Sprintf("产品 ID %d 已更新为「%s」。", id, params.Name), nil
}

func (e *AIToolExecutor) deleteProduct(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		ID float64 `json:"id"`
	}
	json.Unmarshal(args, &params)
	id := int64(params.ID)
	if err := e.catSvc.DeleteProduct(ctx, "AI助手", id); err != nil {
		return "", fmt.Errorf("删除产品失败: %w", err)
	}
	return fmt.Sprintf("产品 ID %d 已删除。", id), nil
}

func (e *AIToolExecutor) createModule(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Name      string  `json:"name"`
		ProductID float64 `json:"productId"`
	}
	json.Unmarshal(args, &params)
	productID := int64(params.ProductID)
	if productID <= 0 {
		productID = e.autoDiscoverProduct(ctx)
		if productID <= 0 {
			return "创建失败：无法自动获取产品 ID，请手动指定 productId。", nil
		}
	}

	req := model.ProductModuleRequest{ProductID: productID, Name: params.Name}
	if err := e.catSvc.CreateProductModule(ctx, "AI助手", req); err != nil {
		return "", fmt.Errorf("创建模块失败: %w", err)
	}
	return fmt.Sprintf("创建成功！模块「%s」已在产品 ID %d 下创建。", params.Name, productID), nil
}

func (e *AIToolExecutor) updateModule(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		ID        float64 `json:"id"`
		Name      string  `json:"name"`
		ProductID float64 `json:"productId"`
	}
	json.Unmarshal(args, &params)
	id := int64(params.ID)
	productID := int64(params.ProductID)

	req := model.ProductModuleRequest{ProductID: productID, Name: params.Name}
	if err := e.catSvc.UpdateProductModule(ctx, "AI助手", id, req); err != nil {
		return "", fmt.Errorf("更新模块失败: %w", err)
	}
	return fmt.Sprintf("模块 ID %d 已更新为「%s」。", id, params.Name), nil
}

func (e *AIToolExecutor) deleteModule(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		ID float64 `json:"id"`
	}
	json.Unmarshal(args, &params)
	id := int64(params.ID)
	if err := e.catSvc.DeleteProductModule(ctx, "AI助手", id); err != nil {
		return "", fmt.Errorf("删除模块失败: %w", err)
	}
	return fmt.Sprintf("模块 ID %d 已删除。", id), nil
}

// ==================== 项目 ====================

func (e *AIToolExecutor) listProjects(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Keyword string `json:"keyword"`
	}
	json.Unmarshal(args, &params)

	page, err := e.catSvc.ListProjects(ctx, "", params.Keyword, 1, 20)
	if err != nil {
		return "", fmt.Errorf("查询项目失败: %w", err)
	}
	if page.Total == 0 {
		return "系统中暂无项目。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 个项目：\n\n| ID | 项目名称 | 状态 |\n|----|----------|------|\n", page.Total))
	for _, m := range toMapSlice(page.Items) {
		sb.WriteString(fmt.Sprintf("| %v | %v | %v |\n", m["id"], m["name"], m["status"]))
	}
	return sb.String(), nil
}

func (e *AIToolExecutor) createProject(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Name string `json:"name"`
	}
	json.Unmarshal(args, &params)

	req := model.ProjectRequest{Name: params.Name}
	if err := e.catSvc.CreateProject(ctx, "AI助手", req); err != nil {
		return "", fmt.Errorf("创建项目失败: %w", err)
	}
	return fmt.Sprintf("创建成功！项目「%s」已创建。", params.Name), nil
}

func (e *AIToolExecutor) updateProject(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		ID   float64 `json:"id"`
		Name string  `json:"name"`
	}
	json.Unmarshal(args, &params)
	id := int64(params.ID)

	// 先查找已有项目，保留其状态
	page, err := e.catSvc.ListProjects(ctx, fmt.Sprintf("%d", id), "", 1, 1)
	if err != nil || page.Total == 0 {
		return "", fmt.Errorf("项目 ID %d 不存在", id)
	}
	existing := toMapSlice(page.Items)[0]
	status := existing["status"].(string)
	if params.Name == "" {
		params.Name = existing["name"].(string)
	}

	req := model.ProjectRequest{Name: params.Name, Status: status}
	if err := e.catSvc.UpdateProject(ctx, "AI助手", id, req); err != nil {
		return "", fmt.Errorf("更新项目失败: %w", err)
	}
	return fmt.Sprintf("项目 ID %d 已更新为「%s」。", id, params.Name), nil
}

func (e *AIToolExecutor) deleteProject(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		ID float64 `json:"id"`
	}
	json.Unmarshal(args, &params)
	id := int64(params.ID)
	if err := e.catSvc.DeleteProject(ctx, "AI助手", id); err != nil {
		return "", fmt.Errorf("删除项目失败: %w", err)
	}
	return fmt.Sprintf("项目 ID %d 已删除。", id), nil
}

// ==================== 测试用例（补充） ====================

func (e *AIToolExecutor) deleteTestCase(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		ID float64 `json:"id"`
	}
	json.Unmarshal(args, &params)
	id := int64(params.ID)
	if err := e.tcSvc.Delete(ctx, "AI助手", id); err != nil {
		return "", fmt.Errorf("删除用例失败: %w", err)
	}
	return fmt.Sprintf("用例 ID %d 已删除。", id), nil
}

// ==================== 执行任务（补充） ====================

func (e *AIToolExecutor) cancelExecutionRun(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		ID float64 `json:"id"`
	}
	json.Unmarshal(args, &params)
	id := int64(params.ID)
	if err := e.execSvc.CancelRun(ctx, "AI助手", id); err != nil {
		return "", fmt.Errorf("取消执行失败: %w", err)
	}
	return fmt.Sprintf("执行 #%d 已取消。", id), nil
}

// ==================== 测试环境（补充） ====================

func (e *AIToolExecutor) createTestObject(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		EnvName   string  `json:"envName"`
		Target    string  `json:"target"`
		ProductID float64 `json:"productId"`
	}
	json.Unmarshal(args, &params)
	productID := int64(params.ProductID)
	if productID <= 0 {
		productID = e.autoDiscoverProduct(ctx)
		if productID <= 0 {
			return "创建失败：无法自动获取产品 ID，请手动指定 productId。", nil
		}
	}

	req := model.TestObjectRequest{
		ProductID: productID, EnvName: params.EnvName, Target: params.Target,
		Owner: "AI助手",
	}
	if err := e.autoSvc.CreateTestObject(ctx, "AI助手", req); err != nil {
		return "", fmt.Errorf("创建测试环境失败: %w", err)
	}
	return fmt.Sprintf("创建成功！测试环境「%s」（目标：%s）已创建。", params.EnvName, params.Target), nil
}

func (e *AIToolExecutor) updateTestObject(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		ID      float64 `json:"id"`
		EnvName string  `json:"envName"`
		Target  string  `json:"target"`
	}
	json.Unmarshal(args, &params)
	id := int64(params.ID)

	// 先查找已有测试环境，获取必填字段（ProductID、Owner 等）
	page, err := e.autoSvc.ListTestObjects(ctx, fmt.Sprintf("%d", id), "", "", 1, 1)
	if err != nil || page.Total == 0 {
		return "", fmt.Errorf("测试环境 ID %d 不存在", id)
	}
	existing := toMapSlice(page.Items)[0]
	productID := int64(existing["productId"].(float64))
	owner := existing["owner"].(string)
	deployEnv := existing["deployEnv"].(string)
	autoType := existing["autoType"].(string)
	queryEnabled := existing["queryEnabled"].(bool)
	writeEnabled := existing["writeEnabled"].(bool)
	if params.EnvName == "" {
		params.EnvName = existing["envName"].(string)
	}
	if params.Target == "" {
		params.Target = existing["target"].(string)
	}

	req := model.TestObjectRequest{
		ProductID: productID, EnvName: params.EnvName, Target: params.Target,
		Owner: owner, DeployEnv: deployEnv, AutoType: autoType,
		QueryEnabled: queryEnabled, WriteEnabled: writeEnabled,
	}
	if err := e.autoSvc.UpdateTestObject(ctx, "AI助手", id, req); err != nil {
		return "", fmt.Errorf("更新测试环境失败: %w", err)
	}
	return fmt.Sprintf("测试环境 ID %d 已更新。", id), nil
}

func (e *AIToolExecutor) deleteTestObject(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		ID float64 `json:"id"`
	}
	json.Unmarshal(args, &params)
	id := int64(params.ID)
	if err := e.autoSvc.DeleteTestObject(ctx, "AI助手", id); err != nil {
		return "", fmt.Errorf("删除测试环境失败: %w", err)
	}
	return fmt.Sprintf("测试环境 ID %d 已删除。", id), nil
}

// ==================== 工具函数 ====================

// autoDiscoverProduct returns the ID of the first product in the system.
func (e *AIToolExecutor) autoDiscoverProduct(ctx context.Context) int64 {
	products, err := e.catSvc.ListProducts(ctx, "", "", 1, 1)
	if err != nil || products.Total == 0 {
		return 0
	}
	b, _ := json.Marshal(products.Items)
	var list []map[string]any
	json.Unmarshal(b, &list)
	if len(list) > 0 {
		if id, ok := list[0]["id"].(float64); ok {
			return int64(id)
		}
	}
	return 0
}

// autoDiscoverProject returns the ID of the first project in the system.
func (e *AIToolExecutor) autoDiscoverProject(ctx context.Context) int64 {
	projects, err := e.catSvc.ListProjects(ctx, "", "", 1, 1)
	if err != nil || projects.Total == 0 {
		return 0
	}
	b, _ := json.Marshal(projects.Items)
	var list []map[string]any
	json.Unmarshal(b, &list)
	if len(list) > 0 {
		if id, ok := list[0]["id"].(float64); ok {
			return int64(id)
		}
	}
	return 0
}

// toMapSlice converts a generic page.Items to []map[string]any via JSON round-trip.
func toMapSlice(items any) []map[string]any {
	b, _ := json.Marshal(items)
	var result []map[string]any
	json.Unmarshal(b, &result)
	return result
}

// parseIntFromJSON handles float64 or string JSON numbers.
func parseIntFromJSON(raw json.RawMessage) (int64, error) {
	var num float64
	if err := json.Unmarshal(raw, &num); err == nil {
		return int64(num), nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, fmt.Errorf("无法解析为数字: %s", string(raw))
	}
	return strconv.ParseInt(s, 10, 64)
}

// truncate shortens a string to maxLen characters.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
