package handler

import (
	"fmt"
	"log/slog"
	"net/http"

	"git.rwth-aachen.de/dsma/publications/software/konda/analyzer"
	"git.rwth-aachen.de/dsma/publications/software/konda/store"
	"git.rwth-aachen.de/dsma/publications/software/konda/types"
	"git.rwth-aachen.de/dsma/publications/software/konda/view"
)

func (h *Handlers) GetRunningTasksHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	tasks := getAndHandleActiveTasks(w, sessionID, h)

	err := view.TaskCardComp(w, view.TaskCardParams{Tasks: tasks})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) CancelTaskHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	taskType := types.TaskType(r.URL.Query().Get("type"))
	if !analyzer.IsValidTaskType(taskType) {
		slog.Error("Wrong task type", "type", taskType)
		view.Error(w, "Wrong task type")
		return
	}

	analyzer.CancelTask(sessionID, h.App, taskType)

	tasks := getAndHandleActiveTasks(w, sessionID, h)
	err := view.TaskCardComp(w, view.TaskCardParams{Tasks: tasks})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) RestartTaskHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	taskType := types.TaskType(r.URL.Query().Get("type"))
	if !analyzer.IsValidTaskType(taskType) {
		slog.Error("Wrong task type", "type", taskType)
		view.Error(w, "Wrong task type")
		return
	}

	analyzer.StartTask(sessionID, h.App, taskType)

	tasks := getAndHandleActiveTasks(w, sessionID, h)
	err := view.TaskCardComp(w, view.TaskCardParams{Tasks: tasks})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func getAndHandleActiveTasks(w http.ResponseWriter, sessionID string, h *Handlers) []*types.Task {
	dirty := false
	tasks := analyzer.ListTasks(sessionID)
	var active []*types.Task
	var failed []*types.Task
	for _, t := range tasks {
		if t.Status == types.Queued || t.Status == types.Running {
			active = append(active, t)
		}
		if t.Status == types.Error {
			failed = append(failed, t)
		}
		if t.Dirty {
			dirty = true
		}
	}

	if len(active) > 0 {
		w.Header().Set("HX-Trigger", "tasks-running")
	} else if dirty {
		analyzer.TasksDisplayed(sessionID, h.App)
		w.Header().Set("HX-Trigger", "tasks-complete")
	}

	return append(active, failed...)
	// return tasks
}
