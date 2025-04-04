package googleapi

import (
	"fmt"
	"github.com/pb33f/libopenapi/utils"
	"log"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/options"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/schema"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/util"
)

func MakePathItems(opts options.Options, spec *v3.Document, md protoreflect.MethodDescriptor, schemas map[string]map[string]struct{}) *orderedmap.Map[string, *v3.PathItem] {
	if opts.IgnoreGoogleapiHTTP {
		return nil
	}
	mdopts := md.Options()
	if !proto.HasExtension(mdopts, annotations.E_Http) {
		return nil
	}
	rule, ok := proto.GetExtension(mdopts, annotations.E_Http).(*annotations.HttpRule)
	if !ok {
		return nil
	}
	return httpRuleToPathMap(opts, spec, schemas, md, rule)
}

func httpRuleToPathMap(opts options.Options, spec *v3.Document, schemas map[string]map[string]struct{}, md protoreflect.MethodDescriptor, rule *annotations.HttpRule) *orderedmap.Map[string, *v3.PathItem] {
	var method, template string
	switch pattern := rule.GetPattern().(type) {
	case *annotations.HttpRule_Get:
		method, template = http.MethodGet, pattern.Get
	case *annotations.HttpRule_Put:
		method, template = http.MethodPut, pattern.Put
	case *annotations.HttpRule_Post:
		method, template = http.MethodPost, pattern.Post
	case *annotations.HttpRule_Delete:
		method, template = http.MethodDelete, pattern.Delete
	case *annotations.HttpRule_Patch:
		method, template = http.MethodPatch, pattern.Patch
	case *annotations.HttpRule_Custom:
		method, template = pattern.Custom.GetKind(), pattern.Custom.GetPath()
	default:
		slog.Warn("invalid type of pattern for HTTP rule", slog.Any("pattern", pattern))
		return nil
	}
	if method == "" {
		slog.Warn("invalid HTTP rule: method is blank", slog.Any("method", md))
		return nil
	}
	if template == "" {
		slog.Warn("invalid HTTP rule: path template is blank", slog.Any("method", md))
		return nil
	}

	tokens, err := RunPathPatternLexer(template)
	if err != nil {
		slog.Warn("unable to parse template pattern", slog.Any("error", err), slog.String("template", template))
		return nil
	}

	paths := orderedmap.New[string, *v3.PathItem]()
	pathItem := &v3.PathItem{}

	fd := md.ParentFile()

	service := md.Parent().(protoreflect.ServiceDescriptor)
	serviceName := string(service.FullName())
	if opts.WithoutFqn {
		serviceName = string(service.Name())
	}
	if opts.TrimServiceSuffix != "" {
		serviceName = strings.TrimSuffix(serviceName, opts.TrimServiceSuffix)
	}

	if len(opts.RenameService) != 0 {
		if serviceName == opts.RenameService[0] {
			serviceName = opts.RenameService[1]
		}
	}

	operationId := string(md.FullName())
	if opts.WithoutFqn {
		operationId = fmt.Sprintf("%s.%s", serviceName, md.Name())
	}

	op := &v3.Operation{
		Summary:     string(md.Name()),
		OperationId: operationId,
		Tags:        []string{serviceName},
		Description: util.FormatComments(fd.SourceLocations().ByDescriptor(md)),
	}

	fieldNamesInPath := map[string]struct{}{}
	for _, param := range partsToParameter(tokens) {
		field, jsonPath := resolveField(md.Input(), param)
		if field != nil {
			// This field is only top level, so we will filter out the param from
			// query/param or request body
			fieldNamesInPath[string(field.Name())] = struct{}{}
			fieldNamesInPath[strings.Join(jsonPath, ".")] = struct{}{} // sometimes JSON field names are used
			loc := fd.SourceLocations().ByDescriptor(field)
			op.Parameters = append(op.Parameters, &v3.Parameter{
				Name:        param,
				Required:    proto.Bool(true),
				In:          "path",
				Description: util.FormatComments(loc),
				Schema:      schema.FieldToSchema(opts, nil, field),
			})
		} else {
			slog.Warn("path field not found", slog.String("param", param))
		}
	}

	switch rule.Body {
	case "":
		op.Parameters = append(op.Parameters, flattenToParams(opts, md.Input(), "", fieldNamesInPath)...)
		// Remove the reference to the schema.
		id := string(md.Input().FullName())
		references := schemas[id]
		delete(references, fmt.Sprintf("%s-input", md.FullName()))

		// If the requested object is no longer referenced remove it completely from the spec
		if len(references) == 0 {
			_, present := spec.Components.Schemas.Delete(util.DescriptorToId(opts, md.Input()))
			if !present {
				log.Fatalf("Wanted to delete schema %s but it was not found", id)
			}
		}
	case "*":
		if len(fieldNamesInPath) > 0 {
			_, s := schema.MessageToSchema(opts, md.Input())
			for name := range fieldNamesInPath {
				s.Properties.Delete(name)

				index := slices.Index(s.Required, name)
				if index != -1 {
					// Require to copy the required array as deleting it from slice removes it from both the
					// components.schema as this path item.
					newRequired := make([]string, 0, len(s.Required)-1)
					for i, req := range s.Required {
						if i == index {
							continue
						}
						newRequired = append(newRequired, req)
					}

					s.Required = newRequired
					if len(s.Required) == 0 {
						s.Required = nil
					}
				}
			}
			if s.Properties.Len() > 0 {
				op.RequestBody = util.MethodToRequestBody(opts, md, base.CreateSchemaProxy(s), false)

				// Remove the reference to the schema.
				id := string(md.Input().FullName())
				references := schemas[id]
				delete(references, fmt.Sprintf("%s-input", md.FullName()))

				// If the requested object is no longer referenced remove it completely from the spec
				if len(references) == 0 {
					_, present := spec.Components.Schemas.Delete(util.DescriptorToId(opts, md.Input()))
					if !present {
						log.Fatalf("Wanted to delete schema %s but it was not found", id)
					}
				}
			}
		} else {

			// Remove the reference to the schema.
			id := string(md.Input().FullName())
			references := schemas[id]
			delete(references, fmt.Sprintf("%s-input", md.FullName()))

			// If the requested object is no longer referenced remove it completely from the spec
			if len(references) == 0 {
				_, present := spec.Components.Schemas.Delete(util.DescriptorToId(opts, md.Input()))
				if !present {
					log.Fatalf("Wanted to delete schema %s but it was not found", id)
				}
				// return request body with parameters
				_, s := schema.MessageToSchema(opts, md.Input())
				op.RequestBody = util.MethodToRequestBody(opts, md, base.CreateSchemaProxy(s), false)
			} else {
				inputId := util.FormatTypeRef(opts, util.DescriptorToId(opts, md.Input()))
				s := base.CreateSchemaProxyRef("#/components/schemas/" + inputId)
				op.RequestBody = util.MethodToRequestBody(opts, md, s, false)
			}
		}

	default:
		if field, _ := resolveField(md.Input(), rule.Body); field != nil {
			loc := fd.SourceLocations().ByDescriptor(field)
			bodySchema := schema.FieldToSchema(opts, nil, field)
			op.RequestBody = &v3.RequestBody{
				Description: util.FormatComments(loc),
				Content:     util.MakeMediaTypes(opts, bodySchema, false, false),
			}
		} else {
			slog.Warn("body field not found", slog.String("param", rule.Body))
		}
	}

	// Responses
	codeMap := orderedmap.New[string, *v3.Response]()
	mediaType := orderedmap.New[string, *v3.MediaType]()
	var outputSchema *base.SchemaProxy
	if rule.ResponseBody == "" {
		id := string(md.Output().FullName())
		references := schemas[id]
		// If this is the only method referencing the schema, then reference it directly
		// Otherwise, reference it by id, other methods might already use the schema as reference.
		if opts.TrimOneUseResponseType && len(references) == 1 {
			delete(references, fmt.Sprintf("%s-output", md.FullName()))
			_, present := spec.Components.Schemas.Delete(util.DescriptorToId(opts, md.Output()))
			if !present {
				log.Fatalf("Wanted to delete schema %s but it was not found", id)
			}
			// return request body with parameters
			_, s := schema.MessageToSchema(opts, md.Output())
			outputSchema = base.CreateSchemaProxy(s)
		} else {
			outputId := util.FormatTypeRef(opts, util.DescriptorToId(opts, md.Output()))
			outputSchema = base.CreateSchemaProxyRef("#/components/schemas/" + outputId)
		}
	} else {
		if fd, _ := resolveField(md.Output(), rule.ResponseBody); fd != nil {
			outputSchema = schema.FieldToSchema(opts, nil, fd)
		}
	}

	mediaType.Set("application/json", &v3.MediaType{Schema: outputSchema})
	codeMap.Set("200", &v3.Response{
		Description: "Success",
		Content:     mediaType,
	})

	op.Responses = &v3.Responses{
		Codes: codeMap,
	}
	if !opts.TrimConnectRPC {
		op.Responses.Default = &v3.Response{
			Description: "Error",
			Content: util.MakeMediaTypes(
				opts,
				base.CreateSchemaProxyRef("#/components/schemas/connect.error"),
				false,
				false,
			),
		}
	}

	if opts.CustomDefaultError {
		op.Responses.Default = &v3.Response{
			Description: "Error",
			Content: util.MakeMediaTypes(
				opts,
				base.CreateSchemaProxyRef("#/components/schemas/Error"),
				false,
				false,
			),
		}
	}

	if opts.GlobalHeader {
		globalParam := &v3.Parameter{
			Name:        "Rootline-Version",
			Required:    util.BoolPtr(true),
			In:          "header",
			Description: "The version of the Rootline API.",
			Schema: base.CreateSchemaProxy(&base.Schema{
				Type:    []string{"string"},
				Default: utils.CreateStringNode(opts.Version),
			}),
		}
		op.Parameters = append([]*v3.Parameter{globalParam}, op.Parameters...)
	}

	switch method {
	case http.MethodGet:
		pathItem.Get = op
	case http.MethodPut:
		pathItem.Put = op
	case http.MethodPost:
		pathItem.Post = op
	case http.MethodDelete:
		pathItem.Delete = op
	case http.MethodPatch:
		pathItem.Patch = op
	default:
	}
	paths.Set(partsToOpenAPIPath(tokens), pathItem)

	for _, binding := range rule.AdditionalBindings {
		pathMap := httpRuleToPathMap(opts, nil, nil, md, binding)
		for pair := pathMap.First(); pair != nil; pair = pair.Next() {
			path := util.MakePath(opts, pair.Key())
			paths.Set(path, pair.Value())
		}
	}
	return paths
}

