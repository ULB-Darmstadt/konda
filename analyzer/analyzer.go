package analyzer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"git.rwth-aachen.de/dsma/publications/software/konda/app"
	"git.rwth-aachen.de/dsma/publications/software/konda/chatter"
	azureOpenAI "git.rwth-aachen.de/dsma/publications/software/konda/chatter/azure_open_ai"
	"git.rwth-aachen.de/dsma/publications/software/konda/store"
	"git.rwth-aachen.de/dsma/publications/software/konda/types"
	"git.rwth-aachen.de/dsma/publications/software/konda/view"
	"git.rwth-aachen.de/dsma/publications/software/konda/web"
	"git.rwth-aachen.de/dsma/publications/software/konda/workspace"
	"github.com/joho/godotenv"
)

var (
	chat *chatter.AIChatter
)

func init() {
	initializeChatter()
}

type ContextSummaryParams struct {
	Domain  string
	Remarks string
	Context []FileContent
}

type DatasetSummaryParams struct {
	Domain         string
	ContextSummary string
	FileTree       string
	FileContent    []FileContent
}

func PerformContextSummary(sessionID string, a *app.App) error {
	workDir, err := a.GetWorkspace(sessionID)
	if err != nil {
		return err
	}
	domain, err := a.GetDomain(sessionID)
	if err != nil {
		return err
	}
	remarks, err := a.GetRemarks(sessionID)
	if err != nil {
		return err
	}
	context, err := ExtractFolder(workDir.ContextDir, FullContext, 150000)
	if err != nil {
		return err
	}

	var contextSummary string
	contextTemplPath := "static/data/prompt_templates/summarize_context_prompt.md"
	contextPromptParams := ContextSummaryParams{
		Domain:  domain,
		Remarks: remarks,
		Context: context,
	}
	err = prompt(&contextSummary, contextTemplPath, contextPromptParams, workDir)
	if err != nil {
		return err
	}

	return a.UpdateContextSummary(sessionID, func(_ string) string {
		return contextSummary
	})
}

func PerformDatasetSummary(sessionID string, a *app.App) error {
	workDir, err := a.GetWorkspace(sessionID)
	if err != nil {
		return err
	}
	domain, err := a.GetDomain(sessionID)
	if err != nil {
		return err
	}
	contextSummary, err := a.GetContextSummary(sessionID)
	if err != nil {
		return err
	}
	fileContent, err := ExtractFolder(workDir.DatasetDir, SampleOnly, 150000)
	if err != nil {
		return err
	}

	// TODO: move file tree to store, create if necessary
	fileTree, err := workspace.CreateFileTree(workDir.DatasetDir)
	if err != nil {
		return fmt.Errorf("error creating file tree: %w", err)
	}
	var datasetSummary string
	datasetTemplPath := "static/data/prompt_templates/summarize_dataset_prompt.md"
	datasetPromptParams := DatasetSummaryParams{
		Domain:         domain,
		ContextSummary: contextSummary,
		FileTree:       fileTree,
		FileContent:    fileContent,
	}
	err = prompt(&datasetSummary, datasetTemplPath, datasetPromptParams, workDir)
	if err != nil {
		return err
	}

	return a.UpdateDatasetSummary(sessionID, func(_ string) string {
		return datasetSummary
	})
}

func PerformOntologyUpload(sessionID string, a *app.App) error {
	ontos, err := a.GetSelectedOntologies(sessionID)
	if err != nil {
		return err
	}
	return a.WriteSelectedOntologies(sessionID, ontos)
}

func PerformOntologyEmbedding(sessionID string, a *app.App, progress types.ProgressCallback) error {
	return store.CreateEmbeddings(store.GetSessionOntoDB(sessionID), progress)
}

type NERPromptParams struct {
	Domain          string
	Remarks         string
	OntologyTerms   []types.URIMatch
	ContextSummary  string
	FileTree        string
	FileContent     []FileContent
	DatasetSummary  string
	ExtractionCount int
}

