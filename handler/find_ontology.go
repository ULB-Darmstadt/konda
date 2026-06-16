package handler

import (
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.rwth-aachen.de/dsma/publications/software/konda/analyzer"
	"git.rwth-aachen.de/dsma/publications/software/konda/app"
	"git.rwth-aachen.de/dsma/publications/software/konda/search"
	"git.rwth-aachen.de/dsma/publications/software/konda/store"
	"git.rwth-aachen.de/dsma/publications/software/konda/types"
	"git.rwth-aachen.de/dsma/publications/software/konda/view"
)

const (
	DURATION_PER_CLASS = 30 * time.Millisecond
)

func (h *Handlers) GetFindOntologyHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	analyzer.StartTaskOnce(sessionID, h.App, types.TaskContextSummary)
	analyzer.StartTaskOnce(sessionID, h.App, types.TaskDatasetSummary)

	fromScratch, err := h.App.GetFromScratch(sessionID)
	if err != nil {
		slog.Error("Error getting FromScratch param from store", "details", err)
	}
	eCount, err := h.App.GetExtractionCount(sessionID)
	if err != nil {
		slog.Error("Error getting extraction count from store", "details", err)
	}
	prefix, err := h.App.GetPrefix(sessionID)
	if err != nil {
		slog.Error("Error getting Prefix from store", "details", err)
	}
	if prefix == app.DEFAULT_PREFIX {
		prefix = ""
	}
	namespace, err := h.App.GetNamespace(sessionID)
	if err != nil {
		slog.Error("Error getting namespace from store", "details", err)
	}
	if namespace == app.DEFAULT_NAMESPACE {
		namespace = ""
	}

	params := view.FindOntologyParams{
		FromScratch:     fromScratch,
		Namespace:       namespace,
		Prefix:          prefix,
		ExtractionCount: eCount,
	}

	err = view.FindOntology(w, params)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) PostFindOntologyHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	needOntologyUpdate := false
	needNERUpdate := false

	// Extraction Count
	eCountStr := r.FormValue("extractionCount")
	eCount, err := strconv.Atoi(eCountStr)
	if err != nil {
		eCount = app.DEFAULT_EXTRACTION_COUNT
	}
	oldECount, err := h.App.GetExtractionCount(sessionID)
	if err != nil {
		slog.Error("Could not get extraction count", "details", err)
	}
	err = h.App.UpdateExtractionCount(sessionID, func(_ int) int { return eCount })
	if err != nil {
		slog.Error("Error updating extraction count", "details", err)
	}
	needNERUpdate = needNERUpdate || (oldECount != eCount)

	// Prefix
	prefix := r.FormValue("prefix")
	oldPrefix, err := h.App.GetPrefix(sessionID)
	if err != nil {
		slog.Error("Could not get prefix", "details", err)
	}
	err = h.App.UpdatePrefix(sessionID, func(_ string) string { return prefix })
	if err != nil {
		slog.Error("Error updating prefix", "details", err)
	}
	needNERUpdate = needNERUpdate || (oldPrefix != prefix && prefix != "")

	// Namespace
	namespace := r.FormValue("namespace")
	oldNamespace, err := h.App.GetNamespace(sessionID)
	if err != nil {
		slog.Error("Could not get namespace", "details", err)
	}
	err = h.App.UpdateNamespace(sessionID, func(_ string) string { return namespace })
	if err != nil {
		slog.Error("Error updating namespace", "details", err)
	}
	needNERUpdate = needNERUpdate || (oldNamespace != namespace && namespace != "")

	// FromScratch
	var fromScratch bool
	if r.FormValue("fromScratch") == "on" {
		fromScratch = true
	}
	oldFromScratch, err := h.App.GetFromScratch(sessionID)
	if err != nil {
		slog.Error("Could not get fromScratch", "details", err)
	}
	err = h.App.UpdateFromScratch(sessionID, func(_ bool) bool { return fromScratch })
	if err != nil {
		slog.Error("Could not update fromScratch", "details", err)
	}
	needNERUpdate = needNERUpdate || oldFromScratch != fromScratch

	// Ontology uploads
	selectedOntologies, err := h.App.GetSelectedOntologies(sessionID)
	if err != nil {
		slog.Error("Error getting selectedOntologies", "details", err)
		view.Error(w, "Error getting your selected ontologies")
		return
	}
	uploadedOntologies := convertUploadedToOntos(r.MultipartForm.File["ontology-file"])
	if len(uploadedOntologies) > 0 {
		needOntologyUpdate = true
		combined := append(selectedOntologies, uploadedOntologies...)
		err := h.App.UpdateSelectedOntologies(sessionID, func(_ []types.Ontology) []types.Ontology { return combined })
		if err != nil {
			slog.Error("Error updating selected ontologies", "details", err)
			view.Error(w, "Error updating selected ontologies")
			return
		}
	}

	selectedOntologiesDirty, err := h.App.GetSelectedOntologiesDirty(sessionID)
	if err != nil {
		slog.Error("Error getting selectedOntologiesDirty", "details", err)
	}
	needOntologyUpdate = needOntologyUpdate || selectedOntologiesDirty

	if needOntologyUpdate {
		err := h.App.UpdateSelectedOntologiesDirty(sessionID, func(_ bool) bool { return false })
		if err != nil {
			slog.Error("Error clearing selectedOntologiesDirty state", "details", err)
		}
		analyzer.StartTask(sessionID, h.App, types.TaskOntologyUpload)
		analyzer.StartTask(sessionID, h.App, types.TaskOntologyEmbedding)
	}
	if needNERUpdate {
		analyzer.StartTask(sessionID, h.App, types.TaskNER)
	}

	w.Header().Set("HX-Redirect", "/tool/entity-recognition")
}

