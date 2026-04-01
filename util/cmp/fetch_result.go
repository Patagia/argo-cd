package cmp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FetchResultFile is the name of the optional JSON file a fetch-capable plugin may write to its
// working directory during the fetch or generate phase. ArgoCD reads it after generation and
// populates the corresponding fields in the manifest response so they appear in the UI.
const FetchResultFile = ".argocd-cmp-fetch-result.json"

// FetchResult is the structured response a fetch-capable plugin can return to ArgoCD by writing
// FetchResultFile to its working directory.
//
// Example written by the plugin's fetch command:
//
//	{
//	    "revision":     "sha256:abc123…",
//	    "verifyResult": "Verified OK\ncosign attestation: …",
//	    "metadata": {
//	        "version":     "1.2.3",
//	        "description": "My app",
//	        "createdAt":   "2024-01-01T00:00:00Z"
//	    }
//	}
type FetchResult struct {
	// Revision is the resolved revision of the fetched source (e.g. an OCI content digest
	// "sha256:…"). When non-empty ArgoCD uses this as the application revision shown in the UI
	// instead of the raw tag or revision string from the sync request.
	Revision string `json:"revision,omitempty"`

	// VerifyResult is the output of source verification performed by the plugin (e.g. the output
	// of cosign verify). When non-empty ArgoCD shows this in the UI as the verification result.
	VerifyResult string `json:"verifyResult,omitempty"`

	// Metadata contains optional human-readable metadata about the fetched source. ArgoCD caches
	// this and surfaces it in the UI (revision panel) in place of OCI image annotations. Any
	// fetch-capable plugin may populate this regardless of the underlying source type (OCI, S3,
	// HTTPS, etc.).
	Metadata *SourceMetadata `json:"metadata,omitempty"`
}

// SourceMetadata contains optional human-readable information about a fetched source. The fields
// mirror the OpenContainers image annotation conventions so they map naturally to OCI sources, but
// the struct is intentionally source-agnostic: plugins fetching from S3, HTTPS, or any other
// backend can populate whichever fields are meaningful for their source type.
type SourceMetadata struct {
	CreatedAt   string `json:"createdAt,omitempty"`
	Authors     string `json:"authors,omitempty"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	SourceURL   string `json:"sourceURL,omitempty"`
	DocsURL     string `json:"docsURL,omitempty"`
}

// ReadFetchResult reads FetchResultFile from appDir. Returns nil without error if the file does
// not exist — the file is optional. Returns an error only if the file exists but cannot be parsed.
func ReadFetchResult(appDir string) (*FetchResult, error) {
	path := filepath.Join(appDir, FetchResultFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("error reading %s: %w", FetchResultFile, err)
	}
	var result FetchResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", FetchResultFile, err)
	}
	return &result, nil
}
