// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.4
// 	protoc        v5.29.3
// source: openapi/annotations.proto

package openapi

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type OpenApiOptions struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Public        *bool                  `protobuf:"varint,1,opt,name=public,proto3,oneof" json:"public,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenApiOptions) Reset() {
	*x = OpenApiOptions{}
	mi := &file_openapi_annotations_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenApiOptions) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenApiOptions) ProtoMessage() {}

func (x *OpenApiOptions) ProtoReflect() protoreflect.Message {
	mi := &file_openapi_annotations_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenApiOptions.ProtoReflect.Descriptor instead.
func (*OpenApiOptions) Descriptor() ([]byte, []int) {
	return file_openapi_annotations_proto_rawDescGZIP(), []int{0}
}

func (x *OpenApiOptions) GetPublic() bool {
	if x != nil && x.Public != nil {
		return *x.Public
	}
	return false
}

type OpenApiFilterPrivate struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Private       *bool                  `protobuf:"varint,1,opt,name=private,proto3,oneof" json:"private,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpenApiFilterPrivate) Reset() {
	*x = OpenApiFilterPrivate{}
	mi := &file_openapi_annotations_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpenApiFilterPrivate) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenApiFilterPrivate) ProtoMessage() {}

func (x *OpenApiFilterPrivate) ProtoReflect() protoreflect.Message {
	mi := &file_openapi_annotations_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenApiFilterPrivate.ProtoReflect.Descriptor instead.
func (*OpenApiFilterPrivate) Descriptor() ([]byte, []int) {
	return file_openapi_annotations_proto_rawDescGZIP(), []int{1}
}

func (x *OpenApiFilterPrivate) GetPrivate() bool {
	if x != nil && x.Private != nil {
		return *x.Private
	}
	return false
}

var file_openapi_annotations_proto_extTypes = []protoimpl.ExtensionInfo{
	{
		ExtendedType:  (*descriptorpb.MethodOptions)(nil),
		ExtensionType: (*OpenApiOptions)(nil),
		Field:         2506,
		Name:          "openapi.method_params",
		Tag:           "bytes,2506,opt,name=method_params",
		Filename:      "openapi/annotations.proto",
	},
	{
		ExtendedType:  (*descriptorpb.FieldOptions)(nil),
		ExtensionType: (*OpenApiFilterPrivate)(nil),
		Field:         2506,
		Name:          "openapi.field_params",
		Tag:           "bytes,2506,opt,name=field_params",
		Filename:      "openapi/annotations.proto",
	},
}

// Extension fields to descriptorpb.MethodOptions.
var (
	// optional openapi.OpenApiOptions method_params = 2506;
	E_MethodParams = &file_openapi_annotations_proto_extTypes[0]
)

// Extension fields to descriptorpb.FieldOptions.
var (
	// optional openapi.OpenApiFilterPrivate field_params = 2506;
	E_FieldParams = &file_openapi_annotations_proto_extTypes[1]
)

var File_openapi_annotations_proto protoreflect.FileDescriptor

var file_openapi_annotations_proto_rawDesc = string([]byte{
	0x0a, 0x19, 0x6f, 0x70, 0x65, 0x6e, 0x61, 0x70, 0x69, 0x2f, 0x61, 0x6e, 0x6e, 0x6f, 0x74, 0x61,
	0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x07, 0x6f, 0x70, 0x65,
	0x6e, 0x61, 0x70, 0x69, 0x1a, 0x20, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x6f, 0x72,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x38, 0x0a, 0x0e, 0x4f, 0x70, 0x65, 0x6e, 0x41, 0x70,
	0x69, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x12, 0x1b, 0x0a, 0x06, 0x70, 0x75, 0x62, 0x6c,
	0x69, 0x63, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x48, 0x00, 0x52, 0x06, 0x70, 0x75, 0x62, 0x6c,
	0x69, 0x63, 0x88, 0x01, 0x01, 0x42, 0x09, 0x0a, 0x07, 0x5f, 0x70, 0x75, 0x62, 0x6c, 0x69, 0x63,
	0x22, 0x41, 0x0a, 0x14, 0x4f, 0x70, 0x65, 0x6e, 0x41, 0x70, 0x69, 0x46, 0x69, 0x6c, 0x74, 0x65,
	0x72, 0x50, 0x72, 0x69, 0x76, 0x61, 0x74, 0x65, 0x12, 0x1d, 0x0a, 0x07, 0x70, 0x72, 0x69, 0x76,
	0x61, 0x74, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x48, 0x00, 0x52, 0x07, 0x70, 0x72, 0x69,
	0x76, 0x61, 0x74, 0x65, 0x88, 0x01, 0x01, 0x42, 0x0a, 0x0a, 0x08, 0x5f, 0x70, 0x72, 0x69, 0x76,
	0x61, 0x74, 0x65, 0x3a, 0x5d, 0x0a, 0x0d, 0x6d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x5f, 0x70, 0x61,
	0x72, 0x61, 0x6d, 0x73, 0x12, 0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f, 0x70, 0x74,
	0x69, 0x6f, 0x6e, 0x73, 0x18, 0xca, 0x13, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x17, 0x2e, 0x6f, 0x70,
	0x65, 0x6e, 0x61, 0x70, 0x69, 0x2e, 0x4f, 0x70, 0x65, 0x6e, 0x41, 0x70, 0x69, 0x4f, 0x70, 0x74,
	0x69, 0x6f, 0x6e, 0x73, 0x52, 0x0c, 0x6d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x50, 0x61, 0x72, 0x61,
	0x6d, 0x73, 0x3a, 0x60, 0x0a, 0x0c, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x5f, 0x70, 0x61, 0x72, 0x61,
	0x6d, 0x73, 0x12, 0x1d, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2e, 0x46, 0x69, 0x65, 0x6c, 0x64, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e,
	0x73, 0x18, 0xca, 0x13, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1d, 0x2e, 0x6f, 0x70, 0x65, 0x6e, 0x61,
	0x70, 0x69, 0x2e, 0x4f, 0x70, 0x65, 0x6e, 0x41, 0x70, 0x69, 0x46, 0x69, 0x6c, 0x74, 0x65, 0x72,
	0x50, 0x72, 0x69, 0x76, 0x61, 0x74, 0x65, 0x52, 0x0b, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x50, 0x61,
	0x72, 0x61, 0x6d, 0x73, 0x42, 0x42, 0x5a, 0x40, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63,
	0x6f, 0x6d, 0x2f, 0x73, 0x75, 0x64, 0x6f, 0x72, 0x61, 0x6e, 0x64, 0x6f, 0x6d, 0x2f, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x63, 0x2d, 0x67, 0x65, 0x6e, 0x2d, 0x63, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74,
	0x2d, 0x6f, 0x70, 0x65, 0x6e, 0x61, 0x70, 0x69, 0x2f, 0x6f, 0x70, 0x65, 0x6e, 0x61, 0x70, 0x69,
	0x3b, 0x6f, 0x70, 0x65, 0x6e, 0x61, 0x70, 0x69, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_openapi_annotations_proto_rawDescOnce sync.Once
	file_openapi_annotations_proto_rawDescData []byte
)

func file_openapi_annotations_proto_rawDescGZIP() []byte {
	file_openapi_annotations_proto_rawDescOnce.Do(func() {
		file_openapi_annotations_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_openapi_annotations_proto_rawDesc), len(file_openapi_annotations_proto_rawDesc)))
	})
	return file_openapi_annotations_proto_rawDescData
}