func resolveField(md protoreflect.MessageDescriptor, param string) (protoreflect.FieldDescriptor, []string) {
	jsonParts := []string{}
	current := md
	var fd protoreflect.FieldDescriptor
	for _, paramPart := range strings.Split(param, ".") {
		if field := fieldByName(current, paramPart); field == nil {
			return nil, nil
		} else {
			fd = field
			jsonParts = append(jsonParts, fd.JSONName())
			current = field.Message()
		}
	}
	return fd, jsonParts
}

func fieldByName(md protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
	slog.Debug("fieldByName", "name", md.FullName(), "name", name)
	fields := md.Fields()
	if field := fields.ByName(protoreflect.Name(name)); field != nil {
		return field
	}
	if field := fields.ByJSONName(name); field != nil {
		return field
	}
	return nil
}

func partsToParameter(tokens []Token) []string {
	params := []string{}
	for _, token := range tokens {
		if token.Type == TokenVariable {
			params = append(params, token.Value)
		}
	}
	return params
}

func partsToOpenAPIPath(tokens []Token) string {
	var b strings.Builder
	for _, token := range tokens {
		switch token.Type {
		case TokenSlash:
			b.WriteByte('/')
		case TokenEOF:
		case TokenColon:
			b.WriteByte(':')
		case TokenLiteral:
			b.WriteString(token.Value)
		case TokenIdent:
			b.WriteString(token.Value)
		case TokenVariable:
			b.WriteByte('{')
			b.WriteString(token.Value)
			b.WriteByte('}')
		}
	}
	return b.String()
}

func flattenToParams(opts options.Options, md protoreflect.MessageDescriptor, jsonPrefix string, seen map[string]struct{}) []*v3.Parameter {
	params := []*v3.Parameter{}
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		paramName := jsonPrefix + string(field.JSONName())
		if opts.WithProtoNames {
			paramName = string(field.Name())
		}
		// exclude fields already found in the path
		if _, ok := seen[string(field.FullName())]; ok {
			continue
		}
		if _, ok := seen[paramName]; ok {
			continue
		}
		seen[string(field.FullName())] = struct{}{}
		switch field.Kind() {
		case protoreflect.MessageKind:
			params = append(params, flattenToParams(opts, field.Message(), paramName+".", seen)...)
		default:
			parent := &base.Schema{}
			schema := schema.FieldToSchema(opts, base.CreateSchemaProxy(parent), field)
			var required *bool
			if len(parent.Required) > 0 {
				required = util.BoolPtr(true)
			}
			loc := field.ParentFile().SourceLocations().ByDescriptor(field)
			params = append(params, &v3.Parameter{
				Name:        paramName,
				In:          "query",
				Description: util.FormatComments(loc),
				Schema:      schema,
				Required:    required,
			})
		}
	}
	return params
}
