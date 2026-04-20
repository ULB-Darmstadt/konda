package handler

import (
	"cmp"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"git.rwth-aachen.de/dsma/publications/software/konda/analyzer"
	"git.rwth-aachen.de/dsma/publications/software/konda/app"
	"git.rwth-aachen.de/dsma/publications/software/konda/store"
	"git.rwth-aachen.de/dsma/publications/software/konda/types"
	"git.rwth-aachen.de/dsma/publications/software/konda/view"
	"github.com/google/uuid"
)

func (h *Handlers) GetOntologyMappingHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	analyzer.StartTaskOnce(sessionID, h.App, types.TaskOntologyMapping)

	err := view.OntologyMapping(w)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) PostOntologyMappingHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	err := setMappingsFromForm(r, h.App)
	if err != nil {
		slog.Error("Error setting mappings from form", "details", err)
		view.Error(w, "Error setting mappings from form")
		return
	}
	needUpdate, err := h.App.GetMappingsDirty(sessionID)
	if err != nil {
		slog.Error("Error getting mappings dirty flag", "details", err)
	}
	if needUpdate {
		err = h.App.UpdateMappingsDirty(sessionID, func(bool) bool { return false })
		if err != nil {
			slog.Error("Error clearing mappings dirty flag", "details", err)
		}
		analyzer.StartTask(sessionID, h.App, types.TaskKnowledgeGraphCreation)
	}
	w.Header().Set("HX-Redirect", "/tool/knowledge-graph")
}

