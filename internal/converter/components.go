package converter

import (
	"fmt"
	"log/slog"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	highv3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/pb33f/libopenapi/utils"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gopkg.in/yaml.v3"

	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/options"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/util"
)

func fileToComponents(opts options.Options, fd protoreflect.FileDescriptor) (*v3.Components, map[string]map[string]struct{}, error) {
	// Add schema from messages/enums
	components := &highv3.Components{
		Schemas:         orderedmap.New[string, *base.SchemaProxy](),
		Responses:       orderedmap.New[string, *v3.Response](),
		Parameters:      orderedmap.New[string, *v3.Parameter](),
		Examples:        orderedmap.New[string, *base.Example](),
		RequestBodies:   orderedmap.New[string, *v3.RequestBody](),
		Headers:         orderedmap.New[string, *v3.Header](),
		SecuritySchemes: orderedmap.New[string, *v3.SecurityScheme](),
		Links:           orderedmap.New[string, *v3.Link](),
		Callbacks:       orderedmap.New[string, *v3.Callback](),
		Extensions:      orderedmap.New[string, *yaml.Node](),
	}
	st := NewState(opts)
	slog.Debug("start collection")
	st.CollectFile(fd)
	slog.Debug("collection complete", slog.String("file", string(fd.Name())), slog.Int("messages", len(st.Messages)), slog.Int("enum", len(st.Enums)))
	components.Schemas = stateToSchema(st)

	if opts.WithoutFqn {
		foo := make(map[string]string)
		for message := range st.Messages {
			messageWithoutFqn := util.TrimFqn(opts, message)
			if clash, ok := foo[messageWithoutFqn]; ok {
				return nil, nil, fmt.Errorf("message with identical name, '%s' and '%s, last occurence will be used, results in incorrect openapi spec", clash, string(message.FullName()))
			}
			foo[messageWithoutFqn] = string(message.FullName())
		}
	}

	hasGetRequests := false
	hasMethods := false

	// Add requestBodies and responses for methods
	services := fd.Services()
	for i := 0; i < services.Len(); i++ {
		service := services.Get(i)
		methods := service.Methods()
		for j := 0; j < methods.Len(); j++ {
			method := methods.Get(j)
			hasGet := methodHasGet(opts, method)
			if hasGet {
				hasGetRequests = true
			}
			hasMethods = true
		}
	}

	if hasGetRequests {
		if opts.TrimConnectRPC != true {
			components.Schemas.Set("encoding", base.CreateSchemaProxy(&base.Schema{
				Title:       "encoding",
				Description: "Define which encoding or 'Message-Codec' to use",
				Enum: []*yaml.Node{
					utils.CreateStringNode("proto"),
					utils.CreateStringNode("json"),
				},
			}))

			components.Schemas.Set("base64", base.CreateSchemaProxy(&base.Schema{
				Title:       "base64",
				Description: "Specifies if the message query param is base64 encoded, which may be required for binary data",
				Type:        []string{"boolean"},
			}))

			components.Schemas.Set("compression", base.CreateSchemaProxy(&base.Schema{
				Title:       "compression",
				Description: "Which compression algorithm to use for this request",
				Enum: []*yaml.Node{
					utils.CreateStringNode("identity"),
					utils.CreateStringNode("gzip"),
					utils.CreateStringNode("br"),
				},
			}))
			components.Schemas.Set("connect", base.CreateSchemaProxy(&base.Schema{
				Title:       "connect",
				Description: "Define the version of the Connect protocol",
				Enum: []*yaml.Node{
					utils.CreateStringNode("v1"),
				},
			}))
		}
	}
	if hasMethods {
		if opts.TrimConnectRPC != true {
			components.Schemas.Set("connect-protocol-version", base.CreateSchemaProxy(&base.Schema{
				Title:       "Connect-Protocol-Version",
				Description: "Define the version of the Connect protocol",
				Type:        []string{"number"},
				Enum:        []*yaml.Node{utils.CreateIntNode("1")},
				Const:       utils.CreateIntNode("1"),
			}))

			components.Schemas.Set("connect-timeout-header", base.CreateSchemaProxy(&base.Schema{
				Title:       "Connect-Timeout-Ms",
				Description: "Define the timeout, in ms",
				Type:        []string{"number"},
			}))
			connectErrorProps := orderedmap.New[string, *base.SchemaProxy]()
			connectErrorProps.Set("code", base.CreateSchemaProxy(&base.Schema{
				Description: "The status code, which should be an enum value of [google.rpc.Code][google.rpc.Code].",
				Type:        []string{"string"},
				Examples:    []*yaml.Node{utils.CreateStringNode("not_found")},
				Enum: []*yaml.Node{
					utils.CreateStringNode("canceled"),
					utils.CreateStringNode("unknown"),
					utils.CreateStringNode("invalid_argument"),
					utils.CreateStringNode("deadline_exceeded"),
					utils.CreateStringNode("not_found"),
					utils.CreateStringNode("already_exists"),
					utils.CreateStringNode("permission_denied"),
					utils.CreateStringNode("resource_exhausted"),
					utils.CreateStringNode("failed_precondition"),
					utils.CreateStringNode("aborted"),
					utils.CreateStringNode("out_of_range"),
					utils.CreateStringNode("unimplemented"),
					utils.CreateStringNode("internal"),
					utils.CreateStringNode("unavailable"),
					utils.CreateStringNode("data_loss"),
					utils.CreateStringNode("unauthenticated"),
				},
			}))
			connectErrorProps.Set("message", base.CreateSchemaProxy(&base.Schema{
				Description: "A developer-facing error message, which should be in English. Any user-facing error message should be localized and sent in the [google.rpc.Status.details][google.rpc.Status.details] field, or localized by the client.",
				Type:        []string{"string"},
			}))
			connectErrorProps.Set("detail", base.CreateSchemaProxyRef("#/components/schemas/google.protobuf.Any"))
			components.Schemas.Set("connect.error", base.CreateSchemaProxy(&base.Schema{
				Title:                "Connect Error",
				Description:          `Error type returned by Connect: https://connectrpc.com/docs/go/errors/#http-representation`,
				Properties:           connectErrorProps,
				Type:                 []string{"object"},
				AdditionalProperties: &base.DynamicValue[*base.SchemaProxy, bool]{N: 1, B: true},
			}))
			anyPair := util.NewGoogleAny()
			components.Schemas.Set(anyPair.ID, base.CreateSchemaProxy(anyPair.Schema))
		}

		if opts.CustomDefaultError {
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

			components.Schemas.Set("Error", base.CreateSchemaProxy(&base.Schema{
				Title:       "Error",
				Description: `Error object containing information about the error.`,
				Properties:  errorProps,
				Type:        []string{"object"},
			}))
		}
	}

	return components, st.ReferralMessages, nil
}
