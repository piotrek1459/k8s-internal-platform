package cluster

import (
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
)

// Helm persists raw Kubernetes errors in failed-release descriptions before
// returning them to the adapter. Classifying the returned error alone cannot
// protect storage. Sanitize new failure writes at the storage boundary, without
// deleting or rewriting previously recorded release evidence.
type secretSafeReleaseDriver struct{ driver.Driver }

func safeReleaseDescription(value *release.Release) *release.Release {
	if value == nil || value.Info == nil || value.Info.Status != release.StatusFailed {
		return value
	}
	copy := *value
	info := *value.Info
	copy.Info = &info
	info.Description = "platform operation failed; inspect application diagnostics and operation history"
	return &copy
}
func (d secretSafeReleaseDriver) Create(key string, value *release.Release) error {
	return d.Driver.Create(key, safeReleaseDescription(value))
}
func (d secretSafeReleaseDriver) Update(key string, value *release.Release) error {
	return d.Driver.Update(key, safeReleaseDescription(value))
}
