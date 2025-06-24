package s3_storage_service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"

	flag "github.com/spf13/pflag"
)

type S3Uploader interface {
	Upload(ctx context.Context, input *s3.PutObjectInput, opts ...func(*manager.Uploader)) (*manager.UploadOutput, error)
}

type S3Downloader interface {
	Download(ctx context.Context, w io.WriterAt, input *s3.GetObjectInput, options ...func(*manager.Downloader)) (n int64, err error)
}

type S3StorageServiceConfig struct {
	Enable              bool   `mapstructure:"Enable"`
	AccessKey           string `mapstructure:"AccessKey"`
	Bucket              string `mapstructure:"Bucket"`
	ObjectPrefix        string `mapstructure:"ObjectPrefix"`
	Region              string `mapstructure:"Region"`
	SecretKey           string `mapstructure:"SecretKey"`
	DiscardAfterTimeout bool   `mapstructure:"DiscardAfterTimeout"`
}

var DefaultS3StorageServiceConfig = S3StorageServiceConfig{
	Enable: false,
}

func S3ConfigAddOptions(prefix string, f *flag.FlagSet) {
	f.Bool(prefix+".Enable", DefaultS3StorageServiceConfig.Enable, "enable storage/retrieval of sequencer batch data from an AWS S3 bucket")
	f.String(prefix+".AccessKey", DefaultS3StorageServiceConfig.AccessKey, "S3 access key")
	f.String(prefix+".Bucket", DefaultS3StorageServiceConfig.Bucket, "S3 bucket")
	f.String(prefix+".ObjectPrefix", DefaultS3StorageServiceConfig.ObjectPrefix, "prefix to add to S3 objects")
	f.String(prefix+".Region", DefaultS3StorageServiceConfig.Region, "S3 region")
	f.String(prefix+".SecretKey", DefaultS3StorageServiceConfig.SecretKey, "S3 secret key")
	f.Bool(prefix+".DiscardAfterTimeout", DefaultS3StorageServiceConfig.DiscardAfterTimeout, "discard data after its expiry timeout")
}

type S3StorageService struct {
	client              *s3.Client
	bucket              string
	objectPrefix        string
	uploader            S3Uploader
	downloader          S3Downloader
	discardAfterTimeout bool
}

func NewS3StorageService(config S3StorageServiceConfig) (*S3StorageService, error) {
	client, err := buildS3Client(config.AccessKey, config.SecretKey, config.Region)
	if err != nil {
		return nil, err
	}
	return &S3StorageService{
		client:              client,
		bucket:              config.Bucket,
		objectPrefix:        config.ObjectPrefix,
		uploader:            manager.NewUploader(client),
		downloader:          manager.NewDownloader(client),
		discardAfterTimeout: config.DiscardAfterTimeout,
	}, nil
}

func buildS3Client(accessKey, secretKey, region string) (*s3.Client, error) {
	cfg, err := awsConfig.LoadDefaultConfig(context.TODO(), awsConfig.WithRegion(region), func(options *awsConfig.LoadOptions) error {
		// remain backward compatible with accessKey and secretKey credentials provided via cli flags
		if accessKey != "" && secretKey != "" {
			options.Credentials = credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg), nil
}

func (s3s *S3StorageService) GetByHash(ctx context.Context, key common.Hash) ([]byte, error) {
	log.Trace("avail.S3StorageService.GetByHash", "key", prettyHash(key), "this", s3s)

	buf := manager.NewWriteAtBuffer([]byte{})
	_, err := s3s.downloader.Download(ctx, buf, &s3.GetObjectInput{
		Bucket: aws.String(s3s.bucket),
		Key:    aws.String(s3s.objectPrefix + EncodeStorageServiceKey(key)),
	})
	return buf.Bytes(), err
}

func (s3s *S3StorageService) Put(ctx context.Context, value []byte, timeout uint64, commitment common.Hash) error {
	logPut("avail.S3StorageService.Store", value, timeout, s3s)
	putObjectInput := s3.PutObjectInput{
		Bucket: aws.String(s3s.bucket),
		Key:    aws.String(s3s.objectPrefix + EncodeStorageServiceKey(commitment)),
		Body:   bytes.NewReader(value)}
	if s3s.discardAfterTimeout && timeout <= math.MaxInt64 {
		// #nosec G115
		expires := time.Unix(int64(timeout), 0)
		putObjectInput.Expires = &expires
	}
	_, err := s3s.uploader.Upload(ctx, &putObjectInput)
	if err != nil {
		log.Error("avail.S3StorageService.Store", "err", err)
	}
	return err
}

func (s3s *S3StorageService) Sync(ctx context.Context) error {
	return nil
}

func (s3s *S3StorageService) Close(ctx context.Context) error {
	return nil
}

func (s3s *S3StorageService) String() string {
	return fmt.Sprintf("S3StorageService(:%s)", s3s.bucket)
}

func (s3s *S3StorageService) HealthCheck(ctx context.Context) error {
	_, err := s3s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s3s.bucket)})
	return err
}

func EncodeStorageServiceKey(key common.Hash) string {
	return key.Hex()[2:]
}

func logPut(store string, data []byte, timeout uint64, reader *S3StorageService, more ...interface{}) {
	if len(more) == 0 {
		// #nosec G115
		log.Trace(
			store, "message", firstFewBytes(data), "timeout", time.Unix(int64(timeout), 0),
			"this", reader,
		)
	} else {
		// #nosec G115
		log.Trace(
			store, "message", firstFewBytes(data), "timeout", time.Unix(int64(timeout), 0),
			"this", reader, more,
		)
	}
}

func prettyHash(hash common.Hash) string {
	return firstFewBytes(hash.Bytes())
}

func firstFewBytes(b []byte) string {
	if len(b) < 9 {
		return fmt.Sprintf("[% x]", b)
	} else {
		return fmt.Sprintf("[% x ... ]", b[:8])
	}
}
