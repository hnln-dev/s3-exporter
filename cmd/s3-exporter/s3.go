package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

func newClient(cfg *aws.Config) *s3.S3 {
	sess := session.Must(session.NewSession(cfg))

	return s3.New(sess)
}

func listBuckets(svc *s3.S3) ([]string, error) {
	bucketList, err := svc.ListBuckets(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}

	if len(bucketList.Buckets) == 0 {
		return nil, fmt.Errorf("no buckets found")
	}

	var buckets []string
	for _, b := range bucketList.Buckets {
		buckets = append(buckets, *b.Name)
	}

	return buckets, nil
}

func listObjects(svc *s3.S3, bucketName *string, prefix *string, continuationToken *string) (*s3.ListObjectsV2Output, error) {
	input := &s3.ListObjectsV2Input{
		Bucket:            bucketName,
		Prefix:            prefix,
		ContinuationToken: continuationToken,
	}

	return svc.ListObjectsV2(input)
}
