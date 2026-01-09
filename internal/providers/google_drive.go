package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GoogleDriveProvider implements CloudProvider for Google Drive
type GoogleDriveProvider struct {
	name       string
	config     *AuthConfig
	tokens     *TokenInfo
	httpClient *http.Client
}

// NewGoogleDriveProvider creates a new Google Drive provider
func NewGoogleDriveProvider(name string, config *AuthConfig) *GoogleDriveProvider {
	return &GoogleDriveProvider{
		name:       name,
		config:     config,
		httpClient: &http.Client{Timeout: 10 * time.Minute}, // Longer timeout for large file uploads
	}
}

// Type returns the provider type identifier
func (p *GoogleDriveProvider) Type() string {
	return "google_drive"
}

// Name returns the display name for this provider instance
func (p *GoogleDriveProvider) Name() string {
	return p.name
}

// GetAuthURL returns the OAuth authorization URL
func (p *GoogleDriveProvider) GetAuthURL(state string) string {
	baseURL := "https://accounts.google.com/o/oauth2/v2/auth"
	scopes := "https://www.googleapis.com/auth/drive.file https://www.googleapis.com/auth/drive.metadata.readonly"

	params := url.Values{}
	params.Set("client_id", p.config.ClientID)
	params.Set("redirect_uri", p.config.RedirectURL)
	params.Set("response_type", "code")
	params.Set("scope", scopes)
	params.Set("state", state)
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")

	return baseURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for tokens
func (p *GoogleDriveProvider) ExchangeCode(ctx context.Context, code string) (*TokenInfo, error) {
	tokenURL := "https://oauth2.googleapis.com/token"

	data := url.Values{}
	data.Set("client_id", p.config.ClientID)
	data.Set("client_secret", p.config.ClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", p.config.RedirectURL)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s - %s", resp.Status, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	tokens := &TokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		Expiry:       time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	p.tokens = tokens
	return tokens, nil
}

// SetTokens sets the OAuth tokens for this provider
func (p *GoogleDriveProvider) SetTokens(tokens *TokenInfo) {
	p.tokens = tokens
}

// RefreshToken refreshes the access token
func (p *GoogleDriveProvider) RefreshToken(ctx context.Context) (*TokenInfo, error) {
	if p.tokens == nil || p.tokens.RefreshToken == "" {
		return nil, ErrNotAuthenticated
	}

	tokenURL := "https://oauth2.googleapis.com/token"

	data := url.Values{}
	data.Set("client_id", p.config.ClientID)
	data.Set("client_secret", p.config.ClientSecret)
	data.Set("refresh_token", p.tokens.RefreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed: %s - %s", resp.Status, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	p.tokens.AccessToken = tokenResp.AccessToken
	p.tokens.TokenType = tokenResp.TokenType
	p.tokens.Expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return p.tokens, nil
}

// IsAuthenticated returns whether the provider has valid tokens
func (p *GoogleDriveProvider) IsAuthenticated() bool {
	if p.tokens == nil {
		return false
	}
	return p.tokens.AccessToken != "" && time.Now().Before(p.tokens.Expiry)
}

// ensureValidToken refreshes the token if needed
func (p *GoogleDriveProvider) ensureValidToken(ctx context.Context) error {
	if p.tokens == nil {
		return ErrNotAuthenticated
	}
	if time.Now().After(p.tokens.Expiry.Add(-5 * time.Minute)) {
		_, err := p.RefreshToken(ctx)
		return err
	}
	return nil
}

// makeAuthenticatedRequest makes an HTTP request with the access token
func (p *GoogleDriveProvider) makeAuthenticatedRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	if err := p.ensureValidToken(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.tokens.AccessToken)

	return p.httpClient.Do(req)
}

// Upload uploads a file from a local path to Google Drive
func (p *GoogleDriveProvider) Upload(ctx context.Context, localPath string, remotePath string) (*FileMetadata, error) {
	if !p.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}

	// Open the local file
	file, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open local file: %w", err)
	}
	defer file.Close()

	// Get file info for size
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	return p.UploadStream(ctx, file, remotePath, fileInfo.Size())
}

// UploadStream uploads data from a reader to Google Drive
func (p *GoogleDriveProvider) UploadStream(ctx context.Context, reader io.Reader, remotePath string, size int64) (*FileMetadata, error) {
	if err := p.ensureValidToken(ctx); err != nil {
		return nil, err
	}

	// Parse the remote path to get folder and filename
	fileName := filepath.Base(remotePath)
	parentPath := filepath.Dir(remotePath)

	// Get or create parent folder ID
	parentID := "root"
	if parentPath != "" && parentPath != "." && parentPath != "/" {
		folderMeta, err := p.ensureFolderPath(ctx, parentPath)
		if err != nil {
			return nil, fmt.Errorf("failed to ensure folder path: %w", err)
		}
		parentID = folderMeta.ID
	}

	// Detect mime type from filename
	mimeType := detectMimeType(fileName)

	// Create multipart upload request
	// First, create file metadata
	metadata := map[string]interface{}{
		"name":    fileName,
		"parents": []string{parentID},
	}

	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// For files > 5MB, use resumable upload; otherwise use multipart
	if size > 5*1024*1024 {
		return p.resumableUpload(ctx, reader, fileName, mimeType, parentID, size)
	}

	// Simple multipart upload for smaller files
	return p.multipartUpload(ctx, reader, metadataBytes, mimeType, size)
}

