package anthropic

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestAuditAnthropicImageMediaOrderAndEncoding(t *testing.T) {
	t.Parallel()
	message := types.NewUserMessage("look")
	message.Media = []types.Media{
		&types.ImageMedia{URL: "https://example.test/image.jpg"},
		&types.ImageMedia{Base64Data: "aW1hZ2U=", MimeType: "image/jpeg"},
		&types.FileReferenceMedia{ID: "file_1", Kind: "document"},
		&types.ImageMedia{Data: []byte("image")},
		&types.ImageMedia{URL: "data:image/webp;base64,aW1hZ2U="},
	}
	got := (&Provider{}).buildContent(message)
	want := []map[string]any{
		{"type": "text", "text": "look"},
		{"type": "image", "source": map[string]any{"type": "url", "url": "https://example.test/image.jpg"}},
		{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/jpeg", "data": "aW1hZ2U="}},
		{"type": "document", "source": map[string]any{"type": "file", "file_id": "file_1"}},
		{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "aW1hZ2U="}},
		{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/webp", "data": "aW1hZ2U="}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("content=%#v", got)
	}
}

func TestAuditAnthropicNestedToolDefinition(t *testing.T) {
	t.Parallel()
	for _, flat := range []bool{false, true} {
		tool := types.Tool{Function: &types.ToolFunction{Name: "nested", Description: "nested description", Parameters: map[string]any{"type": "object"}}}
		wantName, wantDescription := "nested", "nested description"
		if flat {
			tool.Name, tool.Description, tool.InputSchema = "flat", "flat description", map[string]any{}
			wantName, wantDescription = tool.Name, tool.Description
		}
		tools, err := (&Provider{}).transformTools([]types.Tool{tool})
		if err != nil {
			t.Fatal(err)
		}
		got := tools[0]
		if got["name"] != wantName || got["description"] != wantDescription {
			t.Fatal(got)
		}
		var params map[string]any
		if err := json.Unmarshal(got["input_schema"].(json.RawMessage), &params); err != nil {
			t.Fatal(err)
		}
		if (!flat && params["type"] != "object") || (flat && len(params) != 0) {
			t.Fatal(params)
		}
	}
}