func PerformNER(sessionID string, a *app.App) error {
	workDir, err := a.GetWorkspace(sessionID)
	if err != nil {
		return err
	}
	domain, err := a.GetDomain(sessionID)
	if err != nil {
		return err
	}
	eCount, err := a.GetExtractionCount(sessionID)
	if err != nil {
		return err
	}
	remarks, err := a.GetRemarks(sessionID)
	if err != nil {
		return err
	}
	contextSummary, err := a.GetContextSummary(sessionID)
	if err != nil {
		return err
	}
	datasetSummary, err := a.GetContextSummary(sessionID)
	if err != nil {
		return err
	}
	fileTree, err := workspace.CreateFileTree(workDir.DatasetDir)
	if err != nil {
		return fmt.Errorf("error creating file tree: %w", err)
	}
	nTerms := 150
	fileContent, err := ExtractFolder(workDir.DatasetDir, SampleOnly, 150000)
	if err != nil {
		return err
	}

	contextTerms, err := store.SimilaritySearch(contextSummary, store.GetSessionOntoDB(sessionID), "classEmbeddings", nTerms)
	if err != nil {
		return fmt.Errorf("error searching for context ontology terms: %w", err)
	}
	datasetTerms, err := store.SimilaritySearch(datasetSummary, store.GetSessionOntoDB(sessionID), "classEmbeddings", nTerms)
	if err != nil {
		return fmt.Errorf("error searching for dataset ontology terms: %w", err)
	}
	terms := types.MergeMatches(nTerms, contextTerms, datasetTerms)

	for i := range terms {
		terms[i].URI = ""
	}

	var entities []types.Entity
	nerTemplPath := "static/data/prompt_templates/ner_prompt.md"
	nerPromptParams := NERPromptParams{
		Domain:          domain,
		Remarks:         remarks,
		OntologyTerms:   terms,
		ContextSummary:  contextSummary,
		FileTree:        fileTree,
		FileContent:     fileContent,
		DatasetSummary:  datasetSummary,
		ExtractionCount: eCount,
	}
	err = prompt(&entities, nerTemplPath, nerPromptParams, workDir)
	if err != nil {
		return err
	}

	return a.UpdateEntities(sessionID, func(_ []types.Entity) []types.Entity {
		return entities
	})
}

type REPromptParams struct {
	Domain            string
	Remarks           string
	OntologyRelations []types.URIMatch
	Entities          []string
	ContextSummary    string
	DatasetSummary    string
	ExtractionCount   int
}

func PerformRE(sessionID string, a *app.App) error {
	workDir, err := a.GetWorkspace(sessionID)
	if err != nil {
		return err
	}
	domain, err := a.GetDomain(sessionID)
	if err != nil {
		return err
	}
	eCount, err := a.GetExtractionCount(sessionID)
	if err != nil {
		return err
	}
	remarks, err := a.GetRemarks(sessionID)
	if err != nil {
		return err
	}
	entities, err := a.GetEntities(sessionID)
	if err != nil {
		return err
	}
	contextSummary, err := a.GetContextSummary(sessionID)
	if err != nil {
		return err
	}
	datasetSummary, err := a.GetContextSummary(sessionID)
	if err != nil {
		return err
	}
	nTerms := 150

	contextTerms, err := store.SimilaritySearch(contextSummary, store.GetSessionOntoDB(sessionID), "relationshipEmbeddings", nTerms)
	if err != nil {
		return fmt.Errorf("error searching for context relationships: %w", err)
	}
	datasetTerms, err := store.SimilaritySearch(datasetSummary, store.GetSessionOntoDB(sessionID), "relationshipEmbeddings", nTerms)
	if err != nil {
		return fmt.Errorf("error searching for dataset relationships: %w", err)
	}
	ontoRelations := types.MergeMatches(nTerms, contextTerms, datasetTerms)

	for i := range ontoRelations {
		ontoRelations[i].URI = ""
	}

	var entityList []string
	for _, e := range entities {
		entityList = append(entityList, e.Entity, e.Type)
	}

	var relations []types.Relation
	reTemplPath := "static/data/prompt_templates/re_prompt.md"
	rePromptParams := REPromptParams{
		Domain:            domain,
		Remarks:           remarks,
		OntologyRelations: ontoRelations,
		Entities:          entityList,
		ContextSummary:    contextSummary,
		DatasetSummary:    datasetSummary,
		ExtractionCount:   eCount,
	}
	err = prompt(&relations, reTemplPath, rePromptParams, workDir)
	if err != nil {
		return err
	}

	return a.UpdateRelations(sessionID, func(_ []types.Relation) []types.Relation {
		// iterate backwards to avoid slice shifting issues
		for i := len(relations) - 1; i >= 0; i-- {
			r := relations[i]
			if !slices.Contains(entityList, r.Subject) || !slices.Contains(entityList, r.Object) {
				relations = slices.Delete(relations, i, i+1)
			}
		}
		return relations
	})
}

