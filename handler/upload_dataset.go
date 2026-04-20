package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"git.rwth-aachen.de/dsma/publications/software/konda/analyzer"
	"git.rwth-aachen.de/dsma/publications/software/konda/store"
	"git.rwth-aachen.de/dsma/publications/software/konda/types"
	"git.rwth-aachen.de/dsma/publications/software/konda/view"
	"git.rwth-aachen.de/dsma/publications/software/konda/web"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

const (
	CONTEXT_FILES = "Context Files"
	DATASET_FILES = "Dataset Files"
	ENDPOINT      = "/tool/upload-dataset/"
)

var allDomains = getDomains()

func (h *Handlers) GetUploadDatasetHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	domain, err := h.App.GetDomain(sessionID)
	if err != nil {
		slog.Error("Unable to get domain", "details", err)
	}
	remarks, err := h.App.GetRemarks(sessionID)
	if err != nil {
		slog.Error("Unable to get remarks", "details", err)
	}

	data := view.UploadDatasetParams{
		Domain:  domain,
		Remarks: remarks,
	}
	err = view.UploadDataset(w, data)
	if err != nil {
		slog.Error("Error executing template", "details", err)
	}
}

func (h *Handlers) PostUploadDatasetHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	needUpdate := false

	const maxUploadSize = int64(500 << 20)
	err := r.ParseMultipartForm(maxUploadSize)
	if err != nil {
		view.Error(w, "Error parsing form data, please try again")
		return
	}
	// Check file sizes
	files := r.MultipartForm.File["context_files"]
	files = append(files, r.MultipartForm.File["dataset"]...)
	for _, fileHeader := range files {
		if fileHeader.Size > maxUploadSize {
			view.Error(w, "File size exceeds 500 MB limit")
			return
		}
	}

	// Domain
	domain := r.FormValue("domain-search")
	oldDomain, err := h.App.GetDomain(sessionID)
	if err != nil {
		slog.Error("Could not get domain", "details", err)
	}
	err = h.App.UpdateDomain(sessionID, func(string) string { return domain })
	if err != nil {
		slog.Error("Could not update domain", "details", err)
	}
	needUpdate = needUpdate || oldDomain != domain

	// Remarks
	remarks := r.FormValue("remarks")
	oldRemarks, err := h.App.GetRemarks(sessionID)
	if err != nil {
		slog.Error("Could not get remarks", "details", err)
	}
	err = h.App.UpdateRemarks(sessionID, func(string) string { return remarks })
	if err != nil {
		slog.Error("Could not update remarks", "details", err)
	}
	needUpdate = needUpdate || oldRemarks != remarks

	contextFiles := r.MultipartForm.File["context_files"]
	datasetFiles := r.MultipartForm.File["dataset"]

	prevCFiles, err := h.App.GetContextFiles(sessionID)
	if err != nil {
		slog.Error("Could not get context files", "details", err)
	}
	prevDFile, err := h.App.GetDatasetFile(sessionID)
	if err != nil {
		slog.Error("Could not get dataset file", "details", err)
	}
	needsContext := len(prevCFiles) == 0
	needsDataset := prevDFile == ""

	if needsContext && len(contextFiles) == 0 {
		view.Error(w, "Please upload at least one context file")
		return
	}
	if needsDataset && len(datasetFiles) == 0 {
		view.Error(w, "Please upload a dataset")
		return
	}
	needUpdate = needUpdate || needsContext || needsDataset

	// Skip the rest if no updates
	if len(contextFiles) == 0 && len(datasetFiles) == 0 && !needUpdate {
		w.Header().Set("HX-Redirect", "/tool/find-ontology")
		return
	}
	// Ensure Workspace
	_, err = h.App.EnsureWorkspace(sessionID)
	if err != nil {
		slog.Error("Error ensuring workspace", "details", err)
		view.Error(w, "Internal Server Error: We could not ensure a workspace for your files")
		return
	}

	// Process Context Files
	if len(contextFiles) > 0 {
		cFiles, err := types.ParseMultipartFiles(contextFiles)
		if err != nil {
			slog.Error("Failed to parse context files", "details", err)
			view.Error(w, "We ran into an issue parsing your context files, please try again")
			return
		}
		err = h.App.SetContextFiles(sessionID, cFiles)
		if err != nil {
			slog.Error("Failed to save context files", "details", err)
			view.Error(w, "We ran into an issue saving your context files, please try again")
			return
		}
	}

	// Process Dataset File
	if len(datasetFiles) == 1 {
		dFiles, err := types.ParseMultipartFiles(datasetFiles)
		if err != nil {
			slog.Error("Failed to parse dataset file", "details", err)
			view.Error(w, "We ran into an issue parsing your dataset, please try again")
			return
		}
		err = h.App.SetDatasetFile(sessionID, dFiles[0])
		if err != nil {
			slog.Error("Failed to save dataset file", "details", err)
			view.Error(w, "We ran into an issue saving your dataset, please try again")
			return
		}
	}

	analyzer.StartTask(sessionID, h.App, types.TaskUpload)
	analyzer.StartTaskOnce(sessionID, h.App, types.TaskContextSummary)
	analyzer.StartTaskOnce(sessionID, h.App, types.TaskDatasetSummary)

	w.Header().Set("HX-Redirect", "/tool/find-ontology")
}

