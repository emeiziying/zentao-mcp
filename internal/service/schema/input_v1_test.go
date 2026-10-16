package schema

import (
	"context"
	"github.com/getkin/kin-openapi/openapi3"
	"testing"
)

func TestV1RequiredBodyAndPathValidation(t *testing.T) {
	body := openapi3.NewObjectSchema().WithProperty("name", openapi3.NewStringSchema())
	body.Required = []string{"name"}
	op := &openapi3.Operation{RequestBody: &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.Content{"application/json": &openapi3.MediaType{Schema: body.NewRef()}}}}}
	path := &openapi3.PathItem{Parameters: openapi3.Parameters{&openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "taskID", In: "path", Required: true, Schema: openapi3.NewIntegerSchema().NewRef()}}}}
	resolved := resolveInputJSONSchema(t, New(nil).buildToolInputSchema(context.Background(), &openapi3.T{}, op, path))
	for _, input := range []map[string]any{{"taskID": float64(42)}, {"payload": map[string]any{"name": "任务"}}, {"taskID": float64(42), "payload": map[string]any{}}} {
		if err := resolved.Validate(input); err == nil {
			t.Errorf("invalid write accepted: %v", input)
		}
	}
	if err := resolved.Validate(map[string]any{"taskID": float64(42), "payload": map[string]any{"name": "任务"}}); err != nil {
		t.Fatal(err)
	}
}
