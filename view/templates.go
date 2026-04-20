package view

import (
	"embed"
	"encoding/json"
	"html/template"
	"io"
	"io/fs"
	"net/http"

	"git.rwth-aachen.de/dsma/publications/software/konda/types"
	"github.com/google/uuid"
)

//go:embed *
var viewFS embed.FS

const (
	Layout = "layout.html"
)

var (
	// Utility
	index     = parsePage("index.html")
	notFound  = parsePage("not-found.html")
	errorComp = parseComponent("components/error-box.html")

	// Tool
	uploadDatasetTmpl      = parsePage("upload-dataset.html")
	findOntologyTmpl       = parsePage("find-ontology.html")
	entityRecognitionTmpl  = parsePage("entity-recognition.html")
	relationExtractionTmpl = parsePage("relation-extraction.html")
	ontologyMappingTmpl    = parsePage("ontology-mapping.html")
	knowledgeGraphTmpl     = parsePage("knowledge-graph.html")

	// Components
	searchDomainsComp = parseComponent("components/upload-dataset/search-domains.html")

	searchOntologiesComp = parseComponent("components/find-ontology/search-ontologies.html")

	entitiesComp    = parseComponent("components/entity-recognition/entity-rows.html")
	relationsComp   = parseComponent("components/relation-extraction/relation-rows.html")
	mappingsComp    = parseComponent("components/ontology-mapping/mapping-rows.html")
	addMappingComp  = parseComponent("components/ontology-mapping/add-mapping.html")
	searchTermsComp = parseComponent("components/ontology-mapping/search-terms.html")

	selectedItemsComp = parseComponent("components/selected-items.html")

	taskCardComp = parseComponent("components/task-card.html")

	codeBlockComp = parseComponent("components/code-block.html")
)

type ToolLayoutParams struct {
	Step int
}

// Index

func Index(w io.Writer) error {
	return index.ExecuteTemplate(w, Layout, nil)
}

// Utility Pages

func NotFound(w io.Writer) error {
	return notFound.ExecuteTemplate(w, Layout, nil)
}

func Empty(w io.Writer) error {
	w.Write([]byte(""))
	return nil
}

func Error(w io.Writer, data string) error {
	return errorComp.ExecuteTemplate(w, "error-box", data)
}

// Upload Dataset

type UploadDatasetParams struct {
	ToolLayoutParams
	Domain  string
	Remarks string
}

func UploadDataset(w io.Writer, params UploadDatasetParams) error {
	params.ToolLayoutParams = ToolLayoutParams{
		Step: 1,
	}
	return uploadDatasetTmpl.ExecuteTemplate(w, Layout, params)
}

func SearchDomainsComp(w http.ResponseWriter, data []string) error {
	return searchDomainsComp.ExecuteTemplate(w, "search-domains", data)
}

// Find Ontology

type FindOntologyParams struct {
	ToolLayoutParams
	FromScratch     bool
	Prefix          string
	Namespace       string
	ExtractionCount int
}

func FindOntology(w io.Writer, data FindOntologyParams) error {
	data.ToolLayoutParams = ToolLayoutParams{
		Step: 2,
	}
	return findOntologyTmpl.ExecuteTemplate(w, Layout, data)
}

type OntologySearchParams struct {
	OntologyName string
	Term         string
	Description  string
}

func OntologySearchComp(w http.ResponseWriter, data []OntologySearchParams) error {
	// Wrapping calls to execute template with data struct for type safety
	// w.Header().Set("Content-Type", "text/html")
	return searchOntologiesComp.ExecuteTemplate(w, "search-ontologies", data)
}

// Entity Recognition

func EntityRecognition(w io.Writer) error {
	data := ToolLayoutParams{
		Step: 3,
	}
	return entityRecognitionTmpl.ExecuteTemplate(w, Layout, data)
}

func EntitiesComp(w io.Writer, data []types.Entity) error {
	return entitiesComp.ExecuteTemplate(w, "entity-rows", data)
}

// Relation Extraction

func RelationExtraction(w io.Writer) error {
	data := ToolLayoutParams{
		Step: 4,
	}
	return relationExtractionTmpl.ExecuteTemplate(w, Layout, data)
}

