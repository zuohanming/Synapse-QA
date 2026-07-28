package service

import (
	"encoding/json"
	"testing"
)

func TestParseAPIProcessingConfiguration(t *testing.T) {
	extractors, assertions, err := parseAPIProcessingConfiguration(json.RawMessage(`{
		"jsonpath":"[{\"name\":\"user_id\",\"expression\":\"$.data.id\"}]",
		"regex":[{"name":"token","expression":"token=(.+)"}],
		"assertions":"[{\"type\":\"status\",\"operator\":\"equals\",\"expected\":200}]"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(extractors) != 2 || extractors[0]["type"] != "jsonpath" || extractors[1]["type"] != "regex" {
		t.Fatalf("提取配置解析错误: %#v", extractors)
	}
	if len(assertions) != 1 {
		t.Fatalf("断言配置解析错误: %#v", assertions)
	}
}

func TestParseAPIProcessingConfigurationRejectsObject(t *testing.T) {
	if _, _, err := parseAPIProcessingConfiguration(json.RawMessage(`{"assertions":{}}`)); err == nil {
		t.Fatal("断言配置不是数组时应返回错误")
	}
}
