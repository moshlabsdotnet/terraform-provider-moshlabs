// Copyright 2026 Mosh Labs
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"strings"
)

// contextInit mirrors the `init` object accepted by the system-context module:
// the already-resolved context of the parent scope, passed down so a child
// scope doesn't have to re-derive it.
type contextInit struct {
	Platform    *string
	Account     *string
	Environment *string
	Service     *string
	Metadata    map[string]*string
}

type contextInput struct {
	Init        contextInit
	Platform    *string
	Account     *string
	Environment *string
	Service     *string
	Metadata    map[string]*string
}

// contextResult is the resolved context. Metadata values are *string because
// the HCL module's output keeps declared-but-unset optional attributes as
// explicit nulls (e.g. `cloud = null` at account scope) — consumers index
// them directly (`context.metadata.cloud`), so the key has to exist even when
// the value doesn't.
type contextResult struct {
	Scope       string
	Platform    *string
	Account     *string
	Environment *string
	Service     *string
	Metadata    map[string]*string
	Labels      map[string]string
}

// metadataAttr is one attribute of the object() type constraint a
// system-context submodule declares for its own `metadata` variable.
type metadataAttr struct {
	Name     string
	Required bool
}

// Per-scope metadata types, mirroring the object() constraint each HCL
// submodule (modules/platform, modules/account, modules/environment,
// modules/service) declares for its `metadata` variable. In HCL, converting
// the merged metadata into one of these object types does three things this
// package reproduces in coerceMetadata: unlisted attributes are dropped
// (letting callers pass a broad object, e.g. system-service's whole
// `var.config`), required attributes must be present (a null value still
// counts as present), and unset optional attributes come back as null.
var (
	platformMetadataAttrs = []metadataAttr{
		{Name: "organization", Required: true},
		{Name: "status", Required: true},
		{Name: "control_plane", Required: true},
	}
	accountMetadataAttrs = []metadataAttr{
		{Name: "ecosystem", Required: true},
		{Name: "cloud"},
		{Name: "region"},
	}
	environmentMetadataAttrs = []metadataAttr{
		{Name: "region"},
	}
	serviceMetadataAttrs = []metadataAttr{
		{Name: "version"},
	}
)

// computeContext reproduces the platform -> account -> environment -> service
// cascade implemented by modules/system-context (and its submodules) as a
// single pure function. It exists so the same graph node that used to fan out
// into 4 nested module calls becomes one data source read.
func computeContext(in contextInput) (contextResult, error) {
	// variables.tf validations run against the raw inputs, before coalesce —
	// so an explicit "" counts as set here even though it resolves to null.
	if in.Platform == nil && in.Init.Platform == nil {
		return contextResult{}, fmt.Errorf("platform must be set via platform or init.platform")
	}
	if (in.Environment != nil || in.Init.Environment != nil) && in.Account == nil && in.Init.Account == nil {
		return contextResult{}, fmt.Errorf("environment requires an account")
	}
	if (in.Service != nil || in.Init.Service != nil) && in.Environment == nil && in.Init.Environment == nil {
		return contextResult{}, fmt.Errorf("service requires an environment")
	}

	platform := coalesceStringPtr(in.Platform, in.Init.Platform)
	account := coalesceStringPtr(in.Account, in.Init.Account)
	environment := coalesceStringPtr(in.Environment, in.Init.Environment)
	service := coalesceStringPtr(in.Service, in.Init.Service)
	topMetadata := mergeMetadata(in.Init.Metadata, in.Metadata)

	// The HCL indexes module.account[0] / module.environment[0] from the
	// child scope, so a parent that passed validation (raw "") but resolved
	// to null fails there.
	if environment != nil && account == nil {
		return contextResult{}, fmt.Errorf("environment %q requires an account", *environment)
	}
	if service != nil && environment == nil {
		return contextResult{}, fmt.Errorf("service %q requires an environment", *service)
	}

	// modules/platform is unconditionally instantiated, so its validation
	// always runs — every scope needs organization/status/control_plane,
	// in practice inherited via init.metadata.
	platformOwn, err := coerceMetadata("platform", topMetadata, platformMetadataAttrs)
	if err != nil {
		return contextResult{}, err
	}
	if err := validatePlatformMetadata(platformOwn); err != nil {
		return contextResult{}, err
	}
	platformMetadata := mergeMetadata(platformOwn, map[string]*string{"platform": platform, "scope": strptr("platform")})
	platformLabels := mergeMetadata(
		map[string]*string{"platform": platform},
		filterKeys(platformMetadata, "scope"),
		map[string]*string{"terraform": strptr("true")},
	)

	result := contextResult{
		Scope:    "platform",
		Platform: platform,
		Metadata: platformMetadata,
		Labels:   normalizeLabels(platformLabels),
	}
	if account == nil {
		return result, nil
	}

	accountOwn, err := coerceMetadata("account", topMetadata, accountMetadataAttrs)
	if err != nil {
		return contextResult{}, err
	}
	if err := validateAccountMetadata(accountOwn); err != nil {
		return contextResult{}, err
	}
	accountMetadata := mergeMetadata(platformMetadata, accountOwn, map[string]*string{"scope": strptr("account")})
	accountLabels := mergeMetadata(
		platformLabels,
		map[string]*string{"platform": platform, "account": account},
		filterKeys(accountMetadata, "scope", "platform", "ecosystem"),
	)

	result.Scope = "account"
	result.Account = account
	result.Metadata = accountMetadata
	result.Labels = normalizeLabels(accountLabels)
	if environment == nil {
		return result, nil
	}

	environmentOwn, err := coerceMetadata("environment", topMetadata, environmentMetadataAttrs)
	if err != nil {
		return contextResult{}, err
	}
	environmentMetadata := mergeMetadata(accountMetadata, environmentOwn, map[string]*string{"scope": strptr("environment")})
	environmentLabels := mergeMetadata(
		accountLabels,
		map[string]*string{"platform": platform, "account": account, "environment": environment},
		filterKeys(environmentMetadata, "scope"),
	)

	result.Scope = "environment"
	result.Environment = environment
	result.Metadata = environmentMetadata
	result.Labels = normalizeLabels(environmentLabels)
	if service == nil {
		return result, nil
	}

	serviceOwn, err := coerceMetadata("service", topMetadata, serviceMetadataAttrs)
	if err != nil {
		return contextResult{}, err
	}
	serviceMetadata := mergeMetadata(environmentMetadata, serviceOwn, map[string]*string{"scope": strptr("service")})
	serviceLabels := mergeMetadata(
		environmentLabels,
		map[string]*string{"platform": platform, "account": account, "environment": environment, "service": service},
		filterKeys(serviceMetadata, "scope"),
		map[string]*string{"env": environment},
	)

	result.Scope = "service"
	result.Service = service
	result.Metadata = serviceMetadata
	result.Labels = normalizeLabels(serviceLabels)
	return result, nil
}