type OntologyMappingParams struct {
	// TODO: handle from scratch differently? Maybe don't even do the mapping?
	// FromScratch   bool
	Domain          string
	ContextSummary  string
	DatasetSummary  string
	ClassMatches    []types.URIMatch
	RelationMatches []types.URIMatch
	Entities        []string
	Relations       []string
	FullRelations   []types.Relation
}

type OntologyMappingResponse map[string]types.URIMatch

// TODO: refactor the performxyz functions so that they return correct errors and can be cancelled
func PerformOntologyMapping(sessionID string, a *app.App, progress types.ProgressCallback) error {
	relations, err := a.GetRelations(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get relations: %w", err)
	}
	entities, err := a.GetEntities(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get entities: %w", err)
	}
	domain, err := a.GetDomain(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get domain: %w", err)
	}
	workDir, err := a.GetWorkspace(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}
	contextSummary, err := a.GetContextSummary(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get context summary: %w", err)
	}
	datasetSummary, err := a.GetDatasetSummary(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get dataset summary: %w", err)
	}
	ontoDB := store.GetSessionOntoDB(sessionID)

	chunkSize := 10
	nSearchResults := 4
	minSimilarityScore := 0.6

	chunks := chunkWithAssociations(entities, relations, chunkSize)
	matches := make([]types.URIMatchMap, 0, len(chunks))

	if progress != nil {
		progress(0.0, "Starting Ontology Mapping")
	}

	for i, chunk := range chunks {
		classMatchMap, relationMatchMap, err := getVectorDBMatches(
			chunk.Entities, chunk.Relations, ontoDB, minSimilarityScore, nSearchResults,
		)
		if err != nil {
			return fmt.Errorf("error retrieving vector DB matches: %w", err)
		}

		// Flatten and deduplicate matches
		uniqueClassMap := make(map[string]types.URIMatch)
		for _, matches := range classMatchMap {
			for _, match := range matches {
				uniqueClassMap[match.URI] = match
			}
		}
		var classMatches []types.URIMatch
		for _, v := range uniqueClassMap {
			classMatches = append(classMatches, v)
		}
		sortURIMatches(classMatches)

		uniqueRelationMap := make(map[string]types.URIMatch)
		for _, matches := range relationMatchMap {
			for _, match := range matches {
				uniqueRelationMap[match.URI] = match
			}
		}
		var relationMatches []types.URIMatch
		for _, v := range uniqueRelationMap {
			relationMatches = append(relationMatches, v)
		}
		sortURIMatches(relationMatches)

		// Prompt input
		var entityLabels []string
		for _, e := range chunk.Entities {
			entityLabels = append(entityLabels, e.Entity, e.Type)
		}
		entityLabels = app.UniqueStrings(entityLabels)
		var relationVerbs []string
		for _, r := range chunk.Relations {
			relationVerbs = append(relationVerbs, r.Verb)
		}
		relationVerbs = app.UniqueStrings(relationVerbs)
		contextPromptParams := OntologyMappingParams{
			Domain:          domain,
			ContextSummary:  contextSummary,
			DatasetSummary:  datasetSummary,
			ClassMatches:    classMatches,
			RelationMatches: relationMatches,
			Entities:        entityLabels,
			Relations:       relationVerbs,
			FullRelations:   chunk.Relations,
		}
		var chunkResponse OntologyMappingResponse
		promptTemplPath := "static/data/prompt_templates/ontology_mapping.md"
		err = prompt(&chunkResponse, promptTemplPath, contextPromptParams, workDir, i)
		if err != nil {
			return fmt.Errorf("ontology mapping prompt failed: %w", err)
		}

		chunkMatchMap := make(types.URIMatchMap)
		for k, v := range chunkResponse {
			if v.URI != "" && v.Label != "" {
				chunkMatchMap[k] = []types.URIMatch{v}
			}
		}

		matches = append(matches, chunkMatchMap)

		if progress != nil {
			ratio := float64(i+1) / float64(len(chunks))
			progress(ratio, fmt.Sprintf("Processed chunk %d of %d", i+1, len(chunks)))
		}
	}

	bestMatch := types.MergeMatchMaps(1, matches...)

	err = a.UpdateMappings(sessionID, func(_ types.URIMatchMap) types.URIMatchMap { return bestMatch })
	if err != nil {
		return fmt.Errorf("failed to update mappings: %w", err)
	}
	err = a.UpdateEntities(sessionID, func(_ []types.Entity) []types.Entity { return entities })
	if err != nil {
		return fmt.Errorf("failed to update entities: %w", err)
	}
	err = a.UpdateRelations(sessionID, func(_ []types.Relation) []types.Relation { return relations })
	if err != nil {
		return fmt.Errorf("failed to update relations: %w", err)
	}

	return nil
}

