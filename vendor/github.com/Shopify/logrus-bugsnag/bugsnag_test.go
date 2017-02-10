package logrus_bugsnag

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/bugsnag/bugsnag-go"
)

type stackFrame struct {
	Method     string `json:"method"`
	File       string `json:"file"`
	LineNumber int    `json:"lineNumber"`
}

type exception struct {
	Message    string       `json:"message"`
	Stacktrace []stackFrame `json:"stacktrace"`
}

type event struct {
	Exceptions []exception      `json:"exceptions"`
	Metadata   bugsnag.MetaData `json:"metaData"`
}

type notice struct {
	Events []event `json:"events"`
}

func TestNoticeReceived(t *testing.T) {
	c := make(chan event, 1)
	expectedMessage := "foo"
	expectedMetadataLen := 3
	expectedFields := []string{"animal", "size", "omg"}
	expectedValues := []interface{}{"walrus", float64(9009), true}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var notice notice
		data, _ := ioutil.ReadAll(r.Body)
		if err := json.Unmarshal(data, &notice); err != nil {
			t.Error(err)
		}
		r.Body.Close()

		c <- notice.Events[0]
	}))
	defer ts.Close()

	hook := &bugsnagHook{}

	bugsnag.Configure(bugsnag.Configuration{
		Endpoint:     ts.URL,
		ReleaseStage: "production",
		APIKey:       "12345678901234567890123456789012",
		Synchronous:  true,
	})

	log := logrus.New()
	log.Hooks.Add(hook)

	log.WithFields(logrus.Fields{
		"error":  errors.New(expectedMessage),
		"animal": "walrus",
		"size":   9009,
		"omg":    true,
	}).Error("Bugsnag will not see this string")

	select {
	case event := <-c:
		exception := event.Exceptions[0]
		if exception.Message != expectedMessage {
			t.Errorf("Unexpected message received: got %q, expected %q", exception.Message, expectedMessage)
		}

		if len(exception.Stacktrace) < 1 {
			t.Error("Bugsnag error does not have a stack trace")
		}

		metadata, ok := event.Metadata["metadata"]
		if !ok {
			t.Error("Expected a Metadata field to be present in the bugsnag metadata")
		}

		if ok && len(metadata) != expectedMetadataLen {
			t.Error("Unexpected metadata length, got %d, expected %d", len(metadata), expectedMetadataLen)
		}

		for idx, field := range expectedFields {
			val, ok := metadata[field]
			if !ok {
				t.Errorf("Expected field %q not found", field)
			}

			if val != expectedValues[idx] {
				t.Errorf("For field %q, found value %v, expected value %v", field, val, expectedValues[idx])
			}
		}

		topFrame := exception.Stacktrace[0]
		if topFrame.Method != "TestNoticeReceived" {
			t.Errorf("Unexpected method on top of call stack: got %q, expected %q", topFrame.Method,
				"TestNoticeReceived")
		}

	case <-time.After(time.Second):
		t.Error("Timed out; no notice received by Bugsnag API")
	}
}
