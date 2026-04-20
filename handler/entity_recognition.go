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

func (h *Handlers) GetEntityRecognitionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	analyzer.StartTaskOnce(sessionID, h.App, types.TaskOntologyUpload)
	analyzer.StartTaskOnce(sessionID, h.App, types.TaskOntologyEmbedding)
	analyzer.StartTaskOnce(sessionID, h.App, types.TaskNER)

	err := view.EntityRecognition(w)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) PostEntityRecognitionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	err := setEntityFromForm(r, h.App)
	if err != nil {
		slog.Error("Error setting the entities from form", "details", err)
		view.Error(w, "Error setting the entities from form")
		return
	}
	needUpdate, err := h.App.GetEntitiesDirty(sessionID)
	if err != nil {
		slog.Error("Error getting entities dirty flag", "details", err)
	}
	if needUpdate {
		err = h.App.UpdateEntitiesDirty(sessionID, func(_ bool) bool { return false })
		if err != nil {
			slog.Error("Error clearing entityDirty state", "details", err)
		}
		analyzer.StartTask(sessionID, h.App, types.TaskRE)
	}
	w.Header().Set("HX-Redirect", "/tool/relation-extraction")
}

func (h *Handlers) GetEntitiesHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	data, err := h.App.GetEntities(sessionID)
	if err != nil {
		slog.Error("Error getting entities", "details", err)
		view.Error(w, "Error getting entities")
		return
	}
	err = view.EntitiesComp(w, data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) AddEntityHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())

	err := h.App.UpdateEntities(sessionID, func(entities []types.Entity) []types.Entity {
		return append(entities, types.Entity{})
	})
	if err != nil {
		slog.Error("Error adding entity", "details", err)
		view.Error(w, "Error adding entity")
		return
	}
	err = h.App.UpdateEntitiesDirty(sessionID, func(_ bool) bool { return true })
	if err != nil {
		slog.Error("Error setting entities dirty flag", "details", err)
	}

	data, err := h.App.GetEntities(sessionID)
	if err != nil {
		slog.Error("Error retrieving updated entity list", "details", err)
		view.Error(w, "Error retrieving updated entity list")
		return
	}

	if err := view.EntitiesComp(w, data); err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) DeleteEntityHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	id := r.PathValue("id")

	index, err := strconv.Atoi(id)
	if err != nil {
		slog.Error("Error converting id to int", "id", id, "error", err)
		view.Error(w, "Error deleting the entity")
		return
	}

	err = h.App.DeleteEntity(sessionID, index)
	if err != nil {
		slog.Error("Error deleting Entity", "details", err)
		view.Error(w, "Error deleting the entity")
		return
	}
	err = h.App.UpdateEntitiesDirty(sessionID, func(_ bool) bool { return true })
	if err != nil {
		slog.Error("Error setting entities dirty flag", "details", err)
	}
	data, err := h.App.GetEntities(sessionID)
	if err != nil {
		slog.Error("Error retrieving entities after delete", "details", err)
		view.Error(w, "Error retrieving entities after delete")
	}

	if err := view.EntitiesComp(w, data); err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) UpdateEntityHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	id := r.PathValue("id")
	needUpdate := false
	entityName := r.FormValue(fmt.Sprintf("entity-name-%s", id))
	entityType := r.FormValue(fmt.Sprintf("entity-type-%s", id))

	index, err := strconv.Atoi(id)
	if err != nil {
		slog.Error("Error converting id to int", "id", id, "error", err)
		view.Error(w, "Error updating entity")
		return
	}

	oldEntities, err := h.App.GetEntities(sessionID)
	if err != nil {
		slog.Error("Error retrieving updated entity list", "details", err)
		view.Error(w, "Error retrieving updated entity list")
		return
	}

	needUpdate = needUpdate || oldEntities[index].Entity != entityName
	needUpdate = needUpdate || oldEntities[index].Type != entityType

	if needUpdate {
		err = h.App.UpdateEntities(sessionID, func(entities []types.Entity) []types.Entity {
			if index >= 0 && index < len(entities) {
				entities[index].Entity = entityName
				entities[index].Type = entityType
			}
			return entities
		})
		if err != nil {
			slog.Error("Error updating entity", "details", err)
			view.Error(w, "Error updating entity")
			return
		}
		if err := h.App.UpdateEntitiesDirty(sessionID, func(_ bool) bool { return true }); err != nil {
			slog.Error("Error setting entities dirty flag", "details", err)
		}

	}
	entities, err := h.App.GetEntities(sessionID)
	if err != nil {
		slog.Error("Error retrieving updated entity list", "details", err)
		view.Error(w, "Error retrieving updated entity list")
		return
	}

	if err := view.EntitiesComp(w, entities); err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func setEntityFromForm(r *http.Request, app *app.App) error {
	sessionID := store.GetSessionID(r.Context())
	updated := false

	entities, err := app.GetEntities(sessionID)
	if err != nil {
		return err
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
		if index, ok := extractIndex(key, "entity-name-"); ok && index < len(entities) {
			if entities[index].Entity != val {
				entities[index].Entity = val
				updated = true
			}
		} else if index, ok := extractIndex(key, "entity-type-"); ok && index < len(entities) {
			if entities[index].Type != val {
				entities[index].Type = val
				updated = true
			}
		}
	}

	filtered := entities[:0]
	for _, e := range entities {
		if e.Entity == "" || e.Type == "" {
			continue
		}
		filtered = append(filtered, e)
	}
	entities = filtered

	if updated {
		if err := app.UpdateEntitiesDirty(sessionID, func(_ bool) bool { return true }); err != nil {
			return err
		}
	}

	return app.UpdateEntities(sessionID, func(_ []types.Entity) []types.Entity {
		return entities
	})

}
