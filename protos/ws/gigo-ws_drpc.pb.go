// Code generated by protoc-gen-go-drpc. DO NOT EDIT.
// protoc-gen-go-drpc version: v0.0.32
// source: gigo-ws.proto

package ws

import (
	context "context"
	errors "errors"
	protojson "google.golang.org/protobuf/encoding/protojson"
	proto "google.golang.org/protobuf/proto"
	drpc "storj.io/drpc"
	drpcerr "storj.io/drpc/drpcerr"
)

type drpcEncoding_File_gigo_ws_proto struct{}

func (drpcEncoding_File_gigo_ws_proto) Marshal(msg drpc.Message) ([]byte, error) {
	return proto.Marshal(msg.(proto.Message))
}

func (drpcEncoding_File_gigo_ws_proto) MarshalAppend(buf []byte, msg drpc.Message) ([]byte, error) {
	return proto.MarshalOptions{}.MarshalAppend(buf, msg.(proto.Message))
}

func (drpcEncoding_File_gigo_ws_proto) Unmarshal(buf []byte, msg drpc.Message) error {
	return proto.Unmarshal(buf, msg.(proto.Message))
}

func (drpcEncoding_File_gigo_ws_proto) JSONMarshal(msg drpc.Message) ([]byte, error) {
	return protojson.Marshal(msg.(proto.Message))
}

func (drpcEncoding_File_gigo_ws_proto) JSONUnmarshal(buf []byte, msg drpc.Message) error {
	return protojson.Unmarshal(buf, msg.(proto.Message))
}

type DRPCGigoWSClient interface {
	DRPCConn() drpc.Conn

	Echo(ctx context.Context, in *EchoRequest) (*EchoResponse, error)
	CreateWorkspace(ctx context.Context, in *CreateWorkspaceRequest) (*CreateWorkspaceResponse, error)
	StartWorkspace(ctx context.Context, in *StartWorkspaceRequest) (*StartWorkspaceResponse, error)
	StopWorkspace(ctx context.Context, in *StopWorkspaceRequest) (*StopWorkspaceResponse, error)
	DestroyWorkspace(ctx context.Context, in *DestroyWorkspaceRequest) (*DestroyWorkspaceResponse, error)
}

type drpcGigoWSClient struct {
	cc drpc.Conn
}

func NewDRPCGigoWSClient(cc drpc.Conn) DRPCGigoWSClient {
	return &drpcGigoWSClient{cc}
}

func (c *drpcGigoWSClient) DRPCConn() drpc.Conn { return c.cc }

func (c *drpcGigoWSClient) Echo(ctx context.Context, in *EchoRequest) (*EchoResponse, error) {
	out := new(EchoResponse)
	err := c.cc.Invoke(ctx, "/ws.GigoWS/Echo", drpcEncoding_File_gigo_ws_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcGigoWSClient) CreateWorkspace(ctx context.Context, in *CreateWorkspaceRequest) (*CreateWorkspaceResponse, error) {
	out := new(CreateWorkspaceResponse)
	err := c.cc.Invoke(ctx, "/ws.GigoWS/CreateWorkspace", drpcEncoding_File_gigo_ws_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcGigoWSClient) StartWorkspace(ctx context.Context, in *StartWorkspaceRequest) (*StartWorkspaceResponse, error) {
	out := new(StartWorkspaceResponse)
	err := c.cc.Invoke(ctx, "/ws.GigoWS/StartWorkspace", drpcEncoding_File_gigo_ws_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcGigoWSClient) StopWorkspace(ctx context.Context, in *StopWorkspaceRequest) (*StopWorkspaceResponse, error) {
	out := new(StopWorkspaceResponse)
	err := c.cc.Invoke(ctx, "/ws.GigoWS/StopWorkspace", drpcEncoding_File_gigo_ws_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *drpcGigoWSClient) DestroyWorkspace(ctx context.Context, in *DestroyWorkspaceRequest) (*DestroyWorkspaceResponse, error) {
	out := new(DestroyWorkspaceResponse)
	err := c.cc.Invoke(ctx, "/ws.GigoWS/DestroyWorkspace", drpcEncoding_File_gigo_ws_proto{}, in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type DRPCGigoWSServer interface {
	Echo(context.Context, *EchoRequest) (*EchoResponse, error)
	CreateWorkspace(context.Context, *CreateWorkspaceRequest) (*CreateWorkspaceResponse, error)
	StartWorkspace(context.Context, *StartWorkspaceRequest) (*StartWorkspaceResponse, error)
	StopWorkspace(context.Context, *StopWorkspaceRequest) (*StopWorkspaceResponse, error)
	DestroyWorkspace(context.Context, *DestroyWorkspaceRequest) (*DestroyWorkspaceResponse, error)
}

type DRPCGigoWSUnimplementedServer struct{}

func (s *DRPCGigoWSUnimplementedServer) Echo(context.Context, *EchoRequest) (*EchoResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), drpcerr.Unimplemented)
}

func (s *DRPCGigoWSUnimplementedServer) CreateWorkspace(context.Context, *CreateWorkspaceRequest) (*CreateWorkspaceResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), drpcerr.Unimplemented)
}

func (s *DRPCGigoWSUnimplementedServer) StartWorkspace(context.Context, *StartWorkspaceRequest) (*StartWorkspaceResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), drpcerr.Unimplemented)
}

