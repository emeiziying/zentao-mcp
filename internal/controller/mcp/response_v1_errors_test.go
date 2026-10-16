package mcp

import "testing"

func TestV1BusinessErrorsAreNotSuccessfulTools(t *testing.T) {
	for _, body := range []string{`{"status":"fail","message":"denied"}`, `{"status":"failed","message":"denied"}`, `{"status":"error"}`, `{"result":"fail"}`, `{"result":false}`, `{"error":"not authorized"}`, `{"error":{"name":["required"]}}`} {
		if !responseFailed(200, []byte(body), true) {
			t.Errorf("business failure accepted: %s", body)
		}
	}
	for _, body := range []string{`{"status":"doing","id":42}`, `{"code":401,"name":"domain code"}`, `{"result":true}`, `{"error":""}`, `{"tasks":[]}`, `{"status":"success","data":[]}`} {
		if responseFailed(200, []byte(body), true) {
			t.Errorf("valid business result rejected: %s", body)
		}
	}
}
