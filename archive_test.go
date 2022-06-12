package main

import (
	"os"
	"testing"
)

func init() {
	loadDotEnv()
}

func TestS3List(t *testing.T) {
	s3, err := NewS3Client()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s3.client.ListBuckets(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, bucket := range resp.Buckets {
		t.Log(*bucket.Name)
	}
}

func TestS3(t *testing.T) {
	const gate = "ICBM_Run_Integration_Tests"
	if os.Getenv(gate) == "" {
		t.Skip("set .env variable", gate, "to run this test")
	}

	s3, err := NewS3Client()
	if err != nil {
		t.Fatal(err)
	}
	key := tmpFilename("testS3-")
	err = s3.Put(key, []byte("test"))
	if err != nil {
		t.Fatal(key, err)
	}
	data, err := s3.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "test" {
		t.Fatal(key, "data mismatch")
	}
	err = s3.Delete(key)
	if err != nil {
		t.Fatal(key, err)
	}
}