func (s *DRPCGigoWSUnimplementedServer) StopWorkspace(context.Context, *StopWorkspaceRequest) (*StopWorkspaceResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), drpcerr.Unimplemented)
}

func (s *DRPCGigoWSUnimplementedServer) DestroyWorkspace(context.Context, *DestroyWorkspaceRequest) (*DestroyWorkspaceResponse, error) {
	return nil, drpcerr.WithCode(errors.New("Unimplemented"), drpcerr.Unimplemented)
}

type DRPCGigoWSDescription struct{}

func (DRPCGigoWSDescription) NumMethods() int { return 5 }

func (DRPCGigoWSDescription) Method(n int) (string, drpc.Encoding, drpc.Receiver, interface{}, bool) {
	switch n {
	case 0:
		return "/ws.GigoWS/Echo", drpcEncoding_File_gigo_ws_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCGigoWSServer).
					Echo(
						ctx,
						in1.(*EchoRequest),
					)
			}, DRPCGigoWSServer.Echo, true
	case 1:
		return "/ws.GigoWS/CreateWorkspace", drpcEncoding_File_gigo_ws_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCGigoWSServer).
					CreateWorkspace(
						ctx,
						in1.(*CreateWorkspaceRequest),
					)
			}, DRPCGigoWSServer.CreateWorkspace, true
	case 2:
		return "/ws.GigoWS/StartWorkspace", drpcEncoding_File_gigo_ws_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCGigoWSServer).
					StartWorkspace(
						ctx,
						in1.(*StartWorkspaceRequest),
					)
			}, DRPCGigoWSServer.StartWorkspace, true
	case 3:
		return "/ws.GigoWS/StopWorkspace", drpcEncoding_File_gigo_ws_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCGigoWSServer).
					StopWorkspace(
						ctx,
						in1.(*StopWorkspaceRequest),
					)
			}, DRPCGigoWSServer.StopWorkspace, true
	case 4:
		return "/ws.GigoWS/DestroyWorkspace", drpcEncoding_File_gigo_ws_proto{},
			func(srv interface{}, ctx context.Context, in1, in2 interface{}) (drpc.Message, error) {
				return srv.(DRPCGigoWSServer).
					DestroyWorkspace(
						ctx,
						in1.(*DestroyWorkspaceRequest),
					)
			}, DRPCGigoWSServer.DestroyWorkspace, true
	default:
		return "", nil, nil, nil, false
	}
}

func DRPCRegisterGigoWS(mux drpc.Mux, impl DRPCGigoWSServer) error {
	return mux.Register(impl, DRPCGigoWSDescription{})
}

type DRPCGigoWS_EchoStream interface {
	drpc.Stream
	SendAndClose(*EchoResponse) error
}

type drpcGigoWS_EchoStream struct {
	drpc.Stream
}

func (x *drpcGigoWS_EchoStream) SendAndClose(m *EchoResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_gigo_ws_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCGigoWS_CreateWorkspaceStream interface {
	drpc.Stream
	SendAndClose(*CreateWorkspaceResponse) error
}

type drpcGigoWS_CreateWorkspaceStream struct {
	drpc.Stream
}

func (x *drpcGigoWS_CreateWorkspaceStream) SendAndClose(m *CreateWorkspaceResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_gigo_ws_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCGigoWS_StartWorkspaceStream interface {
	drpc.Stream
	SendAndClose(*StartWorkspaceResponse) error
}

type drpcGigoWS_StartWorkspaceStream struct {
	drpc.Stream
}

func (x *drpcGigoWS_StartWorkspaceStream) SendAndClose(m *StartWorkspaceResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_gigo_ws_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCGigoWS_StopWorkspaceStream interface {
	drpc.Stream
	SendAndClose(*StopWorkspaceResponse) error
}

type drpcGigoWS_StopWorkspaceStream struct {
	drpc.Stream
}

func (x *drpcGigoWS_StopWorkspaceStream) SendAndClose(m *StopWorkspaceResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_gigo_ws_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}

type DRPCGigoWS_DestroyWorkspaceStream interface {
	drpc.Stream
	SendAndClose(*DestroyWorkspaceResponse) error
}

type drpcGigoWS_DestroyWorkspaceStream struct {
	drpc.Stream
}

func (x *drpcGigoWS_DestroyWorkspaceStream) SendAndClose(m *DestroyWorkspaceResponse) error {
	if err := x.MsgSend(m, drpcEncoding_File_gigo_ws_proto{}); err != nil {
		return err
	}
	return x.CloseSend()
}
