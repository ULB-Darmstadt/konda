package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"git.rwth-aachen.de/dsma/publications/software/konda/analyzer"
	"git.rwth-aachen.de/dsma/publications/software/konda/app"
	"git.rwth-aachen.de/dsma/publications/software/konda/store"
	"git.rwth-aachen.de/dsma/publications/software/konda/types"
	"git.rwth-aachen.de/dsma/publications/software/konda/view"
)

func (h *Handlers) GetRelationExtractionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	analyzer.StartTaskOnce(sessionID, h.App, types.TaskRE)

	err := view.RelationExtraction(w)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) PostRelationExtractionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	err := setRelationsFromForm(r, h.App)
	if err != nil {
		slog.Error("Error setting relations from form", "details", err)
		view.Error(w, "Error setting relations from form")
		return
	}

	needUpdate, err := h.App.GetRelationsDirty(sessionID)
	if err != nil {
		slog.Error("Error getting relations dirty flag", "details", err)
	}

	if needUpdate {
		if err := h.App.UpdateRelationsDirty(sessionID, func(bool) bool { return false }); err != nil {
			slog.Error("Error clearing relation dirty flag", "details", err)
		}
		analyzer.StartTask(sessionID, h.App, types.TaskOntologyMapping)
	}

	w.Header().Set("HX-Redirect", "/tool/ontology-mapping")
}

