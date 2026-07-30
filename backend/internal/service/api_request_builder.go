package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"synapseqa/backend/internal/model"
)

var apiVariablePattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

type apiRequestConfiguration struct {
	Headers  any            `json:"headers"`
	Params   any            `json:"params"`
	Body     any            `json:"body"`
	BodyType string         `json:"bodyType"`
	Auth     map[string]any `json:"auth"`
}

func (s *APIAutomationService) PreviewRequest(ctx context.Context, userID, interfaceID int64, req model.APIRequestPreviewRequest) (model.APIRequestPreview, error) {
	return s.buildAPIRequest(ctx, userID, interfaceID, req, true)
}

func (s *APIAutomationService) buildAPIRequest(ctx context.Context, userID, interfaceID int64, req model.APIRequestPreviewRequest, maskSecrets bool) (model.APIRequestPreview, error) {
	item, err := s.repo.GetInterface(ctx, userID, interfaceID)
	if err != nil || !s.repo.CanAccessProject(ctx, userID, item.ProjectID) {
		return model.APIRequestPreview{}, errors.New("接口不存在或无权访问")
	}
	method, requestPath, configuration := item.Method, item.Path, item.Configuration
	if req.Snapshot != nil {
		method = strings.ToUpper(strings.TrimSpace(req.Snapshot.Method))
		requestPath = strings.TrimSpace(req.Snapshot.Path)
		configuration = req.Snapshot.Configuration
	}
	if method == "" || requestPath == "" {
		return model.APIRequestPreview{}, errors.New("请求方法和路径不能为空")
	}
	target := requestPath
	envName := ""
	if parsed, parseErr := url.Parse(requestPath); parseErr != nil || !parsed.IsAbs() {
		if req.TestObjectID <= 0 {
			return model.APIRequestPreview{}, errors.New("相对路径必须选择测试环境")
		}
		testObject, objectErr := s.repo.TestObject(ctx, req.TestObjectID, item.ProductID)
		if objectErr != nil {
			return model.APIRequestPreview{}, errors.New("测试环境不存在或不属于当前产品")
		}
		envName = testObject.EnvName
		target = strings.TrimRight(testObject.Target, "/") + "/" + strings.TrimLeft(requestPath, "/")
	} else if req.TestObjectID > 0 {
		testObject, objectErr := s.repo.TestObject(ctx, req.TestObjectID, item.ProductID)
		if objectErr != nil {
			return model.APIRequestPreview{}, errors.New("测试环境不存在或不属于当前产品")
		}
		envName = testObject.EnvName
	}
	var config apiRequestConfiguration
	if len(configuration) > 0 {
		if err := json.Unmarshal(configuration, &config); err != nil {
			return model.APIRequestPreview{}, errors.New("接口配置格式无效")
		}
	}
	if err := s.revealAPIAuth(config.Auth); err != nil {
		return model.APIRequestPreview{}, err
	}
	variables, err := s.buildAPIVariables(ctx, item.ProductID, envName, req.TemporaryVariables)
	if err != nil {
		return model.APIRequestPreview{}, err
	}
	target, err = resolveAPIVariables(target, variables)
	if err != nil {
		return model.APIRequestPreview{}, err
	}
	parsedURL, err := url.Parse(target)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return model.APIRequestPreview{}, errors.New("最终请求 URL 无效")
	}
	query, err := stringMap(config.Params)
	if err != nil {
		return model.APIRequestPreview{}, errors.New("查询参数必须是 JSON 对象")
	}
	values := parsedURL.Query()
	for key, value := range query {
		resolved, resolveErr := resolveAPIVariables(value, variables)
		if resolveErr != nil {
			return model.APIRequestPreview{}, resolveErr
		}
		values.Set(key, resolved)
	}
	parsedURL.RawQuery = values.Encode()

	headers := map[string]string{}
	projectHeaders, err := s.repo.ListProjectHeaders(ctx, item.ProjectID, "")
	if err != nil {
		return model.APIRequestPreview{}, errors.New("读取项目默认请求头失败")
	}
	for _, header := range projectHeaders {
		if header.Enabled {
			setAPIHeader(headers, header.Name, header.Value)
		}
	}
	interfaceHeaders, err := stringMap(config.Headers)
	if err != nil {
		return model.APIRequestPreview{}, errors.New("请求头必须是 JSON 对象")
	}
	for key, value := range interfaceHeaders {
		setAPIHeader(headers, key, value)
	}
	for key, value := range req.TemporaryHeaders {
		setAPIHeader(headers, key, value)
	}
	for key, value := range headers {
		resolved, resolveErr := resolveAPIVariables(value, variables)
		if resolveErr != nil {
			return model.APIRequestPreview{}, resolveErr
		}
		headers[key] = resolved
	}
	body, warnings, err := buildAPIBody(config.BodyType, config.Body)
	if err != nil {
		return model.APIRequestPreview{}, errors.New("请求体格式无效")
	}
	body, err = resolveAPIVariables(body, variables)
	if err != nil {
		return model.APIRequestPreview{}, err
	}
	applyAPIAuth(headers, parsedURL, config.Auth, variables)
	if maskSecrets {
		headers = maskPreviewHeaders(headers)
	}
	return model.APIRequestPreview{Method: method, URL: parsedURL.String(), Headers: headers, Body: body, Warnings: warnings}, nil
}

