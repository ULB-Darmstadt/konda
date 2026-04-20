package types

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"sort"

	"github.com/google/uuid"
)

type FileKind int

const (
	Context FileKind = iota
	Dataset
)

func (k FileKind) String() string {
	switch k {
	case Context:
		return "CTX"
	case Dataset:
		return "DSET"
	default:
		return "UNKOWN"
	}
}

type UploadFile struct {
	FileName string
	Content  []byte
}

func ParseMultipartFiles(files []*multipart.FileHeader) ([]UploadFile, error) {
	var uploadFiles []UploadFile

	if len(files) == 0 {
		return nil, fmt.Errorf("no files provided")
	}

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return nil, err
		}
		defer file.Close()

		content, err := io.ReadAll(file) // Read file into []byte
		if err != nil {
			return nil, err
		}

		uploadFiles = append(uploadFiles, UploadFile{
			FileName: fileHeader.Filename,
			Content:  content,
		})
	}

	return uploadFiles, nil
}

func ConvOntoToUploadFile(ontos []Ontology) []UploadFile {
	var files []UploadFile
	for _, o := range ontos {
		if o.FileName == "" || o.Content == "" {
			fmt.Println("skipping", o.FileName, o.Content)
			continue
		}
		files = append(files, UploadFile{
			FileName: o.FileName,
			Content:  []byte(o.Content),
		})
	}
	return files
}

type Ontology struct {
	IRI           string
	OntologyName  string
	SearchTerm    string
	Description   string
	Source        string
	Content       string
	FileName      string
	NumberOfItems int
}

type Entity struct {
	Entity         string   `json:"entity"`
	Type           string   `json:"type"`
	SuggestedTypes []string `json:"suggested_types,omitempty"`
}

type Relation struct {
	Subject        string   `json:"subject"`
	Verb           string   `json:"verb"`
	Object         string   `json:"object"`
	SuggestedVerbs []string `json:"suggested_verbs,omitempty"`
}

type URIMatchMap map[string][]URIMatch

type URIMatch struct {
	ID          uuid.UUID `json:",omitzero"`
	Label       string    `json:"label"`
	Description string    `json:"description,omitzero"`
	URI         string    `json:"uri,omitzero"`
	Inverse     bool      `json:"inverse,omitzero"`
	Replace     bool      `json:"replace,omitzero"`
	Score       float64   `json:"-"`
}

func (m URIMatchMap) GetBestMatch(key string) URIMatch {
	if match, ok := m[key]; ok {
		if len(match) > 0 {
			return match[0]
		}
	}
	return URIMatch{}
}

func MergeMatchMaps(number int, maps ...URIMatchMap) URIMatchMap {
	combinedMap := make(URIMatchMap)

	for _, m := range maps {
		for key, values := range m {
			combinedMap[key] = append(combinedMap[key], values...)
		}
	}

	for key, values := range combinedMap {
		sort.Slice(values, func(i, j int) bool {
			return values[i].Score > values[j].Score // Higher scores come first
		})
		if len(values) > number {
			combinedMap[key] = values[:number]
		}
	}

	return combinedMap
}

func MergeMatches(number int, matchSlices ...[]URIMatch) []URIMatch {
	var allMatches []URIMatch
	for _, m := range matchSlices {
		allMatches = append(allMatches, m...)
	}

	// Sort all matches by score in descending order
	sort.SliceStable(allMatches, func(i, j int) bool {
		return allMatches[i].Score > allMatches[j].Score
	})

	// Use a map to track unique URIs
	uniqueMatches := make([]URIMatch, 0, number)
	seen := make(map[string]bool)

	for _, match := range allMatches {
		if !seen[match.URI] {
			seen[match.URI] = true
			uniqueMatches = append(uniqueMatches, match)
			if len(uniqueMatches) == number {
				break
			}
		}
	}

	return uniqueMatches
}

type TaskStatus string
type TaskType string

const (
	NotStarted TaskStatus = "not started"
	Queued     TaskStatus = "queued"
	Blocked    TaskStatus = "blocked"
	Running    TaskStatus = "running"
	Success    TaskStatus = "success"
	Error      TaskStatus = "error"
	Canceled   TaskStatus = "canceled"
	Stale      TaskStatus = "stale"

	TaskUpload                 TaskType = "Upload"
	TaskContextSummary         TaskType = "Context Summary"
	TaskDatasetSummary         TaskType = "Dataset Summary"
	TaskOntologyUpload         TaskType = "Ontology Upload"
	TaskOntologyEmbedding      TaskType = "Ontology Embedding"
	TaskNER                    TaskType = "Named Entity Recognition"
	TaskRE                     TaskType = "Relation Extraction"
	TaskOntologyMapping        TaskType = "Ontology Mapping"
	TaskKnowledgeGraphCreation TaskType = "Knowledge Graph Creation"
)

var TaskOrder = map[TaskType]int{
	TaskUpload:                 0,
	TaskContextSummary:         1,
	TaskDatasetSummary:         2,
	TaskOntologyUpload:         3,
	TaskOntologyEmbedding:      4,
	TaskNER:                    5,
	TaskRE:                     6,
	TaskOntologyMapping:        7,
	TaskKnowledgeGraphCreation: 8,
}

type TaskKey struct {
	SessionID string
	Type      TaskType
}

type Task struct {
	Key        TaskKey
	Status     TaskStatus
	Err        string
	CancelFunc context.CancelFunc `json:"-"`
	Progress   float64
	Message    string
	Dirty      bool
}

type ProgressCallback func(progress float64, message string)