type RelationExtractionParmas struct {
	Relations    []types.Relation
	EntityLabels []string
}

func RelationsComp(w io.Writer, data RelationExtractionParmas) error {
	return relationsComp.ExecuteTemplate(w, "relation-rows", data)
}

// Ontology Mapping

func OntologyMapping(w io.Writer) error {
	data := ToolLayoutParams{
		Step: 5,
	}
	return ontologyMappingTmpl.ExecuteTemplate(w, Layout, data)
}

type OntologyMappingParams struct {
	Rows    []OntologyMappingRow
	Adding  bool
	Partial bool
}

type OntologyMappingRow struct {
	ID          uuid.UUID
	RawLabel    string
	OntoTerm    string
	URI         string
	Description string
	Invert      bool
	Replace     bool
	IsRelation  bool
}

func OntologyMappingComp(w io.Writer, data OntologyMappingParams) error {
	return mappingsComp.ExecuteTemplate(w, "mapping-rows", data)
}

type AddOntologyMappingParams struct {
	Adding        bool
	AllLabels     []string
	SelectedLabel string
	Mapping       types.URIMatch
}

func AddOntologyMappingComp(w io.Writer, data AddOntologyMappingParams) error {
	return addMappingComp.ExecuteTemplate(w, "add-mapping", data)
}

type SearchTermParams struct {
	Mappings []types.URIMatch
	Adding   bool
}

func SearchTermsComp(w io.Writer, data SearchTermParams) error {
	return searchTermsComp.ExecuteTemplate(w, "search-terms", data)
}

// Knowledge Graph
type KnowledgeGraphParams struct {
	ToolLayoutParams
	Prefix         string
	Namespace      string
	KnowledgeGraph string
	Ontology       string
}

func KnowledgeGraph(w io.Writer, data KnowledgeGraphParams) error {
	data.ToolLayoutParams = ToolLayoutParams{
		Step: 6,
	}
	return knowledgeGraphTmpl.ExecuteTemplate(w, Layout, data)
}

// Common

type SelectedItemsParams struct {
	ID             string
	Name           string
	Endpoint       string
	ProcessingTime string
	Items          []SelectedItem
}

type SelectedItem struct {
	Title      string
	Subtitle   string
	Badge      string
	BadgeColor string
}

func SelectedItemsComp(w http.ResponseWriter, data SelectedItemsParams) error {
	return selectedItemsComp.ExecuteTemplate(w, "selected-items", data)
}

type TaskCardParams struct {
	Tasks []*types.Task
}

func TaskCardComp(w http.ResponseWriter, data TaskCardParams) error {
	return taskCardComp.ExecuteTemplate(w, "task-card", data)
}

// Knowledge Graph

func CodeBlockComp(w http.ResponseWriter, data string) error {
	return codeBlockComp.ExecuteTemplate(w, "code-block", data)
}

// Utilities

func parsePage(page string) *template.Template {
	baseTemplates := []string{
		"layout.html",
		"tool-layout.html",
	}

	componentTemplates, err := fs.Glob(viewFS, "components/*.html")
	if err != nil {
		panic(err)
	}
	pageTemplate := "pages/" + page

	allTemplates := append(baseTemplates, componentTemplates...)
	allTemplates = append(allTemplates, pageTemplate)

	return template.Must(
		template.New("layout.html").Funcs(Funcs).ParseFS(viewFS, allTemplates...))
}

func parseComponent(component string) *template.Template {
	componentTemplates, err := fs.Glob(viewFS, "components/*.html")
	if err != nil {
		panic(err)
	}

	allComponents := append(componentTemplates, component)
	return template.Must(
		template.New("component.html").Funcs(Funcs).ParseFS(viewFS, allComponents...))
}

// Example for custom functions
var (
	Funcs = template.FuncMap{
		"toJSON": toJSON,
		"args":   args,
		"plusOne": func(a int) int {
			return a + 1
		},
		"mul": func(a, b float64) float64 {
			return a * b
		},
	}
)

// Function to convert struct to JSON
func toJSON(data any) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}

func args(v ...any) []any {
	return v
}
