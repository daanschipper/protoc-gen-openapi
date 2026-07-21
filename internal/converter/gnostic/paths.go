package gnostic

import (
	goa3 "github.com/google/gnostic/openapiv3"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/options"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func PathItemWithMethodAnnotations(opts options.Options, item *v3.PathItem, md protoreflect.MethodDescriptor) *v3.PathItem {
	if !proto.HasExtension(md.Options(), goa3.E_Operation.TypeDescriptor().Type()) {
		return item
	}

	ext := proto.GetExtension(md.Options(), goa3.E_Operation.TypeDescriptor().Type())
	gnosticOperation, ok := ext.(*goa3.Operation)
	if !ok {
		return item
	}
	operations := item.GetOperations()
	for kv := operations.First(); kv != nil; kv = kv.Next() {
		oper := kv.Value()
		if gnosticOperation.Deprecated {
			t := true
			oper.Deprecated = &t
		}

		newParams := make([]*v3.Parameter, 0, len(gnosticOperation.Parameters))
		for _, param := range gnosticOperation.Parameters {
			newParams = append(newParams, toParameter(opts, param))
		}
		oper.Parameters = util.MergeParameters(oper.Parameters, newParams)

		if gnosticOperation.RequestBody != nil {
			body := toRequestBody(opts, gnosticOperation.RequestBody.GetRequestBody())
			if !opts.MergeExamples {
				oper.RequestBody = body
			} else {
				// Merge example from gnostic annotation into the proto-derived body.
				mergeRequestBodyExamples(oper.RequestBody, body)
			}
		}

		if gnosticOperation.Responses != nil {
			responses := toResponses(opts, gnosticOperation.Responses)
			for pair := responses.Codes.First(); pair != nil; pair = pair.Next() {
				if !opts.MergeExamples {
					oper.Responses.Codes.Set(pair.Key(), pair.Value())
				} else {
					// Merge example from gnostic annotation into the existing response.
					if response, exists := oper.Responses.Codes.Get(pair.Key()); exists {
						mergeResponseExamples(response, pair.Value())
					}
				}
			}
			if responses.Default != nil {
				oper.Responses.Default = responses.Default
			}
			for pair := responses.Extensions.First(); pair != nil; pair = pair.Next() {
				oper.Responses.Extensions.Set(pair.Key(), pair.Value())
			}
		}

		if gnosticOperation.Callbacks != nil {
			oper.Callbacks = toCallbacks(opts, gnosticOperation.Callbacks)
		}

		if gnosticOperation.Security != nil {
			oper.Security = toSecurityRequirements(gnosticOperation.Security)
		}
		oper.Servers = toServers(gnosticOperation.Servers)

		if gnosticOperation.Summary != "" {
			oper.Summary = gnosticOperation.Summary
		}
		if gnosticOperation.Description != "" {
			oper.Description = gnosticOperation.Description
		}
		oper.Tags = append(gnosticOperation.Tags, oper.Tags...)

		if exDocs := toExternalDocs(gnosticOperation.ExternalDocs); exDocs != nil {
			oper.ExternalDocs = exDocs
		}

		if gnosticOperation.OperationId != "" {
			oper.OperationId = gnosticOperation.OperationId
		}

		if gnosticOperation.SpecificationExtension != nil {
			oper.Extensions = toExtensions(gnosticOperation.GetSpecificationExtension())
		}
	}
	return item
}

func mergeRequestBodyExamples(protoRequestBody, gnosticRequestBody *v3.RequestBody) {
	if protoRequestBody == nil || gnosticRequestBody == nil {
		return
	}
	mergeContent(protoRequestBody.Content, gnosticRequestBody.Content)
}

func mergeResponseExamples(protoResponse, gnosticResponse *v3.Response) {
	if protoResponse == nil || gnosticResponse == nil {
		return
	}
	mergeContent(protoResponse.Content, gnosticResponse.Content)
}

func mergeContent(protoContent, gnosticContent *orderedmap.Map[string, *v3.MediaType]) {
	if protoContent == nil || gnosticContent == nil {
		return
	}
	for pair := protoContent.First(); pair != nil; pair = pair.Next() {
		protoMediaType := pair.Key()
		protoMediaTypeObj := pair.Value()

		if gnosticMediaTypeObj, exists := gnosticContent.Get(protoMediaType); exists {
			if gnosticMediaTypeObj.Examples != nil && gnosticMediaTypeObj.Examples.Len() > 0 {
				// Only use the gnostic examples if the proto request body does not have examples.
				if protoMediaTypeObj.Examples == nil {
					protoMediaTypeObj.Examples = gnosticMediaTypeObj.Examples
					continue
				}
			}
		}
	}
}
