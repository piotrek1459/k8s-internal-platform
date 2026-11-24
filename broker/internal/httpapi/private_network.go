package httpapi

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"

	"k8s-internal-platform/broker/internal/platform"
)

// PrivateNetwork is installation configuration, not a DNS/TLS readiness claim.
// The private gateway supplies TLS; the broker never trusts forwarding headers
// to decide whether a cookie should be Secure.
type PrivateNetwork struct {
	ManagementHostname string `json:"managementHostname"`
	ApplicationDomain  string `json:"applicationDomain"`
	HTTPSPort          int    `json:"httpsPort"`
}

func (n PrivateNetwork) Validate() error {
	for _, name := range []string{n.ManagementHostname, n.ApplicationDomain} {
		if len(name) > 253 || len(strings.Split(name, ".")) < 2 {
			return fmt.Errorf("private networking requires valid lowercase DNS names")
		}
		for _, label := range strings.Split(name, ".") {
			if len(validation.IsDNS1123Label(label)) != 0 {
				return fmt.Errorf("private networking requires valid lowercase DNS names")
			}
		}
	}
	if n.ManagementHostname == n.ApplicationDomain || strings.HasSuffix(n.ManagementHostname, "."+n.ApplicationDomain) {
		return fmt.Errorf("management hostname must be outside the application domain")
	}
	if n.HTTPSPort < 1024 || n.HTTPSPort > 65535 {
		return fmt.Errorf("private HTTPS port must be between 1024 and 65535")
	}
	return nil
}

func (n PrivateNetwork) applicationURL(host string) string {
	if host == "" || n.HTTPSPort == 0 {
		return ""
	}
	if n.HTTPSPort != 443 {
		host += ":" + strconv.Itoa(n.HTTPSPort)
	}
	return (&url.URL{Scheme: "https", Host: host, Path: "/"}).String()
}

func (r *Router) validatePrivateExposure(e platform.Exposure) error {
	if e.Mode != platform.ExposureLAN {
		return nil
	}
	if !r.cfg.PrivateIngressEnabled {
		return platform.FieldErrorf(platform.CodeUnsupportedSpec, "exposure", "private ingress is not configured on this installation")
	}
	n := r.cfg.PrivateNetwork
	if n.ApplicationDomain == "" {
		return nil // Explicit legacy/operator-managed private ingress.
	}
	suffix := "." + n.ApplicationDomain
	label, matches := strings.CutSuffix(e.Hostname, suffix)
	if !matches || label == "" || strings.Contains(label, ".") || e.Hostname == n.ManagementHostname {
		return platform.FieldErrorf(platform.CodeInvalidInput, "exposure.hostname",
			"LAN hostname must be one label beneath %s and cannot use the management hostname", n.ApplicationDomain)
	}
	return nil
}

// Route transitions require absence verification after failed Helm mutations.
// Until that recovery contract is implemented, private-LAN installations retain
// their initial exposure while ordinary image/configuration changes continue.
func (r *Router) validateExposureChange(previous, next platform.Exposure) error {
	if r.cfg.PrivateNetwork.ApplicationDomain != "" && previous != next {
		return platform.FieldErrorf(platform.CodeUnsupportedSpec, "exposure",
			"access mode and hostname cannot change after creation; create a separate application")
	}
	return nil
}

func (r *Router) applicationAccess(ctx context.Context, app platform.Application) (map[string]any, error) {
	id := app.LastSuccessfulRevision
	if id == "" {
		id = app.DesiredRevisionID
	}
	revision, err := r.cfg.Store.GetRevision(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read application access revision: %w", err)
	}
	access := map[string]any{"mode": revision.Spec.Exposure.Mode, "revisionId": revision.ID}
	if revision.Spec.Exposure.Mode == platform.ExposureLAN {
		access["hostname"] = revision.Spec.Exposure.Hostname
		access["url"] = r.cfg.PrivateNetwork.applicationURL(revision.Spec.Exposure.Hostname)
	}
	return access, nil
}
