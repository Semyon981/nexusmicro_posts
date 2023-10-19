package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/NexusIT-Dev/nexusmicro_publications/pb"
	"github.com/go-kit/log"
	"github.com/gocql/gocql"
	"github.com/godruoyi/go-snowflake"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type service struct {
	bucketDuration time.Duration
	cses           *gocql.Session
	signingKey     []byte
	storagecli     pb.StorageClient
	userscli       pb.UsersClient
	linkedacccli   pb.LinkedaccClient
}

func NewService(cses *gocql.Session,
	signingKey []byte,
	bucketDuration time.Duration,
	storagecli pb.StorageClient,
	userscli pb.UsersClient,
	linkedacccli pb.LinkedaccClient,
) pb.PostsServer {
	return &service{
		cses:           cses,
		signingKey:     signingKey,
		bucketDuration: bucketDuration,
		storagecli:     storagecli,
		userscli:       userscli,
		linkedacccli:   linkedacccli,
	}
}

func GetUnaryInterceptor(signingKey []byte, logger log.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {

		token := ""

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, ErrInternal(fmt.Errorf("failed to get metadata"))
		}
		headertoken := md.Get("authorization")
		if headertoken != nil && len(headertoken) == 1 {
			stringsheader := strings.Split(headertoken[0], " ")

			if stringsheader[0] != "Bearer" {
				return nil, ErrInvalidAccessToken
			}
			if len(stringsheader) != 2 {
				return nil, ErrInvalidAccessToken
			}
			token = stringsheader[1]
		} else {
			return nil, ErrInvalidAccessToken
		}

		jwttoken, err := jwt.ParseWithClaims(token, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
			}
			return signingKey, nil
		})

		if err != nil {
			return nil, ErrInvalidAccessToken
		}

		if claims, ok := jwttoken.Claims.(*jwt.RegisteredClaims); ok && jwttoken.Valid {
			ctx = context.WithValue(ctx, claims.Subject, claims.ID)
			ctx = metadata.NewOutgoingContext(ctx, md)
		} else {
			return nil, ErrInvalidAccessToken
		}

		return handler(ctx, req)
	}
}

func (s service) GetPostById(ctx context.Context, req *pb.GetPostByIdRequest) (*pb.GetPostByIdResponse, error) {

	user_id, err := strconv.ParseInt(ctx.Value("user").(string), 10, 64)
	if err != nil {
		return nil, ErrInternal(err)
	}

	res := &pb.GetPostByIdResponse{Post: &pb.Post{}}

	bucket := snowflake.ParseID(req.Id).Timestamp / uint64(s.bucketDuration.Milliseconds())

	att := []*pb.AttachmentId{}
	err = s.cses.Query("SELECT id, message, owner_id, attachments FROM posts WHERE bucket = ? AND id = ?", bucket, req.Id).Scan(&res.Post.Id, &res.Post.Message, &res.Post.OwnerId, &att)

	if err != nil {
		if err == gocql.ErrNotFound {
			return nil, ErrPostNotFound
		}
		return nil, ErrInternal(err)
	}

	attach, err := s.storagecli.GetAttachments(ctx, &pb.GetAttachmentsRequest{Ids: att})
	if err != nil {
		if status.Code(err) == codes.Unavailable {
			return nil, ErrServiceStorageUnvaliable
		}
		fmt.Println(err)
		return nil, err
	}
	res.Post.Attachments = attach.Attachments

	sid := snowflake.ParseID(res.Post.Id)
	res.Post.Time = timestamppb.New(sid.GenerateTime().Local())

	res.Post.Likes = &pb.LikesInfo{}
	err = s.cses.Query("SELECT Count(*) FROM likes WHERE post_id = ?", res.Post.Id).Scan(&res.Post.Likes.Count)
	if err != nil {
		return nil, ErrInternal(err)
	}

	var cntlikes int64
	res.Post.Likes.Liked = new(bool)
	err = s.cses.Query("SELECT Count(*) FROM likes WHERE post_id = ? AND owner_id = ?", res.Post.Id, user_id).Scan(&cntlikes)
	if err != nil {
		return nil, ErrInternal(err)
	}
	if cntlikes > 0 {
		*res.Post.Likes.Liked = true
	}

	res.Post.Comments = &pb.CommentsInfo{}
	err = s.cses.Query("SELECT Count(*) FROM comments WHERE post_id = ?", res.Post.Id).Scan(&res.Post.Comments.Count)
	if err != nil {
		return nil, ErrInternal(err)
	}

	if req.Extended && req.CommentsLimit > 0 {

		commentsres, err := s.GetCommentsList(ctx, &pb.GetCommentsListRequest{PostId: res.Post.Id, Limit: req.CommentsLimit, Extended: req.CommentsExtended, SortDir: req.CommentsSortDir, Fields: req.CommentsFields})
		if err != nil {
			return nil, err
		}

		res.Post.Comments.Items = commentsres.Comments
	}

	return res, nil
}

