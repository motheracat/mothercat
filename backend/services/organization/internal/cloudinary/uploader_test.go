package cloudinary

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestUploadSignsRequestAndReturnsOnlyCloudinaryHTTPSURL(t *testing.T) {
	uploader, err := New("example-cloud", "api-key", "api-secret")
	if err != nil {
		t.Fatal(err)
	}
	uploader.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != "https://api.cloudinary.com/v1_1/example-cloud/image/upload" {
			t.Fatalf("request = %s %s", request.Method, request.URL)
		}
		mediaType, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/form-data" {
			t.Fatalf("multipart content type %q: %v", request.Header.Get("Content-Type"), err)
		}
		reader := multipart.NewReader(request.Body, params["boundary"])
		fields := make(map[string]string)
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			value, err := io.ReadAll(part)
			if err != nil {
				t.Fatal(err)
			}
			if part.FormName() == "file" {
				if string(value) != "image-bytes" || part.FileName() != "profile.png" {
					t.Fatalf("image part name=%q content=%q", part.FileName(), value)
				}
			} else {
				fields[part.FormName()] = string(value)
			}
		}
		parameters := map[string]string{
			"folder": fields["folder"], "public_id": fields["public_id"], "timestamp": fields["timestamp"],
		}
		if fields["api_key"] != "api-key" || fields["signature"] != sign(parameters, "api-secret") ||
			fields["folder"] != "mothercat/profile-pictures" || len(fields["public_id"]) != 32 {
			t.Fatalf("invalid Cloudinary fields: %+v", fields)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"secure_url":"https://res.cloudinary.com/example-cloud/image/upload/profile.png"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	imageURL, err := uploader.Upload(context.Background(), "image/png", []byte("image-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if imageURL != "https://res.cloudinary.com/example-cloud/image/upload/profile.png" {
		t.Fatalf("Upload() URL = %q", imageURL)
	}
}

func TestUploadRejectsURLOutsideConfiguredCloud(t *testing.T) {
	uploader, err := New("example-cloud", "api-key", "api-secret")
	if err != nil {
		t.Fatal(err)
	}
	uploader.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"secure_url":"https://example.test/profile.png"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	if _, err := uploader.Upload(context.Background(), "image/png", []byte("image-bytes")); err == nil {
		t.Fatal("Upload() accepted an external URL")
	}
}
