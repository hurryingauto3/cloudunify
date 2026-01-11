package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	msGraphBaseURL   = "https://graph.microsoft.com/v1.0"
	msAuthURL        = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
	msTokenURL       = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	oneDriveUploadChunkSize = 10 * 1024 * 1024 // 10MB chunks for resumable upload
)

// OneDriveProvider implements CloudProvider for Microsoft OneDrive
type OneDriveProvider struct {
	name       string
	config     *AuthConfig
	tokens     *TokenInfo
	httpClient *http.Client
}

// NewOneDriveProvider creates a new OneDrive provider
func NewOneDriveProvider(name string, config *AuthConfig) *OneDriveProvider {
	return &OneDriveProvider{
		name:       name,
		config:     config,
		httpClient: &http.Client{Timeout: 10 * time.Minute},
	}
}

// Type returns the provider type identifier
func (p *OneDriveProvider) Type() string {
	return "onedrive"
}

// Name returns the display name for this provider instance
func (p *OneDriveProvider) Name() string {
	return p.name
}

// GetAuthURL returns the OAuth authorization URL
func (p *OneDriveProvider) GetAuthURL(state string) string {
	scopes := "Files.ReadWrite.All User.Read offline_access"

	params := url.Values{}
	params.Set("client_id", p.config.ClientID)
	params.Set("redirect_uri", p.config.RedirectURL)
	params.Set("response_type", "code")
	params.Set("scope", scopes)
	params.Set("state", state)

	return msAuthURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for tokens
func (p *OneDriveProvider) ExchangeCode(ctx context.Context, code string) (*TokenInfo, error) {
	data := url.Values{}
	data.Set("client_id", p.config.ClientID)
	data.Set("client_secret", p.config.ClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", p.config.RedirectURL)

	req, err := http.NewRequestWithContext(ctx, "POST", msTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
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
func (p *OneDriveProvider) SetTokens(tokens *TokenInfo) {
	p.tokens = tokens
}

// RefreshToken refreshes the access token
func (p *OneDriveProvider) RefreshToken(ctx context.Context) (*TokenInfo, error) {
	if p.tokens == nil || p.tokens.RefreshToken == "" {
		return nil, ErrNotAuthenticated
	}

	data := url.Values{}
	data.Set("client_id", p.config.ClientID)
	data.Set("client_secret", p.config.ClientSecret)
	data.Set("refresh_token", p.tokens.RefreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, "POST", msTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
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
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	p.tokens.AccessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		p.tokens.RefreshToken = tokenResp.RefreshToken
	}
	p.tokens.TokenType = tokenResp.TokenType
	p.tokens.Expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return p.tokens, nil
}

// IsAuthenticated returns whether the provider has valid tokens
func (p *OneDriveProvider) IsAuthenticated() bool {
	if p.tokens == nil {
		return false
	}
	// Check if we have a refresh token (allowing re-authentication)
	if p.tokens.RefreshToken != "" {
		return true
	}
	return p.tokens.AccessToken != "" && time.Now().Before(p.tokens.Expiry)
}

// ensureValidToken refreshes the token if needed
func (p *OneDriveProvider) ensureValidToken(ctx context.Context) error {
	if p.tokens == nil {
		return ErrNotAuthenticated
	}

	// If we have a refresh token and the access token is expired (or close to expiring), refresh it
	if p.tokens.RefreshToken != "" && (p.tokens.AccessToken == "" || time.Now().After(p.tokens.Expiry.Add(-5*time.Minute))) {
		_, err := p.RefreshToken(ctx)
		return err
	}

	if p.tokens.AccessToken == "" {
		return ErrNotAuthenticated
	}

	return nil
}

// makeAuthenticatedRequest makes an HTTP request with the access token
func (p *OneDriveProvider) makeAuthenticatedRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
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

// Upload uploads a file from a local path to OneDrive
func (p *OneDriveProvider) Upload(ctx context.Context, localPath string, remotePath string) (*FileMetadata, error) {
	if !p.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}

	file, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open local file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	return p.UploadStream(ctx, file, remotePath, fileInfo.Size())
}

// UploadStream uploads data from a reader to OneDrive
func (p *OneDriveProvider) UploadStream(ctx context.Context, reader io.Reader, remotePath string, size int64) (*FileMetadata, error) {
	if err := p.ensureValidToken(ctx); err != nil {
		return nil, err
	}

	// Normalize path (remove leading slash)
	remotePath = strings.TrimPrefix(remotePath, "/")

	// For files <= 4MB, use simple upload
	if size <= 4*1024*1024 {
		return p.simpleUpload(ctx, reader, remotePath, size)
	}

	// For larger files, use upload session
	return p.resumableUpload(ctx, reader, remotePath, size)
}

// simpleUpload handles uploads for files <= 4MB
func (p *OneDriveProvider) simpleUpload(ctx context.Context, reader io.Reader, remotePath string, size int64) (*FileMetadata, error) {
	// Read all content (small file)
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}

	// URL encode the path
	encodedPath := url.PathEscape(remotePath)
	uploadURL := fmt.Sprintf("%s/me/drive/root:/%s:/content", msGraphBaseURL, encodedPath)

	req, err := http.NewRequestWithContext(ctx, "PUT", uploadURL, bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.tokens.AccessToken)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return p.parseItemResponse(resp.Body)
}

// resumableUpload handles uploads for files > 4MB using upload sessions
func (p *OneDriveProvider) resumableUpload(ctx context.Context, reader io.Reader, remotePath string, size int64) (*FileMetadata, error) {
	// Step 1: Create upload session
	encodedPath := url.PathEscape(remotePath)
	sessionURL := fmt.Sprintf("%s/me/drive/root:/%s:/createUploadSession", msGraphBaseURL, encodedPath)

	sessionBody := map[string]interface{}{
		"item": map[string]interface{}{
			"@microsoft.graph.conflictBehavior": "replace",
		},
	}
	sessionBodyBytes, _ := json.Marshal(sessionBody)

	req, err := http.NewRequestWithContext(ctx, "POST", sessionURL, bytes.NewReader(sessionBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create session request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.tokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create session failed with status %d: %s", resp.StatusCode, string(body))
	}

	var sessionResp struct {
		UploadURL string `json:"uploadUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		return nil, fmt.Errorf("failed to decode session response: %w", err)
	}

	// Step 2: Upload in chunks
	var offset int64 = 0
	buffer := make([]byte, oneDriveUploadChunkSize)

	for offset < size {
		n, err := io.ReadFull(reader, buffer)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, fmt.Errorf("failed to read chunk: %w", err)
		}
		if n == 0 {
			break
		}

		chunk := buffer[:n]
		endByte := offset + int64(n) - 1

		chunkReq, err := http.NewRequestWithContext(ctx, "PUT", sessionResp.UploadURL, bytes.NewReader(chunk))
		if err != nil {
			return nil, fmt.Errorf("failed to create chunk request: %w", err)
		}

		chunkReq.Header.Set("Content-Length", fmt.Sprintf("%d", n))
		chunkReq.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, endByte, size))

		chunkResp, err := p.httpClient.Do(chunkReq)
		if err != nil {
			return nil, fmt.Errorf("chunk upload failed: %w", err)
		}

		if chunkResp.StatusCode == http.StatusOK || chunkResp.StatusCode == http.StatusCreated {
			// Upload complete
			metadata, err := p.parseItemResponse(chunkResp.Body)
			chunkResp.Body.Close()
			return metadata, err
		} else if chunkResp.StatusCode == http.StatusAccepted {
			// More chunks needed
			chunkResp.Body.Close()
		} else {
			body, _ := io.ReadAll(chunkResp.Body)
			chunkResp.Body.Close()
			return nil, fmt.Errorf("chunk upload failed with status %d: %s", chunkResp.StatusCode, string(body))
		}

		offset += int64(n)
	}

	return nil, fmt.Errorf("upload completed without final response")
}

// Download downloads a file to a writer
func (p *OneDriveProvider) Download(ctx context.Context, fileID string, writer io.Writer) error {
	if !p.IsAuthenticated() {
		return ErrNotAuthenticated
	}

	stream, err := p.DownloadStream(ctx, fileID)
	if err != nil {
		return err
	}
	defer stream.Close()

	_, err = io.Copy(writer, stream)
	return err
}

// DownloadStream returns a reader for streaming download
func (p *OneDriveProvider) DownloadStream(ctx context.Context, fileID string) (io.ReadCloser, error) {
	if err := p.ensureValidToken(ctx); err != nil {
		return nil, err
	}

	// Get download URL from item metadata
	itemURL := fmt.Sprintf("%s/me/drive/items/%s", msGraphBaseURL, fileID)
	resp, err := p.makeAuthenticatedRequest(ctx, "GET", itemURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get item: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get item failed with status %d: %s", resp.StatusCode, string(body))
	}

	var item struct {
		DownloadURL string `json:"@microsoft.graph.downloadUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("failed to decode item response: %w", err)
	}

	if item.DownloadURL == "" {
		return nil, fmt.Errorf("no download URL available")
	}

	// Download from the URL (no auth needed for pre-authenticated URL)
	downloadReq, err := http.NewRequestWithContext(ctx, "GET", item.DownloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}

	downloadResp, err := p.httpClient.Do(downloadReq)
	if err != nil {
		return nil, fmt.Errorf("download request failed: %w", err)
	}

	if downloadResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(downloadResp.Body)
		downloadResp.Body.Close()
		return nil, fmt.Errorf("download failed with status %d: %s", downloadResp.StatusCode, string(body))
	}

	return downloadResp.Body, nil
}

// SupportsRangeRequests returns true - OneDrive supports range requests
func (p *OneDriveProvider) SupportsRangeRequests() bool {
	return true
}

// DownloadRange downloads a byte range from a file
func (p *OneDriveProvider) DownloadRange(ctx context.Context, fileID string, start, end int64) (io.ReadCloser, error) {
	if err := p.ensureValidToken(ctx); err != nil {
		return nil, err
	}

	// Get download URL from item metadata
	itemURL := fmt.Sprintf("%s/me/drive/items/%s", msGraphBaseURL, fileID)
	resp, err := p.makeAuthenticatedRequest(ctx, "GET", itemURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get item: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get item failed with status %d: %s", resp.StatusCode, string(body))
	}

	var item struct {
		DownloadURL string `json:"@microsoft.graph.downloadUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("failed to decode item response: %w", err)
	}

	if item.DownloadURL == "" {
		return nil, fmt.Errorf("no download URL available")
	}

	// Download range from the URL
	downloadReq, err := http.NewRequestWithContext(ctx, "GET", item.DownloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}
	downloadReq.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	downloadResp, err := p.httpClient.Do(downloadReq)
	if err != nil {
		return nil, fmt.Errorf("download request failed: %w", err)
	}

	if downloadResp.StatusCode != http.StatusOK && downloadResp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(downloadResp.Body)
		downloadResp.Body.Close()
		return nil, fmt.Errorf("download failed with status %d: %s", downloadResp.StatusCode, string(body))
	}

	return downloadResp.Body, nil
}

// Delete removes a file from OneDrive
func (p *OneDriveProvider) Delete(ctx context.Context, fileID string) error {
	if err := p.ensureValidToken(ctx); err != nil {
		return err
	}

	deleteURL := fmt.Sprintf("%s/me/drive/items/%s", msGraphBaseURL, fileID)
	resp, err := p.makeAuthenticatedRequest(ctx, "DELETE", deleteURL, nil)
	if err != nil {
		return fmt.Errorf("delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetFile retrieves metadata for a specific file
func (p *OneDriveProvider) GetFile(ctx context.Context, fileID string) (*FileMetadata, error) {
	if err := p.ensureValidToken(ctx); err != nil {
		return nil, err
	}

	itemURL := fmt.Sprintf("%s/me/drive/items/%s", msGraphBaseURL, fileID)
	resp, err := p.makeAuthenticatedRequest(ctx, "GET", itemURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get item: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get item failed with status %d: %s", resp.StatusCode, string(body))
	}

	return p.parseItemResponse(resp.Body)
}

// ListFiles lists files in a directory
func (p *OneDriveProvider) ListFiles(ctx context.Context, path string) ([]*FileMetadata, error) {
	if err := p.ensureValidToken(ctx); err != nil {
		return nil, err
	}

	var listURL string
	path = strings.Trim(path, "/")

	if path == "" || path == "." {
		listURL = fmt.Sprintf("%s/me/drive/root/children?$top=200", msGraphBaseURL)
	} else {
		encodedPath := url.PathEscape(path)
		listURL = fmt.Sprintf("%s/me/drive/root:/%s:/children?$top=200", msGraphBaseURL, encodedPath)
	}

	var allFiles []*FileMetadata

	for listURL != "" {
		resp, err := p.makeAuthenticatedRequest(ctx, "GET", listURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list files: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("list files failed with status %d: %s", resp.StatusCode, string(body))
		}

		var listResp struct {
			Value    []oneDriveItem `json:"value"`
			NextLink string         `json:"@odata.nextLink"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode list response: %w", err)
		}
		resp.Body.Close()

		for _, item := range listResp.Value {
			allFiles = append(allFiles, item.toFileMetadata())
		}

		listURL = listResp.NextLink
	}

	return allFiles, nil
}

// CreateFolder creates a new folder in OneDrive
func (p *OneDriveProvider) CreateFolder(ctx context.Context, path string) (*FileMetadata, error) {
	if err := p.ensureValidToken(ctx); err != nil {
		return nil, err
	}

	path = strings.Trim(path, "/")
	folderName := filepath.Base(path)
	parentPath := filepath.Dir(path)

	var createURL string
	if parentPath == "" || parentPath == "." {
		createURL = fmt.Sprintf("%s/me/drive/root/children", msGraphBaseURL)
	} else {
		encodedPath := url.PathEscape(parentPath)
		createURL = fmt.Sprintf("%s/me/drive/root:/%s:/children", msGraphBaseURL, encodedPath)
	}

	body := map[string]interface{}{
		"name":                              folderName,
		"folder":                            map[string]interface{}{},
		"@microsoft.graph.conflictBehavior": "fail",
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", createURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create folder request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.tokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create folder request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create folder failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return p.parseItemResponse(resp.Body)
}

// GetQuota returns the storage quota information
func (p *OneDriveProvider) GetQuota(ctx context.Context) (*QuotaInfo, error) {
	if err := p.ensureValidToken(ctx); err != nil {
		return nil, err
	}

	driveURL := fmt.Sprintf("%s/me/drive", msGraphBaseURL)
	resp, err := p.makeAuthenticatedRequest(ctx, "GET", driveURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get drive info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get drive failed with status %d: %s", resp.StatusCode, string(body))
	}

	var driveResp struct {
		Quota struct {
			Total     int64 `json:"total"`
			Used      int64 `json:"used"`
			Remaining int64 `json:"remaining"`
		} `json:"quota"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&driveResp); err != nil {
		return nil, fmt.Errorf("failed to decode drive response: %w", err)
	}

	return &QuotaInfo{
		TotalBytes: driveResp.Quota.Total,
		UsedBytes:  driveResp.Quota.Used,
		FreeBytes:  driveResp.Quota.Remaining,
	}, nil
}

// oneDriveItem represents a OneDrive item from the Graph API
type oneDriveItem struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Size             int64     `json:"size"`
	LastModifiedTime time.Time `json:"lastModifiedDateTime"`
	Folder           *struct{} `json:"folder,omitempty"`
	File             *struct {
		MimeType string `json:"mimeType"`
	} `json:"file,omitempty"`
	ParentReference *struct {
		Path string `json:"path"`
	} `json:"parentReference,omitempty"`
}

func (item *oneDriveItem) toFileMetadata() *FileMetadata {
	path := item.Name
	if item.ParentReference != nil && item.ParentReference.Path != "" {
		// Path looks like "/drive/root:/folder/subfolder"
		parentPath := strings.TrimPrefix(item.ParentReference.Path, "/drive/root:")
		parentPath = strings.TrimPrefix(parentPath, "/")
		if parentPath != "" {
			path = parentPath + "/" + item.Name
		}
	}

	mimeType := "application/octet-stream"
	if item.File != nil && item.File.MimeType != "" {
		mimeType = item.File.MimeType
	}

	return &FileMetadata{
		ID:       item.ID,
		Name:     item.Name,
		Path:     path,
		Size:     item.Size,
		MimeType: mimeType,
		ModTime:  item.LastModifiedTime,
		IsDir:    item.Folder != nil,
	}
}

// parseItemResponse parses a OneDrive item response
func (p *OneDriveProvider) parseItemResponse(body io.Reader) (*FileMetadata, error) {
	var item oneDriveItem
	if err := json.NewDecoder(body).Decode(&item); err != nil {
		return nil, fmt.Errorf("failed to decode item: %w", err)
	}
	return item.toFileMetadata(), nil
}
