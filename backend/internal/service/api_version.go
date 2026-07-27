package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"synapseqa/backend/internal/model"
)

func (s *APIAutomationService) ListInterfaceVersions(ctx context.Context, userID, interfaceID int64) ([]model.APIInterfaceVersion, error) {
	if _, err := s.GetInterface(ctx, userID, interfaceID); err != nil {
		return nil, err
	}
	items, err := s.repo.ListInterfaceVersions(ctx, interfaceID)
	if err != nil {
		return nil, errors.New("读取接口版本失败")
	}
	for index := range items {
		items[index].Snapshot = maskVersionSnapshot(items[index].Snapshot)
	}
	return items, nil
}

func (s *APIAutomationService) GetInterfaceVersion(ctx context.Context, userID, interfaceID int64, version int) (model.APIInterfaceVersion, error) {
	if _, err := s.GetInterface(ctx, userID, interfaceID); err != nil {
		return model.APIInterfaceVersion{}, err
	}
	item, err := s.repo.GetInterfaceVersion(ctx, interfaceID, version)
	if err != nil {
		return item, errors.New("接口版本不存在")
	}
	item.Snapshot = maskVersionSnapshot(item.Snapshot)
	return item, nil
}

func (s *APIAutomationService) DiffInterfaceVersions(ctx context.Context, userID, interfaceID int64, sourceVersion, targetVersion int) (model.APIInterfaceVersionDiff, error) {
	source, err := s.GetInterfaceVersion(ctx, userID, interfaceID, sourceVersion)
	if err != nil {
		return model.APIInterfaceVersionDiff{}, err
	}
	target, err := s.GetInterfaceVersion(ctx, userID, interfaceID, targetVersion)
	if err != nil {
		return model.APIInterfaceVersionDiff{}, err
	}
	var left, right map[string]any
	if json.Unmarshal(source.Snapshot, &left) != nil || json.Unmarshal(target.Snapshot, &right) != nil {
		return model.APIInterfaceVersionDiff{}, errors.New("接口版本快照无效")
	}
	changes := map[string]map[string]any{}
	for key, oldValue := range left {
		if !reflect.DeepEqual(oldValue, right[key]) {
			changes[key] = map[string]any{"before": oldValue, "after": right[key]}
		}
	}
	for key, newValue := range right {
		if _, exists := left[key]; !exists {
			changes[key] = map[string]any{"before": nil, "after": newValue}
		}
	}
	return model.APIInterfaceVersionDiff{SourceVersion: sourceVersion, TargetVersion: targetVersion, Changes: changes}, nil
}

func (s *APIAutomationService) RestoreInterfaceVersion(ctx context.Context, userID int64, actor string, interfaceID int64, sourceVersion int) (int, error) {
	current, err := s.repo.GetInterface(ctx, userID, interfaceID)
	if err != nil || !s.repo.CanAccessProject(ctx, userID, current.ProjectID) {
		return 0, errors.New("接口不存在或无权访问")
	}
	version, err := s.repo.GetInterfaceVersion(ctx, interfaceID, sourceVersion)
	if err != nil {
		return 0, errors.New("接口版本不存在")
	}
	var req model.APIInterfaceRequest
	if err := json.Unmarshal(version.Snapshot, &req); err != nil {
		return 0, errors.New("接口版本快照无效")
	}
	req.Revision = current.Revision
	req, normalized, projectID, err := s.normalizeInterface(ctx, req)
	if err != nil {
		return 0, err
	}
	if !s.repo.CanAccessProject(ctx, userID, projectID) {
		return 0, errors.New("无权访问版本所属项目")
	}
	newVersion, err := s.repo.RestoreInterfaceVersion(ctx, interfaceID, req, normalized, actor, sourceVersion)
	if err != nil {
		return 0, errors.New("回滚失败，接口可能已被其他用户修改")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "回滚接口版本", req.Name)
	return newVersion, nil
}

func maskVersionSnapshot(raw json.RawMessage) json.RawMessage {
	var snapshot model.APIInterfaceRequest
	if json.Unmarshal(raw, &snapshot) != nil {
		return raw
	}
	snapshot.Configuration = maskAPIConfiguration(snapshot.Configuration)
	masked, err := json.Marshal(snapshot)
	if err != nil {
		return raw
	}
	return masked
}
