package converter_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi/datamodel"
	"github.com/pseudomuto/protokit/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/options"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	pluginpb "google.golang.org/protobuf/types/pluginpb"
	"gopkg.in/yaml.v3"
)

var scenarios = []Scenario{
	{Name: "standard", Options: "allow-get,with-streaming,with-service-descriptions"},
	{Name: "direct_proto_message", Options: "allow-get,with-streaming,with-service-descriptions,trim-connect-rpc,direct-proto-message"},
	{Name: "merge_examples", Options: "allow-get,with-streaming,with-service-descriptions,trim-connect-rpc,direct-proto-message,merge-examples"},
	{Name: "proto_names", Options: "with-proto-names"},
	{Name: "path_prefix", Options: "path-prefix=/testing/1234"},
	{Name: "with_proto_annotations", Options: "with-proto-annotations"},
	{Name: "trim_unused_type", Options: "trim-unused-types"},
	{Name: "trim_one_use_response_type", Options: "trim-unused-types,trim-one-use-response-types"},
	{Name: "trim_message_suffix", Options: "trim-connect-rpc,trim-message-suffix=SuffixToTrim"},
	{Name: "trim_service_suffix", Options: "trim-connect-rpc,trim-service-suffix=SuffixToTrim,without-fqn"},
	{Name: "trim_enum_name_prefix", Options: "trim-connect-rpc,trim-enum-name-prefix"},
	{Name: "explicit_optional_fields", Options: "explicit-optional-fields,allow-get,with-streaming,with-service-descriptions,trim-connect-rpc"},
	{Name: "filter_public", Options: "explicit-optional-fields,allow-get,with-streaming,with-service-descriptions,trim-connect-rpc,filter-public,filter-private"},
	{Name: "with_base", Options: "base=testdata/with_base/base.yaml,trim-unused-types"},
	{Name: "with_global_header", Options: "global-header,trim-unused-types,trim-connect-rpc,version=1.1.2"},
	{Name: "with_specification_extensions", Options: "base=testdata/with_specification_extensions/base.yaml,trim-unused-types"},
	{Name: "with_version", Options: "allow-get,with-streaming,with-service-descriptions,version=1.1.1"},
	{Name: "without_fqn", Options: "trim-connect-rpc,without-fqn"},
	{Name: "trim_unused_type_filter_public", Options: "trim-connect-rpc,trim-unused-types,filter-public"},
}

type Scenario struct {
	Name    string
	Options string
}

func generateAndCheckResult(t *testing.T, options, format, protofile string) string {
	relPath := path.Join("internal", "converter", protofile)

	// Make Generation Request
	pf, err := utils.LoadDescriptorSet("testdata", "fileset.binpb")
	require.NoError(t, err)
	req := utils.CreateGenRequest(pf, relPath)
	var sb strings.Builder
	sb.WriteString("debug,format=")
	sb.WriteString(format)
	if len(options) > 0 {
		sb.WriteString(",")
		sb.WriteString(options)
	}
	req.Parameter = proto.String(sb.String())

	b, err := proto.Marshal(req)
	require.NoError(t, err)

	// Call the conversion code!
	resp, err := converter.ConvertFrom(bytes.NewBuffer(b))
	require.NoError(t, err)
	require.Len(t, resp.File, 1)
	file := resp.File[0]
	assert.NotNil(t, file.Name)
	assert.Equal(t, strings.TrimSuffix(relPath, filepath.Ext(relPath))+".openapi."+format, file.GetName())

	// Load in our expected output and compare it against what we actually got
	outputPath := makeOutputPath(protofile, format)
	_, statErr := os.Stat(outputPath)
	switch {
	case errors.Is(statErr, os.ErrNotExist):
		assert.NoError(t, os.MkdirAll(filepath.Dir(outputPath), 0755))
		assert.NoError(t, os.WriteFile(outputPath, []byte(file.GetContent()), 0644))
	case statErr != nil:
		require.NoError(t, statErr)
	default:
		expectedFile, err := os.ReadFile(outputPath)
		require.NoError(t, err)
		assert.Equal(t, string(expectedFile), file.GetContent())
	}
	return file.GetContent()
}