func (s service) NewPost(ctx context.Context, req *pb.NewPostRequest) (*pb.NewPostResponse, error) {

	user_id, err := strconv.ParseInt(ctx.Value("user").(string), 10, 64)
	if err != nil {
		return nil, ErrInternal(err)
	}

	if len(req.AttachmentsIds) == 0 && req.Message == "" {
		return nil, ErrEmptyContent
	}

	id := snowflake.ID()
	bucket := snowflake.ParseID(id).Timestamp / uint64(s.bucketDuration.Milliseconds())
	sid := snowflake.ParseID(id)

	res := &pb.NewPostResponse{Post: &pb.Post{
		Id:       id,
		Time:     timestamppb.New(sid.GenerateTime()),
		OwnerId:  user_id,
		Message:  req.Message,
		Likes:    &pb.LikesInfo{Count: new(int64)},
		Comments: &pb.CommentsInfo{Count: new(int64)},
	}}

	if len(req.AttachmentsIds) != 0 {
		att, err := s.storagecli.GetAttachments(ctx, &pb.GetAttachmentsRequest{Ids: req.AttachmentsIds})
		if err != nil {
			if status.Code(err) == codes.Unavailable {
				return nil, ErrServiceStorageUnvaliable
			} else if status.Code(err) == codes.Internal {
				return nil, err
			}
			return nil, ErrInvalidAttachments
		}
		res.Post.Attachments = att.Attachments
	}

	err = s.cses.Query("INSERT INTO posts (bucket, id, message, attachments, owner_id) VALUES (?, ?, ?, ?, ?)", bucket, id, req.Message, req.AttachmentsIds, user_id).Exec()
	if err != nil {
		return nil, ErrInternal(err)
	}

	_, err = s.linkedacccli.NewExternalPost(ctx, &pb.NewExternalPostRequest{
		PostId: id,
		Ids:    req.LinkedaccIds,
	})
	if err != nil {
		if status.Code(err) == codes.Unavailable {
			return nil, ErrServiceLinkedaccUnvaliable
		}
		return nil, err
	}

	return res, nil
}

