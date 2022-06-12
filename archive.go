package main

// Archive management. Save and restore local data to an S3 compatible service.
// Bundle individual ICBM reports into dailies.

// todo: get rid of disk entirely, use S3
// sync by hand for setup to b2 bucket
// Bucket contents:
// - copies of all received reports
// 		https://<bucket>.<endpoint>/data/<fridge>/archive/yyyymmddhhmmss.json.gz
// - daily rollups
// 		https://<bucket>.<endpoint>/data/<fridge>/day/yyyymmdd.json.gz

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

const ( // todo: move to .env and set in CI / fly
	s3Endpoint = "https://s3.us-west-002.backblazeb2.com"
	s3Region   = "us-west-002"
	s3Bucket   = "lunarville-icbm"
)

type Archive struct {
	client *s3.S3
}

var s3client *Archive

func init() {
	var err error
	s3client, err = NewS3Client()
	if err != nil {
		log.Println("Could not initialize S3 client:", err)
	}
}

func NewS3Client() (ar *Archive, err error) {
	key, secret := os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY")
	if key == "" || secret == "" {
		err = fmt.Errorf("no S3 creds found in environment")
		return
	}
	s3Config := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(key, secret, ""),
		Endpoint:         aws.String(s3Endpoint),
		Region:           aws.String(s3Region),
		S3ForcePathStyle: aws.Bool(true),
	}
	newSession, err := session.NewSession(s3Config)
	if err != nil {
		return
	}
	ar = &Archive{client: s3.New(newSession)}
	return
}

func (ar *Archive) List(prefix string) ([]string, error) {
	var keys []string
	params := &s3.ListObjectsInput{
		Bucket:    aws.String(s3Bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	}
	resp, err := ar.client.ListObjects(params)
	if err == nil {
		for _, key := range resp.Contents {
			keys = append(keys, *key.Key)
		}
	}
	return keys, err
}

func (ar *Archive) Put(key string, data []byte) error {
	if ar == nil {
		return fmt.Errorf("storage not initialized")
	}
	_, err := ar.client.PutObject(&s3.PutObjectInput{
		Body:   bytes.NewReader(data),
		Bucket: aws.String(s3Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to upload: %s/%s: %w", s3Bucket, key, err)
	}
	return nil
}

func (ar *Archive) Get(key string) (data []byte, err error) {
	obj, err := ar.client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s3Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		fmt.Println("failed to download file", err)
		return
	}
	defer obj.Body.Close()
	return ioutil.ReadAll(obj.Body)
}

func (ar *Archive) Delete(key string) error {
	_, err := ar.client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(s3Bucket),
		Key:    aws.String(key),
	})
	return err
}