func (s *APIAutomationService) buildAPIVariables(ctx context.Context, productID int64, envName string, temporary map[string]any) (map[string]any, error) {
	items, err := s.repo.GlobalVariables(ctx, productID, envName)
	if err != nil {
		return nil, errors.New("读取全局变量失败")
	}
	result := map[string]any{}
	for _, item := range items {
		value, parseErr := typedAPIVariable(item.Category, item.Value)
		if parseErr != nil {
			return nil, fmt.Errorf("全局变量 %s 的值无效", item.Name)
		}
		result[item.Name] = value
	}
	for key, value := range temporary {
		result[key] = value
	}
	return result, nil
}

func typedAPIVariable(valueType, value string) (any, error) {
	switch strings.ToLower(valueType) {
	case "number", "boolean", "json":
		var result any
		if err := json.Unmarshal([]byte(value), &result); err != nil {
			return nil, err
		}
		return result, nil
	default:
		return value, nil
	}
}

func stringMap(value any) (map[string]string, error) {
	if value == nil {
		return map[string]string{}, nil
	}
	if text, ok := value.(string); ok {
		if strings.TrimSpace(text) == "" {
			return map[string]string{}, nil
		}
		var result map[string]string
		return result, json.Unmarshal([]byte(text), &result)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]string
	return result, json.Unmarshal(raw, &result)
}

func bodyText(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	if text, ok := value.(string); ok {
		return text, nil
	}
	raw, err := json.Marshal(value)
	return string(raw), err
}

func buildAPIBody(bodyType string, value any) (string, []string, error) {
	switch strings.ToLower(strings.TrimSpace(bodyType)) {
	case "", "raw", "json":
		text, err := bodyText(value)
		return text, nil, err
	case "none":
		return "", nil, nil
	case "urlencoded":
		fields, err := stringMap(value)
		if err != nil {
			return "", nil, err
		}
		values := url.Values{}
		for key, field := range fields {
			values.Set(key, field)
		}
		return values.Encode(), nil, nil
	case "form_data":
		fields, err := stringMap(value)
		if err != nil {
			return "", nil, err
		}
		raw, err := json.Marshal(fields)
		return string(raw), []string{"form-data 预览仅展示字段，实际 multipart boundary 由执行器生成"}, err
	default:
		return "", nil, errors.New("不支持的请求体类型")
	}
}

func resolveAPIVariables(value string, variables map[string]any) (string, error) {
	current := value
	for depth := 0; depth < 10; depth++ {
		missing, changed := "", false
		current = apiVariablePattern.ReplaceAllStringFunc(current, func(match string) string {
			name := apiVariablePattern.FindStringSubmatch(match)[1]
			raw, exists := variables[name]
			if !exists {
				missing = name
				return match
			}
			changed = true
			if text, ok := raw.(string); ok {
				return text
			}
			encoded, _ := json.Marshal(raw)
			return string(encoded)
		})
		if missing != "" {
			return "", fmt.Errorf("变量 %s 未定义", missing)
		}
		if !changed || !apiVariablePattern.MatchString(current) {
			return current, nil
		}
	}
	return "", errors.New("变量引用超过最大深度或存在循环")
}

func setAPIHeader(headers map[string]string, name, value string) {
	name = strings.TrimSpace(name)
	if name == "" || restrictedAPIHeader(name) {
		return
	}
	for current := range headers {
		if strings.EqualFold(current, name) {
			if value == "" {
				delete(headers, current)
			} else {
				headers[current] = value
			}
			return
		}
	}
	if value != "" {
		headers[name] = value
	}
}

func restrictedAPIHeader(name string) bool {
	for _, item := range []string{"Host", "Content-Length", "Transfer-Encoding", "Connection"} {
		if strings.EqualFold(name, item) {
			return true
		}
	}
	return false
}

func applyAPIAuth(headers map[string]string, target *url.URL, auth map[string]any, variables map[string]any) {
	if auth == nil {
		return
	}
	resolve := func(key string) string {
		value, _ := resolveAPIVariables(fmt.Sprint(auth[key]), variables)
		return value
	}
	switch strings.ToLower(fmt.Sprint(auth["type"])) {
	case "bearer":
		setAPIHeader(headers, "Authorization", "Bearer "+resolve("token"))
	case "basic":
		token := base64.StdEncoding.EncodeToString([]byte(resolve("username") + ":" + resolve("password")))
		setAPIHeader(headers, "Authorization", "Basic "+token)
	case "api_key":
		name, value := resolve("name"), resolve("value")
		if strings.EqualFold(fmt.Sprint(auth["in"]), "query") {
			query := target.Query()
			query.Set(name, value)
			target.RawQuery = query.Encode()
		} else {
			setAPIHeader(headers, name, value)
		}
	}
}

func maskPreviewHeaders(headers map[string]string) map[string]string {
	result := make(map[string]string, len(headers))
	for key, value := range headers {
		lower := strings.ToLower(key)
		if strings.EqualFold(key, "Authorization") || strings.EqualFold(key, "Cookie") || strings.Contains(lower, "token") || strings.Contains(lower, "key") {
			result[key] = "******"
		} else {
			result[key] = value
		}
	}
	return result
}
