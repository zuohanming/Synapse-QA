package service

import (
	"testing"
)

func TestResolveAPIVariables(t *testing.T) {
	result, err := resolveAPIVariables("/users/${userId}?role=${role}", map[string]any{
		"userId": 1001,
		"role":   "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != "/users/1001?role=admin" {
		t.Fatalf("变量解析结果不正确: %s", result)
	}
}

func TestResolveAPIVariablesRejectsMissingAndCycle(t *testing.T) {
	if _, err := resolveAPIVariables("${missing}", map[string]any{}); err == nil {
		t.Fatal("缺失变量应返回错误")
	}
	if _, err := resolveAPIVariables("${a}", map[string]any{"a": "${b}", "b": "${a}"}); err == nil {
		t.Fatal("循环变量应返回错误")
	}
}

func TestSetAPIHeaderPrecedenceAndRestrictions(t *testing.T) {
	headers := map[string]string{"authorization": "old"}
	setAPIHeader(headers, "Authorization", "new")
	setAPIHeader(headers, "Host", "example.com")
	if headers["authorization"] != "new" {
		t.Fatalf("同名请求头没有按大小写不敏感方式覆盖: %#v", headers)
	}
	if _, exists := headers["Host"]; exists {
		t.Fatal("受限请求头不应进入最终请求")
	}
}

func TestMaskPreviewHeaders(t *testing.T) {
	masked := maskPreviewHeaders(map[string]string{
		"Authorization": "Bearer secret",
		"X-Trace":       "trace",
	})
	if masked["Authorization"] != "******" || masked["X-Trace"] != "trace" {
		t.Fatalf("请求头脱敏结果不正确: %#v", masked)
	}
}

func TestTypedAPIVariable(t *testing.T) {
	number, err := typedAPIVariable("number", "12.5")
	if err != nil || number != 12.5 {
		t.Fatalf("数字变量解析失败: %#v, %v", number, err)
	}
	jsonValue, err := typedAPIVariable("json", `{"enabled":true}`)
	if err != nil || jsonValue.(map[string]any)["enabled"] != true {
		t.Fatalf("JSON 变量解析失败: %#v, %v", jsonValue, err)
	}
}

func TestBuildAPIBody(t *testing.T) {
	body, warnings, err := buildAPIBody("urlencoded", map[string]any{"name": "张三", "role": "admin"})
	if err != nil || body != "name=%E5%BC%A0%E4%B8%89&role=admin" || len(warnings) != 0 {
		t.Fatalf("urlencoded 请求体构建失败: %s, %#v, %v", body, warnings, err)
	}
	body, warnings, err = buildAPIBody("form_data", `{"name":"demo"}`)
	if err != nil || body == "" || len(warnings) != 1 {
		t.Fatalf("form-data 预览构建失败: %s, %#v, %v", body, warnings, err)
	}
}
