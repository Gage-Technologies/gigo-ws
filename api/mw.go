package api

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/bwmarrin/snowflake"
	"github.com/gage-technologies/gigo-lib/logging"
	"storj.io/drpc"
	"storj.io/drpc/drpcmetadata"
)

type streamWrapper struct {
	drpc.Stream
	ctx context.Context
	id  int64
}

type DrpcMiddlewareOptions struct {
	WaitGroup     *sync.WaitGroup
	Handler       drpc.Handler
	Logger        logging.Logger
	SnowflakeNode *snowflake.Node
}

type DrpcMiddleware struct {
	wg            *sync.WaitGroup
	handler       drpc.Handler
	logger        logging.Logger
	snowflakeNode *snowflake.Node
}

func NewDrpcMiddleware(opts DrpcMiddlewareOptions) *DrpcMiddleware {
	return &DrpcMiddleware{
		wg:            opts.WaitGroup,
		handler:       opts.Handler,
		logger:        opts.Logger,
		snowflakeNode: opts.SnowflakeNode,
	}
}

func (s *streamWrapper) Context() context.Context {
	return s.ctx
}

func (mw *DrpcMiddleware) initRpc(rpc string, metadata map[string]string) int64 {
	// create unique id for the request
	id := mw.snowflakeNode.Generate().Int64()
	mw.logger.Debugf("beginning rpc %q\n    id: %d\n    meta: %+v", rpc, id, metadata)
	// increment wait group to track inflight connection
	mw.wg.Add(1)
	return id
}

func (mw *DrpcMiddleware) completeRpc(id int64, rpc string) {
	// decrement wait group on exit
	defer mw.wg.Done()

	// handle panic
	if r := recover(); r != nil {
		fmt.Println("panic: ", rpc)
		mw.logger.Error(fmt.Sprintf(
			"panic in rpc %q\n    id: %d\n    err: %v\n    stack:\n%s",
			rpc, id, r, string(debug.Stack()),
		))
		// flush on panic so we can be sure that we see the log
		// even if we fail anyway
		mw.logger.Flush()
		return
	}

	mw.logger.Debugf("closing rpc %q\n    id: %d", rpc, id)
}

func (mw *DrpcMiddleware) HandleRPC(stream drpc.Stream, rpc string) (err error) {
	fmt.Println("starting: ", rpc)
	metadata, _ := drpcmetadata.Get(stream.Context())
	id := mw.initRpc(rpc, metadata)
	defer mw.completeRpc(id, rpc)
	return mw.handler.HandleRPC(&streamWrapper{
		Stream: stream,
		ctx:    context.WithValue(stream.Context(), "id", id),
		id:     id,
	}, rpc)
}