// multipartUpload handles simple multipart uploads for files <= 5MB
func (p *GoogleDriveProvider) multipartUpload(ctx context.Context, reader io.Reader, metadata []byte, mimeType string, size int64) (*FileMetadata, error) {
	// Create multipart body
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add metadata part
	metadataHeader := make(textproto.MIMEHeader)
	metadataHeader.Set("Content-Type", "application/json; charset=UTF-8")
	metadataPart, err := writer.CreatePart(metadataHeader)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata part: %w", err)
	}
	metadataPart.Write(metadata)

	// Add file content part
	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Type", mimeType)
	filePart, err := writer.CreatePart(fileHeader)
	if err != nil {
		return nil, fmt.Errorf("failed to create file part: %w", err)
	}
	if _, err := io.Copy(filePart, reader); err != nil {
		return nil, fmt.Errorf("failed to copy file content: %w", err)
	}
	writer.Close()

	// Make the upload request
	uploadURL := "https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart&fields=id,name,size,mimeType,modifiedTime,parents"

	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.tokens.AccessToken)
	req.Header.Set("Content-Type", "multipart/related; boundary="+writer.Boundary())

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return p.parseFileResponse(resp.Body)
}

// resumableUpload handles resumable uploads for larger files
func (p *GoogleDriveProvider) resumableUpload(ctx context.Context, reader io.Reader, fileName, mimeType, parentID string, size int64) (*FileMetadata, error) {
	// Step 1: Initiate resumable upload session
	metadata := map[string]interface{}{
		"name":    fileName,
		"parents": []string{parentID},
	}
	metadataBytes, _ := json.Marshal(metadata)

	initURL := "https://www.googleapis.com/upload/drive/v3/files?uploadType=resumable"
	req, err := http.NewRequestWithContext(ctx, "POST", initURL, bytes.NewReader(metadataBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create init request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.tokens.AccessToken)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type", mimeType)
	req.Header.Set("X-Upload-Content-Length", fmt.Sprintf("%d", size))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("init upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("init upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Get the resumable upload URL
	uploadURL := resp.Header.Get("Location")
	if uploadURL == "" {
		return nil, fmt.Errorf("no upload URL in response")
	}

	// Step 2: Upload entire file content in a single PUT request
	// For simplicity, we'll upload the whole file at once rather than chunking
	uploadReq, err := http.NewRequestWithContext(ctx, "PUT", uploadURL, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload request: %w", err)
	}

	uploadReq.Header.Set("Content-Length", fmt.Sprintf("%d", size))
	uploadReq.Header.Set("Content-Type", mimeType)
	uploadReq.ContentLength = size

	uploadResp, err := p.httpClient.Do(uploadReq)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer uploadResp.Body.Close()

	if uploadResp.StatusCode != http.StatusOK && uploadResp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(uploadResp.Body)
		return nil, fmt.Errorf("upload failed with status %d: %s", uploadResp.StatusCode, string(respBody))
	}

	return p.parseFileResponse(uploadResp.Body)
}

// ensureFolderPath creates folder path if it doesn't exist and returns the leaf folder ID
func (p *GoogleDriveProvider) ensureFolderPath(ctx context.Context, path string) (*FileMetadata, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	parentID := "root"

	var lastFolder *FileMetadata

	for _, part := range parts {
		if part == "" {
			continue
		}

		// Search for existing folder
		query := fmt.Sprintf("name='%s' and '%s' in parents and mimeType='application/vnd.google-apps.folder' and trashed=false", part, parentID)
		searchURL := fmt.Sprintf("https://www.googleapis.com/drive/v3/files?q=%s&fields=files(id,name)", url.QueryEscape(query))

		resp, err := p.makeAuthenticatedRequest(ctx, "GET", searchURL, nil)
		if err != nil {
			return nil, err
		}

		var searchResult struct {
			Files []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"files"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		if len(searchResult.Files) > 0 {
			// Folder exists
			parentID = searchResult.Files[0].ID
			lastFolder = &FileMetadata{
				ID:    parentID,
				Name:  part,
				IsDir: true,
			}
		} else {
			// Create folder
			folderMeta := map[string]interface{}{
				"name":     part,
				"mimeType": "application/vnd.google-apps.folder",
				"parents":  []string{parentID},
			}
			metaBytes, _ := json.Marshal(folderMeta)

			createResp, err := p.makeAuthenticatedRequest(ctx, "POST", "https://www.googleapis.com/drive/v3/files?fields=id,name", bytes.NewReader(metaBytes))
			if err != nil {
				return nil, err
			}

			var created struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
				createResp.Body.Close()
				return nil, err
			}
			createResp.Body.Close()

			parentID = created.ID
			lastFolder = &FileMetadata{
				ID:    parentID,
				Name:  part,
				IsDir: true,
			}
		}
	}

	return lastFolder, nil
}

// parseFileResponse parses the Drive API file response
func (p *GoogleDriveProvider) parseFileResponse(body io.Reader) (*FileMetadata, error) {
	var fileResp struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Size         string `json:"size"`
		MimeType     string `json:"mimeType"`
		ModifiedTime string `json:"modifiedTime"`
	}

	if err := json.NewDecoder(body).Decode(&fileResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var size int64
	fmt.Sscanf(fileResp.Size, "%d", &size)

	modTime, _ := time.Parse(time.RFC3339, fileResp.ModifiedTime)

	return &FileMetadata{
		ID:       fileResp.ID,
		Name:     fileResp.Name,
		Path:     fileResp.Name,
		Size:     size,
		MimeType: fileResp.MimeType,
		ModTime:  modTime,
		IsDir:    fileResp.MimeType == "application/vnd.google-apps.folder",
	}, nil
}

// detectMimeType returns the MIME type based on file extension
func detectMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	mimeTypes := map[string]string{
		".mp4":  "video/mp4",
		".mov":  "video/quicktime",
		".avi":  "video/x-msvideo",
		".mkv":  "video/x-matroska",
		".webm": "video/webm",
		".m4v":  "video/x-m4v",
		".wmv":  "video/x-ms-wmv",
		".flv":  "video/x-flv",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".pdf":  "application/pdf",
		".txt":  "text/plain",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".zip":  "application/zip",
		".json": "application/json",
	}

	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

// Download downloads a file to a writer
func (p *GoogleDriveProvider) Download(ctx context.Context, fileID string, writer io.Writer) error {
	if !p.IsAuthenticated() {
		return ErrNotAuthenticated
	}

	// TODO: Implement actual Google Drive download
	return nil
}

// DownloadStream returns a reader for streaming download
func (p *GoogleDriveProvider) DownloadStream(ctx context.Context, fileID string) (io.ReadCloser, error) {
	if !p.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}

	// TODO: Implement streaming download from Google Drive
	return nil, fmt.Errorf("not implemented")
}

