package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"synapseqa/backend/internal/model"
)

func (s *APIAutomationService) StartAPITestRun(ctx context.Context, userID int64, actor string, req model.APITestRunStartRequest) (model.APITestRunBatch, error) {
	if s.executorRepo == nil {
		return model.APITestRunBatch{}, errors.New("执行器调度尚未配置")
	}
	defaultConcurrency, maxConcurrency, batchSize, _ := s.apiRunPolicy(ctx)
	if len(req.CaseIDs) == 0 || len(req.CaseIDs) > batchSize {
		return model.APITestRunBatch{}, errors.New("请选择 1 至 1000 条接口测试用例")
	}
	req.EnvName = strings.TrimSpace(req.EnvName)
	if req.EnvName == "" {
		return model.APITestRunBatch{}, errors.New("请选择执行环境")
	}
	if req.Concurrency <= 0 {
		req.Concurrency = defaultConcurrency
	}
	if req.Concurrency > maxConcurrency {
		return model.APITestRunBatch{}, errors.New("单批次并发数不能超过 100")
	}
	batchID := fmt.Sprintf("api-case-batch-%d", time.Now().UnixNano())
	projectID := int64(0)
	instances := []model.APITestRunInstance{}
	options, _ := json.Marshal(req)
	for _, caseID := range req.CaseIDs {
		item, err := s.repo.GetAPITestCase(ctx, userID, caseID)
		if err != nil || item.CurrentVersion <= 0 {
			return model.APITestRunBatch{}, fmt.Errorf("用例 %d 不存在或尚未发布", caseID)
		}
		if projectID == 0 {
			projectID = item.ProjectID
		}
		if item.ProjectID != projectID {
			return model.APITestRunBatch{}, errors.New("同一批次只能执行同一项目的用例")
		}
		version, err := s.repo.GetAPITestCaseVersion(ctx, caseID, item.CurrentVersion)
		if err != nil {
			return model.APITestRunBatch{}, fmt.Errorf("用例 %d 的发布版本不存在", caseID)
		}
		payloads, err := s.buildAPITestInstancePayloads(ctx, item, version, req)
		if err != nil {
			return model.APITestRunBatch{}, fmt.Errorf("用例 %s 预检失败：%w", item.Name, err)
		}
		for datasetIndex, payload := range payloads {
			taskID := fmt.Sprintf("api-case-%d-%d-%d", caseID, datasetIndex, time.Now().UnixNano())
			snapshot, _ := json.Marshal(payload)
			instances = append(instances, model.APITestRunInstance{
				BatchID: batchID, TaskID: taskID, CaseID: caseID, CaseVersion: item.CurrentVersion,
				DatasetIndex: datasetIndex, Snapshot: snapshot,
			})
		}
	}
	if len(instances) > 50000 {
		return model.APITestRunBatch{}, errors.New("单个批次展开后最多 50000 个执行实例")
	}
	batch := model.APITestRunBatch{
		BatchID: batchID, ProjectID: projectID, EnvName: req.EnvName, Status: "queued",
		TotalInstances: len(instances), QueuedInstances: len(instances), Options: options, TriggeredBy: actor,
	}
	if err := s.repo.CreateAPITestRunBatch(ctx, batch, instances); err != nil {
		return model.APITestRunBatch{}, errors.New("创建接口用例执行批次失败")
	}
	s.dispatchAPITestInstances(context.Background(), batchID, req.Concurrency, req.ExecutorID)
	if s.systemRepo != nil {
		_ = s.systemRepo.LogOperation(ctx, actor, "执行接口测试用例", batchID)
	}
	return batch, nil
}

