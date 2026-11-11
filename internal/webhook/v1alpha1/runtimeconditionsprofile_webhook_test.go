/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"testing"

	resolver "github.com/runtimeconditions/rc-extension-resolver"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const widgetExtensionURI = "mem://widget.extension.yaml"

var widgetExtensionDoc = []byte(`
metadata:
  id: ` + widgetExtensionURI + `
spec:
  kinds:
    - name: widget
  interfaceTypes:
    - name: http
      targetKind: widget
  schemas:
    - id: widget-http
      appliesToKind: widget
      appliesToInterfaceType: http
      schema:
        $schema: https://json-schema.org/draft/2020-12/schema
        type: object
        required: [kind, interface]
        properties:
          kind:
            const: widget
          interface:
            type: object
            required: [type, uri]
            properties:
              type:
                const: http
              uri:
                type: string
                minLength: 1
`)

func newTestValidator() *RuntimeConditionsProfileValidator {
	return &RuntimeConditionsProfileValidator{
		Loader: resolver.NewInMemoryLoader(map[string][]byte{
			widgetExtensionURI: widgetExtensionDoc,
		}),
	}
}

func newAdmissionRequest(t *testing.T, profileJSON string) admission.Request {
	t.Helper()
	return admission.Request{
		Object: runtime.RawExtension{Raw: []byte(profileJSON)},
	}
}

