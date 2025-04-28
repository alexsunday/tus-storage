package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/tus/tusd/v2/pkg/filestore"
	tusd "github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/memorylocker"
	"github.com/tus/tusd/v2/pkg/s3store"
)

var (
	logger   = slog.New(slog.NewTextHandler(os.Stderr, nil))
	redisUrl = flag.String("redis", "redis://localhost:6379/0", "redis url")
	minioUrl = flag.String("minio", "http://localhost:9000", "minio url")
	secret   = flag.String("secret", "", "secret key")
)

func loadStore(minioUrl string) (*s3store.S3Store, error) {
	var ctx = context.Background()
	s3Config, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to load config %w", err)
	}

	var s3Client = s3.NewFromConfig(s3Config, func(o *s3.Options) {
		var endpoint = minioUrl
		o.BaseEndpoint = &endpoint
		o.UsePathStyle = true
	})

	// Create a new FileStore instance which is responsible for
	// storing the uploaded file on disk in the specified directory.
	// This path _must_ exist before tusd will store uploads in it.
	// If you want to save them on a different medium, for example
	// a remote FTP server, you can implement your own storage backend
	// by implementing the tusd.DataStore interface.

	store := s3store.New("attachment", s3Client)
	return &store, nil
}

func loadDebugStore() (*filestore.FileStore, error) {
	store := filestore.New("./attachments")
	return &store, nil
}

func main() {
	flag.Parse()
	if *redisUrl == "" {
		logger.Error("redis url is required")
		return
	}
	if *secret == "" {
		logger.Error("secret key is required")
		return
	}
	if *minioUrl == "" {
		logger.Error("minio url is required")
		return
	}

	store, err := loadStore(*minioUrl)
	// store, err := loadDebugStore()
	if err != nil {
		logger.Error("unable to load store", "error", err)
		return
	}

	// A locking mechanism helps preventing data loss or corruption from
	// parallel requests to a upload resource. A good match for the disk-based
	// storage is the filelocker package which uses disk-based file lock for
	// coordinating access.
	// More information is available at https://tus.github.io/tusd/advanced-topics/locks/.
	locker := memorylocker.New()

	// A storage backend for tusd may consist of multiple different parts which
	// handle upload creation, locking, termination and so on. The composer is a
	// place where all those separated pieces are joined together. In this example
	// we only use the file store but you may plug in multiple.
	composer := tusd.NewStoreComposer()
	store.UseIn(composer)
	locker.UseIn(composer)

	// Create a new HTTP handler for the tusd server by providing a configuration.
	// The StoreComposer property must be set to allow the handler to function.
	handler, err := tusd.NewHandler(tusd.Config{
		BasePath:              "/attachments/",
		StoreComposer:         composer,
		NotifyCompleteUploads: true,
	})
	if err != nil {
		log.Fatalf("unable to create handler: %s", err)
	}

	mapper, err := NewFileIdMapRedisImpl(*redisUrl)
	if err != nil {
		logger.Error("unable to create file id map", "error", err)
		return
	}

	// Start another goroutine for receiving events from the handler whenever
	// an upload is completed. The event will contains details about the upload
	// itself and the relevant HTTP request.
	go func() {
		for {
			event := <-handler.CompleteUploads
			log.Printf("Upload %s finished\n", event.Upload.ID)
			onUploadComplete(event, mapper)
		}
	}()

	authSecret, err := base64.StdEncoding.DecodeString(*secret)
	if err != nil {
		logger.Error("unable to decode secret key", "error", err)
		return
	}
	var authenticator = NewAuth(authSecret)

	o1 := http.StripPrefix("/attachments/", AuthCheck(authenticator, handler))
	o2 := http.StripPrefix("/attachments", AuthCheck(authenticator, handler))

	// Right now, nothing has happened since we need to start the HTTP server on
	// our own. In the end, tusd will start listening on and accept request at
	// http://localhost:8080/files
	http.Handle("/attachments/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixFileNamePath(w, r, o1, mapper)
	}))
	http.Handle("/attachments", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixFileNamePath(w, r, o2, mapper)
	}))
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatalf("unable to listen: %s", err)
	}
}

func onUploadComplete(event tusd.HookEvent, mapper FileIdMap) {
	// 1. Get the file name from the event
	fileName, ok := event.Upload.MetaData["filename"]
	if !ok {
		logger.Error("unable to get file name from event")
		return
	}

	// 2. Get the file id from the event
	fileId := event.Upload.ID
	// 3. Save the file name and file id to redis
	err := mapper.SetFileIdMap(fileName, fileId)
	if err != nil {
		logger.Error("unable to set file id map", "error", err)
		return
	}
	logger.Info("file id map saved", "fileName", fileName, "fileId", fileId)
}

// 将 path 里的 ID 替换为 FileName
func fixFileNamePath(w http.ResponseWriter, r *http.Request, origin http.Handler, mapper FileIdMap) {
	if r.Method != "GET" {
		origin.ServeHTTP(w, r)
		return
	}
	logger.Info("fixFileNamePath", "path", r.URL.Path)
	fName := r.URL.Path[len("/attachments/"):]
	fileId, err := mapper.GetFileId(fName)
	if err != nil {
		logger.Error("unable to get file id", "error", err)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("file not found"))
		return
	}
	logger.Info("file id", "fileId", fileId)

	r2 := new(http.Request)
	*r2 = *r
	r2.URL = new(url.URL)
	*r2.URL = *r.URL
	r2.URL.Path = "/attachments/" + fileId
	r2.URL.RawPath = "/attachments/" + fileId
	r.RequestURI = r2.URL.Path

	origin.ServeHTTP(w, r2)
}

func AuthCheck(authenticator Auth, inner http.Handler) http.Handler {
	return &authHandler{authenticator, inner}
}

type authHandler struct {
	authenticator Auth
	inner         http.Handler
}

func (a *authHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// GET 请求不需要认证
	if r.Method == "GET" {
		a.inner.ServeHTTP(w, r)
		return
	}
	// 其他请求需要认证
	user, pass, _ := r.BasicAuth()
	if err := a.authenticator.Check(user, pass); err != nil {
		logger.Warn("basic auth check failed", "error", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	a.inner.ServeHTTP(w, r)
}
