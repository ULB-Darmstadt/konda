package analyzer

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"

	"path/filepath"
	"sort"

	"github.com/gabriel-vasile/mimetype"
)

type ExtractionStrategy int

const (
	FullContext ExtractionStrategy = iota
	SampleOnly
)

type FileContent struct {
	FileName  string `json:"file_name"`
	Content   string `json:"content"`
	IsSampled bool   `json:"is_sampled"`
	Priority  int    `json:"-"`
}

// ExtractFolder processes files according to the given strategy and context limit.
func ExtractFolder(root string, strategy ExtractionStrategy, maxContextSize int) ([]FileContent, error) {
	var files []FileContent

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		mimeType, err := DetectMimeType(path)
		if err != nil {
			log.Printf("MIME detection error for %s: %v\n", path, err)
			return nil
		}

		files = append(files, FileContent{
			FileName: path,
			Priority: determinePriority(mimeType, info.Size()),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Priority > files[j].Priority
	})

	totalFiles := len(files)
	contextPerFile := maxContextSize
	if totalFiles > 0 {
		contextPerFile = maxContextSize / totalFiles
	}

	var usedContext int
	var result []FileContent

	for _, file := range files {
		sample := strategy == SampleOnly
		if strategy == FullContext && usedContext > int(float64(maxContextSize)*0.8) {
			sample = true
		}

		content, err := ExtractFile(file.FileName, sample, file.Priority, contextPerFile)
		if err != nil || content == nil {
			slog.Warn("Error extracting file:", "file", file.FileName, "details", err)
			continue
		}

		if strategy == FullContext && usedContext+len(content) > maxContextSize {
			// return error to user for context files
			return nil, fmt.Errorf("context limit exceeded: please remove some context files and try again")
		} else if usedContext+len(content) > maxContextSize {
			slog.Warn("Context limit reached skipping all files after:", "file", file.FileName)
			break
		}

		file.Content = string(content)
		file.IsSampled = sample
		result = append(result, file)
		usedContext += len(content)
	}

	return result, nil
}

func determinePriority(mime string, size int64) int {
	switch mime {
	case "application/pdf", "text/plain", "text/markdown",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.oasis.opendocument.text":
		return 100
	case "application/json", "application/x-yaml", "application/toml":
		if size < 2<<20 {
			return 80
		}
		return 50
	case "text/csv", "text/tsv":
		if size < 2<<20 {
			return 60
		}
		return 30
	default:
		return 10
	}
}

// ExtractFile dynamically truncates content based on file priority
func ExtractFile(filePath string, sample bool, priority, dynamicMaxLength int) ([]byte, error) {
	mimeType, err := DetectMimeType(filePath)
	if err != nil {
		return nil, fmt.Errorf("mime detection failed: %w", err)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed opening file: %w", err)
	}
	defer file.Close()

	var content []byte

	if processor, found := mimeProcessors[mimeType]; found {
		content, err = processor(file, sample)
		if err != nil {
			return nil, err
		}
	} else {
		content, err = extractFallback(file)
		if err != nil {
			return nil, err
		}
		slog.Warn("Using Fallback", "file", filePath)
	}

	if priority < 100 && len(content) > dynamicMaxLength {
		return enforceMaxSize(content, dynamicMaxLength), nil
	}

	return content, nil
}

func extractFallback(file *os.File) ([]byte, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	if IsBinary(data) {
		return nil, nil
	}
	return data, nil
}

func enforceMaxSize(data []byte, max int) []byte {
	if len(data) <= max {
		return data
	}
	return append(data[:max-3], []byte("...")...)
}

func IsBinary(data []byte) bool {
	if len(data) > 8000 {
		data = data[:8000]
	}
	return bytes.Contains(data, []byte{0})
}

func DetectMimeType(filePath string) (string, error) {
	// this tries to detect the mime type based on content
	mime, err := mimetype.DetectFile(filePath)
	if err != nil {
		return "", err
	}

	baseMime, _, _ := strings.Cut(mime.String(), ";")
	baseMime = strings.TrimSpace(baseMime)

	// overwrite mime type if not able to detect based on content
	if baseMime == "text/plain" {
		ext := strings.ToLower(filepath.Ext(filePath))
		switch ext {
		case ".json":
			baseMime = "application/json"
		case ".csv":
			baseMime = "text/csv"
		case ".tsv":
			baseMime = "text/tsv"
		case ".md":
			baseMime = "text/markdown"
		case ".pdf":
			baseMime = "application/pdf"
		}
	}

	return baseMime, nil
}
