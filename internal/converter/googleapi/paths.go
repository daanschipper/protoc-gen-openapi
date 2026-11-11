package googleapi

import (
	"fmt"
	"iter"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/pb33f/libopenapi/utils"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	goa3 "github.com/google/gnostic/openapiv3"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/options"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/schema"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/util"
)

// namedPathPattern is a regular expression to match named path patterns in the form {name=path/*/pattern}
var namedPathPattern = regexp.MustCompile("{(.+)=(.+)}")

// PathItemsResult holds path items and any parameters whose default
// descriptions should only be applied after all annotation processing
// (e.g. gnostic) has had a chance to set descriptions first.
type PathItemsResult struct {
	PathItems      *orderedmap.Map[string, *v3.PathItem]
	DeferredParams *orderedmap.Map[string, []*v3.Parameter]
}

func MakePathItems(opts options.Options, md protoreflect.MethodDescriptor, spec *v3.Document, referrals map[string]map[string]struct{}) (*PathItemsResult, bool) {
	if opts.IgnoreGoogleapiHTTP {
		return nil, false
	}
	mdopts := md.Options()
	if !proto.HasExtension(mdopts, annotations.E_Http) {
		return nil, false
	}
	rule, ok := proto.GetExtension(mdopts, annotations.E_Http).(*annotations.HttpRule)
	if !ok {
		return nil, false
	}
	return httpRuleToPathMap(opts, md, rule, nil, spec, referrals), true
}

// dropReferrerAndMaybeDelete removes the given referrer from the input/output
// schema's referral set. If no referrers remain, the component schema is deleted
// and true is returned so the caller can inline the type instead of $ref'ing it.
// Used by trim-one-use-types to inline a single-use request body and drop it
// from components when no longer referenced.
func dropReferrerAndMaybeDelete(opts options.Options, spec *v3.Document, referrals map[string]map[string]struct{}, md protoreflect.MessageDescriptor, referrer string) bool {
	if spec == nil || referrals == nil {
		return false
	}
	refs := referrals[string(md.FullName())]
	delete(refs, referrer)
	if len(refs) == 0 {
		spec.Components.Schemas.Delete(util.DescriptorToId(opts, md))
		return true
	}
	return false
}

