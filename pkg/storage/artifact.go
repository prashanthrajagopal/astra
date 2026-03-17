package storage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

// UploadCodegenJSON uploads JSON bytes to gs://bucket/key when bucket is non-empty.
// Uses GOOGLE_APPLICATION_CREDENTIALS or workload identity. Returns gs URI or empty if bucket unset.
func UploadCodegenJSON(ctx context.Context, bucket, agentID, taskID string, body []byte) (string, error) {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return "", nil
	}
	key := fmt.Sprintf("codegen/%s/%s/%d.json", agentID, taskID, time.Now().UnixNano())
	client, err := storage.NewClient(ctx, option.WithScopes(storage.ScopeReadWrite))
	if err != nil {
		return "", err
	}
	defer client.Close()
	w := client.Bucket(bucket).Object(key).NewWriter(ctx)
	w.ContentType = "application/json"
	if _, err := w.Write(body); err != nil {
		_ = w.Close()
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return "gs://" + bucket + "/" + key, nil
}

// WriteLocalArtifact writes to ARTIFACT_DIR when GCS_BUCKET is empty (dev fallback).
func WriteLocalArtifact(agentID, taskID string, body []byte) (string, error) {
	dir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if dir == "" {
		return "", nil
	}
	sub := fmt.Sprintf("%s/%s", agentID, taskID)
	path := dir + "/" + sub + "/result.json"
	if err := os.MkdirAll(dir+"/"+sub, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", err
	}
	return "file://" + path, nil
}
