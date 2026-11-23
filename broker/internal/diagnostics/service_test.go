package diagnostics

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"k8s-internal-platform/broker/internal/platform"
)

type testStore struct {
	platform.Store
	app      platform.Application
	revision platform.Revision
	reads    int
	change   bool
}

func (s *testStore) GetApplication(context.Context, string) (platform.Application, error) {
	s.reads++
	app := s.app
	if s.change {
		app.Version += int64(s.reads)
	}
	return app, nil
}
func (s *testStore) GetRevision(context.Context, string) (platform.Revision, error) {
	return s.revision, nil
}

type testReader struct {
	values    [][]byte
	valueErr  error
	logs      string
	events    platform.EventResult
	logReads  int
	limit     int64
	truncated bool
}

func (r *testReader) RedactionValues(context.Context) ([][]byte, error) { return r.values, r.valueErr }
func (r *testReader) ReadLogs(_ context.Context, _ string, _ platform.LogOptions, limit int64) (platform.LogResult, error) {
	r.logReads++
	r.limit = limit
	return platform.LogResult{Pod: "owned-pod", Container: "application", Text: r.logs, Truncated: r.truncated}, nil
}
func (r *testReader) ReadEvents(context.Context, string, int) (platform.EventResult, error) {
	return r.events, nil
}
func (*testReader) CompareManifest(context.Context, string, string) ([]platform.DriftDifference, error) {
	return []platform.DriftDifference{}, nil
}

type testRenderer struct{}

func (testRenderer) RenderExecution(string, platform.Revision) (string, error) {
	return "manifest", nil
}
func newServiceFixture() (*Service, *testStore, *testReader) {
	store := &testStore{app: platform.Application{ID: "app_1", Version: 1, DesiredRevisionID: "rev_1"}, revision: platform.Revision{ID: "rev_1", AppID: "app_1", Frozen: &platform.FrozenSpec{ChartArtifactDigest: "chart", ValuesHash: "values"}}}
	reader := &testReader{}
	return New(store, reader, testRenderer{}), store, reader
}

func TestLogsMaskKnownEncodingsBeforeClipping(t *testing.T) {
	s, _, reader := newServiceFixture()
	secret := "private-configuration/+ with newline\nvalue"
	reader.values = [][]byte{[]byte(secret)}
	quoted, _ := json.Marshal(secret)
	encoded := []string{secret, base64.StdEncoding.EncodeToString([]byte(secret)), base64.RawURLEncoding.EncodeToString([]byte(secret)), url.QueryEscape(secret), string(quoted[1 : len(quoted)-1])}
	reader.logs = "normal-log\n" + strings.Join(encoded, "\n") + "\n" + strings.Repeat("z", platform.MaxLogBytes)
	result, err := s.Logs(context.Background(), "app_1", platform.LogOptions{TailLines: 100, SinceSeconds: 3600})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range encoded {
		if strings.Contains(result.Text, value) {
			t.Fatal("known secret encoding was exposed")
		}
	}
	if !result.Truncated || len(result.Text) > platform.MaxLogBytes || !strings.Contains(result.Text, "normal-log") || reader.limit <= platform.MaxLogBytes {
		t.Fatal("log bounds/overlap were not enforced")
	}
	reader.logs = strings.Repeat("z", platform.MaxLogBytes-10) + secret
	result, err = s.Logs(context.Background(), "app_1", platform.LogOptions{TailLines: 100, SinceSeconds: 3600})
	if err != nil || strings.Contains(result.Text, "private-co") {
		t.Fatalf("boundary exposed a secret prefix: %v", err)
	}
}

func TestRedactorMergesOverlapsAndMasksTruncatedSuffix(t *testing.T) {
	s, _, r := newServiceFixture()
	r.values = [][]byte{[]byte("alpha-bravo"), []byte("bravo-charlie")}
	mask, err := s.redactor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		input     string
		truncated bool
		want      string
	}{
		{"alpha-bravo-charlie", false, "[REDACTED]"},
		{"safe alpha-bra", true, "safe [REDACTED]"},
		{"safe alpha-bravo", true, "safe [REDACTED]"},
	} {
		got, err := mask.mask(test.input, test.truncated)
		if err != nil || got != test.want {
			t.Fatalf("redaction=%q error=%v", got, err)
		}
	}
	r.values = [][]byte{[]byte("x")}
	mask, _ = s.redactor(context.Background())
	if _, err := mask.mask(strings.Repeat("x", 9000), false); err == nil {
		t.Fatal("unbounded match count accepted")
	}
	for _, test := range []struct {
		text, pattern string
		want          int
	}{{"aaaaaaaaab", "aaaaab", 0}, {"safe abcab", "abcabc", 5}, {"abcabc", "abcabc", 0}} {
		if got := suffixPrefix(test.text, test.pattern); got != test.want {
			t.Fatalf("suffix=%d want %d", got, test.want)
		}
	}
}