var file_openapi_annotations_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_openapi_annotations_proto_goTypes = []any{
	(*OpenApiOptions)(nil),             // 0: openapi.OpenApiOptions
	(*OpenApiFilterPrivate)(nil),       // 1: openapi.OpenApiFilterPrivate
	(*descriptorpb.MethodOptions)(nil), // 2: google.protobuf.MethodOptions
	(*descriptorpb.FieldOptions)(nil),  // 3: google.protobuf.FieldOptions
}
var file_openapi_annotations_proto_depIdxs = []int32{
	2, // 0: openapi.method_params:extendee -> google.protobuf.MethodOptions
	3, // 1: openapi.field_params:extendee -> google.protobuf.FieldOptions
	0, // 2: openapi.method_params:type_name -> openapi.OpenApiOptions
	1, // 3: openapi.field_params:type_name -> openapi.OpenApiFilterPrivate
	4, // [4:4] is the sub-list for method output_type
	4, // [4:4] is the sub-list for method input_type
	2, // [2:4] is the sub-list for extension type_name
	0, // [0:2] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_openapi_annotations_proto_init() }
func file_openapi_annotations_proto_init() {
	if File_openapi_annotations_proto != nil {
		return
	}
	file_openapi_annotations_proto_msgTypes[0].OneofWrappers = []any{}
	file_openapi_annotations_proto_msgTypes[1].OneofWrappers = []any{}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_openapi_annotations_proto_rawDesc), len(file_openapi_annotations_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 2,
			NumServices:   0,
		},
		GoTypes:           file_openapi_annotations_proto_goTypes,
		DependencyIndexes: file_openapi_annotations_proto_depIdxs,
		MessageInfos:      file_openapi_annotations_proto_msgTypes,
		ExtensionInfos:    file_openapi_annotations_proto_extTypes,
	}.Build()
	File_openapi_annotations_proto = out.File
	file_openapi_annotations_proto_goTypes = nil
	file_openapi_annotations_proto_depIdxs = nil
}
