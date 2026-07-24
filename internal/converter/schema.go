package converter

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/fatih/camelcase"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/pb33f/libopenapi/utils"
	"go.yaml.in/yaml/v4"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/options"
	schemapkg "github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/schema"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/util"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/visibility"
)

// referralCollector mirrors the fork's old collection pass to record, per message
// full-name, the set of "referrers" pointing at it: method `-input`/`-output`
// pseudo-refs, field full-names, and (for nested/file types) their own name. The
// resulting counts drive trim-one-use-types (responses + single-use request bodies).
type referralCollector struct {
	opts      options.Options
	seen      map[protoreflect.FullName]struct{}
	referrals map[string]map[string]struct{}
}

// collectReferrals builds the referral map for a single file, visiting the same
// messages that get emitted as components (respecting trim-unused-types,
// filter-public, and field filtering).
func collectReferrals(opts options.Options, fd protoreflect.FileDescriptor) map[string]map[string]struct{} {
	rc := &referralCollector{
		opts:      opts,
		seen:      map[protoreflect.FullName]struct{}{},
		referrals: map[string]map[string]struct{}{},
	}
	if !opts.TrimUnusedTypes {
		messages := fd.Messages()
		for i := 0; i < messages.Len(); i++ {
			m := messages.Get(i)
			rc.collectMessage(string(m.FullName()), m)
		}
	}
	services := fd.Services()
	for i := 0; i < services.Len(); i++ {
		methods := services.Get(i).Methods()
		for j := 0; j < methods.Len(); j++ {
			method := methods.Get(j)
			if opts.FilterPublic && !methodIsPublic(method) {
				continue
			}
			rc.collectMessage(fmt.Sprintf("%s-input", method.FullName()), method.Input())
			rc.collectMessage(fmt.Sprintf("%s-output", method.FullName()), method.Output())
		}
	}
	return rc.referrals
}

func (rc *referralCollector) addReferral(name protoreflect.FullName, referral string) {
	key := string(name)
	if rc.referrals[key] == nil {
		rc.referrals[key] = map[string]struct{}{}
	}
	rc.referrals[key][referral] = struct{}{}
}

func (rc *referralCollector) collectMessage(referral string, tt protoreflect.MessageDescriptor) {
	if tt == nil {
		return
	}
	rc.addReferral(tt.FullName(), referral)
	if _, ok := rc.seen[tt.FullName()]; ok {
		return
	}
	rc.seen[tt.FullName()] = struct{}{}

	fields := tt.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if schemapkg.FieldFilteredOut(rc.opts, tt, field) {
			continue
		}
		if field.Message() != nil && !field.IsMap() {
			rc.collectMessage(string(field.FullName()), field.Message())
		}
		if field.IsMap() {
			if mv := field.MapValue(); mv != nil && mv.Message() != nil {
				rc.collectMessage(string(field.FullName()), mv.Message())
			}
		}
	}
	messages := tt.Messages()
	for i := 0; i < messages.Len(); i++ {
		m := messages.Get(i)
		if !m.IsMapEntry() {
			rc.collectMessage(string(m.FullName()), m)
		}
	}
}

// addCustomDefaultErrorSchema adds the fork's "Error" component schema referenced
// by the custom-default-error default response.
func addCustomDefaultErrorSchema(doc *v3.Document) {
	errorProps := orderedmap.New[string, *base.SchemaProxy]()
	errorProps.Set("http_status_code", base.CreateSchemaProxy(&base.Schema{
		Description: "The HTTP status code of the response.",
		Examples:    []*yaml.Node{utils.CreateStringNode("404")},
		Type:        []string{"integer"},
		Format:      "int32",
	}))
	errorProps.Set("error_type", base.CreateSchemaProxy(&base.Schema{
		Description: "The type of error returned.",
		Type:        []string{"string"},
		Examples:    []*yaml.Node{utils.CreateStringNode("validation_error")},
		Enum: []*yaml.Node{
			utils.CreateStringNode("idempotency_error"),
			utils.CreateStringNode("internal_error"),
			utils.CreateStringNode("security_error"),
			utils.CreateStringNode("syntax_error"),
			utils.CreateStringNode("validation_error"),
		},
	}))
	errorProps.Set("error_code", base.CreateSchemaProxy(&base.Schema{
		Description: "A short explanation detailing the error.",
		Examples:    []*yaml.Node{utils.CreateStringNode("not_found")},
		Type:        []string{"string"},
	}))
	errorProps.Set("error_message", base.CreateSchemaProxy(&base.Schema{
		Description: "A developer-facing error message.",
		Examples:    []*yaml.Node{utils.CreateStringNode("Resource not found")},
		Type:        []string{"string"},
	}))
	errorProps.Set("field", base.CreateSchemaProxy(&base.Schema{
		Description: "Indicates the field that caused the error, if applicable.",
		Examples:    []*yaml.Node{utils.CreateStringNode("id")},
		Type:        []string{"string"},
	}))

	doc.Components.Schemas.Set("Error", base.CreateSchemaProxy(&base.Schema{
		Title:       "Error",
		Description: `Error object containing information about the error.`,
		Properties:  errorProps,
		Type:        []string{"object"},
	}))
}

