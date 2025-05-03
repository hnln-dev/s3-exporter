package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func Probe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for k := range r.URL.Query() {
			if !slices.Contains([]string{
				"endpoint", "bucket", "prefix", "delimiter", "region", "forcePathStyle", "depth",
			}, k) {
				http.Error(w, "Invalid query parameter: "+k, http.StatusBadRequest)
				return
			}
		}

		endpoint := r.URL.Query().Get("endpoint")
		bucketName := r.URL.Query().Get("bucket")
		prefix := r.URL.Query().Get("prefix")
		delimiter := r.URL.Query().Get("delimiter")
		region := r.URL.Query().Get("region")

		forcePathStyle := false
		var err error

		if r.URL.Query().Get("forcePathStyle") != "" {
			forcePathStyle, err = strconv.ParseBool(r.URL.Query().Get("forcePathStyle"))
			if err != nil {
				http.Error(w, "Invalid forcePathStyle parameter: "+err.Error(), http.StatusBadRequest)
				return
			}
		}

		depth := 0

		if r.URL.Query().Get("depth") != "" {
			depth, err = strconv.Atoi(r.URL.Query().Get("depth"))
			if err != nil {
				http.Error(w, "Invalid depth parameter", http.StatusBadRequest)
				return
			}
		}

		accessKey, secretKey, useAuth := r.BasicAuth()

		cfg := &aws.Config{
			Region:           &region,
			S3ForcePathStyle: &forcePathStyle,
			Endpoint:         &endpoint,
		}

		if useAuth {
			cfg.Credentials = credentials.NewStaticCredentials(accessKey, secretKey, "")
		} else {
			cfg.Credentials = credentials.AnonymousCredentials
		}

		s3 := newClient(cfg)

		var buckets []string

		if len(bucketName) > 0 {
			buckets = []string{bucketName}
		} else {
			bucketList, err := s3.ListBuckets(nil)
			if err != nil {
				http.Error(w, "Error listing buckets: "+err.Error(), http.StatusInternalServerError)
				return
			}

			if len(bucketList.Buckets) == 0 {
				http.Error(w, "No buckets found", http.StatusNotFound)
				return
			}

			for _, b := range bucketList.Buckets {
				buckets = append(buckets, *b.Name)
			}
		}

		var (
			s3ObjectCount = prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: "s3_object_count",
					Help: "Total number of objects in S3 bucket",
				},
				[]string{"bucket", "prefix"},
			)
			s3ObjectSize = prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: "s3_object_size_sum_bytes",
					Help: "Total size of objects in S3 bucket",
				},
				[]string{"bucket", "prefix"},
			)
		)

		for _, bucket := range buckets {
			var continuationToken *string

			for {
				result, err := listObjects(s3, &bucket, &prefix, continuationToken)
				if err != nil {
					http.Error(w, "Error listing objects: "+err.Error(), http.StatusInternalServerError)
					return
				}

				for _, object := range result.Contents {
					trimmedKey := strings.TrimPrefix(*object.Key, prefix)
					prefixParts := strings.SplitN(trimmedKey, delimiter, depth+1)[:depth]

					labels := prometheus.Labels{
						"bucket": bucket,
						"prefix": prefix + strings.Join(prefixParts, delimiter),
					}

					if depth > 0 {
						labels["prefix"] = labels["prefix"] + delimiter
					}

					s3ObjectCount.With(labels).Inc()
					s3ObjectSize.With(labels).Add(float64(*object.Size))
				}

				if result.IsTruncated != nil && *result.IsTruncated && result.NextContinuationToken != nil {
					continuationToken = result.NextContinuationToken
				} else {
					break
				}
			}
		}

		reg := prometheus.NewRegistry()

		reg.MustRegister(s3ObjectCount, s3ObjectSize)

		promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(w, r)
	}
}

var Version = "devel"

func main() {
	listenAddr := flag.String("listen-address", ":9340", "Address to listen on")
	tlsCertFile := flag.String("tls.cert-file", "", "Path to TLS certificate file")
	tlsKeyFile := flag.String("tls.key-file", "", "Path to TLS key file")

	printVersion := flag.Bool("version", false, "Print version and exit")

	flag.Parse()

	if *printVersion {
		fmt.Println("s3-exporter", Version)
		os.Exit(0)
	}

	mux := http.NewServeMux()

	mux.Handle("GET /probe", Probe())
	mux.Handle("GET /metrics", promhttp.Handler())

	server := &http.Server{
		Addr:    *listenAddr,
		Handler: mux,
	}

	errs := make(chan error)

	go func() {
		defer close(errs)
		if *tlsCertFile != "" && *tlsKeyFile != "" {
			slog.Info("Starting server...", "Proto", "https", "ListenAddr", *listenAddr)
			errs <- server.ListenAndServeTLS(*tlsCertFile, *tlsKeyFile)
		} else {
			slog.Info("Starting server...", "Proto", "http", "ListenAddr", *listenAddr)
			errs <- server.ListenAndServe()
		}
	}()

	go func() {
		sigChan := make(chan os.Signal, 1)
		// os.Interrupt works on Windows too, syscall.SIGINT does not
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		slog.Info("Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		wg := sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			if err := server.Shutdown(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errs <- err
			}
		}()

		<-sigChan
		slog.Info("Killing the server...")
		cancel()
		wg.Wait()
	}()

	for err := range errs {
		if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.DeadlineExceeded) {
			slog.Error("Server ran into a problem", "Error", err)
		}
	}

}