func TestHandle_AllowsValidProfile(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`"],
		"conditions": [
			{"name": "primary", "kind": "widget", "interface": {"type": "http", "uri": "https://example.com"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Fatalf("expected allowed, got denied: %s", resp.Result.Message)
	}
}

func TestHandle_DeniesUnknownKind(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`"],
		"conditions": [
			{"name": "primary", "kind": "gadget", "interface": {"type": "http"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected denied for unknown kind, got allowed")
	}
}

func TestHandle_DeniesUnknownInterfaceType(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`"],
		"conditions": [
			{"name": "primary", "kind": "widget", "interface": {"type": "grpc"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected denied for unknown interface type, got allowed")
	}
}

func TestHandle_DeniesConditionFailingSchema(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`"],
		"conditions": [
			{"name": "primary", "kind": "widget", "interface": {"type": "http"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected denied for missing required uri, got allowed")
	}
}

func TestHandle_DeniesBrokenExtensionReference(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": ["mem://does-not-exist.extension.yaml"],
		"conditions": []
	}`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected denied for a broken extension reference, got allowed")
	}
}

func TestHandle_AllowsMultipleIndependentExtensions(t *testing.T) {
	gadgetURI := "mem://gadget.extension.yaml"
	loader := resolver.NewInMemoryLoader(map[string][]byte{
		widgetExtensionURI: widgetExtensionDoc,
		gadgetURI: []byte(`
metadata:
  id: ` + gadgetURI + `
spec:
  kinds:
    - name: gadget
`),
	})
	v := &RuntimeConditionsProfileValidator{Loader: loader}
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`", "`+gadgetURI+`"],
		"conditions": [
			{"name": "primary", "kind": "widget", "interface": {"type": "http", "uri": "https://example.com"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Fatalf("expected allowed, got denied: %s", resp.Result.Message)
	}
}

func TestHandle_MalformedObjectIsErrored(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `not json`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected errored response for malformed object, got allowed")
	}
}

// --- Additional edge cases: everything below is expected to deny/error. ---

func TestHandle_DeniesConditionWhenNoExtensionsDeclared(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": [],
		"conditions": [
			{"name": "primary", "kind": "widget", "interface": {"type": "http", "uri": "https://example.com"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected denied: no extensions declared, so no kind can be valid")
	}
}

func TestHandle_DeniesConditionMissingKindField(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`"],
		"conditions": [
			{"name": "primary", "interface": {"type": "http", "uri": "https://example.com"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected denied: condition has no kind field at all")
	}
}

func TestHandle_DeniesConditionMissingInterfaceField(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`"],
		"conditions": [
			{"name": "primary", "kind": "widget"}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected denied: condition has no interface field at all")
	}
}

func TestHandle_DeniesConditionWithNonObjectInterface(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`"],
		"conditions": [
			{"name": "primary", "kind": "widget", "interface": "http"}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected denied: interface is a string, not an object")
	}
}

func TestHandle_DeniesConditionWithNonStringKind(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`"],
		"conditions": [
			{"name": "primary", "kind": 42, "interface": {"type": "http", "uri": "https://example.com"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected denied: kind is a number, not a string")
	}
}

func TestHandle_DeniesDependencyCycle(t *testing.T) {
	cycAURI := "mem://cyc-a.extension.yaml"
	cycBURI := "mem://cyc-b.extension.yaml"
	loader := resolver.NewInMemoryLoader(map[string][]byte{
		cycAURI: []byte(`
metadata: {id: ` + cycAURI + `}
spec:
  dependencies: [` + cycBURI + `]
`),
		cycBURI: []byte(`
metadata: {id: ` + cycBURI + `}
spec:
  dependencies: [` + cycAURI + `]
`),
	})
	v := &RuntimeConditionsProfileValidator{Loader: loader}
	req := newAdmissionRequest(t, `{
		"extensions": ["`+cycAURI+`"],
		"conditions": []
	}`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected denied: extensions form a dependency cycle")
	}
}

func TestHandle_AllowsAmbiguousKindViaExplicitExtension(t *testing.T) {
	otherWidgetURI := "mem://other-widget.extension.yaml"
	loader := resolver.NewInMemoryLoader(map[string][]byte{
		widgetExtensionURI: widgetExtensionDoc,
		otherWidgetURI: []byte(`
metadata: {id: ` + otherWidgetURI + `}
spec:
  kinds:
    - name: widget
`),
	})
	v := &RuntimeConditionsProfileValidator{Loader: loader}
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`", "`+otherWidgetURI+`"],
		"conditions": [
			{"name": "primary", "kind": "widget", "extension": "`+widgetExtensionURI+`", "interface": {"type": "http", "uri": "https://example.com"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Fatalf("expected allowed: two extensions may both declare kind \"widget\", got denied: %s", resp.Result.Message)
	}
	if len(resp.Warnings) != 0 {
		t.Fatalf("expected no warnings when extension is given explicitly, got %v", resp.Warnings)
	}
}

func TestHandle_AllowsAmbiguousKindViaFallbackWithWarning(t *testing.T) {
	otherWidgetURI := "mem://other-widget.extension.yaml"
	loader := resolver.NewInMemoryLoader(map[string][]byte{
		widgetExtensionURI: widgetExtensionDoc,
		otherWidgetURI: []byte(`
metadata: {id: ` + otherWidgetURI + `}
spec:
  kinds:
    - name: widget
`),
	})
	v := &RuntimeConditionsProfileValidator{Loader: loader}
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`", "`+otherWidgetURI+`"],
		"conditions": [
			{"name": "primary", "kind": "widget", "interface": {"type": "http", "uri": "https://example.com"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Fatalf("expected allowed: no extension field should fall back to the first-resolved owner, got denied: %s", resp.Result.Message)
	}
	if len(resp.Warnings) != 1 {
		t.Fatalf("expected one warning about fallback resolution, got %v", resp.Warnings)
	}
}

func TestHandle_DeniesConditionExtensionThatDoesNotDefineKind(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`"],
		"conditions": [
			{"name": "primary", "kind": "widget", "extension": "mem://not-a-resolved-extension.yaml", "interface": {"type": "http", "uri": "https://example.com"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected denied: the named extension was never resolved for this profile")
	}
}

func TestHandle_DeniesConditionWithUncompilableSchema(t *testing.T) {
	brokenURI := "mem://broken-schema.extension.yaml"
	loader := resolver.NewInMemoryLoader(map[string][]byte{
		brokenURI: []byte(`
metadata: {id: ` + brokenURI + `}
spec:
  kinds: [{name: widget}]
  interfaceTypes: [{name: http, targetKind: widget}]
  schemas:
    - id: widget-http-broken
      appliesToKind: widget
      appliesToInterfaceType: http
      schema:
        $schema: https://json-schema.org/draft/2020-12/schema
        $ref: '#/$defs/nope'
`),
	})
	v := &RuntimeConditionsProfileValidator{Loader: loader}
	req := newAdmissionRequest(t, `{
		"extensions": ["`+brokenURI+`"],
		"conditions": [
			{"name": "primary", "kind": "widget", "interface": {"type": "http"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected denied: bound schema has an unresolvable $ref and can't compile")
	}
}

func TestHandle_DeniesWhenOneOfSeveralConditionsIsInvalid(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`"],
		"conditions": [
			{"name": "good", "kind": "widget", "interface": {"type": "http", "uri": "https://example.com"}},
			{"name": "bad", "kind": "widget", "interface": {"type": "http"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected denied: second condition is missing required uri")
	}
}

func TestHandle_DeniesInvalidConditionEvenWhenMarkedOptional(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`"],
		"conditions": [
			{"name": "primary", "optional": true, "kind": "gadget", "interface": {"type": "http"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if resp.Allowed {
		t.Fatal("expected denied: optional=true doesn't excuse an unknown kind")
	}
}

// --- Robustness checks: these should still be *allowed*, to make sure the
// failure-mode tests above aren't just an over-eager validator. ---

func TestHandle_AllowsDuplicateExtensionURIs(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`", "`+widgetExtensionURI+`"],
		"conditions": [
			{"name": "primary", "kind": "widget", "interface": {"type": "http", "uri": "https://example.com"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Fatalf("expected allowed: the same extension URI twice isn't a real conflict, got denied: %s", resp.Result.Message)
	}
}

func TestHandle_AllowsNoConditionsAtAll(t *testing.T) {
	v := newTestValidator()
	req := newAdmissionRequest(t, `{
		"extensions": ["`+widgetExtensionURI+`"],
		"conditions": []
	}`)

	resp := v.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Fatalf("expected allowed: nothing to validate, got denied: %s", resp.Result.Message)
	}
}

// A reference is not the same string as the definition's metadata.id. A
// Condition pins its extension by the reference the profile declared.
func TestHandle_AllowsConditionExtensionPinnedByReference(t *testing.T) {
	const reference = "mem://widget-reference"
	loader := resolver.NewInMemoryLoader(map[string][]byte{reference: widgetExtensionDoc})
	v := &RuntimeConditionsProfileValidator{Loader: loader}
	req := newAdmissionRequest(t, `{
		"extensions": ["`+reference+`"],
		"conditions": [
			{"kind": "widget", "extension": "`+reference+`", "interface": {"type": "http", "uri": "https://example.com"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Fatalf("expected allowed: the Condition names the declared reference, got denied: %s", resp.Result.Message)
	}
}

// The short <url>:<version> form of an official extension, used as the
// Condition's extension selector exactly as the profile spelled it.
func TestHandle_AllowsConditionExtensionPinnedByShortOfficialReference(t *testing.T) {
	const (
		reference = "https://runtimeconditions.io/extensions/widget:v1"
		location  = "https://runtimeconditions.io/extensions/widget/v1/runtimeconditions.extension.yaml"
	)
	doc := []byte(`metadata: {id: ` + location + `, version: v1}` + string(widgetExtensionDoc[len("\nmetadata:\n  id: "+widgetExtensionURI):]))
	v := &RuntimeConditionsProfileValidator{Loader: resolver.NewInMemoryLoader(map[string][]byte{location: doc})}
	req := newAdmissionRequest(t, `{
		"extensions": ["`+reference+`"],
		"conditions": [
			{"kind": "widget", "extension": "`+reference+`", "interface": {"type": "http", "uri": "https://example.com"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Fatalf("expected allowed: the Condition names the short reference from the profile, got denied: %s", resp.Result.Message)
	}
}

// A profile that lists extensions by their full ids, where one extension also
// depends on another that the profile lists itself.
func TestHandle_AllowsFullIdsWithSharedDependency(t *testing.T) {
	const (
		base = "https://runtimeconditions.io/extensions/"
		core = base + "core/v1/runtimeconditions.extension.yaml"
		more = base + "more/v1/runtimeconditions.extension.yaml"
	)
	loader := resolver.NewInMemoryLoader(map[string][]byte{
		core: []byte("metadata: {id: " + core + ", version: v1}\nspec:\n  kinds: [{name: widget}]\n  interfaceTypes: [{name: http, targetKind: widget}]\n" +
			"  schemas:\n    - id: widget-http\n      appliesToKind: widget\n      appliesToInterfaceType: http\n      schema: {type: object}\n"),
		more: []byte("metadata: {id: " + more + ", version: v1}\nspec:\n  dependencies: [" + core + "]\n"),
	})
	v := &RuntimeConditionsProfileValidator{Loader: loader}
	req := newAdmissionRequest(t, `{
		"extensions": ["`+core+`", "`+more+`"],
		"conditions": [
			{"kind": "widget", "extension": "`+core+`", "interface": {"type": "http"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Fatalf("expected allowed, got denied: %s", resp.Result.Message)
	}
	if len(resp.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", resp.Warnings)
	}
}

// The profile lists both extensions in the short <url>:<version> form, while
// the second one depends on the first by its full file URL. That is one file
// under two names, not two extensions that declare the same type.
func TestHandle_AllowsShortFormExtensionsWithFullIdDependency(t *testing.T) {
	const (
		shortCore = "https://runtimeconditions.io/extensions/core:v1"
		fullCore  = "https://runtimeconditions.io/extensions/core/v1/runtimeconditions.extension.yaml"
		shortMore = "https://runtimeconditions.io/extensions/more:v1"
		fullMore  = "https://runtimeconditions.io/extensions/more/v1/runtimeconditions.extension.yaml"
	)
	loader := resolver.NewInMemoryLoader(map[string][]byte{
		fullCore: []byte("metadata: {id: " + fullCore + ", version: v1}\nspec:\n  kinds: [{name: widget}]\n  interfaceTypes: [{name: http, targetKind: widget}]\n" +
			"  schemas:\n    - id: widget-http\n      appliesToKind: widget\n      appliesToInterfaceType: http\n      schema: {type: object}\n"),
		fullMore: []byte("metadata: {id: " + fullMore + ", version: v1}\nspec:\n  dependencies: [" + fullCore + "]\n"),
	})
	v := &RuntimeConditionsProfileValidator{Loader: loader}
	req := newAdmissionRequest(t, `{
		"extensions": ["`+shortCore+`", "`+shortMore+`"],
		"conditions": [
			{"kind": "widget", "extension": "`+shortCore+`", "interface": {"type": "http"}},
			{"kind": "widget", "extension": "`+fullCore+`", "interface": {"type": "http"}},
			{"kind": "widget", "interface": {"type": "http"}}
		]
	}`)

	resp := v.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Fatalf("expected allowed, got denied: %s", resp.Result.Message)
	}
	if len(resp.Warnings) != 0 {
		t.Fatalf("one file under two names is not an ambiguous kind, got warnings %v", resp.Warnings)
	}
}
