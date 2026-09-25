package storage

import "testing"

func TestS3ReferenceRejectsOtherBuckets(t *testing.T) {
	backend := &S3{bucket: "private", prefix: "payloads"}
	key, err := backend.parseReference("s3://private/payloads/item.bin")
	if err != nil || key != "payloads/item.bin" {
		t.Fatalf("parse reference = %q, %v", key, err)
	}
	if _, err := backend.parseReference("s3://other/payloads/item.bin"); err == nil {
		t.Fatal("reference from another bucket was accepted")
	}
}
