package service

import (
	"encoding/json"
	"testing"
)

func TestValidateSystemSetting(t *testing.T) {
	tests := []struct {
		name     string
		groupKey string
		value    string
		wantErr  bool
	}{
		{name: "有效执行策略", groupKey: "execution", value: `{"defaultConcurrency":5,"maxConcurrency":20,"batchSize":1000,"executorOfflineSeconds":45}`},
		{name: "默认并发不能超过上限", groupKey: "execution", value: `{"defaultConcurrency":21,"maxConcurrency":20,"batchSize":1000,"executorOfflineSeconds":45}`, wantErr: true},
		{name: "有效安全策略", groupKey: "security", value: `{"sessionHours":24,"passwordMinLength":8,"loginFailureLimit":5,"lockMinutes":15}`},
		{name: "密码长度不能低于八位", groupKey: "security", value: `{"sessionHours":24,"passwordMinLength":7,"loginFailureLimit":5,"lockMinutes":15}`, wantErr: true},
		{name: "通知强制项自动开启", groupKey: "notification", value: `{"executionSuccess":false,"executionFailure":true,"executorOffline":true,"systemAlert":false,"securityAlert":false}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := validateSystemSetting(test.groupKey, json.RawMessage(test.value))
			if (err != nil) != test.wantErr {
				t.Fatalf("validateSystemSetting() error = %v, wantErr %v", err, test.wantErr)
			}
			if test.groupKey == "notification" && err == nil {
				var value map[string]bool
				if json.Unmarshal(result, &value) != nil || !value["systemAlert"] || !value["securityAlert"] {
					t.Fatalf("通知强制项未保持开启：%s", result)
				}
			}
		})
	}
}
