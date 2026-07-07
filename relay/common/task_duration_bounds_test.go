package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTaskDurationBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newContext := func(body string) (*gin.Context, *RelayInfo) {
		request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = request
		return context, &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}
	}

	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "huge duration is rejected",
			body:    `{"model":"sora-2","prompt":"a cat","duration":9999999999}`,
			wantErr: true,
		},
		{
			name:    "huge seconds string is rejected",
			body:    `{"model":"sora-2","prompt":"a cat","seconds":"9999999999"}`,
			wantErr: true,
		},
		{
			name:    "negative duration is rejected",
			body:    `{"model":"sora-2","prompt":"a cat","duration":-8}`,
			wantErr: true,
		},
		{
			name:    "huge metadata durationSeconds is rejected",
			body:    `{"model":"veo-3.1-generate-preview","prompt":"a cat","metadata":{"durationSeconds":9999999999}}`,
			wantErr: true,
		},
		{
			name:    "huge metadata duration is rejected",
			body:    `{"model":"wan2.6-i2v","prompt":"a cat","metadata":{"duration":9999999999}}`,
			wantErr: true,
		},
		{
			name: "normal duration is accepted",
			body: `{"model":"sora-2","prompt":"a cat","seconds":"8"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" multipart direct", func(t *testing.T) {
			context, info := newContext(tt.body)
			taskErr := ValidateMultipartDirect(context, info)
			if tt.wantErr {
				require.NotNil(t, taskErr)
				require.Equal(t, "invalid_seconds", taskErr.Code)
				return
			}
			require.Nil(t, taskErr)
		})

		t.Run(tt.name+" basic task request", func(t *testing.T) {
			context, info := newContext(tt.body)
			taskErr := ValidateBasicTaskRequest(context, info, constant.TaskActionGenerate)
			if tt.wantErr {
				require.NotNil(t, taskErr)
				require.Equal(t, "invalid_seconds", taskErr.Code)
				return
			}
			require.Nil(t, taskErr)
		})
	}
}
