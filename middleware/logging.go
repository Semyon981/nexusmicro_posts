package middleware

import (
	"context"
	"time"

	"github.com/go-kit/log"

	"github.com/NexusIT-Dev/nexusmicro_publications/pb"
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/log/level"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Middleware func(pb.PostsServer) pb.PostsServer

func LoggingMiddleware(logger log.Logger, requestCount metrics.Counter, requestLatency metrics.Histogram) Middleware {
	return func(next pb.PostsServer) pb.PostsServer {
		return &loggingMiddleware{
			next:           next,
			logger:         logger,
			requestCount:   requestCount,
			requestLatency: requestLatency,
		}
	}
}

type loggingMiddleware struct {
	logger         log.Logger
	next           pb.PostsServer
	requestCount   metrics.Counter
	requestLatency metrics.Histogram
}

func parseCode(code codes.Code, logger log.Logger) log.Logger {
	switch code {
	case codes.Internal:
		return level.Error(logger)
	case codes.Unknown:
		return level.Error(logger)
	default:
		return level.Info(logger)
	}
}

func (mw *loggingMiddleware) logfunc(begin time.Time, method string, err error) {
	code := status.Code(err)
	logger := parseCode(code, mw.logger)
	_ = logger.Log(
		"method", method,
		"err", err,
		"took", time.Since(begin),
	)
	lvs := []string{"method", method, "code", code.String()}
	mw.requestCount.With(lvs...).Add(1)
	mw.requestLatency.With(lvs...).Observe(float64(time.Since(begin).Microseconds()))
}

func (mw *loggingMiddleware) NewPost(ctx context.Context, req *pb.NewPostRequest) (*pb.NewPostResponse, error) {
	start_time := time.Now()
	res, err := mw.next.NewPost(ctx, req)
	mw.logfunc(start_time, "NewPost", err)
	return res, err
}
func (mw *loggingMiddleware) GetPostsList(ctx context.Context, req *pb.GetPostsListRequest) (*pb.GetPostsListResponse, error) {
	start_time := time.Now()
	res, err := mw.next.GetPostsList(ctx, req)
	mw.logfunc(start_time, "GetPostsList", err)
	return res, err
}
func (mw *loggingMiddleware) GetPostsUser(ctx context.Context, req *pb.GetPostsUserRequest) (*pb.GetPostsUserResponse, error) {
	start_time := time.Now()
	res, err := mw.next.GetPostsUser(ctx, req)
	mw.logfunc(start_time, "GetPostsUser", err)
	return res, err
}
func (mw *loggingMiddleware) AddLike(ctx context.Context, req *pb.AddLikeRequest) (*pb.AddLikeResponse, error) {
	start_time := time.Now()
	res, err := mw.next.AddLike(ctx, req)
	mw.logfunc(start_time, "AddLike", err)
	return res, err
}
func (mw *loggingMiddleware) DeleteLike(ctx context.Context, req *pb.DeleteLikeRequest) (*pb.DeleteLikeResponse, error) {
	start_time := time.Now()
	res, err := mw.next.DeleteLike(ctx, req)
	mw.logfunc(start_time, "DeleteLike", err)
	return res, err
}
func (mw *loggingMiddleware) WriteComment(ctx context.Context, req *pb.WriteCommentRequest) (*pb.WriteCommentResponse, error) {
	start_time := time.Now()
	res, err := mw.next.WriteComment(ctx, req)
	mw.logfunc(start_time, "WriteComment", err)
	return res, err
}
func (mw *loggingMiddleware) GetCommentsList(ctx context.Context, req *pb.GetCommentsListRequest) (*pb.GetCommentsListResponse, error) {
	start_time := time.Now()
	res, err := mw.next.GetCommentsList(ctx, req)
	mw.logfunc(start_time, "GetCommentsList", err)
	return res, err
}
func (mw *loggingMiddleware) UpdatePost(ctx context.Context, req *pb.UpdatePostRequest) (*pb.UpdatePostResponse, error) {
	start_time := time.Now()
	res, err := mw.next.UpdatePost(ctx, req)
	mw.logfunc(start_time, "UpdatePost", err)
	return res, err
}

func (mw *loggingMiddleware) GetPostById(ctx context.Context, req *pb.GetPostByIdRequest) (*pb.GetPostByIdResponse, error) {
	start_time := time.Now()
	res, err := mw.next.GetPostById(ctx, req)
	mw.logfunc(start_time, "GetPostById", err)
	return res, err
}