func AddMessageSchemas(opts options.Options, md protoreflect.MessageDescriptor, doc *v3.Document) {
	if md == nil {
		return
	}
	if visibility.ShouldBeFiltered(visibility.GetVisibilityRule(md), opts.AllowedVisibilities) {
		opts.Logger.Debug("Filtering message due to visibility", slog.String("message", string(md.FullName())), slog.Any("restriction_selectors", opts.AllowedVisibilities))
		return
	}
	if _, ok := doc.Components.Schemas.Get(util.DescriptorToId(opts, md)); ok {
		return
	}
	// Key by DescriptorToId so the component id matches how fields reference it
	// (ReferenceFieldToSchema uses DescriptorToId). MessageToSchema returns the same
	// id for regular messages, but well-known wrappers return their FQN, which under
	// without-fqn diverges from the short ref and leaves the $ref unresolved.
	_, schema := schemapkg.MessageToSchema(opts, md)
	if schema != nil {
		doc.Components.Schemas.Set(util.DescriptorToId(opts, md), base.CreateSchemaProxy(schema))
	}

	// Messages can have fields. Skip types reachable only through fields that are
	// filtered out (filter-private / required-field-extension allowlist), so they
	// are trimmed alongside those fields.
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if schemapkg.FieldFilteredOut(opts, md, field) {
			continue
		}
		AddFieldToSchema(opts, field, doc)
	}

	// Messages can have enums
	enums := md.Enums()
	for i := 0; i < enums.Len(); i++ {
		AddEnumToSchema(opts, enums.Get(i), doc)
	}

	// Messages can have messages
	messages := md.Messages()
	for i := 0; i < messages.Len(); i++ {
		AddMessageSchemas(opts, messages.Get(i), doc)
	}
}

func AddFieldToSchema(opts options.Options, fd protoreflect.FieldDescriptor, doc *v3.Document) {
	if fd == nil {
		return
	}
	AddEnumToSchema(opts, fd.Enum(), doc)
	AddMessageSchemas(opts, fd.Message(), doc)
	AddFieldToSchema(opts, fd.MapKey(), doc)
	AddFieldToSchema(opts, fd.MapValue(), doc)
}

func AddEnumToSchema(opts options.Options, ed protoreflect.EnumDescriptor, doc *v3.Document) {
	if ed == nil {
		return
	}
	if visibility.ShouldBeFiltered(visibility.GetVisibilityRule(ed), opts.AllowedVisibilities) {
		opts.Logger.Debug("Filtering enum due to visibility", slog.String("enum", string(ed.FullName())), slog.Any("restriction_selectors", opts.AllowedVisibilities))
		return
	}
	if _, ok := doc.Components.Schemas.Get(util.DescriptorToId(opts, ed)); ok {
		return
	}
	name, schema := enumToSchema(opts, ed)
	if schema != nil {
		doc.Components.Schemas.Set(name, base.CreateSchemaProxy(schema))
	}
}

func enumToSchema(opts options.Options, tt protoreflect.EnumDescriptor) (string, *base.Schema) {
	opts.Logger.Debug("enumToSchema", slog.Any("descriptor", tt.FullName()))
	children := []*yaml.Node{}
	values := tt.Values()
	for i := 0; i < values.Len(); i++ {
		value := values.Get(i)
		if visibility.ShouldBeFiltered(visibility.GetVisibilityRule(value), opts.AllowedVisibilities) {
			opts.Logger.Debug("Filtering enum value due to visibility", slog.String("enum_value", string(value.FullName())), slog.Any("restriction_selectors", opts.AllowedVisibilities))
			continue // Skip this enum value
		}
		name := string(value.Name())
		if opts.TrimEnumNamePrefix {
			prefix := strings.ToUpper(strings.Join(camelcase.Split(string(tt.Name())), "_")) + "_"
			name = strings.TrimPrefix(name, prefix)
		}
		children = append(children, utils.CreateStringNode(name))
		if opts.IncludeNumberEnumValues {
			children = append(children, utils.CreateIntNode(strconv.FormatInt(int64(value.Number()), 10)))
		}
	}

	title := string(tt.Name())
	if opts.FullyQualifiedMessageNames {
		title = string(tt.FullName())
	}
	title = util.TrimMessageSuffix(opts, title)
	types := []string{"string"}
	if opts.IncludeNumberEnumValues {
		types = append(types, "number")
	}
	s := &base.Schema{
		Title:       title,
		Description: util.FormatComments(tt.ParentFile().SourceLocations().ByDescriptor(tt)),
		Type:        types,
		Enum:        children,
	}
	return util.DescriptorToId(opts, tt), s
}
