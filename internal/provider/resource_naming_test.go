package provider

import "testing"

func boolptr(b bool) *bool { return &b }

// These first tests port modules/system-resource/tests/main.tftest.hcl's
// fixtures 1:1 (same names, same inputs, same expected output) — that file
// is the closest thing system-resource has to a spec, so matching it exactly
// is the bar for calling this a faithful port.

func TestComputeResourceName_StandardAccount(t *testing.T) {
	got, err := computeResourceName(resourceNameInput{
		Account: strptr("test-account"),
		Name:    strptr("resource"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "resource" {
		t.Fatalf("got %q, want %q", got, "resource")
	}
}

func TestComputeResourceName_StandardEnvironment(t *testing.T) {
	got, err := computeResourceName(resourceNameInput{
		Account:     strptr("test-account"),
		Environment: strptr("test-environment"),
		Name:        strptr("resource"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "test-environment-resource" {
		t.Fatalf("got %q, want %q", got, "test-environment-resource")
	}
}

func TestComputeResourceName_StandardService(t *testing.T) {
	got, err := computeResourceName(resourceNameInput{
		Account:     strptr("test-account"),
		Environment: strptr("test-environment"),
		Service:     strptr("test-service"),
		Name:        strptr("resource"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "test-environment-test-service-resource" {
		t.Fatalf("got %q, want %q", got, "test-environment-test-service-resource")
	}
}

func TestComputeResourceName_Delimiter(t *testing.T) {
	got, err := computeResourceName(resourceNameInput{
		Account:     strptr("test-account"),
		Environment: strptr("test-environment"),
		Service:     strptr("test-service"),
		Name:        strptr("resource"),
		Delimiter:   strptr("."),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "test-environment.test-service.resource" {
		t.Fatalf("got %q, want %q", got, "test-environment.test-service.resource")
	}
}

func TestComputeResourceName_Scoped(t *testing.T) {
	got, err := computeResourceName(resourceNameInput{
		Account:     strptr("test-account"),
		Environment: strptr("test-environment"),
		Service:     strptr("test-service"),
		Name:        strptr("resource"),
		Scoped:      boolptr(false),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "resource" {
		t.Fatalf("got %q, want %q", got, "resource")
	}
}

func TestComputeResourceName_RootScopeAccount(t *testing.T) {
	got, err := computeResourceName(resourceNameInput{
		Account:     strptr("test-account"),
		Environment: strptr("test-environment"),
		Service:     strptr("test-service"),
		Name:        strptr("resource"),
		RootScope:   strptr("account"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "test-account-test-environment-test-service-resource" {
		t.Fatalf("got %q, want %q", got, "test-account-test-environment-test-service-resource")
	}
}

func TestComputeResourceName_RootScopeService(t *testing.T) {
	got, err := computeResourceName(resourceNameInput{
		Account:     strptr("test-account"),
		Environment: strptr("test-environment"),
		Service:     strptr("test-service"),
		Name:        strptr("resource"),
		RootScope:   strptr("service"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "test-service-resource" {
		t.Fatalf("got %q, want %q", got, "test-service-resource")
	}
}

// ---------------------------------------------------------------------
// Edge cases beyond the .tftest.hcl fixtures — verified empirically against
// the real (already-migrated) system-resource module before porting.
// ---------------------------------------------------------------------

func TestComputeResourceName_RootScopeRequestsUnsetLevel_NoErrorNoStrayDelimiter(t *testing.T) {
	// root_scope=account with only account set (environment/service both
	// nil): compact() drops the nulls in the original HCL rather than
	// erroring or leaving a stray delimiter — verified empirically against
	// the live module (returned "acme-bucket", not "acme--bucket" or an
	// error).
	got, err := computeResourceName(resourceNameInput{
		Account:   strptr("acme"),
		Name:      strptr("bucket"),
		RootScope: strptr("account"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "acme-bucket" {
		t.Fatalf("got %q, want %q", got, "acme-bucket")
	}
}

func TestComputeResourceName_RootScopeService_OnlyServiceIncludedEvenWithAccountAndEnvironmentSet(t *testing.T) {
	got, err := computeResourceName(resourceNameInput{
		Account:     strptr("acme"),
		Environment: strptr("prod"),
		Service:     strptr("api"),
		Name:        strptr("queue"),
		RootScope:   strptr("service"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "api-queue" {
		t.Fatalf("got %q, want %q", got, "api-queue")
	}
}

func TestComputeResourceName_ScopedFalseWithoutNameErrors(t *testing.T) {
	_, err := computeResourceName(resourceNameInput{
		Account: strptr("acme"),
		Scoped:  boolptr(false),
	})
	if err == nil {
		t.Fatal("expected an error when scoped=false and name is unset")
	}
}

func TestComputeResourceName_ScopedFalseWithEmptyStringNameErrors(t *testing.T) {
	// An explicit empty string must be treated the same as unset, matching
	// coalesce-style handling used throughout this package.
	_, err := computeResourceName(resourceNameInput{
		Account: strptr("acme"),
		Name:    strptr(""),
		Scoped:  boolptr(false),
	})
	if err == nil {
		t.Fatal("expected an error when scoped=false and name is empty string")
	}
}

func TestComputeResourceName_InvalidRootScopeErrors(t *testing.T) {
	_, err := computeResourceName(resourceNameInput{
		Account:   strptr("acme"),
		Name:      strptr("bucket"),
		RootScope: strptr("platform"),
	})
	if err == nil {
		t.Fatal(`expected an error for root_scope "platform"`)
	}
}

func TestComputeResourceName_DefaultRootScopeIsEnvironment(t *testing.T) {
	// No root_scope given: account/environment/service context but default
	// should exclude account, matching "standard_service" behavior without
	// explicitly passing root_scope.
	got, err := computeResourceName(resourceNameInput{
		Account:     strptr("test-account"),
		Environment: strptr("test-environment"),
		Service:     strptr("test-service"),
		Name:        strptr("resource"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "test-environment-test-service-resource" {
		t.Fatalf("got %q, want %q (default root_scope should behave like \"environment\")", got, "test-environment-test-service-resource")
	}
}

func TestComputeResourceName_NameSanitizesDotsAndUnderscores(t *testing.T) {
	got, err := computeResourceName(resourceNameInput{
		Account: strptr("acme"),
		Name:    strptr("my.weird_name.here"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my-weird-name-here" {
		t.Fatalf("got %q, want %q", got, "my-weird-name-here")
	}
}

func TestComputeResourceName_NoNameScopedTrueReturnsJustPrefix(t *testing.T) {
	// name is optional as long as scoped isn't false — omitting it just
	// yields the scope prefix on its own.
	got, err := computeResourceName(resourceNameInput{
		Account:     strptr("acme"),
		Environment: strptr("prod"),
		Service:     strptr("api"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "prod-api" {
		t.Fatalf("got %q, want %q", got, "prod-api")
	}
}

func TestComputeResourceName_EmptyDelimiterFallsBackToDash(t *testing.T) {
	got, err := computeResourceName(resourceNameInput{
		Account:     strptr("acme"),
		Environment: strptr("prod"),
		Name:        strptr("bucket"),
		Delimiter:   strptr(""),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "prod-bucket" {
		t.Fatalf("got %q, want %q", got, "prod-bucket")
	}
}

func TestComputeResourceName_EverythingEmptyReturnsEmptyString(t *testing.T) {
	got, err := computeResourceName(resourceNameInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty string", got)
	}
}