func validateOpenAPISpec(t *testing.T, protofile string, spec string) {
	config := datamodel.DocumentConfiguration{
		IgnorePolymorphicCircularReferences: true,
		IgnoreArrayCircularReferences:       true,
		SkipCircularReferenceCheck:          true,
	}

	document, err := libopenapi.NewDocumentWithConfiguration([]byte(spec), &config)
	require.NoError(t, err)

	var errs []error
	validate, errs := validator.NewValidator(document)
	require.Len(t, errs, 0, errs)

	// Load in validation test cases to check OpenAPI specifications against sample requests
	testCaseFilePath := strings.TrimSuffix(protofile, filepath.Ext(protofile)) + ".cases.yaml"
	if _, err := os.Stat(testCaseFilePath); errors.Is(err, os.ErrNotExist) {
		t.Logf("No cases file: %+s", testCaseFilePath)
		return
	}

	testCaseFileBytes, err := os.ReadFile(testCaseFilePath)
	require.NoError(t, err)

	testCaseFile := &TestCaseFile{}
	if err := yaml.Unmarshal(testCaseFileBytes, testCaseFile); err != nil {
		require.NoError(t, err)
	}

	for _, testCase := range testCaseFile.Cases {
		testCase := testCase
		if testCase.Method == "" {
			testCase.Method = "POST"
		}
		if len(testCase.Errors) == 0 {
			testCase.Errors = []string{}
		}
		t.Run(testCase.Name, func(tt *testing.T) {
			var body io.Reader
			if len(testCase.Body) > 0 {
				body = strings.NewReader(testCase.Body)
			}
			path := testCase.Path
			if len(testCase.Query) > 0 {
				path += "?" + testCase.Query
			}
			req, err := http.NewRequest(testCase.Method, path, body)
			for k, v := range testCase.Headers {
				req.Header.Add(k, v)
			}
			require.NoError(tt, err)

			ok, errs := validate.ValidateHttpRequest(req)
			require.Len(tt, errs, len(testCase.Errors), "Incorrect number of errors: %+v", errs)

			for i, err := range errs {
				assert.Regexp(tt, testCase.Errors[i], err.Error())
			}
			assert.Equal(tt, ok, len(testCase.Errors) == 0)
		})
	}
}

// TestConvert uses data in testdata/ to make requests to generate openapi documents,
// checks if they are valid OpenAPI documents and validates that a list of example
// requests would conform to the OpenAPI spec or fail.
func TestConvert(t *testing.T) {
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			paths, err := filepath.Glob("testdata/" + scenario.Name + "/**.proto")
			require.NoError(t, err)
			for _, protofile := range paths {
				protofile := protofile

				formats := []string{"yaml", "json"}

				for _, format := range formats {
					format := format
					t.Run(path.Base(protofile)+"→"+format, func(t *testing.T) {
						spec := generateAndCheckResult(t, scenario.Options, format, protofile)
						// Validate
						t.Run("validate", func(tt *testing.T) {
							validateOpenAPISpec(t, protofile, spec)
						})
					})
				}
			}
		})
	}
}

type TestCaseFile struct {
	Cases []TestCase `yaml:"cases"`
}

