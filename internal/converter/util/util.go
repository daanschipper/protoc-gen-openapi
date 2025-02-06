package util

import (
	"fmt"
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/options"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func AppendComponents(spec *v3.Document, components *v3.Components) {
	for pair := components.Schemas.First(); pair != nil; pair = pair.Next() {
		spec.Components.Schemas.Set(pair.Key(), pair.Value())
	}
	for pair := components.Responses.First(); pair != nil; pair = pair.Next() {
		spec.Components.Responses.Set(pair.Key(), pair.Value())
	}
	for pair := components.Parameters.First(); pair != nil; pair = pair.Next() {
		spec.Components.Parameters.Set(pair.Key(), pair.Value())
	}
	for pair := components.Examples.First(); pair != nil; pair = pair.Next() {
		spec.Components.Examples.Set(pair.Key(), pair.Value())
	}
	for pair := components.RequestBodies.First(); pair != nil; pair = pair.Next() {
		spec.Components.RequestBodies.Set(pair.Key(), pair.Value())
	}
	for pair := components.Headers.First(); pair != nil; pair = pair.Next() {
		spec.Components.Headers.Set(pair.Key(), pair.Value())
	}
	for pair := components.SecuritySchemes.First(); pair != nil; pair = pair.Next() {
		spec.Components.SecuritySchemes.Set(pair.Key(), pair.Value())
	}
	for pair := components.Links.First(); pair != nil; pair = pair.Next() {
		spec.Components.Links.Set(pair.Key(), pair.Value())
	}
	for pair := components.Callbacks.First(); pair != nil; pair = pair.Next() {
		spec.Components.Callbacks.Set(pair.Key(), pair.Value())
	}
}

func TypeFieldDescription(opts options.Options, tt protoreflect.FieldDescriptor) string {
	b := strings.Builder{}
	b.WriteString(FormatComments(tt.ParentFile().SourceLocations().ByDescriptor(tt)))
	if opts.WithProtoAnnotations {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString("(proto ")
		switch tt.Kind() {
		case protoreflect.MessageKind:
			b.WriteString(string(tt.Message().FullName()))
		case protoreflect.EnumKind:
			b.WriteString(string(tt.Enum().FullName()))
		default:
			b.WriteString(tt.Kind().String())
		}
		b.WriteByte(')')
	}
	return b.String()
}

func FormatComments(loc protoreflect.SourceLocation) string {
	var builder strings.Builder
	if loc.LeadingComments != "" {
		builder.WriteString(strings.TrimSpace(loc.LeadingComments))
		builder.WriteString(" ")
	}
	if loc.TrailingComments != "" {
		builder.WriteString(strings.TrimSpace(loc.TrailingComments))
		builder.WriteString(" ")
	}
	return strings.TrimSpace(builder.String())
}

func BoolPtr(b bool) *bool {
	return &b
}

func FormatTypeRef(opts options.Options, t string) string {
	return TrimMessageSuffix(opts, strings.TrimPrefix(t, "."))
}

func DescriptorToId(opts options.Options, descriptor protoreflect.Descriptor) string {
	// If in root of document then strip the prefix of the descriptor.
	if descriptor.Parent().Name() == "" {
		return TrimMessageSuffix(opts, string(descriptor.FullName()))
	}

	// Else strip prefix also from parent name.
	// Only one level of nesting is supported.
	return fmt.Sprintf("%s.%s", TrimMessageSuffix(opts, string(descriptor.Parent().FullName())), TrimMessageSuffix(opts, string(descriptor.Name())))
}

func TrimMessageSuffix(opts options.Options, messageName string) string {
	if opts.TrimMessageSuffix == "" {
		return messageName
	}
	return strings.TrimSuffix(messageName, opts.TrimMessageSuffix)
}

func IsMethodDeprecated(md protoreflect.MethodDescriptor) *bool {
	options, ok := md.Options().(*descriptorpb.MethodOptions)
	if !ok || options == nil {
		return nil
	}
	if options.Deprecated == nil {
		return nil
	}
	return options.Deprecated
}

func MethodToRequestBody(opts options.Options, method protoreflect.MethodDescriptor, s *base.SchemaProxy, isStreaming bool) *v3.RequestBody {
	return &v3.RequestBody{
		Content:  MakeMediaTypes(opts, s, true, isStreaming),
		Required: BoolPtr(true),
	}
}

// MakeMediaTypes generates media types with references to the bodies
func MakeMediaTypes(opts options.Options, s *base.SchemaProxy, isRequest, isStreaming bool) *orderedmap.Map[string, *v3.MediaType] {
	mediaTypes := orderedmap.New[string, *v3.MediaType]()
	for _, protocol := range options.Protocols {
		isStreamingDisabled := isStreaming && !opts.WithStreaming
		if isStreaming != protocol.IsStreaming || isStreamingDisabled {
			continue
		}

		_, shouldUse := opts.ContentTypes[protocol.Name]
		if !(isStreaming || shouldUse) {
			continue
		}

		mediaTypes.Set(protocol.ContentType, &v3.MediaType{Schema: s})
	}
	return mediaTypes
}

func MakeFieldName(opts options.Options, fd protoreflect.FieldDescriptor) string {
	if opts.WithProtoNames {
		return string(fd.Name())
	}
	return fd.JSONName()
}

func AppendStringDedupe(strs []string, str string) []string {
	for _, s := range strs {
		if str == s {
			return strs
		}
	}
	return append(strs, str)
}
