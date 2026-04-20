package workspace

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"git.rwth-aachen.de/dsma/publications/software/konda/types"
)

const (
	// TODO: change name to session workspaces, here and in air.toml
	WORKSPACES_DIR = "./tmp-workspaces/"
	CONTEXT        = "context_files"
	DATASET        = "dataset"
	ONTOLOGY       = "ontology"
	METADATA       = "metadata"
	METADATA_FILE  = "workspace_metadata.json"
)

type CleanUpFunc func()

type Workspace struct {
	ID          string `json:"id"`
	ContextDir  string `json:"contextDir"`
	DatasetDir  string `json:"datasetDir"`
	OntologyDir string `json:"ontologyDir"`
	MetadataDir string `json:"metadataDir"`
	CleanUp     func() `json:"-"`
}

func (w Workspace) IsZero() bool {
	return w.ID == "" &&
		w.ContextDir == "" &&
		w.DatasetDir == "" &&
		w.OntologyDir == "" &&
		w.MetadataDir == ""
}

func CreateWorkspace(sessionID string) (*Workspace, error) {
	tmpDir := filepath.Join(WORKSPACES_DIR, sessionID)

	err := os.MkdirAll(tmpDir, 0755)
	if err != nil {
		fmt.Println("Error creating temporary directory:", err)
		return nil, err
	}

	contextDir := filepath.Join(tmpDir, CONTEXT)
	err = os.MkdirAll(contextDir, 0755)
	if err != nil {
		fmt.Println("Error creating context directory:", err)
		return nil, err
	}

	datasetDir := filepath.Join(tmpDir, DATASET)
	err = os.MkdirAll(datasetDir, 0755)
	if err != nil {
		fmt.Println("Error creating dataset directory:", err)
		return nil, err
	}

	ontologyDir := filepath.Join(tmpDir, ONTOLOGY)
	err = os.MkdirAll(ontologyDir, 0755)
	if err != nil {
		fmt.Println("Error creating context directory:", err)
		return nil, err
	}

	metadataDir := filepath.Join(tmpDir, METADATA)
	err = os.MkdirAll(metadataDir, 0755)
	if err != nil {
		fmt.Println("Error creating metadata directory:", err)
		return nil, err
	}

	cleanUp := func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			fmt.Printf("Error removing temporary directory: %s", err)
		}
		fmt.Println("Temporary directory removed.")
	}

	workspace := Workspace{
		ID:          sessionID,
		ContextDir:  contextDir,
		DatasetDir:  datasetDir,
		OntologyDir: ontologyDir,
		MetadataDir: metadataDir,
		CleanUp:     cleanUp,
	}

	return &workspace, nil
}

func CleanUpAllWorkspaces() error {
	err := os.RemoveAll(WORKSPACES_DIR)
	if err != nil {
		return err
	}
	return nil
}

func CleanUpWorkspace(id string) error {
	path := filepath.Join(WORKSPACES_DIR, id)

	err := os.RemoveAll(path)
	if err != nil {
		return err
	}
	return nil
}

func CleanUpWorkDir(workdir string) error {
	err := os.RemoveAll(workdir)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(workdir, 0755); err != nil {
		return err
	}

	return nil
}

