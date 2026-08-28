// Copyright 2026 Mosh Labs
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"strings"
)

// resourceNameInput mirrors modules/system-resource's variables: context
// (account/environment/service, metadata unused — system-resource never
// reads it), name, delimiter, scoped, and root_scope.
type resourceNameInput struct {
	Account     *string
	Environment *string
	Service     *string
	Name        *string
	Delimiter   *string
	Scoped      *bool
	RootScope   *string
}

// computeResourceName reproduces modules/system-resource/main.tf: build a
// delimited prefix from account/environment/service (how much of the
// hierarchy is included is controlled by root_scope — counterintuitively,
// root_scope="account" includes the *most* context, not the least, since it
// means "this resource must be uniquely named at the account level"), then
// append the sanitized resource name.
func computeResourceName(in resourceNameInput) (string, error) {
	rootScope := "environment"
	if in.RootScope != nil && *in.RootScope != "" {
		rootScope = *in.RootScope
	}
	if rootScope != "account" && rootScope != "environment" && rootScope != "service" {
		return "", fmt.Errorf(`root_scope must be "account", "environment", or "service", got %q`, rootScope)
	}

	scoped := true
	if in.Scoped != nil {
		scoped = *in.Scoped
	}

	name := ""
	if in.Name != nil {
		name = *in.Name
	}
	if name == "" && !scoped {
		return "", fmt.Errorf("name must be provided when scoped is false")
	}

	delimiter := "-"
	if in.Delimiter != nil && *in.Delimiter != "" {
		delimiter = *in.Delimiter
	}

	includeAccount := rootScope == "account"
	includeEnvironment := rootScope == "account" || rootScope == "environment"
	includeService := true // rootScope is always account, environment, or service

	var prefixParts []string
	if includeAccount && nonEmpty(in.Account) {
		prefixParts = append(prefixParts, *in.Account)
	}
	if includeEnvironment && nonEmpty(in.Environment) {
		prefixParts = append(prefixParts, *in.Environment)
	}
	if includeService && nonEmpty(in.Service) {
		prefixParts = append(prefixParts, *in.Service)
	}
	prefix := strings.Join(prefixParts, delimiter)

	var nameParts []string
	if scoped && prefix != "" {
		nameParts = append(nameParts, prefix)
	}
	if name != "" {
		sanitized := strings.ReplaceAll(strings.ReplaceAll(name, ".", "-"), "_", "-")
		nameParts = append(nameParts, sanitized)
	}

	return strings.Join(nameParts, delimiter), nil
}

func nonEmpty(s *string) bool {
	return s != nil && *s != ""
}