func (h *Handlers) GetMappingsHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())

	mappingsMap, err := h.App.GetMappings(sessionID)
	if err != nil {
		slog.Error("Error getting mappings", "details", err)
		view.Error(w, "Error getting mappings")
		return
	}
	relations, err := h.App.GetRelations(sessionID)
	if err != nil {
		slog.Error("Error getting relations", "details", err)
		view.Error(w, "Error getting relations")
		return
	}

	err = view.OntologyMappingComp(w, view.OntologyMappingParams{
		Rows: uriMatchMapToOntologyMappingRow(mappingsMap, relations),
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) PostMappingHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())

	err := r.ParseForm()
	if err != nil {
		slog.Error("Failed to parse form", "details", err)
		view.Error(w, "Failed to parse form")
		return
	}

	mappingsMap, err := h.App.GetMappings(sessionID)
	if err != nil {
		slog.Error("Error getting mappings", "details", err)
		view.Error(w, "Error getting mappings")
		return
	}
	relations, err := h.App.GetRelations(sessionID)
	if err != nil {
		slog.Error("Error getting relations", "details", err)
		view.Error(w, "Error getting relations")
		return
	}

	label := r.FormValue("mapping-selected-label-add")
	if label == "" || label == "Select Label" {
		view.Error(w, "No label selected in new mapping")
		err = view.OntologyMappingComp(w, view.OntologyMappingParams{
			Rows: uriMatchMapToOntologyMappingRow(mappingsMap, relations),
		})
		if err != nil {
			slog.Error("Failed to execute ontology mapping component template", "details", err)
			view.Error(w, "Failed to execute ontology mapping component template")
			return
		}
		return
	}
	term := r.FormValue("mapping-term-add")
	uri := r.FormValue("mapping-uri-add")
	if term == "" || uri == "" {
		view.Error(w, "No ontology term selected in new mapping")
		err = view.OntologyMappingComp(w, view.OntologyMappingParams{
			Rows: uriMatchMapToOntologyMappingRow(mappingsMap, relations),
		})
		if err != nil {
			slog.Error("Failed to execute ontology mapping component template", "details", err)
			view.Error(w, "Failed to execute ontology mapping component template")
			return
		}
		return
	}

	description := r.FormValue("mapping-description-add")
	inverse := r.FormValue("mapping-checkbox-invert-add") == "on"
	replace := r.FormValue("mapping-checkbox-replace-add") == "on"

	err = h.App.UpdateMappings(sessionID, func(m types.URIMatchMap) types.URIMatchMap {
		m[label] = []types.URIMatch{{
			ID:          uuid.New(),
			Label:       term,
			URI:         uri,
			Description: description,
			Inverse:     inverse,
			Replace:     replace,
		}}
		return m
	})
	if err != nil {
		slog.Error("Error updating mapping", "details", err)
		view.Error(w, "Error updating mapping")
		return
	}
	err = h.App.UpdateMappingsDirty(sessionID, func(bool) bool { return true })
	if err != nil {
		slog.Error("Error updating mappings dirty flag", "details", err)
	}

	mappings, err := h.App.GetMappings(sessionID)
	if err != nil {
		slog.Error("Error getting the mappings", "details", err)
		view.Error(w, "Error getting the mappings")
		return
	}
	err = view.OntologyMappingComp(w, view.OntologyMappingParams{
		Rows: uriMatchMapToOntologyMappingRow(mappings, relations),
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing OntoMappingComp template: %v", err), http.StatusInternalServerError)
	}
	err = view.AddOntologyMappingComp(w, view.AddOntologyMappingParams{})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing AddOntologyMappingComp template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) UpdateMappingHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to parse form", "details", err)
		view.Error(w, "Failed to parse form")
		return
	}
	idStr := r.FormValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		slog.Error("Invalid UUID given", "IDStr", idStr)
		id = uuid.New()
	}

	uri := r.FormValue("uri")
	var userMatch types.URIMatch
	if uri != "" {
		userMatch, err = store.SearchNodeByURI(store.GetSessionOntoDB(sessionID), uri)
		if err != nil {
			slog.Error("could not find node with URI", "URI", uri, "details", err)
			view.Error(w, "Could not find any node with the provided URI")
			return
		}
	}

	inverse := r.FormValue("mapping-checkbox-invert-"+idStr) == "on"
	replace := r.FormValue("mapping-checkbox-replace-"+idStr) == "on"

	err = h.App.UpdateMappings(sessionID, func(m types.URIMatchMap) types.URIMatchMap {
		for k, matches := range m {
			for i, match := range matches {
				if match.ID == id {
					if userMatch != (types.URIMatch{}) {
						m[k][i] = userMatch
					}
					m[k][i].Inverse = inverse
					m[k][i].Replace = replace
					m[k][i].ID = id
				}
			}
		}
		return m
	})
	if err != nil {
		slog.Error("Error updating mappings", "details", err)
		view.Error(w, "Error updating mappings")
		return
	}
	if err := h.App.UpdateMappingsDirty(sessionID, func(bool) bool { return true }); err != nil {
		slog.Error("Error setting Mappings dirty flag", "details", err)
	}
	relations, err := h.App.GetRelations(sessionID)
	if err != nil {
		slog.Error("Error getting relations", "details", err)
		view.Error(w, "Error getting relations")
		return
	}

	key, mapping, err := h.App.GetMappingByID(sessionID, id)
	if err != nil {
		slog.Error("Error getting the mappings", "details", err)
		view.Error(w, "Error getting the mappings")
		return
	}
	err = view.OntologyMappingComp(w, view.OntologyMappingParams{
		Rows: []view.OntologyMappingRow{
			{
				ID:          mapping.ID,
				RawLabel:    key,
				OntoTerm:    mapping.Label,
				URI:         mapping.URI,
				Description: mapping.Description,
				Invert:      mapping.Inverse,
				Replace:     mapping.Replace,
				IsRelation:  isRelation(key, relations),
			},
		},
		Partial: true,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) DeleteMappingHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	id := r.PathValue("id")
	err := h.App.DeleteMapping(sessionID, id)
	if err != nil {
		slog.Error("Error deleting mapping", "details", err)
		view.Error(w, "Error deleting mapping")
		return
	}
	err = h.App.UpdateMappingsDirty(sessionID, func(bool) bool { return true })
	if err != nil {
		slog.Error("Error setting Mappings dirty flag", "details", err)
	}

	err = view.Empty(w)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) GetAddMappingHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())

	allLabels, err := getAllRawLabels(sessionID, h.App)
	if err != nil {
		slog.Error("Error getting raw labels", "details", err)
		view.Error(w, "Error getting raw labels")
		return
	}

	err = view.AddOntologyMappingComp(w, view.AddOntologyMappingParams{
		Adding:    true,
		AllLabels: allLabels,
		Mapping:   types.URIMatch{},
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) UpdateAddMappingHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	uri := r.FormValue("uri")
	selectedLabel := r.FormValue("mapping-selected-label-add")
	allLabels, err := getAllRawLabels(sessionID, h.App)
	if err != nil {
		slog.Error("Error getting raw labels", "details", err)
		view.Error(w, "Error getting raw labels")
		return
	}
	userMatch, err := store.SearchNodeByURI(store.GetSessionOntoDB(sessionID), uri)
	if err != nil {
		slog.Error("Could not find any node with that URI", "URI", uri, "details", err)
		view.Error(w, "Could not find any node with that URI")
		return
	}

	err = view.AddOntologyMappingComp(w, view.AddOntologyMappingParams{
		Adding:        true,
		AllLabels:     allLabels,
		SelectedLabel: selectedLabel,
		Mapping:       userMatch,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) DeleteAddMappingHandler(w http.ResponseWriter, r *http.Request) {
	err := view.AddOntologyMappingComp(w, view.AddOntologyMappingParams{})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) SearchOntologyTermsHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to parse form", "details", err)
		view.Error(w, "Failed to parse form")
		return
	}
	adding := false
	idStr := r.FormValue("mapping-input-search-term-id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		if idStr != "add" {
			slog.Error("Invalid UUID provided", "details", err)
			view.Error(w, "Invalid UUID provided")
			return
		}
		adding = true
	}
	searchTerm := r.FormValue("mapping-search-term-" + idStr)

	results, err := store.SimilaritySearch(searchTerm, store.GetSessionOntoDB(sessionID), "resourceEmbeddings", 10)
	if err != nil {
		slog.Error("Error searching neo4j database", "details", err)
		view.Error(w, "Error searching database")
		return
	}

	if !adding {
		for i := range results {
			results[i].ID = id
		}
	}

	err = view.SearchTermsComp(w, view.SearchTermParams{Mappings: results, Adding: adding})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func setMappingsFromForm(r *http.Request, app *app.App) error {
	sessionID := store.GetSessionID(r.Context())
	// Parse form values
	if err := r.ParseForm(); err != nil {
		return err
	}
	ids := r.Form["ids"]
	labels := r.Form["labels"]
	terms := r.Form["terms"]
	uris := r.Form["uris"]
	descriptions := r.Form["descriptions"]

	if len(ids) != len(labels) || len(labels) != len(terms) || len(terms) != len(uris) || len(uris) != len(descriptions) {
		return fmt.Errorf("not the same number of form fields")
	}

	return app.UpdateMappings(sessionID, func(m types.URIMatchMap) types.URIMatchMap {
		for i := range ids {
			id, err := uuid.Parse(ids[i])
			if err != nil {
				slog.Warn("Invalid UUID in form", "value", ids[i])
				continue
			}

			inverse, replace := false, false
			if matches, ok := m[labels[i]]; ok && len(matches) > 0 {
				inverse = matches[0].Inverse
				replace = matches[0].Replace
			}

			m[labels[i]] = []types.URIMatch{{
				ID:          id,
				Label:       terms[i],
				Description: descriptions[i],
				URI:         uris[i],
				Inverse:     inverse,
				Replace:     replace,
			}}
		}
		return m
	})
}