// coerceMetadata reproduces converting a map into an object() type: only the
// declared attributes survive, a missing required attribute is an error, and
// a missing optional attribute becomes null.
func coerceMetadata(scope string, m map[string]*string, attrs []metadataAttr) (map[string]*string, error) {
	out := make(map[string]*string, len(attrs))
	for _, a := range attrs {
		v, ok := m[a.Name]
		if !ok && a.Required {
			return nil, fmt.Errorf("%s metadata %q is required", scope, a.Name)
		}
		out[a.Name] = v
	}
	return out, nil
}

// validatePlatformMetadata mirrors modules/platform/variables.tf. A null
// status fails too: contains() rejects a null argument.
func validatePlatformMetadata(m map[string]*string) error {
	status := m["status"]
	if status == nil {
		return fmt.Errorf(`platform metadata "status" must not be null`)
	}
	if *status != "ACTIVE" && *status != "INACTIVE" {
		return fmt.Errorf(`platform metadata "status" must be "ACTIVE" or "INACTIVE", got %q`, *status)
	}
	return nil
}

// validateAccountMetadata mirrors modules/account/variables.tf: ecosystem
// must be "production" or "development" (null fails, as above), and cloud,
// when set, must be "gcp" or "aws".
func validateAccountMetadata(m map[string]*string) error {
	ecosystem := m["ecosystem"]
	if ecosystem == nil {
		return fmt.Errorf(`account metadata "ecosystem" must not be null`)
	}
	if *ecosystem != "production" && *ecosystem != "development" {
		return fmt.Errorf(`account metadata "ecosystem" must be "production" or "development", got %q`, *ecosystem)
	}
	if cloud := m["cloud"]; cloud != nil && *cloud != "gcp" && *cloud != "aws" {
		return fmt.Errorf(`account metadata "cloud" must be "gcp" or "aws", got %q`, *cloud)
	}
	return nil
}

func strptr(s string) *string { return &s }

// coalesceStringPtr returns the first non-nil, non-empty value, or nil if
// none. Mirrors HCL's try(coalesce(...), null), which skips nulls and "".
func coalesceStringPtr(vals ...*string) *string {
	for _, v := range vals {
		if v != nil && *v != "" {
			return v
		}
	}
	return nil
}

// mergeMetadata merges maps left-to-right, later maps overriding earlier
// ones on key collision — including with an explicit null, as HCL merge()
// does. nil maps are skipped.
func mergeMetadata(maps ...map[string]*string) map[string]*string {
	out := map[string]*string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func filterKeys(m map[string]*string, allowed ...string) map[string]*string {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		allowedSet[k] = struct{}{}
	}
	out := map[string]*string{}
	for k, v := range m {
		if _, ok := allowedSet[k]; ok {
			out[k] = v
		}
	}
	return out
}

// normalizeLabels lowercases keys/values and swaps the separators that are
// invalid in cloud label keys/values (underscores in keys, dots in values),
// dropping null values — matching system-context/outputs.tf.
func normalizeLabels(labels map[string]*string) map[string]string {
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		if v == nil {
			continue
		}
		key := strings.ToLower(strings.ReplaceAll(k, "_", "-"))
		val := strings.ToLower(strings.ReplaceAll(*v, ".", "-"))
		out[key] = val
	}
	return out
}