// Delete removes a file from Google Drive
func (p *GoogleDriveProvider) Delete(ctx context.Context, fileID string) error {
	if !p.IsAuthenticated() {
		return ErrNotAuthenticated
	}

	// TODO: Implement actual Google Drive delete
	return nil
}

// GetFile retrieves metadata for a specific file
func (p *GoogleDriveProvider) GetFile(ctx context.Context, fileID string) (*FileMetadata, error) {
	if !p.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}

	// TODO: Implement actual Google Drive file metadata fetch
	return &FileMetadata{
		ID:      fileID,
		Name:    "mock_file",
		ModTime: time.Now(),
	}, nil
}

// ListFiles lists files in a directory
func (p *GoogleDriveProvider) ListFiles(ctx context.Context, path string) ([]*FileMetadata, error) {
	if !p.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}

	// TODO: Implement actual Google Drive file listing
	return []*FileMetadata{}, nil
}

// CreateFolder creates a new folder in Google Drive
func (p *GoogleDriveProvider) CreateFolder(ctx context.Context, path string) (*FileMetadata, error) {
	if !p.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}

	// TODO: Implement actual Google Drive folder creation
	return &FileMetadata{
		ID:      fmt.Sprintf("gdrive_folder_%d", time.Now().UnixNano()),
		Name:    path,
		Path:    path,
		IsDir:   true,
		ModTime: time.Now(),
	}, nil
}

// GetQuota returns the storage quota information
func (p *GoogleDriveProvider) GetQuota(ctx context.Context) (*QuotaInfo, error) {
	if err := p.ensureValidToken(ctx); err != nil {
		return nil, err
	}

	aboutURL := "https://www.googleapis.com/drive/v3/about?fields=storageQuota"

	resp, err := p.makeAuthenticatedRequest(ctx, "GET", aboutURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get quota: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get quota: %s - %s", resp.Status, string(body))
	}

	var aboutResp struct {
		StorageQuota struct {
			Limit        string `json:"limit"`
			Usage        string `json:"usage"`
			UsageInDrive string `json:"usageInDrive"`
		} `json:"storageQuota"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&aboutResp); err != nil {
		return nil, fmt.Errorf("failed to decode quota response: %w", err)
	}

	var limit, usage int64
	fmt.Sscanf(aboutResp.StorageQuota.Limit, "%d", &limit)
	fmt.Sscanf(aboutResp.StorageQuota.Usage, "%d", &usage)

	return &QuotaInfo{
		TotalBytes: limit,
		UsedBytes:  usage,
		FreeBytes:  limit - usage,
	}, nil
}
