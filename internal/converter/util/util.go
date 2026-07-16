package util

import (
	"fmt"
	"log"
	"path"
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/options"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// FieldHasBoolExtension reports whether the field's options carry the extension
// with the given field number set to boolean true. It resolves the extension
// whether it was parsed as a known extension or preserved as an unknown field —
// the latter being the common case for extensions not compiled into this plugin,
// which is why the number, rather than a generated Go type, is the key.
func FieldHasBoolExtension(field protoreflect.FieldDescriptor, number int32) bool {
	options := field.Options()
	if options == nil {
		return false
	}
	message := options.ProtoReflect()

	found := false
	message.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if int32(fd.Number()) == number && fd.Kind() == protoreflect.BoolKind && v.Bool() {
			found = true
			return false
		}
		return true
	})
	if found {
		return true
	}

	// Not known to this plugin's registry: walk the raw unknown option bytes.
	unknown := message.GetUnknown()
	for len(unknown) > 0 {
		num, kind, n := protowire.ConsumeTag(unknown)
		if n < 0 {
			return false
		}
		unknown = unknown[n:]
		if int32(num) == number && kind == protowire.VarintType {
			value, vn := protowire.ConsumeVarint(unknown)
			if vn < 0 {
				return false
			}
			if value == 1 {
				return true
			}
			unknown = unknown[vn:]
			continue
		}
		vn := protowire.ConsumeFieldValue(num, kind, unknown)
		if vn < 0 {
			return false
		}
		unknown = unknown[vn:]
	}
	return false
}

// MessageHasFieldExtension reports whether any field of the message carries the
// given bool extension set to true. It decides whether a message opts in to
// allowlist filtering driven by FilterRequiredFieldExtension.
func MessageHasFieldExtension(message protoreflect.MessageDescriptor, number int32) bool {
	fields := message.Fields()
	for i := 0; i < fields.Len(); i++ {
		if FieldHasBoolExtension(fields.Get(i), number) {
			return true
		}
	}
	return false
}

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
	switch descriptor.(type) {
	case protoreflect.MessageDescriptor:
		return TrimFqn(opts, descriptor)
	case protoreflect.EnumDescriptor:
		return fmt.Sprintf("%s.%s", TrimMessageSuffix(opts, TrimFqn(opts, descriptor.Parent())), TrimMessageSuffix(opts, string(descriptor.Name())))
	}

	log.Fatalf("Unsupported descriptor type: %T", descriptor)
	return ""
}

func TrimFqn(opts options.Options, descriptor protoreflect.Descriptor) string {
	descriptorName := string(descriptor.FullName())
	if opts.WithoutFqn {
		descriptorName = string(descriptor.Name())
	}

	return TrimMessageSuffix(opts, descriptorName)
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

func MakePath(opts options.Options, main string) string {
	return path.Join(opts.PathPrefix, main)
}

func AppendStringDedupe(strs []string, str string) []string {
	for _, s := range strs {
		if str == s {
			return strs
		}
	}
	return append(strs, str)
}