func (s service) GetPostsList(ctx context.Context, req *pb.GetPostsListRequest) (*pb.GetPostsListResponse, error) {

	user_id, err := strconv.ParseInt(ctx.Value("user").(string), 10, 64)
	if err != nil {
		return nil, ErrInternal(err)
	}

	if req.Limit < 0 || req.Limit > 100 {
		return nil, ErrLimitError
	}

	res := &pb.GetPostsListResponse{}
	res.Posts = make([]*pb.Post, 0, req.Limit)

	bucket := snowflake.ParseID(snowflake.ID()).Timestamp / uint64(s.bucketDuration.Milliseconds())

	params := make([]any, 0)
	params = append(params, bucket)

	condition := ""

	if req.LastId > 0 {
		condition += "AND id < ?"
		params = append(params, req.LastId)
	}

	params = append(params, req.Limit)

	for i := 0; i < int(req.Limit); {

		params[len(params)-1] = req.Limit - int64(i)

		iter := s.cses.Query("SELECT id, message, owner_id, attachments FROM posts WHERE bucket = ? "+condition+" ORDER BY id DESC LIMIT ?", params...).Iter()

		att := []*pb.AttachmentId{}
		tmppost := &pb.Post{}
		for iter.Scan(&tmppost.Id, &tmppost.Message, &tmppost.OwnerId, &att) {
			attach, err := s.storagecli.GetAttachments(ctx, &pb.GetAttachmentsRequest{Ids: att})
			if err != nil {
				if status.Code(err) == codes.Unavailable {
					return nil, ErrServiceStorageUnvaliable
				}
				fmt.Println(err)
				return nil, err
			}
			tmppost.Attachments = attach.Attachments

			sid := snowflake.ParseID(tmppost.Id)
			tmppost.Time = timestamppb.New(sid.GenerateTime().Local())

			tmppost.Likes = &pb.LikesInfo{}
			err = s.cses.Query("SELECT Count(*) FROM likes WHERE post_id = ?", tmppost.Id).Scan(&tmppost.Likes.Count)
			if err != nil {
				return nil, ErrInternal(err)
			}

			var cntlikes int64
			tmppost.Likes.Liked = new(bool)
			err = s.cses.Query("SELECT Count(*) FROM likes WHERE post_id = ? AND owner_id = ?", tmppost.Id, user_id).Scan(&cntlikes)
			if err != nil {
				return nil, ErrInternal(err)
			}
			if cntlikes > 0 {
				*tmppost.Likes.Liked = true
			}

			tmppost.Comments = &pb.CommentsInfo{}
			err = s.cses.Query("SELECT Count(*) FROM comments WHERE post_id = ?", tmppost.Id).Scan(&tmppost.Comments.Count)
			if err != nil {
				return nil, ErrInternal(err)
			}

			if req.Extended && req.CommentsLimit > 0 {

				commentsres, err := s.GetCommentsList(ctx, &pb.GetCommentsListRequest{PostId: tmppost.Id, Limit: req.CommentsLimit, Extended: req.CommentsExtended, SortDir: req.CommentsSortDir, Fields: req.CommentsFields})
				if err != nil {
					return nil, err
				}

				tmppost.Comments.Items = commentsres.Comments
			}

			res.Posts = append(res.Posts, tmppost)

			att = []*pb.AttachmentId{}
			tmppost = &pb.Post{}

			i++
		}

		err := iter.Close()
		if err != nil {
			return nil, ErrInternal(err)
		}

		params[0] = params[0].(uint64) - 1

		if params[0].(uint64) == 0 {
			break
		}
	}

	if req.Extended && len(res.Posts) > 0 {

		ids := make([]int64, 0, len(res.Posts))
		for i := range res.Posts {
			ids = append(ids, res.Posts[i].OwnerId)
		}

		usersres, err := s.userscli.GetUsersByIds(ctx, &pb.GetUsersByIdsRequest{Ids: ids, Fields: req.Fields})
		if err != nil {
			if status.Code(err) == codes.Unavailable {
				return nil, ErrServiceUsersUnvaliable
			}
			return nil, err
		}

		if len(usersres.Users) == len(res.Posts) {
			for i := range res.Posts {
				res.Posts[i].Owner = usersres.Users[i]
			}
		}
	}

	return res, nil
}

