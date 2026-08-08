package main

import (
	"os"
	"testing"
	"time"

	"github.com/evanw/esbuild/internal/helpers"
)

func TestHandleBuildRequestReleasesActiveBuildAfterError(t *testing.T) {
	absWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name            string
		modifyRequest   func(map[string]interface{})
		expectedError   string
		expectedMessage string
	}{
		{
			name: "InvalidPluginRegExp",
			modifyRequest: func(request map[string]interface{}) {
				request["plugins"] = []interface{}{map[string]interface{}{
					"name":  "test",
					"onEnd": false,
					"onResolve": []interface{}{map[string]interface{}{
						"id":        0,
						"filter":    "x(?=y)",
						"namespace": "",
					}},
					"onLoad": []interface{}{},
				}}
			},
			expectedError: "[test] \"onResolve\" filter is not a valid Go regular expression: \"x(?=y)\"",
		},
		{
			name: "InvalidContextOptions",
			modifyRequest: func(request map[string]interface{}) {
				request["context"] = true
				request["flags"] = []interface{}{"--external:test"}
			},
			expectedMessage: "Cannot use \"external\" without \"bundle\"",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := serviceType{
				activeBuilds:       make(map[int]*activeBuild),
				keepAliveWaitGroup: helpers.MakeThreadSafeWaitGroup(),
			}
			request := map[string]interface{}{
				"context":       false,
				"key":           0,
				"write":         false,
				"entries":       []interface{}{},
				"flags":         []interface{}{},
				"absWorkingDir": absWorkingDir,
				"nodePaths":     []interface{}{},
			}
			test.modifyRequest(request)

			encoded := service.handleBuildRequest(0, request)
			bytes, _, ok := readLengthPrefixedSlice(encoded)
			if !ok {
				t.Fatal("Invalid encoded response")
			}
			response, ok := decodePacket(bytes)
			if !ok {
				t.Fatal("Invalid response packet")
			}
			value := response.value.(map[string]interface{})
			if test.expectedError != "" && value["error"] != test.expectedError {
				t.Fatalf("Expected error %q, got %q", test.expectedError, value["error"])
			}
			if test.expectedMessage != "" {
				errors := value["errors"].([]interface{})
				if len(errors) != 1 || errors[0].(map[string]interface{})["text"] != test.expectedMessage {
					t.Fatalf("Expected message %q, got %v", test.expectedMessage, errors)
				}
			}

			if service.getActiveBuild(0) != nil {
				t.Fatal("Active build was not released")
			}

			done := make(chan struct{})
			go func() {
				service.keepAliveWaitGroup.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("Keep-alive wait group did not reach zero")
			}
		})
	}
}