func (s *APIAutomationService) buildAPITestInstancePayloads(ctx context.Context, item model.APITestCase, version model.APITestCaseVersion, req model.APITestRunStartRequest) ([]map[string]any, error) {
	var draft model.APITestCaseDraft
	if err := json.Unmarshal(version.Snapshot, &draft); err != nil {
		return nil, errors.New("用例版本快照无效")
	}
	baseVariables := map[string]any{}
	sensitive := map[string]bool{}
	for _, variable := range draft.Variables {
		name := fmt.Sprint(variable["name"])
		if name != "" && variable["enabled"] != false {
			baseVariables[name] = variable["value"]
			sensitive[name] = variable["sensitive"] == true
		}
	}
	for key, value := range req.TemporaryVariables {
		baseVariables[key] = value
	}
	stepPayloads := []map[string]any{}
	for _, step := range draft.Steps {
		interfaceVersion, err := s.repo.GetInterfaceVersion(ctx, step.InterfaceID, step.InterfaceVersion)
		if err != nil {
			return nil, fmt.Errorf("步骤 %s 引用的接口版本不存在", step.Name)
		}
		var snapshot model.APIInterfaceRequest
		if json.Unmarshal(interfaceVersion.Snapshot, &snapshot) != nil {
			return nil, fmt.Errorf("步骤 %s 的接口版本无效", step.Name)
		}
		stepProjectID, err := s.repo.ProductProject(ctx, snapshot.ProductID)
		if err != nil || stepProjectID != item.ProjectID {
			return nil, fmt.Errorf("步骤 %s 引用了其他项目接口", step.Name)
		}
		globals, err := s.repo.ResolveAPIGlobalVariables(ctx, item.ProjectID, snapshot.ProductID, req.EnvName)
		if err != nil {
			return nil, errors.New("读取接口全局变量失败")
		}
		for _, variable := range globals {
			value := variable.Value
			if strings.HasPrefix(value, encryptedAPISecretPrefix) {
				value, err = decryptAPISecret(s.secretKey, value)
				if err != nil {
					return nil, fmt.Errorf("变量 %s 解密失败", variable.Name)
				}
			}
			baseVariables[variable.Name] = typedAPIGlobalValue(variable.ValueType, value)
			sensitive[variable.Name] = variable.Sensitive || variable.ValueType == "secret"
		}
		request, extractors, assertions, err := s.buildAPITestStepTemplate(ctx, item.ProjectID, snapshot, req.EnvName, step.Overrides)
		if err != nil {
			return nil, fmt.Errorf("步骤 %s：%w", step.Name, err)
		}
		stepPayloads = append(stepPayloads, map[string]any{
			"id": step.ID, "name": step.Name, "key": step.Key, "enabled": step.Enabled,
			"phase": step.Phase, "condition": step.Condition, "failurePolicy": step.FailurePolicy,
			"request": request, "extractors": extractors, "assertions": assertions,
		})
	}
	sensitiveNames := []string{}
	for name, value := range sensitive {
		if value {
			sensitiveNames = append(sensitiveNames, name)
		}
	}
	datasets := draft.Datasets
	if len(datasets) == 0 {
		datasets = []model.APITestDataset{{Name: "默认实例", Enabled: true, Values: map[string]any{}}}
	}
	payloads := []map[string]any{}
	for _, dataset := range datasets {
		if !dataset.Enabled {
			continue
		}
		variables := map[string]any{}
		for key, value := range baseVariables {
			variables[key] = value
		}
		for key, value := range dataset.Values {
			variables[key] = value
		}
		payloads = append(payloads, map[string]any{
			"caseId": item.ID, "caseVersion": version.Version, "caseName": item.Name,
			"datasetName": dataset.Name, "variables": variables, "sensitiveVariables": sensitiveNames,
			"steps": stepPayloads,
		})
	}
	if len(payloads) == 0 {
		return nil, errors.New("已启用参数化但没有启用的数据集")
	}
	return payloads, nil
}