// sortURIMatches sorts a slice of URIMatch by Label, then by URI
func sortURIMatches(matches []types.URIMatch) {
	sort.Slice(matches, func(i, j int) bool {
		labelI := strings.ToLower(matches[i].Label)
		labelJ := strings.ToLower(matches[j].Label)

		if labelI == labelJ {
			return strings.ToLower(matches[i].URI) < strings.ToLower(matches[j].URI)
		}
		return labelI < labelJ
	})
}

func getVectorDBMatches(entities []types.Entity, relations []types.Relation, database string, minScore float64, nSearchResults int) (types.URIMatchMap, types.URIMatchMap, error) {
	// Get Class Matches
	classLabelsMap := make(map[string]struct{})
	for _, entity := range entities {
		classLabelsMap[entity.Entity] = struct{}{}
		classLabelsMap[entity.Type] = struct{}{}
	}
	for _, relation := range relations {
		classLabelsMap[relation.Subject] = struct{}{}
		classLabelsMap[relation.Object] = struct{}{}
	}
	var classLabels []string
	for label := range classLabelsMap {
		classLabels = append(classLabels, label)
	}
	classMatches, err := store.BulkSimilaritySearch(classLabels, database, "classEmbeddings", minScore, nSearchResults)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting similar identifiers: %w", err)
	}

	// Get Relation Matches
	relationLabelMap := make(map[string]struct{})
	for _, relation := range relations {
		relationLabelMap[relation.Verb] = struct{}{}
	}

	var relationLabels []string
	for verb := range relationLabelMap {
		relationLabels = append(relationLabels, verb)
	}
	relationMatches, err := store.BulkSimilaritySearch(relationLabels, database, "relationshipEmbeddings", minScore, nSearchResults)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting similar identifiers: %w", err)
	}

	return classMatches, relationMatches, nil

}

type chunk struct {
	Entities  []types.Entity
	Relations []types.Relation
}

// Function to chunk entities and their associated relations returning a copy of the original slices
func chunkWithAssociations(originalEntities []types.Entity, originalRelations []types.Relation, chunkSize int) []chunk {
	// work with copies where we can omit the suggested values
	entities := make([]types.Entity, len(originalEntities))
	for i, e := range originalEntities {
		entities[i] = types.Entity{
			Entity: e.Entity,
			Type:   e.Type,
		}
	}
	relations := make([]types.Relation, len(originalRelations))
	for i, r := range originalRelations {
		relations[i] = types.Relation{
			Subject: r.Subject,
			Verb:    r.Verb,
			Object:  r.Object,
		}
	}

	var chunks []chunk

	totalChunks := int(math.Ceil(float64(len(entities)) / float64(chunkSize)))

	for i := 0; i < totalChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(entities) {
			end = len(entities)
		}

		// Get the current chunk of entities
		entityChunk := entities[start:end]

		// Create lookup sets for entity names and types
		entitySet := make(map[string]struct{})
		typeSet := make(map[string]struct{})

		// Populate sets for quick lookup
		for _, entity := range entityChunk {
			entitySet[entity.Entity] = struct{}{}
			typeSet[entity.Type] = struct{}{}
		}

		var relationChunk []types.Relation
		for _, relation := range relations {
			// Check if the subject or object exists in entitySet
			_, subjectExistsInEntities := entitySet[relation.Subject]
			_, objectExistsInEntities := entitySet[relation.Object]

			// Check if the subject or object exists in typeSet
			_, subjectExistsInTypes := typeSet[relation.Subject]
			_, objectExistsInTypes := typeSet[relation.Object]

			// If any association is found, add to relationChunk
			if subjectExistsInEntities || objectExistsInEntities || subjectExistsInTypes || objectExistsInTypes {
				relationChunk = append(relationChunk, relation)
			}
		}

		// Store the chunk
		chunks = append(chunks, chunk{
			Entities:  entityChunk,
			Relations: relationChunk,
		})
	}

	return chunks
}