func (s service) GetPostsUser(ctx context.Context, req *pb.GetPostsUserRequest) (*pb.GetPostsUserResponse, error) {

	user_id, err := strconv.ParseInt(ctx.Value("user").(string), 10, 64)
	if err != nil {
		return nil, ErrInternal(err)
	}

	if req.UserId == 0 {
		req.UserId = user_id
	}

	res := &pb.GetPostsUserResponse{}
	res.Posts = make([]*pb.Post, 0, req.Limit)

	params := make([]any, 0)
	params = append(params, req.UserId)

	condition := ""

	if req.LastId > 0 {
		condition += "AND id < ?"
		params = append(params, req.LastId)
	}

	params = append(params, req.Limit)

	iter := s.cses.Query("SELECT id, message, owner_id, attachments FROM posts_by_owner_id WHERE owner_id = ? "+condition+" LIMIT ?", params...).Iter()

	att := []*pb.AttachmentId{}
	tmppost := &pb.Post{}
	for iter.Scan(&tmppost.Id, &tmppost.Message, &tmppost.OwnerId, &att) {
		attach, err := s.storagecli.GetAttachments(ctx, &pb.GetAttachmentsRequest{Ids: att})
		if err != nil {
			if status.Code(err) == codes.Unavailable {
				return nil, ErrServiceStorageUnvaliable
			}
			return nil, err
		}
		tmppost.Attachments = attach.Attachments

		sid := snowflake.ParseID(tmppost.Id)
		tmppost.Time = timestamppb.New(sid.GenerateTime().Local())

		tmppost.Likes = &pb.LikesInfo{}
		err = s.cses.Query("SELECT Count(*) FROM likes WHERE post_id = ?", tmppost.Id).Scan(&tmppost.Likes.Count)
		if err != nil {
			return nil, err
		}

		var cntlikes int64
		tmppost.Likes.Liked = new(bool)
		err = s.cses.Query("SELECT Count(*) FROM likes WHERE post_id = ? AND owner_id = ?", tmppost.Id, user_id).Scan(&cntlikes)
		if err != nil {
			return nil, ErrInternal(err)
		}
		if cntlikes > 0 {
			*tmppost.Likes.Liked = true
		}

		tmppost.Comments = &pb.CommentsInfo{}
		err = s.cses.Query("SELECT Count(*) FROM comments WHERE post_id = ?", tmppost.Id).Scan(&tmppost.Comments.Count)
		if err != nil {
			return nil, err
		}

		if req.Extended && req.CommentsLimit > 0 {

			commentsres, err := s.GetCommentsList(ctx, &pb.GetCommentsListRequest{PostId: tmppost.Id, Limit: req.CommentsLimit, Extended: req.CommentsExtended, SortDir: req.CommentsSortDir, Fields: req.CommentsFields})
			if err != nil {
				return nil, err
			}

			tmppost.Comments.Items = commentsres.Comments
		}

		res.Posts = append(res.Posts, tmppost)

		att = []*pb.AttachmentId{}
		tmppost = &pb.Post{}
	}

	err = iter.Close()
	if err != nil {
		return nil, err
	}

	if req.Extended && len(res.Posts) > 0 {

		ids := make([]int64, 0, len(res.Posts))
		for i := range res.Posts {
			ids = append(ids, res.Posts[i].OwnerId)
		}

		usersres, err := s.userscli.GetUsersByIds(ctx, &pb.GetUsersByIdsRequest{Ids: ids, Fields: req.Fields})
		if err != nil {
			if status.Code(err) == codes.Unavailable {
				return nil, ErrServiceUsersUnvaliable
			}
			return nil, err
		}

		if len(usersres.Users) == len(res.Posts) {
			for i := range res.Posts {
				res.Posts[i].Owner = usersres.Users[i]
			}
		}
	}

	return res, nil
}

func (s service) AddLike(ctx context.Context, req *pb.AddLikeRequest) (*pb.AddLikeResponse, error) {

	user_id, err := strconv.ParseInt(ctx.Value("user").(string), 10, 64)
	if err != nil {
		return nil, ErrInternal(err)
	}

	var cntposts int
	err = s.cses.Query("SELECT Count(*) FROM posts WHERE bucket = ? AND id = ?", snowflake.ParseID(req.PostId).Timestamp/uint64(s.bucketDuration.Milliseconds()), req.PostId).Scan(&cntposts)
	if err != nil {
		return nil, ErrInternal(err)
	}
	if cntposts != 1 {
		return nil, ErrPostNotFound
	}

	err = s.cses.Query("INSERT INTO likes (post_id, owner_id) VALUES(?, ?)", req.PostId, user_id).Exec()
	if err != nil {
		return nil, ErrInternal(err)
	}

	return &pb.AddLikeResponse{}, nil
}

func (s service) DeleteLike(ctx context.Context, req *pb.DeleteLikeRequest) (*pb.DeleteLikeResponse, error) {

	user_id, err := strconv.ParseInt(ctx.Value("user").(string), 10, 64)
	if err != nil {
		return nil, ErrInternal(err)
	}

	err = s.cses.Query("DELETE FROM likes WHERE post_id = ? AND owner_id = ?", req.PostId, user_id).Exec()
	if err != nil {
		return nil, ErrInternal(err)
	}

	return &pb.DeleteLikeResponse{}, nil
}

func (s service) WriteComment(ctx context.Context, req *pb.WriteCommentRequest) (*pb.WriteCommentResponse, error) {

	user_id, err := strconv.ParseInt(ctx.Value("user").(string), 10, 64)
	if err != nil {
		return nil, ErrInternal(err)
	}

	var cntposts int
	err = s.cses.Query("SELECT Count(*) FROM posts WHERE bucket = ? AND id = ?", snowflake.ParseID(req.PostId).Timestamp/uint64(s.bucketDuration.Milliseconds()), req.PostId).Scan(&cntposts)
	if err != nil {
		return nil, ErrInternal(err)
	}
	if cntposts != 1 {
		return nil, ErrPostNotFound
	}

	id := snowflake.ID()
	sid := snowflake.ParseID(id)
	res := &pb.WriteCommentResponse{Comment: &pb.Comment{
		Id:      id,
		OwnerId: user_id,
		Message: req.Messaage,
		Time:    timestamppb.New(sid.GenerateTime().Local()),
	}}

	if len(req.AttachmentsIds) != 0 {
		att, err := s.storagecli.GetAttachments(ctx, &pb.GetAttachmentsRequest{Ids: req.AttachmentsIds})
		if err != nil {
			if status.Code(err) == codes.Unavailable {
				return nil, ErrServiceStorageUnvaliable
			} else if status.Code(err) == codes.Internal {
				return nil, err
			}
			return nil, ErrInvalidAttachments
		}
		res.Comment.Attachments = att.Attachments
	}

	err = s.cses.Query("INSERT INTO comments (id, post_id, owner_id, message, attachments) VAlUES (?, ?, ?, ?, ?)", id, req.PostId, user_id, req.Messaage, req.AttachmentsIds).Exec()
	if err != nil {
		return nil, ErrInternal(err)
	}

	return res, nil
}

