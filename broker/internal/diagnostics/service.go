// Package diagnostics provides bounded, read-only application diagnostics.
package diagnostics

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"k8s-internal-platform/broker/internal/platform"
)

type Service struct {
	store    platform.Store
	reader   platform.DiagnosticReader
	renderer platform.ManifestRenderer
	gate     chan struct{}
}

func New(store platform.Store, reader platform.DiagnosticReader, renderer platform.ManifestRenderer) *Service {
	return &Service{store: store, reader: reader, renderer: renderer, gate: make(chan struct{}, 2)}
}

func (s *Service) enter(ctx context.Context, id string) (platform.Application, func(), error) {
	select {
	case s.gate <- struct{}{}:
	default:
		return platform.Application{}, nil, platform.Errorf(platform.CodeDependencyUnavailable, "diagnostics capacity reached; retry later")
	}
	release := func() { <-s.gate }
	app, err := s.store.GetApplication(ctx, id)
	if err != nil {
		release()
		return app, nil, err
	}
	if app.Deleted {
		release()
		return app, nil, platform.Errorf(platform.CodeNotFound, "application has been deleted")
	}
	return app, release, nil
}

func (s *Service) Logs(ctx context.Context, id string, options platform.LogOptions) (platform.LogResult, error) {
	_, release, err := s.enter(ctx, id)
	if err != nil {
		return platform.LogResult{}, err
	}
	defer release()
	if options.TailLines < 1 || options.TailLines > platform.MaxLogLines || options.SinceSeconds < 1 || options.SinceSeconds > 86400 {
		return platform.LogResult{}, platform.Errorf(platform.CodeInvalidInput, "log limits are outside the supported range")
	}
	mask, err := s.redactor(ctx)
	if err != nil {
		return platform.LogResult{}, err
	}
	// Read overlap so truncation cannot expose a prefix of an otherwise complete
	// known secret that straddles the returned-text boundary.
	result, err := s.reader.ReadLogs(ctx, id, options, int64(platform.MaxLogBytes+mask.overlap+1))
	if err != nil {
		return platform.LogResult{}, err
	}
	text, err := mask.mask(result.Text, result.Truncated)
	if err != nil {
		return platform.LogResult{}, err
	}
	text, clipped := bounded(text, platform.MaxLogBytes)
	result.Text = text
	result.Pod = mask.replace(result.Pod)
	result.Container = mask.replace(result.Container)
	result.Truncated = result.Truncated || clipped
	return result, nil
}

func (s *Service) Events(ctx context.Context, id string, limit int) (platform.EventResult, error) {
	_, release, err := s.enter(ctx, id)
	if err != nil {
		return platform.EventResult{}, err
	}
	defer release()
	if limit < 1 || limit > platform.MaxDiagnosticEvents {
		return platform.EventResult{}, platform.Errorf(platform.CodeInvalidInput, "event limit must be between 1 and 100")
	}
	mask, err := s.redactor(ctx)
	if err != nil {
		return platform.EventResult{}, err
	}
	result, err := s.reader.ReadEvents(ctx, id, limit)
	if err != nil {
		return platform.EventResult{}, err
	}
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		result.Truncated = true
	}
	budget := platform.MaxLogBytes
	for i := range result.Items {
		event := &result.Items[i]
		for _, field := range []*string{&event.Message, &event.Reason, &event.Type, &event.Resource} {
			masked, err := mask.mask(*field, false)
			if err != nil {
				return platform.EventResult{}, err
			}
			var clipped bool
			*field, clipped = bounded(masked, min(budget, 4096))
			budget -= len(*field)
			result.Truncated = result.Truncated || clipped
		}
	}
	return result, nil
}

func (s *Service) Drift(ctx context.Context, id string) (platform.DriftReport, error) {
	app, release, err := s.enter(ctx, id)
	if err != nil {
		return platform.DriftReport{}, err
	}
	defer release()
	if s.renderer == nil {
		return platform.DriftReport{}, platform.Errorf(platform.CodeDependencyUnavailable, "execution rendering is unavailable")
	}
	revision, err := s.store.GetRevision(ctx, app.DesiredRevisionID)
	if err != nil {
		return platform.DriftReport{}, err
	}
	if revision.AppID != app.ID || revision.Frozen == nil || revision.Frozen.ChartArtifactDigest == "" || revision.Frozen.ValuesHash == "" {
		return platform.DriftReport{}, platform.Errorf(platform.CodeConflict, "a complete execution snapshot is not available yet")
	}
	manifest, err := s.renderer.RenderExecution(id, revision)
	if err != nil {
		return platform.DriftReport{}, err
	}
	differences, err := s.reader.CompareManifest(ctx, id, manifest)
	if err != nil {
		return platform.DriftReport{}, err
	}
	current, err := s.store.GetApplication(ctx, id)
	if err != nil {
		return platform.DriftReport{}, err
	}
	if current.Version != app.Version || current.DesiredRevisionID != app.DesiredRevisionID || current.ActiveOperationID != app.ActiveOperationID || current.Deleted {
		return platform.DriftReport{}, platform.Errorf(platform.CodeConflict, "application intent changed during inspection; refresh")
	}
	state := "in_sync"
	if len(differences) > 0 {
		state = "drifted"
	}
	return platform.DriftReport{RevisionID: revision.ID, ApplicationVersion: app.Version, OperationActive: app.ActiveOperationID != "", State: state, Differences: differences, ObservedAt: time.Now().UTC()}, nil
}

type redactor struct {
	replacer *strings.Replacer
	patterns []string
	overlap  int
}

func (r redactor) replace(text string) string { return r.replacer.Replace(text) }

func (s *Service) redactor(ctx context.Context) (redactor, error) {
	values, err := s.reader.RedactionValues(ctx)
	if err != nil {
		return redactor{}, platform.Errorf(platform.CodeDependencyUnavailable, "secret-safe diagnostics are unavailable")
	}
	patterns := map[string]bool{}
	budget := 0
	for _, value := range values {
		budget += len(value)
		if budget > 512*1024 {
			return redactor{}, platform.Errorf(platform.CodeDependencyUnavailable, "diagnostic redaction limit reached")
		}
		if len(value) == 0 {
			continue
		}
		quoted, _ := json.Marshal(string(value))
		for _, pattern := range []string{string(value), base64.StdEncoding.EncodeToString(value), base64.RawStdEncoding.EncodeToString(value), base64.URLEncoding.EncodeToString(value), base64.RawURLEncoding.EncodeToString(value), string(quoted[1 : len(quoted)-1]), url.QueryEscape(string(value))} {
			patterns[pattern] = true
		}
	}
	ordered := make([]string, 0, len(patterns))
	for pattern := range patterns {
		ordered = append(ordered, pattern)
	}
	if len(ordered) > 2048 {
		return redactor{}, platform.Errorf(platform.CodeDependencyUnavailable, "diagnostic redaction limit reached")
	}
	sort.Slice(ordered, func(i, j int) bool {
		if len(ordered[i]) == len(ordered[j]) {
			return ordered[i] < ordered[j]
		}
		return len(ordered[i]) > len(ordered[j])
	})
	pairs := make([]string, 0, 2*len(ordered))
	overlap := 0
	for _, pattern := range ordered {
		pairs = append(pairs, pattern, "[REDACTED]")
		overlap = max(overlap, len(pattern)-1)
	}
	return redactor{strings.NewReplacer(pairs...), ordered, overlap}, nil
}

func bounded(text string, limit int) (string, bool) {
	text = strings.ToValidUTF8(text, "�")
	if len(text) <= limit {
		return text, false
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit], true
}
