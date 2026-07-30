package service

import (
	"encoding/json"
	"strings"
	"testing"

	"synapseqa/backend/internal/model"
)

func TestMaskVersionSnapshot(t *testing.T) {
	snapshot, _ := json.Marshal(model.APIInterfaceRequest{
		ProductID:     1,
		Configuration: json.RawMessage(`{"auth":{"type":"bearer","token":"enc:v1:ciphertext"}}`),
	})
	masked := maskVersionSnapshot(snapshot)
	if strings.Contains(string(masked), "enc:v1:ciphertext") || !strings.Contains(string(masked), "******") {
		t.Fatalf("版本快照未脱敏: %s", masked)
	}
}