func (s service) GetCommentsList(ctx context.Context, req *pb.GetCommentsListRequest) (*pb.GetCommentsListResponse, error) {

	if req.Limit < 0 || req.Limit > 100 {
		return nil, ErrLimitError
	}

	res := &pb.GetCommentsListResponse{}
	res.Comments = make([]*pb.Comment, 0, req.Limit)

	params := make([]any, 0)
	params = append(params, req.PostId)

	condition := ""
	order_dir := ""

	if req.LastId > 0 {
		if req.SortDir {
			condition += "AND id > ?"
			order_dir = "ASC"
		} else {
			condition += "AND id < ?"
			order_dir = "DESC"
		}
		params = append(params, req.LastId)
	}

	params = append(params, req.Limit)

	iter := s.cses.Query("SELECT id, post_id, owner_id, message, attachments FROM comments WHERE post_id = ? "+condition+" ORDER BY id "+order_dir+" LIMIT ?", params...).Iter()

	att := []*pb.AttachmentId{}
	tmpcomment := &pb.Comment{}
	for iter.Scan(&tmpcomment.Id, &tmpcomment.PostId, &tmpcomment.OwnerId, &tmpcomment.Message, &att) {
		attach, err := s.storagecli.GetAttachments(ctx, &pb.GetAttachmentsRequest{Ids: att})
		if err != nil {
			if status.Code(err) == codes.Unavailable {
				return nil, ErrServiceStorageUnvaliable
			}
			return nil, err
		}
		tmpcomment.Attachments = attach.Attachments

		sid := snowflake.ParseID(tmpcomment.Id)
		tmpcomment.Time = timestamppb.New(sid.GenerateTime().Local())

		res.Comments = append(res.Comments, tmpcomment)

		att = []*pb.AttachmentId{}
		tmpcomment = &pb.Comment{}
	}

	err := iter.Close()
	if err != nil {
		return nil, err
	}

	if req.Extended && len(res.Comments) > 0 {

		ids := make([]int64, 0, len(res.Comments))
		for i := range res.Comments {
			ids = append(ids, res.Comments[i].OwnerId)
		}

		usersres, err := s.userscli.GetUsersByIds(ctx, &pb.GetUsersByIdsRequest{Ids: ids, Fields: req.Fields})
		if err != nil {
			if status.Code(err) == codes.Unavailable {
				return nil, ErrServiceUsersUnvaliable
			}
			return nil, err
		}

		if len(usersres.Users) == len(res.Comments) {
			for i := range res.Comments {
				res.Comments[i].Owner = usersres.Users[i]
			}
		}

	}

	return res, nil
}

func (s service) UpdatePost(ctx context.Context, req *pb.UpdatePostRequest) (*pb.UpdatePostResponse, error) {

	/*user_id, err := getAuthUser(ctx)
	if err != nil {
		return nil, err
	}

	res := &pb.UpdatePostResponse{Post: &pb.Post{
		OwnerId: user_id,
	}}

	if req.Attachments.Update {
		att, err := s.storagecli.GetAttachments(ctx, &pb.GetAttachmentsRequest{Id: req.Attachments.Value})
		if err != nil {
			if status.Code(err) == codes.Unavailable {
				return nil, ErrServiceStorageUnvaliable
			} else if status.Code(err) == codes.Internal {
				return nil, err
			}
			return nil, ErrInvalidAttachments
		}
		res.Post.Attachments = att.Attachments
	}

	if req.Message.Update {

	}*/

	return nil, nil
}
