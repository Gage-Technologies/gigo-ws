// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        v3.15.8
// source: stop.proto

package ws

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type StopWorkspaceRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// auth is currently unused but we allocate the slot for later
	Auth        string `protobuf:"bytes,1,opt,name=auth,proto3" json:"auth,omitempty"`
	WorkspaceId int64  `protobuf:"varint,2,opt,name=workspace_id,json=workspaceId,proto3" json:"workspace_id,omitempty"`
}

func (x *StopWorkspaceRequest) Reset() {
	*x = StopWorkspaceRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_stop_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StopWorkspaceRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StopWorkspaceRequest) ProtoMessage() {}

func (x *StopWorkspaceRequest) ProtoReflect() protoreflect.Message {
	mi := &file_stop_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StopWorkspaceRequest.ProtoReflect.Descriptor instead.
func (*StopWorkspaceRequest) Descriptor() ([]byte, []int) {
	return file_stop_proto_rawDescGZIP(), []int{0}
}

func (x *StopWorkspaceRequest) GetAuth() string {
	if x != nil {
		return x.Auth
	}
	return ""
}

func (x *StopWorkspaceRequest) GetWorkspaceId() int64 {
	if x != nil {
		return x.WorkspaceId
	}
	return 0
}

type StopWorkspaceResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Status  ResponseCode `protobuf:"varint,1,opt,name=status,proto3,enum=ws.ResponseCode" json:"status,omitempty"`
	Success *Success     `protobuf:"bytes,2,opt,name=success,proto3" json:"success,omitempty"`
	Error   *Error       `protobuf:"bytes,3,opt,name=error,proto3" json:"error,omitempty"`
}

func (x *StopWorkspaceResponse) Reset() {
	*x = StopWorkspaceResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_stop_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StopWorkspaceResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StopWorkspaceResponse) ProtoMessage() {}

func (x *StopWorkspaceResponse) ProtoReflect() protoreflect.Message {
	mi := &file_stop_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StopWorkspaceResponse.ProtoReflect.Descriptor instead.
func (*StopWorkspaceResponse) Descriptor() ([]byte, []int) {
	return file_stop_proto_rawDescGZIP(), []int{1}
}

func (x *StopWorkspaceResponse) GetStatus() ResponseCode {
	if x != nil {
		return x.Status
	}
	return ResponseCode_SUCCESS
}

func (x *StopWorkspaceResponse) GetSuccess() *Success {
	if x != nil {
		return x.Success
	}
	return nil
}

func (x *StopWorkspaceResponse) GetError() *Error {
	if x != nil {
		return x.Error
	}
	return nil
}

var File_stop_proto protoreflect.FileDescriptor

var file_stop_proto_rawDesc = []byte{
	0x0a, 0x0a, 0x73, 0x74, 0x6f, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x02, 0x77, 0x73,
	0x1a, 0x0b, 0x74, 0x79, 0x70, 0x65, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x4d, 0x0a,
	0x14, 0x53, 0x74, 0x6f, 0x70, 0x57, 0x6f, 0x72, 0x6b, 0x73, 0x70, 0x61, 0x63, 0x65, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x61, 0x75, 0x74, 0x68, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x04, 0x61, 0x75, 0x74, 0x68, 0x12, 0x21, 0x0a, 0x0c, 0x77, 0x6f, 0x72,
	0x6b, 0x73, 0x70, 0x61, 0x63, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x03, 0x52,
	0x0b, 0x77, 0x6f, 0x72, 0x6b, 0x73, 0x70, 0x61, 0x63, 0x65, 0x49, 0x64, 0x22, 0x89, 0x01, 0x0a,
	0x15, 0x53, 0x74, 0x6f, 0x70, 0x57, 0x6f, 0x72, 0x6b, 0x73, 0x70, 0x61, 0x63, 0x65, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x28, 0x0a, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x10, 0x2e, 0x77, 0x73, 0x2e, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x43, 0x6f, 0x64, 0x65, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73,
	0x12, 0x25, 0x0a, 0x07, 0x73, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x0b, 0x2e, 0x77, 0x73, 0x2e, 0x53, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x52, 0x07,
	0x73, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x12, 0x1f, 0x0a, 0x05, 0x65, 0x72, 0x72, 0x6f, 0x72,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x09, 0x2e, 0x77, 0x73, 0x2e, 0x45, 0x72, 0x72, 0x6f,
	0x72, 0x52, 0x05, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x42, 0x0b, 0x5a, 0x09, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x73, 0x2f, 0x77, 0x73, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_stop_proto_rawDescOnce sync.Once
	file_stop_proto_rawDescData = file_stop_proto_rawDesc
)

func file_stop_proto_rawDescGZIP() []byte {
	file_stop_proto_rawDescOnce.Do(func() {
		file_stop_proto_rawDescData = protoimpl.X.CompressGZIP(file_stop_proto_rawDescData)
	})
	return file_stop_proto_rawDescData
}

var file_stop_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_stop_proto_goTypes = []interface{}{
	(*StopWorkspaceRequest)(nil),  // 0: ws.StopWorkspaceRequest
	(*StopWorkspaceResponse)(nil), // 1: ws.StopWorkspaceResponse
	(ResponseCode)(0),             // 2: ws.ResponseCode
	(*Success)(nil),               // 3: ws.Success
	(*Error)(nil),                 // 4: ws.Error
}
var file_stop_proto_depIdxs = []int32{
	2, // 0: ws.StopWorkspaceResponse.status:type_name -> ws.ResponseCode
	3, // 1: ws.StopWorkspaceResponse.success:type_name -> ws.Success
	4, // 2: ws.StopWorkspaceResponse.error:type_name -> ws.Error
	3, // [3:3] is the sub-list for method output_type
	3, // [3:3] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_stop_proto_init() }
func file_stop_proto_init() {
	if File_stop_proto != nil {
		return
	}
	file_types_proto_init()
	if !protoimpl.UnsafeEnabled {
		file_stop_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StopWorkspaceRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_stop_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StopWorkspaceResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_stop_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_stop_proto_goTypes,
		DependencyIndexes: file_stop_proto_depIdxs,
		MessageInfos:      file_stop_proto_msgTypes,
	}.Build()
	File_stop_proto = out.File
	file_stop_proto_rawDesc = nil
	file_stop_proto_goTypes = nil
	file_stop_proto_depIdxs = nil
}