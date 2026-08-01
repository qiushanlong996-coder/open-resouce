package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

const objectUploadURLLifetime = 10 * time.Minute

type objectUploadRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Kind        string `json:"kind"`
}

type objectUploadAuthorization struct {
	ObjectKey string            `json:"object_key"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

type objectStorage interface {
	PresignUpload(context.Context, string, string, int64) (objectUploadAuthorization, error)
	PresignDownload(context.Context, string) (string, error)
	// GetObject 读取对象字节，最多读取 limit 字节；超过 limit 时返回 errObjectTooLarge。
	GetObject(ctx context.Context, objectKey string, limit int64) ([]byte, error)
}

var errObjectTooLarge = errors.New("object exceeds size limit")

func (storage *aliyunObjectStorage) GetObject(
	ctx context.Context, objectKey string, limit int64,
) ([]byte, error) {
	result, err := storage.client.GetObject(ctx, &oss.GetObjectRequest{
		Bucket: oss.Ptr(storage.bucket), Key: oss.Ptr(objectKey),
	})
	if err != nil {
		return nil, fmt.Errorf("get OSS object: %w", err)
	}
	defer result.Body.Close()
	if result.ContentLength > limit {
		return nil, errObjectTooLarge
	}
	// 多读一字节用于识别 Content-Length 缺失或不准时的超限情况。
	body, err := io.ReadAll(io.LimitReader(result.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read OSS object: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, errObjectTooLarge
	}
	return body, nil
}

func (storage *aliyunObjectStorage) PresignDownload(ctx context.Context, objectKey string) (string, error) {
	result, err := storage.client.Presign(ctx, &oss.GetObjectRequest{
		Bucket: oss.Ptr(storage.bucket), Key: oss.Ptr(objectKey),
	}, oss.PresignExpires(10*time.Minute))
	if err != nil {
		return "", fmt.Errorf("presign OSS download: %w", err)
	}
	return result.URL, nil
}

type aliyunObjectStorage struct {
	client *oss.Client
	bucket string
}

func newAliyunObjectStorage(region, endpoint, bucket, accessKeyID, accessKeySecret string) (*aliyunObjectStorage, error) {
	if strings.TrimSpace(region) == "" || strings.TrimSpace(bucket) == "" ||
		strings.TrimSpace(accessKeyID) == "" || accessKeySecret == "" {
		return nil, fmt.Errorf("incomplete OSS configuration")
	}
	config := oss.LoadDefaultConfig().
		WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, accessKeySecret)).
		WithRegion(strings.TrimSpace(region)).
		WithConnectTimeout(10 * time.Second).
		WithReadWriteTimeout(30 * time.Second)
	if endpoint = strings.TrimSpace(endpoint); endpoint != "" {
		config = config.WithEndpoint(endpoint)
	}
	return &aliyunObjectStorage{client: oss.NewClient(config), bucket: strings.TrimSpace(bucket)}, nil
}

func (storage *aliyunObjectStorage) PresignUpload(
	ctx context.Context, objectKey, contentType string, size int64,
) (objectUploadAuthorization, error) {
	result, err := storage.client.Presign(ctx, &oss.PutObjectRequest{
		Bucket: oss.Ptr(storage.bucket), Key: oss.Ptr(objectKey),
		ContentType: oss.Ptr(contentType), ContentLength: oss.Ptr(size),
		ForbidOverwrite: oss.Ptr("true"),
	}, oss.PresignExpires(objectUploadURLLifetime))
	if err != nil {
		return objectUploadAuthorization{}, fmt.Errorf("presign OSS upload: %w", err)
	}
	return objectUploadAuthorization{
		ObjectKey: objectKey, Method: result.Method, URL: result.URL,
		Headers: result.SignedHeaders, ExpiresAt: result.Expiration,
	}, nil
}

var objectStorageStore objectStorage

var safeObjectExtension = regexp.MustCompile(`^\.[a-z0-9]{1,10}$`)

func objectUploadAuthorizationHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	if objectStorageStore == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "storage_unavailable", "文件存储暂未启用")
		return
	}
	var input objectUploadRequest
	if decodeJSONBody(request, &input) != nil || !validObjectUpload(input) {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_file", "文件类型或大小不符合要求")
		return
	}
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(input.Filename)))
	if !safeObjectExtension.MatchString(extension) {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_file", "文件扩展名不符合要求")
		return
	}
	if !validObjectExtension(input.Kind, extension) {
		writeAPIError(writer, request, http.StatusUnprocessableEntity, "invalid_file", "文件扩展名与资源类型不匹配")
		return
	}
	now := time.Now().UTC()
	objectKey := fmt.Sprintf("uploads/%s/%04d/%02d/%s%s",
		user.ID, now.Year(), now.Month(), newRequestID(), extension)
	authorization, err := objectStorageStore.PresignUpload(
		request.Context(), objectKey, strings.ToLower(strings.TrimSpace(input.ContentType)), input.Size)
	if err != nil {
		slogErrorStorage(request, err)
		writeAPIError(writer, request, http.StatusBadGateway, "storage_error", "文件存储暂时不可用")
		return
	}
	auditAuth(request, "object_upload_authorized", user.Email, user.ID)
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": authorization, "request_id": requestIDFromContext(request.Context()),
	})
}

func projectResourceDownloadHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/projects/")
	parts := strings.Split(path, "/")
	if len(parts) != 3 || parts[1] != "resources" {
		writeAPIError(writer, request, http.StatusNotFound, "resource_not_found", "项目资源不存在")
		return
	}
	project, found, err := managedProjectRepositoryStore.FindPublishedBySlug(request.Context(), parts[0])
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "项目服务暂时不可用")
		return
	}
	if !found || objectStorageStore == nil {
		writeAPIError(writer, request, http.StatusNotFound, "resource_not_found", "项目资源不存在")
		return
	}
	var key string
	switch parts[2] {
	case "cover":
		key = project.CoverObjectKey
	case "document":
		key = project.DocumentObjectKey
	case "code":
		key = project.CodeObjectKey
	default:
		writeAPIError(writer, request, http.StatusNotFound, "resource_not_found", "项目资源不存在")
		return
	}
	if key == "" {
		writeAPIError(writer, request, http.StatusNotFound, "resource_not_found", "项目资源不存在")
		return
	}
	signedURL, err := objectStorageStore.PresignDownload(request.Context(), key)
	if err != nil {
		slogErrorStorage(request, err)
		writeAPIError(writer, request, http.StatusBadGateway, "storage_error", "文件存储暂时不可用")
		return
	}
	http.Redirect(writer, request, signedURL, http.StatusTemporaryRedirect)
}

func authorInlineAssetHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	user, ok := requireCurrentUser(writer, request)
	if !ok {
		return
	}
	key, err := decodeObjectKey(request.URL.Query().Get("key"))
	if err != nil || !strings.HasPrefix(key, "uploads/"+user.ID+"/") {
		writeAPIError(writer, request, http.StatusNotFound, "resource_not_found", "文件不存在")
		return
	}
	redirectToSignedObject(writer, request, key)
}

func projectInlineAssetHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	slug := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/api/v1/projects/"), "/assets")
	if slug == "" || strings.Contains(slug, "/") {
		writeAPIError(writer, request, http.StatusNotFound, "resource_not_found", "文件不存在")
		return
	}
	project, found, err := managedProjectRepositoryStore.FindPublishedBySlug(request.Context(), slug)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "项目服务暂时不可用")
		return
	}
	key, keyErr := decodeObjectKey(request.URL.Query().Get("key"))
	if !found || keyErr != nil || !strings.HasPrefix(key, "uploads/"+project.OwnerID+"/") ||
		!strings.Contains(project.Description, "oss://"+key) {
		writeAPIError(writer, request, http.StatusNotFound, "resource_not_found", "文件不存在")
		return
	}
	redirectToSignedObject(writer, request, key)
}

func decodeObjectKey(encoded string) (string, error) {
	value, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(value) > 500 || strings.Contains(string(value), "..") {
		return "", fmt.Errorf("invalid object key")
	}
	return string(value), nil
}

func redirectToSignedObject(writer http.ResponseWriter, request *http.Request, key string) {
	if objectStorageStore == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "storage_unavailable", "文件存储暂未启用")
		return
	}
	signedURL, err := objectStorageStore.PresignDownload(request.Context(), key)
	if err != nil {
		slogErrorStorage(request, err)
		writeAPIError(writer, request, http.StatusBadGateway, "storage_error", "文件存储暂时不可用")
		return
	}
	http.Redirect(writer, request, signedURL, http.StatusTemporaryRedirect)
}

func validObjectUpload(input objectUploadRequest) bool {
	contentType := strings.ToLower(strings.TrimSpace(input.ContentType))
	limits := map[string]int64{"image": 10 << 20, "document": 50 << 20, "code": 500 << 20}
	limit, found := limits[input.Kind]
	if !found || input.Size < 1 || input.Size > limit {
		return false
	}
	allowed := false
	switch input.Kind {
	case "image":
		allowed = contentType == "image/jpeg" || contentType == "image/png" ||
			contentType == "image/webp" || contentType == "image/gif"
	case "document":
		allowed = contentType == "application/pdf" || contentType == "text/markdown" ||
			contentType == "text/plain"
	case "code":
		allowed = contentType == "application/zip" || contentType == "application/x-zip-compressed" ||
			contentType == "application/gzip" || contentType == "application/x-gzip" ||
			contentType == "application/x-tar"
	}
	return allowed && strings.TrimSpace(input.Filename) != ""
}

func validObjectExtension(kind, extension string) bool {
	allowed := map[string]map[string]bool{
		"image":    {".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true},
		"document": {".pdf": true, ".md": true, ".txt": true},
		"code":     {".zip": true, ".gz": true, ".tgz": true, ".tar": true},
	}
	return allowed[kind][extension]
}

func slogErrorStorage(request *http.Request, err error) {
	slog.ErrorContext(request.Context(), "object storage operation failed",
		"request_id", requestIDFromContext(request.Context()), "error", err)
}
