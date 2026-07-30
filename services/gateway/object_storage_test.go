package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
)

type fakeObjectStorage struct {
	key, contentType string
	size             int64
}

func TestAliyunObjectStorageIntegration(t *testing.T) {
	region, endpoint, bucket := os.Getenv("OSS_TEST_REGION"), os.Getenv("OSS_TEST_ENDPOINT"), os.Getenv("OSS_TEST_BUCKET")
	accessKeyID, accessKeySecret := os.Getenv("OSS_TEST_ACCESS_KEY_ID"), os.Getenv("OSS_TEST_ACCESS_KEY_SECRET")
	if region == "" || bucket == "" || accessKeyID == "" || accessKeySecret == "" {
		t.Skip("OSS integration environment is not configured")
	}
	storage, err := newAliyunObjectStorage(region, endpoint, bucket, accessKeyID, accessKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("OSS_TEST_CONFIGURE_CORS") == "1" {
		maxAge := int64(3600)
		_, err = storage.client.PutBucketCors(context.Background(), &oss.PutBucketCorsRequest{
			Bucket: oss.Ptr(bucket),
			CORSConfiguration: &oss.CORSConfiguration{CORSRules: []oss.CORSRule{{
				AllowedOrigins: []string{"https://103.236.98.166:8443", "https://www.openresource.cn"},
				AllowedMethods: []string{"PUT", "GET", "HEAD"},
				AllowedHeaders: []string{"*"},
				ExposeHeaders:  []string{"ETag", "x-oss-request-id"},
				MaxAgeSeconds:  &maxAge,
			}}},
		})
		if err != nil {
			t.Fatalf("configure OSS CORS: %v", err)
		}
	}
	body := []byte("OpenResource OSS integration check")
	key := "integration-tests/" + newRequestID() + ".txt"
	authorization, err := storage.PresignUpload(context.Background(), key, "text/plain", int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(authorization.Method, authorization.URL, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range authorization.Headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	responseBody, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("OSS upload status = %d: %s", response.StatusCode, responseBody)
	}
	_, err = storage.client.DeleteObject(context.Background(),
		&oss.DeleteObjectRequest{Bucket: oss.Ptr(bucket), Key: oss.Ptr(key)})
	if err != nil {
		t.Fatalf("delete OSS integration object: %v", err)
	}
}

func (storage *fakeObjectStorage) PresignUpload(
	_ context.Context, key, contentType string, size int64,
) (objectUploadAuthorization, error) {
	storage.key, storage.contentType, storage.size = key, contentType, size
	return objectUploadAuthorization{
		ObjectKey: key, Method: http.MethodPut, URL: "https://oss.example.test/signed",
		Headers:   map[string]string{"Content-Type": contentType},
		ExpiresAt: time.Now().Add(time.Minute),
	}, nil
}

func (storage *fakeObjectStorage) PresignDownload(_ context.Context, key string) (string, error) {
	storage.key = key
	return "https://oss.example.test/download", nil
}

func TestObjectUploadAuthorization(t *testing.T) {
	originalAuth, originalStorage, originalLimiter := authRepositoryStore, objectStorageStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	storage := &fakeObjectStorage{}
	objectStorageStore = storage
	authRateLimiter = newFixedWindowLimiter()
	t.Cleanup(func() {
		authRepositoryStore, objectStorageStore, authRateLimiter = originalAuth, originalStorage, originalLimiter
	})
	cookie, user := registerTestUser(t, "upload@example.com", "上传用户")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/files/presign-upload",
		strings.NewReader(`{"filename":"cover.webp","content_type":"image/webp","size":2048,"kind":"image"}`))
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("presign status = %d: %s", response.Code, response.Body)
	}
	if !strings.HasPrefix(storage.key, "uploads/"+user.ID+"/") ||
		storage.contentType != "image/webp" || storage.size != 2048 {
		t.Fatalf("presign input = %#v", storage)
	}

	invalid := httptest.NewRequest(http.MethodPost, "/api/v1/files/presign-upload",
		strings.NewReader(`{"filename":"payload.exe","content_type":"image/png","size":20,"kind":"image"}`))
	invalid.AddCookie(cookie)
	invalidResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid upload status = %d", invalidResponse.Code)
	}
}

func TestPublishedProjectResourceRedirect(t *testing.T) {
	originalProjects, originalStorage := managedProjectRepositoryStore, objectStorageStore
	repository := newMemoryManagedProjectRepository()
	managedProjectRepositoryStore = repository
	storage := &fakeObjectStorage{}
	objectStorageStore = storage
	t.Cleanup(func() {
		managedProjectRepositoryStore, objectStorageStore = originalProjects, originalStorage
	})
	input := managedProjectInput{
		Slug: "resource-project", Name: "Resource Project",
		Summary:  "用于验证项目资源下载重定向的测试项目",
		Category: "Testing", License: "MIT", CurrentVersion: "1.0.0",
		CodeObjectKey: "uploads/user-owner/2026/07/code.zip",
		Description:   "这是足够长的项目描述，包含图片 ![](oss://uploads/user-owner/2026/07/cover.webp)",
	}
	project, err := repository.Create(context.Background(), "user-owner", input)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = repository.Submit(context.Background(), "user-owner", project.ID, time.Now().UTC())
	_, _ = repository.Review(context.Background(), project.ID, "user-admin", "approve", "", time.Now().UTC())
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/resource-project/resources/code", nil)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusTemporaryRedirect ||
		response.Header().Get("Location") != "https://oss.example.test/download" {
		t.Fatalf("download redirect = %d %s", response.Code, response.Header().Get("Location"))
	}
	inlineKey := base64.RawURLEncoding.EncodeToString([]byte("uploads/user-owner/2026/07/cover.webp"))
	inlineRequest := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/resource-project/assets?key="+inlineKey, nil)
	inlineResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(inlineResponse, inlineRequest)
	if inlineResponse.Code != http.StatusTemporaryRedirect {
		t.Fatalf("inline asset redirect = %d: %s", inlineResponse.Code, inlineResponse.Body)
	}
}
