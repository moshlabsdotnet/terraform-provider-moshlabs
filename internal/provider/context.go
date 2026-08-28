package provider

import (
	"fmt"
	"strings"
)

// contextInit mirrors the `init` object accepted by the system-context module:
// the already-resolved context of the parent scope, passed down so a child
// scope doesn't have to re-derive it.
type contextInit struct {
	Account     *string
	Environment *string
	Service     *string
	Metadata    map[string]string
}

type contextInput struct {
	Init        contextInit
	Account     *string
	Environment *string
	Service     *string
	Metadata    map[string]string
}

type contextResult struct {
	Scope       string
	Account     string
	Environment *string
	Service     *string
	Metadata    map[string]string
	Labels      map[string]string
}

// Per-scope metadata allowlists, mirroring the object() type constraint each
// HCL submodule (modules/account, modules/environment, modules/service)
// declared for its own `metadata` variable. In HCL, converting a richer
// object into one of these narrower object types silently drops any
// attribute not in the target type — that's not a side effect, it's the
// mechanism the original module relied on to let callers pass a broad object
// (e.g. system-service passing its whole `var.config`) without it leaking
// unrelated fields into context.metadata. These lists are the Go
// reimplementation of that same drop-unlisted-keys behavior.
var (
	accountMetadataKeys     = []string{"ecosystem", "platform", "cloud", "region"}
	environmentMetadataKeys = []string{"region"}
	serviceMetadataKeys     = []string{"version"}
)

// computeContext reproduces the account -> environment -> service cascade
// implemented by modules/system-context (and its account/environment/service
// submodules) in the HCL module, as a single pure function. It exists so the
// same graph node that used to fan out into 3+ nested module calls (and,
// transitively, dozens of locals blocks) becomes one data source read.
func computeContext(in contextInput) (contextResult, error) {
	account := coalesceString(in.Account, in.Init.Account)
	environment := coalesceStringPtr(in.Environment, in.Init.Environment)
	service := coalesceStringPtr(in.Service, in.Init.Service)
	topMetadata := mergeStringMaps(in.Init.Metadata, in.Metadata)

	if service != nil && environment == nil {
		return contextResult{}, fmt.Errorf("service %q requires an environment (set directly or via init) — a service always belongs to an environment", *service)
	}

	scope := "account"
	if service != nil {
		scope = "service"
	} else if environment != nil {
		scope = "environment"
	}

	// modules/account is unconditionally instantiated in the HCL regardless
	// of the ultimately-resolved scope, so this validation always runs too —
	// even a service-scoped call fails if ecosystem/platform can't be
	// resolved (in practice they always are, inherited via init.metadata).
	accountOwn := filterKeys(topMetadata, accountMetadataKeys...)
	if err := validateAccountMetadata(accountOwn); err != nil {
		return contextResult{}, err
	}
	accountMetadata := mergeStringMaps(accountOwn, map[string]string{"scope": "account"})
	accountLabels := mergeStringMaps(
		map[string]string{"account": account},
		filterKeys(accountMetadata, "scope", "platform", "ecosystem"),
		map[string]string{"terraform": "true"},
	)

	if scope == "account" {
		return contextResult{
			Scope:    "account",
			Account:  account,
			Metadata: accountMetadata,
			Labels:   normalizeLabels(accountLabels),
		}, nil
	}

	environmentOwn := filterKeys(topMetadata, environmentMetadataKeys...)
	environmentMetadata := mergeStringMaps(accountMetadata, environmentOwn, map[string]string{"scope": "environment"})
	environmentLabels := mergeStringMaps(
		accountLabels,
		map[string]string{"account": account, "environment": *environment},
		// "environment_id" is listed here to match the original HCL, but it
		// can never actually appear: environmentMetadataKeys doesn't include
		// it, so it's always filtered out of environmentMetadata upstream.
		// That's a latent bug in the original module, kept intentionally for
		// exact behavioral parity rather than silently fixed here.
		filterKeys(environmentMetadata, "scope", "environment_id"),
	)

	if scope == "environment" {
		return contextResult{
			Scope:       "environment",
			Account:     account,
			Environment: environment,
			Metadata:    environmentMetadata,
			Labels:      normalizeLabels(environmentLabels),
		}, nil
	}

	serviceOwn := filterKeys(topMetadata, serviceMetadataKeys...)
	serviceMetadata := mergeStringMaps(environmentMetadata, serviceOwn, map[string]string{"scope": "service"})
	serviceLabels := mergeStringMaps(
		environmentLabels,
		map[string]string{"account": account, "environment": *environment, "service": *service},
		filterKeys(serviceMetadata, "scope"),
		map[string]string{"env": *environment},
	)

	return contextResult{
		Scope:       "service",
		Account:     account,
		Environment: environment,
		Service:     service,
		Metadata:    serviceMetadata,
		Labels:      normalizeLabels(serviceLabels),
	}, nil
}

// validateAccountMetadata mirrors modules/account/variables.tf: ecosystem and
// platform are required (object() attributes with no `optional(...)`), and
// ecosystem is further constrained to "production" or "development" by that
// submodule's validation block.
func validateAccountMetadata(m map[string]string) error {
	ecosystem, ok := m["ecosystem"]
	if !ok || ecosystem == "" {
		return fmt.Errorf(`metadata "ecosystem" is required`)
	}
	if ecosystem != "production" && ecosystem != "development" {
		return fmt.Errorf(`metadata "ecosystem" must be "production" or "development", got %q`, ecosystem)
	}
	if platform, ok := m["platform"]; !ok || platform == "" {
		return fmt.Errorf(`metadata "platform" is required`)
	}
	return nil
}

// coalesceString returns the first non-nil, non-empty value, or "" if none.
func coalesceString(vals ...*string) string {
	if v := coalesceStringPtr(vals...); v != nil {
		return *v
	}
	return ""
}

// coalesceStringPtr returns the first non-nil, non-empty value as a pointer,
// or nil if none. Mirrors HCL's coalesce(), which skips null values.
func coalesceStringPtr(vals ...*string) *string {
	for _, v := range vals {
		if v != nil && *v != "" {
			return v
		}
	}
	return nil
}

// mergeStringMaps merges maps left-to-right, later maps overriding earlier
// ones on key collision (HCL merge() semantics). nil maps are skipped.
func mergeStringMaps(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func filterKeys(m map[string]string, allowed ...string) map[string]string {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		allowedSet[k] = struct{}{}
	}
	out := map[string]string{}
	for k, v := range m {
		if _, ok := allowedSet[k]; ok {
			out[k] = v
		}
	}
	return out
}

// normalizeLabels lowercases keys/values and swaps the separators that are
// invalid in cloud label keys/values (underscores in keys, dots in values),
// matching system-context/outputs.tf. An empty value is treated as "unset"
// (dropped) since this package models metadata as map[string]string rather
// than a nullable map, so there's no distinct null to filter on.
func normalizeLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		if v == "" {
			continue
		}
		key := strings.ToLower(strings.ReplaceAll(k, "_", "-"))
		val := strings.ToLower(strings.ReplaceAll(v, ".", "-"))
		out[key] = val
	}
	return out
}
