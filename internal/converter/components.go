package converter

import (
	"log/slog"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	highv3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gopkg.in/yaml.v3"

	"github.com/daanschipper/protoc-gen-openapi/internal/converter/options"
)

func fileToComponents(opts options.Options, fd protoreflect.FileDescriptor) (*highv3.Components, error) {
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

	return components, nil
}