func httpRuleToPathMap(opts options.Options, md protoreflect.MethodDescriptor, rule *annotations.HttpRule, resMap map[string]string, spec *v3.Document, referrals map[string]map[string]struct{}) *PathItemsResult {
	if resMap == nil {
		if opts.ResourceMap != nil {
			resMap = opts.ResourceMap
		} else {
			resMap = make(map[string]string)
			CollectResources(md.ParentFile(), resMap)
		}
	}
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
		opts.Logger.Warn("invalid type of pattern for HTTP rule", slog.Any("pattern", pattern))
		return nil
	}
	if method == "" {
		opts.Logger.Warn("invalid HTTP rule: method is blank", slog.Any("method", md))
		return nil
	}
	if template == "" {
		opts.Logger.Warn("invalid HTTP rule: path template is blank", slog.Any("method", md))
		return nil
	}

	tokens, err := RunPathPatternLexer(template)
	if err != nil {
		opts.Logger.Warn("unable to parse template pattern", slog.Any("error", err), slog.String("template", template))
		return nil
	}

	paths := orderedmap.New[string, *v3.PathItem]()
	pathItem := &v3.PathItem{}

	fd := md.ParentFile()
	service := md.Parent().(protoreflect.ServiceDescriptor)

	operationId := string(md.FullName())
	if opts.WithoutFqn {
		operationId = fmt.Sprintf("%s.%s", util.ServiceTagName(opts, service), md.Name())
	} else if opts.ShortOperationIds {
		operationId = string(service.Name()) + "_" + string(md.Name())
	}
	summary, description := util.FormatOperationComments(fd.SourceLocations().ByDescriptor(md))
	if summary == "" {
		summary = string(md.Name())
	}
	op := &v3.Operation{
		Summary:     summary,
		OperationId: operationId,
		Description: description,
		Deprecated:  util.IsMethodDeprecated(md),
	}

	if !opts.WithoutDefaultTags {
		op.Tags = []string{util.ServiceTagName(opts, service)}
	}

	fieldNamesInPath := map[string]struct{}{}
	var pathParams []*v3.Parameter
	var deferredParams []*v3.Parameter
	for _, param := range partsToParameter(tokens) {
		// Skip the name parameter if it's part of a glob pattern
		if strings.Contains(param, "=") {
			continue
		}
		field, jsonPath := resolveField(opts, md.Input(), param)
		if field != nil {
			// This field is only top level, so we will filter out the param from
			// query/param or request body
			fieldNamesInPath[string(field.FullName())] = struct{}{}
			fieldNamesInPath[strings.Join(jsonPath, ".")] = struct{}{} // sometimes JSON field names are used
			fieldNamesInPath[param] = struct{}{}                       // proto field path, matches with-proto-names property keys
			schemaProxy := schema.FieldToSchema(opts, nil, field)
			newParameter := buildParameter(opts, param, "path", field, schemaProxy, proto.Bool(true), false)
			pathParams = append(pathParams, newParameter)
		} else {
			opts.Logger.Warn("path field not found", slog.String("param", param))
		}
	}

	// Add named path parameters from glob patterns
	for _, token := range tokens {
		if token.Type == TokenLiteral && token.Value == "**" {
			newParameter := &v3.Parameter{
				Name:          "http_path",
				In:            "path",
				Required:      proto.Bool(true),
				Description:   "The trailing part of the path.",
				AllowReserved: true,
				Schema:        base.CreateSchemaProxy(&base.Schema{Type: []string{"string"}}),
			}
			pathParams = append(pathParams, newParameter)
		}
		if token.Type == TokenVariable && strings.Contains(token.Value, "=") {
			matches := namedPathPattern.FindStringSubmatch("{" + token.Value + "}")
			if len(matches) == 3 {
				if matches[2] == "**" {
					paramName := matches[1]
					field, _ := resolveField(opts, md.Input(), paramName)
					var newParameter *v3.Parameter
					if field != nil {
						fieldNamesInPath[string(field.FullName())] = struct{}{}
						fieldNamesInPath[field.JSONName()] = struct{}{}
						fieldNamesInPath[paramName] = struct{}{} // proto field path, matches with-proto-names property keys
						parameterSchema := schema.FieldToSchema(opts, nil, field)
						// Path parameters must be primitives.
						if slices.Contains(parameterSchema.Schema().Type, "object") || slices.Contains(parameterSchema.Schema().Type, "array") {
							desc := ""
							if parameterSchema.Schema() != nil {
								desc = parameterSchema.Schema().Description
							}
							parameterSchema = base.CreateSchemaProxy(&base.Schema{
								Type:        []string{"string"},
								Description: desc,
							})
						}
						newParameter = buildParameter(opts, paramName, "path", field, parameterSchema, proto.Bool(true), true)
					} else {
						newParameter = &v3.Parameter{
							Name:          paramName,
							Required:      proto.Bool(true),
							In:            "path",
							Description:   "The trailing part of the path.",
							AllowReserved: true,
							Schema:        base.CreateSchemaProxy(&base.Schema{Type: []string{"string"}}),
						}
					}
					pathParams = append(pathParams, newParameter)
					continue
				}
				// Store the original field name from the glob pattern to prevent it from appearing
				// in both the path parameters and request body/query parameters
				originalName := matches[1]
				fieldNamesInPath[originalName] = struct{}{}
				// Convert the path from the starred form to use named path parameters.
				// The starred path may be in the form "things/*/otherthings/*" or contain
				// literal segments like "things/*/static/otherthings/*".
				starredPath := matches[2]
				parts := strings.Split(starredPath, "/")
				for i, part := range parts {
					if part != "*" || i == 0 {
						continue
					}
					section := parts[i-1]
					namedPathParameter := singularize(resMap, section)
					newParameter := &v3.Parameter{
						Name:     namedPathParameter,
						In:       "path",
						Required: proto.Bool(true),
						Schema:   base.CreateSchemaProxy(&base.Schema{Type: []string{"string"}}),
					}
					pathParams = append(pathParams, newParameter)
					deferredParams = append(deferredParams, &v3.Parameter{
						Name:        namedPathParameter,
						In:          "path",
						Description: "The " + namedPathParameter + " id.",
					})
				}
			}
		}
	}
	op.Parameters = util.MergeParameters(op.Parameters, pathParams)

	hasGnosticRequestBody := false
	if proto.HasExtension(md.Options(), goa3.E_Operation.TypeDescriptor().Type()) {
		ext := proto.GetExtension(md.Options(), goa3.E_Operation.TypeDescriptor().Type())
		if gnosticOperation, ok := ext.(*goa3.Operation); ok {
			if gnosticOperation.RequestBody != nil {
				hasGnosticRequestBody = true
			}
		}
	}

	if !hasGnosticRequestBody {
		switch rule.Body {
		case "":
			newQueryParams := flattenToParams(opts, md.Input(), "", fieldNamesInPath)
			op.Parameters = util.MergeParameters(op.Parameters, newQueryParams)
			// The input is fully consumed as query/path params; the now-unreferenced
			// request schema is pruned by trim-unreferenced-schemas.
		case "*":
			if len(fieldNamesInPath) > 0 {
				_, s := schema.MessageToSchema(opts, md.Input())
				if s != nil {
					// Remove path parameters from properties.
					// When the message has oneOf fields, MessageToSchema wraps
					// properties and oneOf under AllOf, setting s.Properties to nil.
					// In that case, find the properties inside AllOf.
					props := s.Properties
					allOfIdx := -1
					if props == nil && len(s.AllOf) > 0 {
						for i, entry := range s.AllOf {
							if es := entry.Schema(); es != nil && es.Properties != nil && es.Properties.Len() > 0 {
								props = es.Properties
								allOfIdx = i
								break
							}
						}
					}
					if props != nil {
						for name := range fieldNamesInPath {
							props.Delete(name)
							if s.Required != nil {
								s.Required = slices.DeleteFunc(s.Required, func(s string) bool {
									return s == name
								})
								if len(s.Required) == 0 {
									s.Required = nil
								}
							}
						}
						// Drop the allOf entry if removing path params emptied it.
						if allOfIdx >= 0 && props.Len() == 0 {
							s.AllOf = slices.Delete(s.AllOf, allOfIdx, allOfIdx+1)
						}
					}
					hasContent := (props != nil && props.Len() > 0) || len(s.AllOf) > 0 || len(s.OneOf) > 0
					if hasContent {
						op.RequestBody = util.MethodToRequestBody(opts, md, base.CreateSchemaProxy(s), false)
					}
				}
				// Body inlines the input (minus path params); the now-unreferenced
				// request schema is pruned by trim-unreferenced-schemas.
			} else {
				// Single-use request body: inline the message and drop it from
				// components (trim-one-use-types); else reference the shared schema.
				if opts.TrimOneUseTypes && dropReferrerAndMaybeDelete(opts, spec, referrals, md.Input(), fmt.Sprintf("%s-input", md.FullName())) {
					_, s := schema.MessageToSchema(opts, md.Input())
					op.RequestBody = util.MethodToRequestBody(opts, md, base.CreateSchemaProxy(s), false)
				} else {
					inputName := util.DescriptorToId(opts, md.Input())
					s := base.CreateSchemaProxyRef("#/components/schemas/" + util.FormatTypeRef(opts, inputName))
					op.RequestBody = util.MethodToRequestBody(opts, md, s, false)
				}
			}

		default:
			if field, jsonPath := resolveField(opts, md.Input(), rule.Body); field != nil {
				loc := field.ParentFile().SourceLocations().ByDescriptor(field)
				bodySchema := schema.FieldToSchema(opts, nil, field)
				op.RequestBody = &v3.RequestBody{
					Description: util.FormatComments(loc),
					Content:     util.MakeMediaTypes(opts, bodySchema, false, false),
				}

				// Add any unhandled fields in the request message as query parameters.
				// This covers the case where body: "specific_field" is used, and any fields
				// not in the path or body should become query parameters.
				// This follows Google AIP-127 specification and matches the original gnostic behavior.
				coveredFields := make(map[string]struct{})
				for name := range fieldNamesInPath {
					coveredFields[name] = struct{}{}
				}
				coveredFields[rule.Body] = struct{}{}
				// Also exclude JSON name and descriptor name to prevent snake_case vs camelCase mismatch
				coveredFields[field.JSONName()] = struct{}{}
				coveredFields[string(field.FullName())] = struct{}{}
				// If body is a nested path (a.b.c) also skip its JSON path
				coveredFields[strings.Join(jsonPath, ".")] = struct{}{}

				newQueryParams := flattenToParams(opts, md.Input(), "", coveredFields)
				op.Parameters = util.MergeParameters(op.Parameters, newQueryParams)
			} else {
				opts.Logger.Warn("body field not found", slog.String("param", rule.Body))
			}
		}
	}

	// Responses
	codeMap := orderedmap.New[string, *v3.Response]()

	if !opts.DisableDefaultResponse {
		var outputSchema *base.SchemaProxy
		if rule.ResponseBody == "" {
			// If this is the only method referencing the output schema, inline it
			// and drop the component; otherwise keep the shared $ref.
			refs := referrals[string(md.Output().FullName())]
			if opts.TrimOneUseTypes && spec != nil && len(refs) == 1 {
				delete(refs, fmt.Sprintf("%s-output", md.FullName()))
				spec.Components.Schemas.Delete(util.DescriptorToId(opts, md.Output()))
				_, s := schema.MessageToSchema(opts, md.Output())
				outputSchema = base.CreateSchemaProxy(s)
			} else {
				outputSchema = base.CreateSchemaProxyRef("#/components/schemas/" + util.FormatTypeRef(opts, util.DescriptorToId(opts, md.Output())))
			}
		} else {
			if fd, _ := resolveField(opts, md.Output(), rule.ResponseBody); fd != nil {
				outputSchema = schema.FieldToSchema(opts, nil, fd)
			}
		}

		mediaType := orderedmap.New[string, *v3.MediaType]()
		mediaType.Set("application/json", &v3.MediaType{Schema: outputSchema})
		codeMap.Set("200", &v3.Response{
			Description: "Success",
			Content:     mediaType,
		})
	}

	if opts.WithGoogleErrorDetail {
		errorMediaType := orderedmap.New[string, *v3.MediaType]()
		errorMediaType.Set("application/json", &v3.MediaType{
			Schema: base.CreateSchemaProxyRef("#/components/schemas/google.rpc.Status"),
		})
		codeMap.Set("default", &v3.Response{
			Description: "An unexpected error response.",
			Content:     errorMediaType,
		})
	}

	op.Responses = &v3.Responses{
		Codes: codeMap,
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
	openAPIPath := partsToOpenAPIPath(tokens, resMap)
	paths.Set(openAPIPath, pathItem)

	allDeferred := orderedmap.New[string, []*v3.Parameter]()
	if len(deferredParams) > 0 {
		allDeferred.Set(openAPIPath, deferredParams)
	}

	for _, binding := range rule.AdditionalBindings {
		sub := httpRuleToPathMap(opts, md, binding, resMap, nil, nil)
		for pair := sub.PathItems.First(); pair != nil; pair = pair.Next() {
			path := util.MakePath(opts, pair.Key())
			paths.Set(path, pair.Value())
		}
		for pair := sub.DeferredParams.First(); pair != nil; pair = pair.Next() {
			path := util.MakePath(opts, pair.Key())
			allDeferred.Set(path, pair.Value())
		}
	}
	dedupeOperations(op.OperationId, paths.ValuesFromOldest())
	return &PathItemsResult{
		PathItems:      paths,
		DeferredParams: allDeferred,
	}
}