func convertUploadedToOntos(fileHeaders []*multipart.FileHeader) []types.Ontology {
	var ontologies []types.Ontology
	for _, fh := range fileHeaders {
		file, err := fh.Open()
		if err != nil {
			continue
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			continue
		}

		ontologies = append(ontologies, types.Ontology{
			OntologyName: fh.Filename,
			Description:  "uploaded ontology",
			Source:       "UPL",
			FileName:     fh.Filename,
			Content:      string(content),
		})
	}
	return ontologies
}

func (h *Handlers) SearchOntologyHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	input := r.FormValue("ontology-search")

	searchResults, err := search.QueryForOntology(input)
	if err != nil {
		slog.Error("Error querying search api", "details", err)
		view.Error(w, "Error querying search API")
		return
	}

	err = h.App.UpdateSearchedOntologies(sessionID, func(_ []types.Ontology) []types.Ontology { return searchResults })
	if err != nil {
		slog.Error("Error updating SearchedOntologies", "details", err)
		view.Error(w, "Error updating SearchedOntologies")
		return
	}

	var params []view.OntologySearchParams
	for _, onto := range searchResults {
		params = append(params, view.OntologySearchParams{
			OntologyName: onto.OntologyName,
			Term:         onto.SearchTerm,
			Description:  onto.Description,
		})
	}

	err = view.OntologySearchComp(w, params)
	if err != nil {
		slog.Error("Error executing template", "details", err)
	}
}

func (h *Handlers) GetSelectedOntologiesHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	ontologies, err := h.App.GetSelectedOntologies(sessionID)
	if err != nil {
		slog.Error("Error getting SelectedOntologies", "details", err)
		view.Error(w, "Error getting SelectedOntologies")
		return
	}

	items := convertOntologyToSelectedItem("Ontologies", "/tool/find-ontology/selected-ontologies", ontologies)
	err = view.SelectedItemsComp(w, items)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) AddSelectedOntologiesHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	id, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		slog.Error("Error converting id to int", "details", err)
		view.Error(w, "Error converting adding that ontology")
		return
	}

	searchResults, err := h.App.GetSearchedOntologies(sessionID)
	if err != nil {
		slog.Error("Error getting SearchedOntologies", "details", err)
		view.Error(w, "Error searching for ontologies")
		return
	}
	err = h.App.AddSelectedOntology(sessionID, searchResults[id])
	if err != nil {
		slog.Error("Error adding SelectedOntologies", "details", err)
		view.Error(w, "Error adding that Ontology")
		return
	}
	err = h.App.UpdateSelectedOntologiesDirty(sessionID, func(_ bool) bool { return true })
	if err != nil {
		slog.Error("Error setting selectedOntologiesDirty flag", "details", err)
	}

	ontologies, err := h.App.GetSelectedOntologies(sessionID)
	if err != nil {
		slog.Error("Error getting SelectedOntologies", "details", err)
		view.Error(w, "Error getting the selected ontologies")
		return
	}

	items := convertOntologyToSelectedItem("Ontologies", "/tool/find-ontology/selected-ontologies", ontologies)
	if err := view.SelectedItemsComp(w, items); err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) DeleteSelectedOntologiesHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	deleteID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		slog.Error("Error converting id to int", "details", err)
		view.Error(w, "Error converting deleting that ontology")
		return
	}

	err = h.App.DeleteSelectedOntology(sessionID, deleteID)
	if err != nil {
		slog.Error("Could not delete selected ontology", "details", err)
		view.Error(w, "Could not delete ontology")
	}
	err = h.App.UpdateSelectedOntologiesDirty(sessionID, func(_ bool) bool { return true })
	if err != nil {
		slog.Error("Error setting selectedOntologiesDirty flag", "details", err)
	}

	ontologies, err := h.App.GetSelectedOntologies(sessionID)
	if err != nil {
		slog.Error("Error getting SelectedOntologies", "details", err)
		view.Error(w, "Error deleting Context File")
		return
	}
	items := convertOntologyToSelectedItem("Ontologies", "/tool/find-ontology/selected-ontologies", ontologies)
	err = view.SelectedItemsComp(w, items)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func convertOntologyToSelectedItem(name, endpoint string, ontologies []types.Ontology) view.SelectedItemsParams {
	var count int
	for _, onto := range ontologies {
		count += onto.NumberOfItems
	}
	processingTime := fmt.Sprintf("%v", time.Duration(count)*DURATION_PER_CLASS)
	params := view.SelectedItemsParams{
		ID:             strings.ReplaceAll(name, " ", ""),
		Name:           name,
		Endpoint:       endpoint,
		ProcessingTime: processingTime,
	}
	for _, onto := range ontologies {
		var color string
		switch onto.Source {
		case "TIB":
			color = "badge-primary"
		case "UPL":
			color = "badge-secondary"
		default:
			color = "badge-ghost"
		}
		params.Items = append(params.Items, view.SelectedItem{
			Title:      onto.OntologyName,
			Subtitle:   onto.IRI,
			Badge:      onto.Source,
			BadgeColor: color,
		})
	}
	return params
}