func (s *APIAutomationService) buildAPITestStepTemplate(ctx context.Context, projectID int64, snapshot model.APIInterfaceRequest, envName string, overrides json.RawMessage) (map[string]any, []map[string]any, []map[string]any, error) {
	target := strings.TrimSpace(snapshot.Path)
	parsed, _ := url.Parse(target)
	if parsed == nil || !parsed.IsAbs() {
		testObject, err := s.repo.TestObjectByEnvironment(ctx, snapshot.ProductID, envName)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("产品 %d 缺少环境 %s", snapshot.ProductID, envName)
		}
		target = strings.TrimRight(testObject.Target, "/") + "/" + strings.TrimLeft(target, "/")
		parsed, _ = url.Parse(target)
	}
	var config map[string]any
	if json.Unmarshal(snapshot.Configuration, &config) != nil {
		return nil, nil, nil, errors.New("接口配置无效")
	}
	var overrideMap map[string]any
	_ = json.Unmarshal(overrides, &overrideMap)
	for key, value := range overrideMap {
		config[key] = value
	}
	headers := map[string]string{}
	projectHeaders, err := s.repo.ListProjectHeaders(ctx, projectID, "")
	if err != nil {
		return nil, nil, nil, errors.New("读取项目默认请求头失败")
	}
	for _, header := range projectHeaders {
		if header.Enabled {
			setAPIHeader(headers, header.Name, header.Value)
		}
	}
	interfaceHeaders, err := stringMap(config["headers"])
	if err != nil {
		return nil, nil, nil, errors.New("请求头必须是 JSON 对象")
	}
	for key, value := range interfaceHeaders {
		setAPIHeader(headers, key, value)
	}
	params, err := stringMap(config["params"])
	if err != nil {
		return nil, nil, nil, errors.New("查询参数必须是 JSON 对象")
	}
	query := parsed.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	parsed.RawQuery = strings.ReplaceAll(strings.ReplaceAll(query.Encode(), "%24%7B", "${"), "%7D", "}")
	auth, _ := config["auth"].(map[string]any)
	if err := s.revealAPIAuth(auth); err != nil {
		return nil, nil, nil, err
	}
	switch strings.ToLower(fmt.Sprint(auth["type"])) {
	case "bearer":
		setAPIHeader(headers, "Authorization", "Bearer "+fmt.Sprint(auth["token"]))
	case "api_key":
		setAPIHeader(headers, fmt.Sprint(auth["key"]), fmt.Sprint(auth["value"]))
	}
	body := ""
	if value, exists := config["body"]; exists && value != nil {
		body = fmt.Sprint(value)
	}
	mergedConfiguration, err := json.Marshal(config)
	if err != nil {
		return nil, nil, nil, errors.New("合并接口配置失败")
	}
	extractors, assertions, err := parseAPIProcessingConfiguration(mergedConfiguration)
	if err != nil {
		return nil, nil, nil, err
	}
	request := map[string]any{
		"method": strings.ToUpper(snapshot.Method), "url": parsed.String(), "headers": headers, "body": body,
		"timeoutSeconds": snapshot.TimeoutSeconds, "maxResponseBytes": 20 << 20,
	}
	return request, extractors, assertions, nil
}

func typedAPIGlobalValue(valueType, value string) any {
	switch valueType {
	case "number", "boolean", "json":
		var result any
		if json.Unmarshal([]byte(value), &result) == nil {
			return result
		}
	}
	return value
}

func (s *APIAutomationService) dispatchAPITestInstances(ctx context.Context, batchID string, concurrency int, requestedExecutor string) {
	instances, err := s.repo.ListAPITestRunInstances(ctx, batchID)
	if err != nil {
		return
	}
	active := 0
	for _, item := range instances {
		if item.Status == "running" {
			active++
		}
	}
	for _, item := range instances {
		if active >= concurrency || item.Status != "queued" {
			continue
		}
		executor, err := s.pickRunExecutor(ctx, requestedExecutor)
		if err != nil {
			return
		}
		var payload map[string]any
		if json.Unmarshal(item.Snapshot, &payload) != nil {
			_, _ = s.repo.CompleteAPITestRunInstance(ctx, item.TaskID, "failed", json.RawMessage(`{}`), "执行快照无效")
			continue
		}
		callback := s.callbackBase + "/api/api-automation/test-runs/tasks/" + item.TaskID + "/callback"
		if err := submitAPIExecutorTask(ctx, executor.Endpoint, item.TaskID, "api_case", payload, callback); err != nil {
			_, _ = s.repo.CompleteAPITestRunInstance(ctx, item.TaskID, "failed", json.RawMessage(`{}`), "任务下发失败")
			continue
		}
		_ = s.repo.MarkAPITestRunDispatched(ctx, item.TaskID, executor.ExecutorID)
		active++
	}
}