// dedupeOperations assigns unique operation ids to additional bindings.
// From the OpenAPI v3 spec: "The id MUST be unique among all operations described in the API."
// Since the same gRPC method name is used for operationId, the additional bindings will not be unique,
// so we append a number, starting at 2, when more than one path binds to the same method.
func dedupeOperations(id string, value iter.Seq[*v3.PathItem]) {
	num := 0
	for path := range value {
		for op := range path.GetOperations().ValuesFromOldest() {
			if op.OperationId == id {
				num++
				if num > 1 {
					op.OperationId = fmt.Sprintf("%s%d", id, num)
				}
			}
		}
	}
}

func resolveField(opts options.Options, md protoreflect.MessageDescriptor, param string) (protoreflect.FieldDescriptor, []string) {
	jsonParts := []string{}
	current := md
	var fd protoreflect.FieldDescriptor
	for _, paramPart := range strings.Split(param, ".") {
		if field := fieldByName(opts, current, paramPart); field == nil {
			return nil, nil
		} else {
			fd = field
			jsonParts = append(jsonParts, fd.JSONName())
			current = field.Message()
		}
	}
	return fd, jsonParts
}

func fieldByName(opts options.Options, md protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
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
			// Skip parameters that contain = as they are part of a glob pattern
			if strings.Contains(token.Value, "=") {
				continue
			}
			params = append(params, strings.SplitN(token.Value, "=", 2)[0])
		}
	}
	return params
}

