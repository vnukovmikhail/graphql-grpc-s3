package grpc

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	filev1 "github.com/vnukovmikhail/graphql-grpc-s3/v2/pkg/pb/file/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	filev1.UnimplementedFileServiceServer
	storageDir string
}

func New(storageDir string) (*Server, error) {
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		return nil, err
	}
	return &Server{storageDir: storageDir}, nil
}

func (s *Server) UploadFile(stream filev1.FileService_UploadFileServer) error {
	var (
		file  *os.File
		path  string
		id    string
		total int64
	)
	cleanup := func() {
		if file != nil {
			file.Close()
			os.Remove(path)
		}
	}

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			if file == nil {
				return status.Error(codes.InvalidArgument, "no metadata")
			}
			if err := file.Close(); err != nil {
				return err
			}
			return stream.SendAndClose(&filev1.UploadFileResponse{
				FileId: id, Success: true, Message: "uploaded", BytesUploaded: total,
			})
		}
		if err != nil {
			cleanup()
			return err
		}

		switch x := req.Data.(type) {
		case *filev1.UploadFileRequest_Metadata:
			if file != nil {
				cleanup()
				return status.Error(codes.InvalidArgument, "metadata sent twice")
			}
			id = uuid.NewString()
			path = filepath.Join(s.storageDir, id)
			file, err = os.Create(path)
			if err != nil {
				return err
			}
		case *filev1.UploadFileRequest_ChunkData:
			if file == nil {
				return status.Error(codes.InvalidArgument, "chunk before metadata")
			}
			n, err := file.Write(x.ChunkData)
			if err != nil {
				cleanup()
				return err
			}
			total += int64(n)
		}
	}
}

func (s *Server) DownloadFile(req *filev1.FileDownloadRequest, stream filev1.FileService_DownloadFileServer) error {
	if _, err := uuid.Parse(req.FileId); err != nil {
		return status.Error(codes.InvalidArgument, "invalid file id")
	}
	f, err := os.Open(filepath.Join(s.storageDir, req.FileId))
	if errors.Is(err, os.ErrNotExist) {
		return status.Error(codes.NotFound, "file not found")
	}
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 64*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if sendErr := stream.Send(&filev1.FileDownloadResponse{ChunkData: buf[:n]}); sendErr != nil {
				return sendErr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