func (h *Handlers) GetRelationsHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())

	relations, err := h.App.GetRelations(sessionID)
	if err != nil {
		slog.Error("Error getting relations", "details", err)
		view.Error(w, "Error getting relations")
		return
	}
	entityLabels, err := h.App.GetAllEntityLabels(sessionID)
	if err != nil {
		slog.Error("Error getting entity labels", "details", err)
		view.Error(w, "Error getting entity labels")
		return
	}
	params := view.RelationExtractionParmas{
		Relations:    relations,
		EntityLabels: entityLabels,
	}
	if err := view.RelationsComp(w, params); err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) AddRelationHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())

	err := h.App.UpdateRelations(sessionID, func(current []types.Relation) []types.Relation {
		return append(current, types.Relation{})
	})
	if err != nil {
		slog.Error("Error adding relation", "details", err)
		view.Error(w, "Error adding relation")
		return
	}

	if err := h.App.UpdateRelationsDirty(sessionID, func(bool) bool { return true }); err != nil {
		slog.Error("Error marking relations dirty", "details", err)
	}

	relations, err := h.App.GetRelations(sessionID)
	if err != nil {
		slog.Error("Error re-fetching relations", "details", err)
		view.Error(w, "Error re-fetching relations")
		return
	}
	entityLabels, err := h.App.GetAllEntityLabels(sessionID)
	if err != nil {
		slog.Error("Error getting entity labels", "details", err)
		view.Error(w, "Error getting entity labels")
		return
	}
	params := view.RelationExtractionParmas{
		Relations:    relations,
		EntityLabels: entityLabels,
	}
	if err := view.RelationsComp(w, params); err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) DeleteRelationHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	id := r.PathValue("id")

	index, err := strconv.Atoi(id)
	if err != nil {
		slog.Error("Error converting id to int", "id", id, "error", err)
		view.Error(w, "Error deleting relation")
		return
	}

	if err := h.App.DeleteRelation(sessionID, index); err != nil {
		slog.Error("Error deleting relation", "details", err)
		view.Error(w, "Error deleting relation")
		return
	}

	if err := h.App.UpdateRelationsDirty(sessionID, func(bool) bool { return true }); err != nil {
		slog.Error("Error marking relations dirty", "details", err)
	}

	relations, err := h.App.GetRelations(sessionID)
	if err != nil {
		slog.Error("Error re-fetching relations", "details", err)
		view.Error(w, "Error re-fetching relations")
		return
	}
	entityLabels, err := h.App.GetAllEntityLabels(sessionID)
	if err != nil {
		slog.Error("Error getting entity labels", "details", err)
		view.Error(w, "Error getting entity labels")
		return
	}
	params := view.RelationExtractionParmas{
		Relations:    relations,
		EntityLabels: entityLabels,
	}
	if err := view.RelationsComp(w, params); err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) UpdateRelationHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	id := r.PathValue("id")
	needUpdate := false
	index, err := strconv.Atoi(id)
	if err != nil {
		slog.Error("Error converting id to int", "id", id, "error", err)
		view.Error(w, "Error updating relation")
		return
	}

	subject := r.FormValue(fmt.Sprintf("relation-subject-%s", id))
	verb := r.FormValue(fmt.Sprintf("relation-verb-%s", id))
	object := r.FormValue(fmt.Sprintf("relation-object-%s", id))

	oldRelations, err := h.App.GetRelations(sessionID)
	if err != nil {
		slog.Error("Error re-fetching updated relations", "details", err)
		view.Error(w, "Error re-fetching updated relations")
		return
	}
	needUpdate = needUpdate || oldRelations[index].Subject != subject
	needUpdate = needUpdate || oldRelations[index].Verb != verb
	needUpdate = needUpdate || oldRelations[index].Object != object

	if needUpdate {
		err = h.App.UpdateRelations(sessionID, func(current []types.Relation) []types.Relation {
			if index < 0 || index >= len(current) {
				slog.Warn("Index out of bounds while updating relation", "index", index)
				return current
			}
			current[index].Subject = subject
			current[index].Verb = verb
			current[index].Object = object
			return current
		})
		if err != nil {
			slog.Error("Error updating relation", "details", err)
			view.Error(w, "Error updating relation")
			return
		}
		if err := h.App.UpdateRelationsDirty(sessionID, func(bool) bool { return true }); err != nil {
			slog.Error("Error marking relations dirty", "details", err)
		}
	}

	relations, err := h.App.GetRelations(sessionID)
	if err != nil {
		slog.Error("Error re-fetching updated relations", "details", err)
		view.Error(w, "Error re-fetching updated relations")
		return
	}
	entityLabels, err := h.App.GetAllEntityLabels(sessionID)
	if err != nil {
		slog.Error("Error getting entity labels", "details", err)
		view.Error(w, "Error getting entity labels")
		return
	}
	params := view.RelationExtractionParmas{
		Relations:    relations,
		EntityLabels: entityLabels,
	}
	if err := view.RelationsComp(w, params); err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func setRelationsFromForm(r *http.Request, app *app.App) error {
	sessionID := store.GetSessionID(r.Context())
	updated := false

	relations, err := app.GetRelations(sessionID)
	if err != nil {
		return fmt.Errorf("error getting original relations: %w", err)
	}

	if err := r.ParseForm(); err != nil {
		return fmt.Errorf("failed to parse form: %w", err)
	}

	extractIndex := func(key, prefix string) (int, bool) {
		if !strings.HasPrefix(key, prefix) {
			return 0, false
		}
		index, err := strconv.Atoi(strings.TrimPrefix(key, prefix))
		if err != nil {
			slog.Error("Could not convert index to number", "key", key, "error", err)
			return 0, false
		}
		return index, true
	}

	for key, values := range r.Form {
		if len(values) == 0 {
			continue
		}
		val := values[0]

		switch {
		case strings.HasPrefix(key, "relation-subject-"):
			if index, ok := extractIndex(key, "relation-subject-"); ok && index < len(relations) {
				updated = updated || relations[index].Subject != val
				relations[index].Subject = val
			}
		case strings.HasPrefix(key, "relation-verb-"):
			if index, ok := extractIndex(key, "relation-verb-"); ok && index < len(relations) {
				updated = updated || relations[index].Verb != val
				relations[index].Verb = val
			}
		case strings.HasPrefix(key, "relation-object-"):
			if index, ok := extractIndex(key, "relation-object-"); ok && index < len(relations) {
				updated = updated || relations[index].Object != val
				relations[index].Object = val
			}
		}
	}

	filtered := relations[:0]
	for _, r := range relations {
		if r.Subject == "" || r.Subject == "Select Label" || r.Verb == "" || r.Object == "" || r.Object == "Select Label" {
			continue
		}
		filtered = append(filtered, r)
	}
	relations = filtered

	if updated {
		if err := app.UpdateRelationsDirty(sessionID, func(bool) bool { return true }); err != nil {
			return fmt.Errorf("error setting relations dirty flag: %w", err)
		}
	}

	return app.UpdateRelations(sessionID, func(_ []types.Relation) []types.Relation {
		return relations
	})
}
