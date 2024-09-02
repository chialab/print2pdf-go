package print2pdf

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

// PDFHandler is an interface implementing methods to handle saving a file to a storage location.
type PDFHandler interface {
	// Include io.Closer interface, to accomodate implementations that need it.
	io.Closer
	// Handle writes the input stream to the implemented storage location, returning a URI (local path, URL, ...) or an error if any occur.
	Handle(io.Reader) (string, error)
}

// FileHandler handles saving a file in a local path.
type FileHandler struct {
	handle *os.File
}

// NewFileHandler returns a new instance of FileHandler.
func NewFileHandler(path string) (FileHandler, error) {
	f, err := os.Create(path)
	if err != nil {
		return FileHandler{}, err
	}

	return FileHandler{f}, nil
}

// Implement io.Closer interface.
func (fh FileHandler) Close() error {
	return fh.handle.Close()
}

// Implement PDFHandler interface.
func (fh FileHandler) Handle(r io.Reader) (string, error) {
	buf := make([]byte, 1024*1024)
	_, err := io.CopyBuffer(fh.handle, r, buf)
	if err != nil {
		return "", err
	}

	return fh.handle.Name(), nil
}

// S3Handler handles uploading a file to an AWS S3 bucket.
type S3Handler struct {
	ctx      context.Context
	client   *s3.Client
	bucket   string
	fileName string
}

// NewS3Handler returns a new instance of S3Uploader.
func NewS3Handler(ctx context.Context, bucket, fileName string) (S3Handler, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return S3Handler{}, fmt.Errorf("error loading AWS SDK configuration: %v", err)
	}

	return S3Handler{ctx, s3.NewFromConfig(cfg), bucket, fileName}, nil
}

// Implement io.Closer interface (noop).
func (sh S3Handler) Close() error {
	return nil
}

// Implement PDFHandler interface.
func (sh S3Handler) Handle(r io.Reader) (string, error) {
	uuidv4, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("error generating UUIDv4: %s", err)
	}

	key := fmt.Sprintf("%s/%s", uuidv4, sh.fileName)

	uploader := manager.NewUploader(sh.client)
	res, err := uploader.Upload(sh.ctx, &s3.PutObjectInput{
		Bucket:             &sh.bucket,
		Key:                &key,
		Body:               r,
		ContentDisposition: Ptr("attachment"),
		ContentType:        Ptr("application/pdf"),
	})
	if err != nil {
		return "", fmt.Errorf("error uploading file: %s", err)
	}

	return res.Location, nil
}

// StreamHandler handles streaming a file.
type StreamHandler struct {
	writer io.Writer
}

// NewStreamHandler returns a new instance of StreamHandler, which will stream the file to the provided writer.
func NewStreamHandler(w io.Writer) StreamHandler {
	return StreamHandler{w}
}

// Implement io.Closer interface (noop).
func (sh StreamHandler) Close() error {
	return nil
}

// Implement PDFHandler interface.
func (sh StreamHandler) Handle(r io.Reader) (string, error) {
	buf := make([]byte, 1024*1024)
	_, err := io.CopyBuffer(sh.writer, r, buf)
	if err != nil {
		return "", err
	}

	return "", nil
}