func TestRedactionHonorsCancelledRequestWithoutReturningOutput(t *testing.T) {
	s, _, reader := newServiceFixture()
	reader.values = [][]byte{[]byte("private-value")}
	mask, err := s.redactor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if text, err := mask.maskContext(ctx, "private-value", true); err == nil || text != "" {
		t.Fatal("cancelled redaction returned output")
	}
}

func TestLogsFailClosedBeforeReadingWorkloadOutput(t *testing.T) {
	s, store, r := newServiceFixture()
	r.valueErr = fmt.Errorf("private injected failure")
	_, err := s.Logs(context.Background(), "app_1", platform.LogOptions{TailLines: 100, SinceSeconds: 1})
	if err == nil || strings.Contains(err.Error(), "private injected") || r.logReads != 0 {
		t.Fatal("secret read failure was not safely handled")
	}
	r.valueErr = nil
	r.values = [][]byte{make([]byte, 512*1024+1)}
	if _, err = s.Logs(context.Background(), "app_1", platform.LogOptions{TailLines: 1, SinceSeconds: 1}); err == nil || r.logReads != 0 {
		t.Fatal("oversized redaction accepted")
	}
	store.app.Deleted = true
	_, err = s.Logs(context.Background(), "app_1", platform.LogOptions{TailLines: 1, SinceSeconds: 1})
	var domain *platform.Error
	if !errors.As(err, &domain) || domain.Code != platform.CodeNotFound {
		t.Fatal("deleted application diagnostics were read")
	}
}

func TestEventLimitsAndUTF8(t *testing.T) {
	s, _, r := newServiceFixture()
	r.values = [][]byte{[]byte("fixture-sensitive-value")}
	for range 110 {
		r.events.Items = append(r.events.Items, platform.WorkloadEvent{Message: strings.Repeat("ą", 5000) + "fixture-sensitive-value", Reason: "fixture-sensitive-value"})
	}
	result, err := s.Events(context.Background(), "app_1", 100)
	if err != nil {
		t.Fatal(err)
	}
	bytes := 0
	for _, item := range result.Items {
		bytes += len(item.Message) + len(item.Reason) + len(item.Type) + len(item.Resource)
		if strings.Contains(item.Message+item.Reason, "fixture-sensitive-value") || !utf8.ValidString(item.Message) {
			t.Fatal("event data was not safely masked/bounded")
		}
	}
	if len(result.Items) != 100 || bytes > platform.MaxLogBytes || !result.Truncated {
		t.Fatal("event limits were not enforced")
	}
}

func TestDriftRequiresStableCompleteApplicationIntent(t *testing.T) {
	s, store, _ := newServiceFixture()
	report, err := s.Drift(context.Background(), "app_1")
	if err != nil || report.State != "in_sync" || report.RevisionID != "rev_1" {
		t.Fatalf("drift=%+v error=%v", report, err)
	}
	store.change = true
	if _, err = s.Drift(context.Background(), "app_1"); err == nil {
		t.Fatal("racing application intent reported as current")
	}
	store.change = false
	store.revision.Frozen = nil
	if _, err = s.Drift(context.Background(), "app_1"); err == nil {
		t.Fatal("unfrozen intent rendered")
	}
	store.revision.AppID = "other-app"
	if _, err = s.Drift(context.Background(), "app_1"); err == nil {
		t.Fatal("foreign revision rendered")
	}
}

func TestDiagnosticConcurrencyBound(t *testing.T) {
	s, _, _ := newServiceFixture()
	s.gate <- struct{}{}
	s.gate <- struct{}{}
	if _, err := s.Drift(context.Background(), "app_1"); err == nil {
		t.Fatal("diagnostic concurrency bound ignored")
	}
}
