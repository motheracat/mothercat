package cloudinary

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const maxResponseSize = 1 << 20

var cloudNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type Uploader struct {
	cloudName string
	apiKey    string
	apiSecret string
	client    *http.Client
}

func New(cloudName, apiKey, apiSecret string) (*Uploader, error) {
	cloudName = strings.TrimSpace(cloudName)
	apiKey = strings.TrimSpace(apiKey)
	apiSecret = strings.TrimSpace(apiSecret)
	if !cloudNamePattern.MatchString(cloudName) || apiKey == "" || apiSecret == "" {
		return nil, errors.New("Cloudinary cloud name, API key, and API secret are required")
	}
	return &Uploader{
		cloudName: cloudName,
		apiKey:    apiKey,
		apiSecret: apiSecret,
		client:    &http.Client{Timeout: 12 * time.Second},
	}, nil
}

func (u *Uploader) Upload(ctx context.Context, contentType string, image []byte) (string, error) {
	var randomID [16]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		return "", errors.New("generate Cloudinary asset identifier")
	}
	publicID := hex.EncodeToString(randomID[:])
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	folder := "mothercat/profile-pictures"
	signature := sign(map[string]string{
		"folder": folder, "public_id": publicID, "timestamp": timestamp,
	}, u.apiSecret)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := map[string]string{
		"api_key": u.apiKey, "folder": folder, "public_id": publicID,
		"timestamp": timestamp, "signature": signature,
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return "", fmt.Errorf("prepare Cloudinary upload: %w", err)
		}
	}
	part, err := writer.CreateFormFile("file", "profile"+extension(contentType))
	if err != nil {
		return "", fmt.Errorf("prepare Cloudinary image: %w", err)
	}
	if _, err := part.Write(image); err != nil {
		return "", fmt.Errorf("prepare Cloudinary image: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("finish Cloudinary upload request: %w", err)
	}

	endpoint := "https://api.cloudinary.com/v1_1/" + url.PathEscape(u.cloudName) + "/image/upload"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return "", fmt.Errorf("create Cloudinary upload request: %w", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := u.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("send image to Cloudinary: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return "", fmt.Errorf("read Cloudinary response: %w", err)
	}
	if len(responseBody) > maxResponseSize {
		return "", errors.New("Cloudinary response exceeded the size limit")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("Cloudinary upload returned HTTP %d", response.StatusCode)
	}
	var result struct {
		SecureURL string `json:"secure_url"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return "", fmt.Errorf("decode Cloudinary upload response: %w", err)
	}
	assetURL, err := url.Parse(result.SecureURL)
	if err != nil || assetURL.Scheme != "https" || assetURL.Host != "res.cloudinary.com" ||
		!strings.HasPrefix(assetURL.Path, "/"+u.cloudName+"/image/upload/") {
		return "", errors.New("Cloudinary returned an invalid secure image URL")
	}
	return assetURL.String(), nil
}

func sign(parameters map[string]string, secret string) string {
	keys := []string{"folder", "public_id", "timestamp"}
	message := keys[0] + "=" + parameters[keys[0]]
	for _, key := range keys[1:] {
		message += "&" + key + "=" + parameters[key]
	}
	digest := sha1.Sum([]byte(message + secret))
	return hex.EncodeToString(digest[:])
}

func extension(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	default:
		return ".jpg"
	}
}
