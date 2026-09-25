package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3 struct {
	client *minio.Client
	bucket string
	prefix string
}

func NewS3(endpoint, accessKey, secretKey, token, bucket, prefix string, secure bool) (*S3, error) {
	endpoint = strings.TrimSpace(endpoint)
	bucket = strings.Trim(strings.TrimSpace(bucket), "/")
	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		return nil, fmt.Errorf("S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY, and S3_BUCKET are required")
	}
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, token), Secure: secure})
	if err != nil {
		return nil, err
	}
	return &S3{client: client, bucket: bucket, prefix: strings.Trim(strings.TrimSpace(prefix), "/")}, nil
}

func (s *S3) objectKey(id string) string {
	if s.prefix == "" {
		return id + ".bin"
	}
	return s.prefix + "/" + id + ".bin"
}

func (s *S3) reference(key string) string { return "s3://" + s.bucket + "/" + key }

func (s *S3) parseReference(reference string) (string, error) {
	u, err := url.Parse(reference)
	if err != nil || u.Scheme != "s3" || u.Host != s.bucket || u.Path == "" {
		return "", fmt.Errorf("invalid S3 storage reference")
	}
	return strings.TrimPrefix(u.Path, "/"), nil
}

func (s *S3) Write(id string, data []byte) (string, error) {
	_, err := s.client.PutObject(context.Background(), s.bucket, s.objectKey(id), strings.NewReader(string(data)), int64(len(data)), minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return "", err
	}
	return s.reference(s.objectKey(id)), nil
}

func (s *S3) WriteStream(id string, write func(io.Writer) error) (string, error) {
	reader, writer := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		err := write(writer)
		if err != nil {
			_ = writer.CloseWithError(err)
		} else {
			_ = writer.Close()
		}
		errCh <- err
	}()
	_, putErr := s.client.PutObject(context.Background(), s.bucket, s.objectKey(id), reader, -1, minio.PutObjectOptions{ContentType: "application/octet-stream", PartSize: 64 * 1024 * 1024, ConcurrentStreamParts: true})
	writeErr := <-errCh
	if putErr != nil {
		return "", putErr
	}
	if writeErr != nil {
		return "", writeErr
	}
	return s.reference(s.objectKey(id)), nil
}

func (s *S3) Open(reference string) (io.ReadCloser, error) {
	key, err := s.parseReference(reference)
	if err != nil {
		return nil, err
	}
	return s.client.GetObject(context.Background(), s.bucket, key, minio.GetObjectOptions{})
}

func (s *S3) Delete(reference string) {
	key, err := s.parseReference(reference)
	if err == nil {
		_ = s.client.RemoveObject(context.Background(), s.bucket, key, minio.RemoveObjectOptions{})
	}
}

func (s *S3) CleanupOlderThan(age time.Duration) error {
	cutoff := time.Now().Add(-age)
	for object := range s.client.ListObjects(context.Background(), s.bucket, minio.ListObjectsOptions{Prefix: s.prefix, Recursive: true}) {
		if object.Err != nil {
			return object.Err
		}
		if object.LastModified.Before(cutoff) {
			if err := s.client.RemoveObject(context.Background(), s.bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *S3) CleanupOrphans(active map[string]struct{}) error {
	for object := range s.client.ListObjects(context.Background(), s.bucket, minio.ListObjectsOptions{Prefix: s.prefix, Recursive: true}) {
		if object.Err != nil {
			return object.Err
		}
		if _, ok := active[s.reference(object.Key)]; !ok {
			if err := s.client.RemoveObject(context.Background(), s.bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}