func partsToOpenAPIPath(tokens []Token, resMap map[string]string) string {
	var b strings.Builder
	for _, token := range tokens {
		switch token.Type {
		case TokenSlash:
			b.WriteByte('/')
		case TokenEOF:
		case TokenColon:
			b.WriteByte(':')
		case TokenLiteral:
			if token.Value == "**" {
				b.WriteString("{http_path}")
			} else {
				b.WriteString(token.Value)
			}
		case TokenIdent:
			b.WriteString(token.Value)
		case TokenVariable:
			// Handle the name= prefix by extracting just the path portion
			if strings.Contains(token.Value, "=") {
				matches := namedPathPattern.FindStringSubmatch("{" + token.Value + "}")
				if len(matches) == 3 {
					if matches[2] == "**" {
						b.WriteString("{")
						b.WriteString(matches[1])
						b.WriteString("}")
						continue
					}
					// Convert the path from the starred form to use named path parameters.
					// The starred path may be in the form "things/*/otherthings/*" or contain
					// literal segments like "things/*/static/otherthings/*".
					starredPath := matches[2]
					parts := strings.Split(starredPath, "/")
					for i, part := range parts {
						if part != "*" || i == 0 {
							continue
						}
						section := parts[i-1]
						namedPathParameter := singularize(resMap, section)
						parts[i] = "{" + namedPathParameter + "}"
					}
					// Rewrite the path to use the path parameters.
					newPath := strings.Join(parts, "/")
					b.WriteString(newPath)
				} else {
					b.WriteByte('{')
					b.WriteString(token.Value)
					b.WriteByte('}')
				}
			} else {
				b.WriteByte('{')
				b.WriteString(token.Value)
				b.WriteByte('}')
			}
		}
	}
	return b.String()
}

