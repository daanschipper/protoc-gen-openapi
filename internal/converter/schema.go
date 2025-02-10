package converter

import (
	"github.com/fatih/camelcase"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/pb33f/libopenapi/utils"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gopkg.in/yaml.v3"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/options"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/schema"
	"github.com/sudorandom/protoc-gen-connect-openapi/internal/converter/util"
	oasExtension "github.com/sudorandom/protoc-gen-connect-openapi/openapi"
)

type State struct {
	Opts        options.Options
	CurrentFile protoreflect.FileDescriptor
	Messages    map[protoreflect.MessageDescriptor]struct{}
	Enums       map[protoreflect.EnumDescriptor]struct{}
}

func NewState(opts options.Options) *State {
	return &State{
		Opts:     opts,
		Messages: map[protoreflect.MessageDescriptor]struct{}{},
		Enums:    map[protoreflect.EnumDescriptor]struct{}{},
	}
}

func (st *State) CollectFile(tt protoreflect.FileDescriptor) {
	st.CurrentFile = tt

	// Only collect types from the root if TrimUnusedTypes is off
	if !st.Opts.TrimUnusedTypes {
		// Files can have enums
		enums := tt.Enums()
		for i := 0; i < enums.Len(); i++ {
			st.CollectEnum(enums.Get(i))
		}

		// Files can have messages
		messages := tt.Messages()
		for i := 0; i < messages.Len(); i++ {
			st.CollectMessage(messages.Get(i))
		}
	}

	// Also make sure to pick up messages referenced in service methods
	services := tt.Services()
	for i := 0; i < services.Len(); i++ {
		service := services.Get(i)
		methods := service.Methods()
		for j := 0; j < methods.Len(); j++ {
			method := methods.Get(j)

			// Do not collect messages of non-public methods if TrimUnusedTypes and FilterPublic are on.
			if st.Opts.TrimUnusedTypes && st.Opts.FilterPublic {
				openApiOptionsExtension := proto.GetExtension(method.Options(), oasExtension.E_MethodParams)
				if openApiOptionsExtension == nil {
					continue
				}

				if openApiOptionsExtension == oasExtension.E_MethodParams.InterfaceOf(oasExtension.E_MethodParams.Zero()) {
					continue
				}

				openApiOptions := openApiOptionsExtension.(*oasExtension.OpenApiOptions)
				if !openApiOptions.GetPublic() {
					continue
				}
			}

			st.CollectMessage(method.Input())
			st.CollectMessage(method.Output())
		}
	}
}

func (st *State) CollectEnum(tt protoreflect.EnumDescriptor) {
	if tt == nil {
		return
	}
	// Make sure we're not recursing through the same enum a second time
	if _, ok := st.Enums[tt]; ok {
		return
	}
	st.Enums[tt] = struct{}{}
}

func (st *State) CollectMessage(tt protoreflect.MessageDescriptor) {
	if tt == nil {
		return
	}
	// Make sure we're not recursing through the same message a second time
	if _, ok := st.Messages[tt]; ok {
		return
	}
	st.Messages[tt] = struct{}{}

	// Messages can have fields
	fields := tt.Fields()
	for i := 0; i < fields.Len(); i++ {
		st.CollectField(fields.Get(i))
	}

	// Messages can have enums
	enums := tt.Enums()
	for i := 0; i < enums.Len(); i++ {
		st.CollectEnum(enums.Get(i))
	}

	// Messages can have messages
	messages := tt.Messages()
	for i := 0; i < messages.Len(); i++ {
		message := messages.Get(i)
		st.CollectMessage(message)
	}
}

func (st *State) CollectField(tt protoreflect.FieldDescriptor) {
	if tt == nil {
		return
	}
	st.CollectEnum(tt.Enum())
	st.CollectMessage(tt.Message())
	st.CollectField(tt.MapKey())
	st.CollectField(tt.MapValue())
}

func (st *State) SortedEnums() []protoreflect.EnumDescriptor {
	enums := make([]protoreflect.EnumDescriptor, 0, len(st.Enums))
	for enum := range st.Enums {
		enums = append(enums, enum)
	}
	sort.Slice(enums, func(i, j int) bool {
		return enums[i].FullName() < enums[j].FullName()
	})
	return enums
}

func (st *State) SortedMessages() []protoreflect.MessageDescriptor {
	messages := make([]protoreflect.MessageDescriptor, 0, len(st.Messages))
	for message := range st.Messages {
		messages = append(messages, message)
	}
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].FullName() < messages[j].FullName()
	})
	return messages
}

func enumToSchema(state *State, tt protoreflect.EnumDescriptor) (string, *base.Schema) {
	slog.Debug("enumToSchema", slog.Any("descriptor", tt.FullName()))
	children := []*yaml.Node{}
	values := tt.Values()
	for i := 0; i < values.Len(); i++ {
		value := values.Get(i)

		name := string(value.Name())
		if state.Opts.TrimEnumNamePrefix {
			prefix := strings.ToUpper(strings.Join(camelcase.Split(string(tt.Name())), "_")) + "_"
			name = strings.TrimPrefix(name, prefix)
		}

		children = append(children, utils.CreateStringNode(name))
		if state.Opts.IncludeNumberEnumValues {
			children = append(children, utils.CreateIntNode(strconv.FormatInt(int64(value.Number()), 10)))
		}
	}

	title := string(tt.Name())
	if state.Opts.FullyQualifiedMessageNames {
		title = string(tt.FullName())
	}
	title = util.TrimMessageSuffix(state.Opts, title)
	s := &base.Schema{
		Title:       title,
		Description: util.FormatComments(tt.ParentFile().SourceLocations().ByDescriptor(tt)),
		Type:        []string{"string"},
		Enum:        children,
	}

	return util.DescriptorToId(state.Opts, tt), s
}

func stateToSchema(st *State) *orderedmap.Map[string, *base.SchemaProxy] {
	schemas := orderedmap.New[string, *base.SchemaProxy]()

	for _, enum := range st.SortedEnums() {
		id, schema := enumToSchema(st, enum)
		schemas.Set(id, base.CreateSchemaProxy(schema))
	}

	for _, message := range st.SortedMessages() {
		id, schema := schema.MessageToSchema(st.Opts, message)
		if schema != nil {
			schemas.Set(id, base.CreateSchemaProxy(schema))
		}
	}

	return schemas
}
