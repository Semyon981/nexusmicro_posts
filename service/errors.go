package service

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrLimitError = status.Error(codes.OutOfRange, "limit must be in the range [0;100]")

	ErrInvalidPhone = status.Error(codes.InvalidArgument, "invalid phone")

	ErrInvalidMetadata = status.Error(codes.InvalidArgument, "invalid metadata")

	ErrEmptyContent = status.Error(codes.InvalidArgument, "content is empty")

	ErrInvalidAttachments = status.Error(codes.InvalidArgument, "invalid attachments")

	ErrServiceStorageUnvaliable = status.Error(codes.Internal, "service storage unvaliable")

	ErrServiceUsersUnvaliable = status.Error(codes.Internal, "service users unvaliable")

	ErrPostNotFound = status.Error(codes.NotFound, "post not found")

	ErrInvalidAccessToken = status.Error(codes.Unauthenticated, "invalid access token")

	ErrUnknownSubject = status.Error(codes.Unauthenticated, "unknown subject")

	ErrServiceLinkedaccUnvaliable = status.Error(codes.Unavailable, "service linkedacc unvaliable")
)

func ErrInternal(err error) error {
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	} else {
		return status.Error(codes.Internal, "unknown internal error")
	}
}