func flattenToParams(opts options.Options, md protoreflect.MessageDescriptor, prefix string, seen map[string]struct{}) []*v3.Parameter {
	params := []*v3.Parameter{}
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		// A field that is filtered out of the schemas is not a query parameter either.
		if schema.FieldFilteredOut(opts, md, field) {
			continue
		}
		paramName := prefix + util.MakeFieldName(opts, field)
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
			if util.IsWellKnown(field.Message()) {
				if wk := util.WellKnownToSchema(field.Message()); wk != nil && wk.Schema != nil {
					// These types are represented as complex objects in OpenAPI so they should be flattened
					// and not treated as a single query parameter.
					isComplex := slices.Contains([]string{
						"google.protobuf.Struct",
						"google.protobuf.Value",
						"google.protobuf.Any",
						"google.protobuf.Empty",
					}, wk.ID)

					if !isComplex {
						required := IsFieldRequired(field)
						params = append(params, buildParameter(opts, paramName, "query", field, base.CreateSchemaProxy(wk.Schema), required, false))
						continue
					}
				}
			}
			params = append(params, flattenToParams(opts, field.Message(), paramName+".", seen)...)
		default:
			parent := &base.Schema{}
			schemaProxy := schema.FieldToSchema(opts, base.CreateSchemaProxy(parent), field)
			var required *bool
			// First check field behavior annotations
			required = IsFieldRequired(field)
			// If no field behavior, check if field is in parent's required list
			if required == nil && len(parent.Required) > 0 {
				required = util.BoolPtr(true)
			}
			params = append(params, buildParameter(opts, paramName, "query", field, schemaProxy, required, false))
		}
	}
	return params
}

func CollectResources(fd protoreflect.FileDescriptor, resMap map[string]string) {
	// File-level resource definitions
	if proto.HasExtension(fd.Options(), annotations.E_ResourceDefinition) {
		resDefs := proto.GetExtension(fd.Options(), annotations.E_ResourceDefinition).([]*annotations.ResourceDescriptor)
		for _, res := range resDefs {
			if res.Plural != "" && res.Singular != "" {
				resMap[res.Plural] = res.Singular
			}
		}
	}

	// Message-level resource definitions
	var collectFromMessages func(msgs protoreflect.MessageDescriptors)
	collectFromMessages = func(msgs protoreflect.MessageDescriptors) {
		for i := 0; i < msgs.Len(); i++ {
			m := msgs.Get(i)
			if proto.HasExtension(m.Options(), annotations.E_Resource) {
				res := proto.GetExtension(m.Options(), annotations.E_Resource).(*annotations.ResourceDescriptor)
				if res.Plural != "" && res.Singular != "" {
					resMap[res.Plural] = res.Singular
				}
			}
			collectFromMessages(m.Messages())
		}
	}
	collectFromMessages(fd.Messages())
}

func singularize(resMap map[string]string, plural string) string {
	if s, ok := resMap[plural]; ok {
		return s
	}
	return util.Singular(plural)
}

func buildParameter(opts options.Options, name string, in string, field protoreflect.FieldDescriptor, schemaProxy *base.SchemaProxy, required *bool, allowReserved bool) *v3.Parameter {
	loc := field.ParentFile().SourceLocations().ByDescriptor(field)
	var description string
	// Lift the field description up to the parameter (upstream behavior).
	if schemaProxy != nil && schemaProxy.Schema() != nil {
		description = schemaProxy.Schema().Description
		schemaProxy.Schema().Description = ""
	}
	if description == "" {
		description = util.FormatComments(loc)
	}
	deprecated := util.IsFieldDeprecated(field)
	return &v3.Parameter{
		Name:          name,
		In:            in,
		Required:      required,
		Description:   description,
		Schema:        schemaProxy,
		AllowReserved: allowReserved,
		Deprecated:    deprecated != nil && *deprecated,
	}
}