func (h *Handlers) GetDomainsHandler(w http.ResponseWriter, r *http.Request) {
	searchTerm := r.URL.Query().Get("domain-search")
	matches := fuzzy.RankFindFold(searchTerm, allDomains)
	sort.Sort(matches)

	var strMatches []string
	for _, m := range matches {
		strMatches = append(strMatches, m.Target)
	}

	err := view.SearchDomainsComp(w, strMatches)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) GetUploadedContextFilesHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())

	files, err := h.App.GetContextFiles(sessionID)
	if err != nil {
		slog.Error("Could not get context files", "details", err)
		view.Error(w, "Could not get context files, try to reload the page")
		return
	}

	params := convFilesToParams(CONTEXT_FILES, ENDPOINT+"uploaded-context-files", types.Context, files)
	if err := view.SelectedItemsComp(w, params); err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) DeleteUploadedContextFilesHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())

	idStr := r.PathValue("id")
	index, err := strconv.Atoi(idStr)
	if err != nil {
		slog.Error("Invalid index for context file deletion", "id", idStr)
		view.Error(w, "Error deleting Context File")
		return
	}

	if err := h.App.DeleteContextFile(sessionID, index); err != nil {
		slog.Error("Failed to delete context file", "details", err)
		view.Error(w, "Error deleting Context File")
		return
	}

	files, err := h.App.GetContextFiles(sessionID)
	if err != nil {
		slog.Error("Failed to get updated context files", "details", err)
		view.Error(w, "Error getting Context Files")
		return
	}

	params := convFilesToParams(CONTEXT_FILES, ENDPOINT+"uploaded-context-files", types.Context, files)
	if err := view.SelectedItemsComp(w, params); err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) GetUploadedDatasetFilesHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())

	datasetFile, err := h.App.GetDatasetFile(sessionID)
	if err != nil {
		slog.Error("Could not get dataset file", "details", err)
		view.Error(w, "Error getting dataset file")
		return
	}

	var files []string
	if datasetFile != "" {
		files = append(files, datasetFile)
	}

	params := convFilesToParams(DATASET_FILES, ENDPOINT+"uploaded-dataset-files", types.Dataset, files)
	if err := view.SelectedItemsComp(w, params); err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) DeleteUploadedDatasetFilesHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())

	if err := h.App.DeleteDatasetFile(sessionID); err != nil {
		slog.Error("Failed to delete dataset file", "details", err)
		view.Error(w, "Failed to delete dataset file")
		return
	}

	datasetFile, err := h.App.GetDatasetFile(sessionID)
	if err != nil {
		slog.Error("Failed to fetch dataset file after deletion", "details", err)
		view.Error(w, "Failed to fetch dataset file after deletion")
		return
	}

	var files []string
	if datasetFile != "" {
		files = append(files, datasetFile)
	}

	params := convFilesToParams(DATASET_FILES, ENDPOINT+"uploaded-dataset-files", types.Dataset, files)
	if err := view.SelectedItemsComp(w, params); err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func convFilesToParams(name, endpoint string, kind types.FileKind, fileNames []string) view.SelectedItemsParams {
	params := view.SelectedItemsParams{
		ID:       strings.ReplaceAll(name, " ", ""),
		Name:     name,
		Endpoint: endpoint,
	}

	for _, name := range fileNames {
		color := "badge-ghost"
		switch kind {
		case types.Context:
			color = "badge-primary"
		case types.Dataset:
			color = "badge-secondary"
		}

		params.Items = append(params.Items, view.SelectedItem{
			Title:      kind.String(),
			Subtitle:   name,
			Badge:      kind.String(),
			BadgeColor: color,
		})
	}
	return params
}

// getDomains gets executed at startup and panics if not all required files are available.
func getDomains() []string {
	data, err := web.StaticDir.ReadFile("static/data/domains.json")
	if err != nil {
		panic("Could not read domains file")
	}
	var domains []string
	if err := json.Unmarshal(data, &domains); err != nil {
		panic("Could not unmarshal domains")
	}
	return domains
}
