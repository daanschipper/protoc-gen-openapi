package converter

import (
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/pb33f/libopenapi/utils"
	"go.yaml.in/yaml/v4"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/options"
	schemapkg "github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/schema"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/util"
	oasExtension "github.com/sudorandom/protoc-gen-connect-openapi/openapi"
)

// addWebhooksFromFile emits an OpenAPI 3.1 webhook for every event declared on a
// message annotated (openapi.webhook_envelope). The events are emitted in list order;
// each becomes one webhook whose request body is the envelope narrowed to that event's
// payload, keyed and summarised by the event type and grouped by its category tag.
func addWebhooksFromFile(opts options.Options, fd protoreflect.FileDescriptor, spec *v3.Document) {
	messages := fd.Messages()
	for i := 0; i < messages.Len(); i++ {
		msg := messages.Get(i)
		if events := webhookEnvelope(msg); len(events) > 0 {
			addWebhooksFromEnvelope(opts, msg, events, spec)
		}
	}
}

func addWebhooksFromEnvelope(opts options.Options, envelope protoreflect.MessageDescriptor, events []*oasExtension.WebhookEvent, spec *v3.Document) {
	// Emit the envelope and its transitive payload schemas even when the file has no
	// service and trim-unused-types is on (the normal file loop skips them then).
	AddMessageSchemas(opts, envelope, spec)

	_, envSchema := schemapkg.MessageToSchema(opts, envelope)
	if envSchema == nil || envSchema.Properties == nil {
		return
	}

	// Property name of every oneof member (dropped from a narrowed variant except the
	// event's own payload), the event_type field (pinned to a const), and a lookup from
	// proto field name to descriptor to resolve each event's payload_field.
	oneofMembers := map[string]struct{}{}
	eventTypeField := ""
	fieldByName := map[string]protoreflect.FieldDescriptor{}
	fields := envelope.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		fieldByName[string(field.Name())] = field
		if oneOf := field.ContainingOneof(); oneOf != nil && !oneOf.IsSynthetic() {
			oneofMembers[util.MakeFieldName(opts, field)] = struct{}{}
		}
		if string(field.Name()) == "event_type" {
			eventTypeField = util.MakeFieldName(opts, field)
		}
	}

	seenCategory := map[string]struct{}{}
	for _, event := range events {
		field, ok := fieldByName[event.GetPayloadField()]
		if !ok {
			continue
		}
		payloadField := util.MakeFieldName(opts, field)
		id := webhookSchemaID(event.GetType())
		narrowed := narrowEnvelopeSchema(envSchema, oneofMembers, payloadField, eventTypeField, event.GetType())
		spec.Components.Schemas.Set(id, base.CreateSchemaProxy(narrowed))
		spec.Webhooks.Set(event.GetType(), webhookPathItem(opts, event, id))

		// Register each category as a top-level tag once, in first-seen order, so the
		// tags are defined (redocly) and keep the sidebar's category order.
		if category := event.GetCategory(); category != "" {
			if _, seen := seenCategory[category]; !seen {
				seenCategory[category] = struct{}{}
				spec.Tags = append(spec.Tags, &base.Tag{Name: category})
			}
		}
	}
}

// narrowEnvelopeSchema copies the envelope schema keeping only the scalar envelope
// fields plus the single payload for this event, pins event_type to a const, and drops
// the oneof (anyOf) constraint so the reader sees exactly this event's shape.
func narrowEnvelopeSchema(env *base.Schema, oneofMembers map[string]struct{}, payloadField, eventTypeField, eventType string) *base.Schema {
	props := orderedmap.New[string, *base.SchemaProxy]()
	for pair := env.Properties.First(); pair != nil; pair = pair.Next() {
		name := pair.Key()
		if _, isOneof := oneofMembers[name]; isOneof && name != payloadField {
			continue
		}
		if name == eventTypeField {
			props.Set(name, eventTypeConstSchema(pair.Value(), eventType))
			continue
		}
		props.Set(name, pair.Value())
	}

	required := []string{payloadField}
	if eventTypeField != "" {
		required = append([]string{eventTypeField}, required...)
	}

	return &base.Schema{
		Title:                pascalCase(eventType),
		Description:          env.Description,
		Type:                 []string{"object"},
		Properties:           props,
		Required:             required,
		AdditionalProperties: env.AdditionalProperties,
	}
}

func eventTypeConstSchema(original *base.SchemaProxy, eventType string) *base.SchemaProxy {
	description := ""
	if original != nil {
		if s := original.Schema(); s != nil {
			description = s.Description
		}
	}
	return base.CreateSchemaProxy(&base.Schema{
		Type:        []string{"string"},
		Description: description,
		Const:       utils.CreateStringNode(eventType),
		Examples:    []*yaml.Node{utils.CreateStringNode(eventType)},
	})
}

func webhookPathItem(opts options.Options, event *oasExtension.WebhookEvent, schemaID string) *v3.PathItem {
	responses := &v3.Responses{Codes: orderedmap.New[string, *v3.Response]()}
	responses.Codes.Set("200", &v3.Response{
		Description: "Return a 2xx status code to acknowledge receipt of the event.",
	})

	op := &v3.Operation{
		Summary:     event.GetType(),
		Description: event.GetDescription(),
		OperationId: "webhook." + event.GetType(),
		RequestBody: &v3.RequestBody{
			Content:  util.MakeMediaTypes(opts, base.CreateSchemaProxyRef("#/components/schemas/"+schemaID), true, false),
			Required: util.BoolPtr(true),
		},
		Responses: responses,
	}
	if category := event.GetCategory(); category != "" {
		op.Tags = []string{category}
	}
	return &v3.PathItem{Post: op}
}

func webhookSchemaID(eventType string) string {
	return pascalCase(eventType) + "Event"
}

func pascalCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	})
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		if len(part) > 1 {
			b.WriteString(part[1:])
		}
	}
	return b.String()
}

func webhookEnvelope(msg protoreflect.MessageDescriptor) []*oasExtension.WebhookEvent {
	opts := msg.Options()
	if opts == nil || !proto.HasExtension(opts, oasExtension.E_WebhookEnvelope) {
		return nil
	}
	list, _ := proto.GetExtension(opts, oasExtension.E_WebhookEnvelope).(*oasExtension.WebhookEventList)
	if list == nil {
		return nil
	}
	return list.GetEvent()
}
