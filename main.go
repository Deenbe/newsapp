package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type options struct {
	Bucket string
	Host   string
	Port   int
}

type Post struct {
	Caption string
	Image   string
}

type postSvc struct {
	Bucket   *string
	Uploader *manager.Uploader
	Client   *s3.Client
}

func (s *postSvc) CreateNew(ctx context.Context, caption string, image io.Reader, filename string) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	key := fmt.Sprintf("%s/%s", now.Format("2006-01-02"), id)

	if len(caption) > 256 {
		return "", fmt.Errorf("caption cannot be longer than 256 characters: %d", len(caption))
	}

	encoding := base64.StdEncoding.WithPadding(base64.NoPadding)
	post := encoding.EncodeToString([]byte(caption))
	err = s.saveImage(ctx, aws.String(fmt.Sprintf("%s/%s/image%s", key, post, path.Ext(filename))), image)
	if err != nil {
		return "", err
	}
	return key, nil
}

func (s *postSvc) List(ctx context.Context) ([]*Post, error) {
	_, err := s.Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: s.Bucket,
	})
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (s *postSvc) saveImage(ctx context.Context, key *string, image io.Reader) error {
	_, err := s.Uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: s.Bucket,
		Key:    key,
		Body:   image,
	})
	return err
}

var opts options = options{
	Port: 8000,
}

func init() {
	flag.StringVar(&opts.Bucket, "bucket", "", "s3 bucket name")
	flag.StringVar(&opts.Host, "host", "", "host interface to bind")
	flag.IntVar(&opts.Port, "port", 8080, "host port to bind")
}

type services struct {
	post *postSvc
}

func configureRoutes(services *services) http.Handler {
	r := mux.NewRouter()

	r.Path("/healthcheck").Methods("GET").HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		json.NewEncoder(res).Encode(map[string]string{"status": "healthy"})
	})

	r.Path("/v1/list").Methods("POST").HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		svc := services.post

		f, header, err := req.FormFile("image")
		if err != nil {
			res.WriteHeader(500)
			log.Printf("%v\n", err)
			return
		}
		defer f.Close()

		c := req.FormValue("caption")

		id, err := svc.CreateNew(req.Context(), c, f, header.Filename)
		if err != nil {
			res.WriteHeader(500)
			log.Printf("%v\n", err)
			return
		}
		json.NewEncoder(res).Encode(map[string]interface{}{"id": id})
	})

	return r
}

func initServices(opts *options) *services {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	s3Client := s3.NewFromConfig(cfg)

	svc := &postSvc{
		Bucket:   aws.String(opts.Bucket),
		Uploader: manager.NewUploader(s3Client),
	}

	return &services{
		post: svc,
	}
}

func startServer(opts *options, handler http.Handler) error {
	addr := fmt.Sprintf("%s:%d", opts.Host, opts.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	log.Printf("service started: %s", addr)
	return server.ListenAndServe()
}

func validateOptions() *options {
	if opts.Bucket == "" {
		name, ok := os.LookupEnv("BUCKET_NAME")
		if !ok {
			panic("bucket name is required")
		}
		opts.Bucket = name
	}

	return &opts
}

func main() {
	flag.Parse()
	log.SetFlags(log.Llongfile)
	handler := configureRoutes(initServices(validateOptions()))
	startServer(&opts, handler)
}
