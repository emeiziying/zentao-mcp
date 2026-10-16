package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestV1NestedPayloadSecretsRedactedWithoutMutation(t *testing.T) {
	payload := map[string]any{"name": "任务", "password": "fake-password", "credentials": []any{map[string]any{"Token": "fake-token", "Authorization": "fake-authorization"}}}
	input := map[string]any{"payload": payload, "page": 2}
	encoded, err := json.Marshal(redactInput(input))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fake-password", "fake-token", "fake-authorization"} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("nested secret leaked: %s", secret)
		}
	}
	if !strings.Contains(string(encoded), "任务") {
		t.Error("nonsecret payload lost")
	}
	if payload["password"] != "fake-password" || payload["credentials"].([]any)[0].(map[string]any)["Token"] != "fake-token" {
		t.Error("redaction mutated real request payload")
	}
}
