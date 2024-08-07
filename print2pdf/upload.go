package print2pdf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

// S3 bucket name. Required.
var BucketName = os.Getenv("BUCKET")

// Reference to AWS S3 client, initialized in init function of this package.
var s3Client *s3.Client

// Initialize AWS SDK.
func init() {
	if BucketName == "" {
		fmt.Fprintln(os.Stderr, "missing required environment variable BUCKET")
		os.Exit(1)
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading AWS SDK configuration: %v", err)
		os.Exit(1)
	}

	s3Client = s3.NewFromConfig(cfg)
}

// Upload a file to S3. A random UUIDv4 prefix is added to the filename and used as key in the bucket, to avoid collisions.
func UploadFile(ctx context.Context, fileName string, contents *[]byte) (string, error) {
	uuidv4, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("error generating UUIDv4: %s", err)
	}
	if !strings.HasSuffix(fileName, ".pdf") {
		fileName = fileName + ".pdf"
	}

	key := fmt.Sprintf("%s/%s", uuidv4, fileName)
	dest := fmt.Sprintf("https://%s.s3.dualstack.%s.amazonaws.com/%s", BucketName, s3Client.Options().Region, key)
	defer Elapsed(fmt.Sprintf("Upload PDF of size %s to %s", HumanizeBytes(uint64(len(*contents))), dest))()
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:             &BucketName,
		Key:                &key,
		Body:               bytes.NewReader(*contents),
		ContentDisposition: Ptr("attachment"),
		ContentType:        Ptr("application/pdf"),
	})
	if err != nil {
		return "", fmt.Errorf("error uploading file: %s", err)
	}

	return dest, nil
}