func CreateFileTree(root string) (string, error) {
	var builder strings.Builder

	// Get the absolute path of the root directory
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}
	rootParent := filepath.Base(absRoot)

	// Write the root folder name without leading pipes
	builder.WriteString(rootParent + "/\n")

	// Track open levels for drawing pipes
	levelOpen := map[int]bool{}

	// Walk through the directory structure
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error walking directory %s: %w", path, err)
		}

		// Skip the root directory itself since it's already printed
		if path == root {
			return nil
		}

		// Calculate the relative path and depth
		relPath, err := filepath.Rel(root, path)
		// fmt.Println(relPath)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}
		depth := strings.Count(relPath, string(filepath.Separator)) + 1

		// Get the parent directory
		parentPath := filepath.Dir(path)
		relParentPath, err := filepath.Rel(root, parentPath)
		// fmt.Println(relParentPath)
		if err != nil {
			return fmt.Errorf("failed to get relative parent path: %w", err)
		}
		parentDepth := strings.Count(relParentPath, string(filepath.Separator))

		// Close previous levels when transitioning out of directories
		// fmt.Println("parentDepth: ", parentDepth, " depth: ", depth)
		// fmt.Println("parentPath: ", parentPath, "\npath: ", path)
		// fmt.Println("-----------------")
		if depth <= parentDepth {
			// fmt.Println("Inside if: ", parentDepth, depth)
			// fmt.Println("-----------------")
			levelOpen[depth-1] = false
		}

		// Build the line for the current item
		var line strings.Builder
		for i := 0; i < depth-1; i++ {
			if levelOpen[i] {
				line.WriteString("│   ") // Pipe to show siblings
			} else {
				line.WriteString("    ") // Empty space
			}
		}

		// Add the current item
		if d.IsDir() {
			line.WriteString("├── ")
			line.WriteString(d.Name() + "/\n")

			// Mark this level as open for potential siblings
			levelOpen[depth] = true
		} else {
			line.WriteString("├── ")
			line.WriteString(d.Name() + "\n")
		}

		// Append the line to the builder
		builder.WriteString(line.String())
		return nil
	})

	if err != nil {
		return "", err
	}

	return builder.String(), nil
}

// IsZip checks if the given byte slice is a valid ZIP file
func IsZip(data []byte) bool {
	// Check the magic number for ZIP files (0x50 0x4B for PK)
	return len(data) > 4 && data[0] == 0x50 && data[1] == 0x4B
}

func Unzip(data []byte, outputDir string) error {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("failed to create ZIP reader: %w", err)
	}

	for _, f := range archive.File {
		filePath := filepath.Join(outputDir, f.Name)

		if !strings.HasPrefix(filePath, filepath.Clean(outputDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", filePath)
		}
		if f.FileInfo().IsDir() {
			err = os.MkdirAll(filePath, os.ModePerm)
			if err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return fmt.Errorf("failed to create parent dir path: %w", err)
		}

		dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return fmt.Errorf("failed to open newly created destination file: %w", err)
		}

		fileInArchive, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open zip file entry: %w", err)
		}

		if _, err := io.Copy(dstFile, fileInArchive); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}

		dstFile.Close()
		fileInArchive.Close()
	}
	return nil
}

func SaveWorkspaceMetadata(workspace *Workspace) error {
	metadataJson, err := json.Marshal(workspace)
	if err != nil {
		return fmt.Errorf("unable to marshal metadata: %v", err)
	}

	if err := os.WriteFile(filepath.Join(workspace.MetadataDir, METADATA_FILE), metadataJson, 0755); err != nil {
		return fmt.Errorf("unable to marshal metadata: %v", err)
	}

	return nil
}

func GetWorkspaceMetadata(id string) (*Workspace, error) {
	metadataJson, err := os.ReadFile(filepath.Join(WORKSPACES_DIR, METADATA, METADATA_FILE))
	if err != nil {
		return nil, fmt.Errorf("unable to read metadata file: %v", err)
	}

	var workspace Workspace
	err = json.Unmarshal(metadataJson, &workspace)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal metadata file: %v", err)
	}

	return &workspace, nil
}

func SaveFilesToWorkspace(workDir string, uploadFiles []types.UploadFile) error {
	for _, uf := range uploadFiles {
		filePath := filepath.Join(workDir, uf.FileName)

		err := os.WriteFile(filePath, uf.Content, 0644)
		if err != nil {
			return err
		}
	}
	return nil
}

func DeleteFileFromWorkspace(workDir string, fileName string) error {
	if fileName == "" {
		return nil
	}
	err := os.Remove(filepath.Join(workDir, fileName))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ListFilesInDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("error reading directory %s: %w", dir, err)
	}

	var fileNames []string
	for _, entry := range entries {
		if !entry.IsDir() {
			fileNames = append(fileNames, entry.Name())
		}
	}

	return fileNames, nil
}