func (s *APIAutomationService) pickRunExecutor(ctx context.Context, executorID string) (model.ExecutorView, error) {
	if executorID == "" {
		return s.pickAPIExecutor(ctx)
	}
	executor, err := s.executorRepo.GetByID(ctx, executorID)
	_, _, _, offlineSeconds := s.apiRunPolicy(ctx)
	if err != nil || executor.Endpoint == "" || time.Since(executor.LastHeartbeatAt) > time.Duration(offlineSeconds)*time.Second {
		return executor, errors.New("指定执行器不可用")
	}
	return executor, nil
}

func (s *APIAutomationService) apiRunPolicy(ctx context.Context) (int, int, int, int) {
	defaultConcurrency, maxConcurrency, batchSize, offlineSeconds := 5, 100, 1000, 45
	if s.systemRepo == nil {
		return defaultConcurrency, maxConcurrency, batchSize, offlineSeconds
	}
	item, err := s.systemRepo.GetSystemSettingGroup(ctx, "execution")
	if err != nil {
		return defaultConcurrency, maxConcurrency, batchSize, offlineSeconds
	}
	var value map[string]any
	if json.Unmarshal(item.Value, &value) != nil {
		return defaultConcurrency, maxConcurrency, batchSize, offlineSeconds
	}
	if configured := settingInt(value, "defaultConcurrency"); configured > 0 {
		defaultConcurrency = configured
	}
	if configured := settingInt(value, "maxConcurrency"); configured > 0 {
		maxConcurrency = configured
	}
	if configured := settingInt(value, "batchSize"); configured > 0 {
		batchSize = configured
	}
	if configured := settingInt(value, "executorOfflineSeconds"); configured > 0 {
		offlineSeconds = configured
	}
	return defaultConcurrency, maxConcurrency, batchSize, offlineSeconds
}

func submitAPIExecutorTask(ctx context.Context, endpoint, taskID, taskType string, payload map[string]any, callbackURL string) error {
	body, _ := json.Marshal(map[string]any{
		"taskId": taskID, "type": taskType, "payload": payload, "callbackUrl": callbackURL,
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/tasks", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("执行器返回状态码 %d", response.StatusCode)
	}
	return nil
}

func (s *APIAutomationService) CompleteAPITestRun(ctx context.Context, taskID, status string, result json.RawMessage, errorMessage string) error {
	if status != "success" && status != "canceled" {
		status = "failed"
	}
	batchID, err := s.repo.CompleteAPITestRunInstance(ctx, taskID, status, result, errorMessage)
	if err != nil {
		return err
	}
	batch, err := s.repo.GetAPITestRunBatchInternal(ctx, batchID)
	if err == nil {
		var options model.APITestRunStartRequest
		_ = json.Unmarshal(batch.Options, &options)
		s.dispatchAPITestInstances(context.Background(), batchID, options.Concurrency, options.ExecutorID)
	}
	return nil
}

func (s *APIAutomationService) GetAPITestRun(ctx context.Context, userID int64, batchID string) (map[string]any, error) {
	batch, err := s.repo.GetAPITestRunBatch(ctx, userID, batchID)
	if err != nil {
		return nil, errors.New("执行批次不存在或无权访问")
	}
	instances, err := s.repo.ListAPITestRunInstances(ctx, batchID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"batch": batch, "instances": instances}, nil
}