type TestCase struct {
	Name    string            `yaml:"name"`
	Method  string            `yaml:"method"`
	Path    string            `yaml:"path"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
	Query   string            `yaml:"query"`
	Errors  []string          `yaml:"errors"`
}

func makeOutputPath(protofile, format string) string {
	dir, file := filepath.Split(strings.TrimSuffix(protofile, filepath.Ext(protofile)) + ".openapi." + format)
	return filepath.Join(dir, "output", file)
}

func TestConvertWithOptions(t *testing.T) {
	t.Run("with base file", func(t *testing.T) {
		baseYAML := `
openapi: 3.1.0
info:
  title: Base API
  version: 1.0.0
  x-logo:
    url: https://example.com/logo.png
paths:
  /example/api/path:
    post:
      x-code-samples:
        - language: shell
          label: example-api-path
          source: |
            curl -X POST https://api.example.com/example/api/path \
            -H "Content-Type: application/json" \
            -d '{"email": "user@example.com"}'
`
		opts := options.Options{
			Path:        "test.openapi.yaml",
			Format:      "yaml",
			BaseOpenAPI: []byte(baseYAML),
		}

		req := &pluginpb.CodeGeneratorRequest{
			ProtoFile: []*descriptorpb.FileDescriptorProto{
				{
					Name:    proto.String("test.proto"),
					Package: proto.String("test"),
					MessageType: []*descriptorpb.DescriptorProto{
						{Name: proto.String("TestMessage")},
					},
					Service: []*descriptorpb.ServiceDescriptorProto{
						{
							Name: proto.String("ExampleApiService"),
							Method: []*descriptorpb.MethodDescriptorProto{
								{
									Name:       proto.String("ExampleApiPath"),
									InputType:  proto.String(".test.TestMessage"),
									OutputType: proto.String(".test.TestMessage"),
								},
							},
						},
					},
				},
			},
			FileToGenerate: []string{"test.proto"},
		}

		resp, err := converter.ConvertWithOptions(req, opts)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.File, 1)

		content := resp.File[0].GetContent()
		assert.Contains(t, content, "x-logo:")
		assert.Contains(t, content, "url: https://example.com/logo.png")
		assert.Contains(t, content, "x-code-samples:")

		// Check that the generated content is merged with the base file
		assert.Contains(t, content, "TestMessage")
	})
}

func TestConvertWithNoFqn(t *testing.T) {
	t.Run("with clashing message name", func(t *testing.T) {
		opts := options.Options{
			Path:           "test.openapi.yaml",
			Format:         "yaml",
			WithoutFqn:     true,
			TrimConnectRPC: true,
		}

		req := &pluginpb.CodeGeneratorRequest{
			ProtoFile: []*descriptorpb.FileDescriptorProto{
				{
					Name:    proto.String("a.proto"),
					Package: proto.String("a"),
					MessageType: []*descriptorpb.DescriptorProto{
						{Name: proto.String("TestMessage")},
					},
				},
				{
					Name:    proto.String("b.proto"),
					Package: proto.String("b"),
					MessageType: []*descriptorpb.DescriptorProto{
						{Name: proto.String("TestMessage")},
					},
				},
				{
					Name:    proto.String("test.proto"),
					Package: proto.String("test"),

					Dependency: []string{"a.proto", "b.proto"},
					Service: []*descriptorpb.ServiceDescriptorProto{
						{
							Name: proto.String("ExampleApiService"),
							Method: []*descriptorpb.MethodDescriptorProto{
								{
									Name:       proto.String("ExampleApiPath"),
									InputType:  proto.String(".a.TestMessage"),
									OutputType: proto.String(".b.TestMessage"),
								},
							},
						},
					},
				},
			},
			FileToGenerate: []string{"a", "b", "test.proto"},
		}

		_, err := converter.ConvertWithOptions(req, opts)
		require.Error(t, err)
	})

	t.Run("with clashing message name", func(t *testing.T) {
		opts := options.Options{
			Path:           "test.openapi.yaml",
			Format:         "yaml",
			WithoutFqn:     true,
			TrimConnectRPC: true,
		}

		req := &pluginpb.CodeGeneratorRequest{
			ProtoFile: []*descriptorpb.FileDescriptorProto{
				{
					Name:    proto.String("a.proto"),
					Package: proto.String("a"),
					MessageType: []*descriptorpb.DescriptorProto{
						{Name: proto.String("ZLastMessage")},
					},
				},
				{
					Name:    proto.String("b.proto"),
					Package: proto.String("b"),
					MessageType: []*descriptorpb.DescriptorProto{
						{Name: proto.String("AFirstMessage")},
					},
				},
				{
					Name:    proto.String("test.proto"),
					Package: proto.String("test"),

					Dependency: []string{"a.proto", "b.proto"},
					Service: []*descriptorpb.ServiceDescriptorProto{
						{
							Name: proto.String("ExampleApiService"),
							Method: []*descriptorpb.MethodDescriptorProto{
								{
									Name:       proto.String("ExampleApiPath"),
									InputType:  proto.String(".a.ZLastMessage"),
									OutputType: proto.String(".b.AFirstMessage"),
								},
							},
						},
					},
				},
			},
			FileToGenerate: []string{"a", "b", "test.proto"},
		}

		resp, err := converter.ConvertWithOptions(req, opts)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.File, 1)

		content := resp.File[0].GetContent()
		require.Less(t, strings.Index(content, "AFirstMessage"), strings.Index(content, "ZLastMessage"), "AFirstMessage should be before ZLastMessage")
	})
}