func PerformKnowledgeGraphCreation(sessionID string, a *app.App) error {
	datasetFile, err := a.GetDatasetFile(sessionID)
	if err != nil {
		return err
	}
	prefix, err := a.GetPrefix(sessionID)
	if err != nil {
		return err
	}
	namespace, err := a.GetNamespace(sessionID)
	if err != nil {
		return err
	}
	contextFiles, err := a.GetContextFiles(sessionID)
	if err != nil {
		return err
	}
	entities, err := a.GetEntities(sessionID)
	if err != nil {
		return err
	}
	relations, err := a.GetRelations(sessionID)
	if err != nil {
		return err
	}
	mappings, err := a.GetMappings(sessionID)
	if err != nil {
		return err
	}

	return store.CreateKnowledgeGraph(store.GetSessionDB(sessionID), namespace, prefix, datasetFile, contextFiles, entities, relations, mappings)
}

func initializeChatter() {
	if chat != nil {
		return
	}

	err := godotenv.Load()
	if err != nil {
		return
	}

	apiKey := os.Getenv("AOAI_CHAT_COMPLETIONS_API_KEY")
	model := os.Getenv("AOAI_CHAT_COMPLETIONS_MODEL")
	apiEndpoint := os.Getenv("AOAI_CHAT_COMPLETIONS_ENDPOINT")
	systemMessage := os.Getenv("AOAI_SYSTEM_MESSAGE")

	if apiKey == "" || model == "" || apiEndpoint == "" {
		fmt.Fprintln(os.Stderr, "Missing azure client environment variables")
		return
	}

	azureClient, err := azureOpenAI.NewAzureClient(apiEndpoint, apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating Azure client: %s", err)
		return
	}

	options := &chatter.ChatOptions{
		SystemMessage: systemMessage,
		Temperature:   0.0,
		Model:         model,
	}

	chat = chatter.NewAIChatter(azureClient, options)
}

func prompt[T any](result *T, templPath string, params any, workDir *workspace.Workspace, iteration ...int) error {
	// TODO: refactor this into templates.go
	templ, err := template.New(filepath.Base(templPath)).Funcs(view.Funcs).ParseFS(web.StaticDir, templPath)
	if err != nil {
		return fmt.Errorf("error parsing the template %q: %w", templPath, err)
	}

	var prompt bytes.Buffer

	err = templ.Execute(&prompt, params)
	if err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}

	resp, err := chat.Chat(prompt.String(), nil)
	if err != nil {
		return fmt.Errorf("error communicating with LLM: %w", err)
	}

	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")

	switch v := any(result).(type) {
	case *string:
		*v = resp
	default:
		err = json.Unmarshal([]byte(resp), result)
		if err != nil {
			resp += fmt.Sprintf("error unmarshalling response: %v\n", err)
		}
	}

	prompt.WriteString("\n\nResponse:\n\n")
	prompt.WriteString(resp)

	name := filepath.Base(templPath)
	if len(iteration) == 1 {
		name = strconv.Itoa(iteration[0]) + "_" + name
	}
	err = os.WriteFile(filepath.Join(workDir.MetadataDir, name), prompt.Bytes(), 0755)
	if err != nil {
		return fmt.Errorf("error writing prompt to file: %w", err)
	}
	return nil
}