func uriMatchMapToOntologyMappingRow(mappingsMap types.URIMatchMap, relations []types.Relation) []view.OntologyMappingRow {
	var mappingRows []view.OntologyMappingRow
	for key, match := range mappingsMap {
		if len(match) == 0 {
			continue
		}
		mappingRows = append(mappingRows, view.OntologyMappingRow{
			ID:          match[0].ID,
			RawLabel:    key,
			OntoTerm:    match[0].Label,
			URI:         match[0].URI,
			Description: match[0].Description,
			Invert:      match[0].Inverse,
			Replace:     match[0].Replace,
			IsRelation:  isRelation(key, relations),
		})
	}
	slices.SortFunc(mappingRows, func(a, b view.OntologyMappingRow) int {
		return cmp.Compare(a.RawLabel, b.RawLabel)
	})
	return mappingRows
}

func getAllRawLabels(sessionID string, a *app.App) ([]string, error) {
	entities, err := a.GetEntities(sessionID)
	if err != nil {
		return nil, err
	}
	relations, err := a.GetRelations(sessionID)
	if err != nil {
		return nil, err
	}
	mappings, err := a.GetMappings(sessionID)
	if err != nil {
		return nil, err
	}

	var allLabels []string
	shouldInclude := func(label string) bool {
		mappedValue, exists := mappings[label]
		return !exists || len(mappedValue) == 0
	}

	for _, entity := range entities {
		if shouldInclude(entity.Entity) {
			allLabels = append(allLabels, entity.Entity)
		}
		if shouldInclude(entity.Type) {
			allLabels = append(allLabels, entity.Type)
		}
	}
	for _, relation := range relations {
		if shouldInclude(relation.Subject) {
			allLabels = append(allLabels, relation.Subject)
		}
		if shouldInclude(relation.Verb) {
			allLabels = append(allLabels, relation.Verb)
		}
		if shouldInclude(relation.Object) {
			allLabels = append(allLabels, relation.Object)
		}
	}

	allLabels = app.UniqueStrings(allLabels)
	slices.Sort(allLabels)

	return allLabels, nil
}

func isRelation(label string, relations []types.Relation) bool {
	for _, rel := range relations {
		if label == rel.Verb {
			return true
		}
	}
	return false
}
