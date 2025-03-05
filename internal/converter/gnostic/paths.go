package gnostic

import (
	goa3 "github.com/google/gnostic/openapiv3"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/options"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func PathItemWithMethodAnnotations(opts options.Options, item *v3.PathItem, md protoreflect.MethodDescriptor) *v3.PathItem {
	if !proto.HasExtension(md.Options(), goa3.E_Operation.TypeDescriptor().Type()) {
		return item
	}

	ext := proto.GetExtension(md.Options(), goa3.E_Operation.TypeDescriptor().Type())
	operationOpts, ok := ext.(*goa3.Operation)
	if !ok {
		return item
	}
	operations := item.GetOperations()
	for kv := operations.First(); kv != nil; kv = kv.Next() {
		operation := kv.Value()
		if operationOpts.Deprecated {
			t := true
			operation.Deprecated = &t
		}

		for _, param := range operationOpts.Parameters {
			item.Parameters = append(item.Parameters, toParameter(param))
		}

		if operationOpts.RequestBody != nil {
			body := toRequestBody(operationOpts.RequestBody.GetRequestBody())
			if !opts.MergeExamples {
				operation.RequestBody = body
			} else {
				// Merge example from gnostic annotation.
				mergeRequestBodyExamples(operation.RequestBody, body)
			}
		}

		if operationOpts.Responses != nil {
			responses := toResponses(operationOpts.Responses)
			for pair := responses.Codes.First(); pair != nil; pair = pair.Next() {
				if !opts.MergeExamples {
					operation.Responses.Codes.Set(pair.Key(), pair.Value())
				} else {
					// Merge example from gnostic annotation.
					if response, exists := operation.Responses.Codes.Get(pair.Key()); exists {
						mergeResponseExamples(response, pair.Value())
					}
				}
			}
			if responses.Default != nil {
				operation.Responses.Default = responses.Default
			}
			for pair := responses.Extensions.First(); pair != nil; pair = pair.Next() {
				operation.Responses.Extensions.Set(pair.Key(), pair.Value())
			}
		}

		if operationOpts.Callbacks != nil {
			operation.Callbacks = toCallbacks(operationOpts.Callbacks)
		}

		if security := toSecurityRequirements(operationOpts.Security); len(security) > 0 {
			operation.Security = security
		}
		operation.Servers = toServers(operationOpts.Servers)

		if operationOpts.Summary != "" {
			operation.Summary = operationOpts.Summary
		}
		if operationOpts.Description != "" {
			operation.Description = operationOpts.Description
		}
		operation.Tags = append(operation.Tags, operationOpts.Tags...)

		if exDocs := toExternalDocs(operationOpts.ExternalDocs); exDocs != nil {
			operation.ExternalDocs = exDocs
		}

		if operationOpts.OperationId != "" {
			operation.OperationId = operationOpts.OperationId
		}

		if operationOpts.SpecificationExtension != nil {
			operation.Extensions = toExtensions(operationOpts.GetSpecificationExtension())
		}
	}
	return item
}

func mergeRequestBodyExamples(protoRequestBody, gnosticRequestBody *v3.RequestBody) {
	mergeContent(protoRequestBody.Content, gnosticRequestBody.Content)
}

func mergeResponseExamples(protoResponse, gnosticResponse *v3.Response) {
	mergeContent(protoResponse.Content, gnosticResponse.Content)
}

func mergeContent(protoContent, gnosticContent *orderedmap.Map[string, *v3.MediaType]) {
	for pair := protoContent.First(); pair != nil; pair = pair.Next() {
		protoMediaType := pair.Key()
		protoMediaTypeObj := pair.Value()

		if gnosticMediaTypeObj, exists := gnosticContent.Get(protoMediaType); exists {
			if gnosticMediaTypeObj.Examples != nil && gnosticMediaTypeObj.Examples.Len() > 0 {
				// Only use the gnostic examples if the proto request body does not have examples
				if protoMediaTypeObj.Examples == nil {
					protoMediaTypeObj.Examples = gnosticMediaTypeObj.Examples
					continue
				}
			}
		}
	}
}
