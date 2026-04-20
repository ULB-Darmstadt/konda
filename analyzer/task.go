package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"git.rwth-aachen.de/dsma/publications/software/konda/app"
	"git.rwth-aachen.de/dsma/publications/software/konda/store"
	"git.rwth-aachen.de/dsma/publications/software/konda/types"
)

type TaskManager struct {
	mu    sync.RWMutex
	tasks map[types.TaskKey]*types.Task
}
type TaskFunc func(sessionID string, app *app.App) error

var (
	manager = &TaskManager{
		tasks: make(map[types.TaskKey]*types.Task),
	}

	dependencies = map[types.TaskType][]types.TaskType{
		types.TaskUpload:                 {},
		types.TaskContextSummary:         {types.TaskUpload},
		types.TaskDatasetSummary:         {types.TaskUpload, types.TaskContextSummary},
		types.TaskOntologyUpload:         {},
		types.TaskOntologyEmbedding:      {types.TaskOntologyUpload},
		types.TaskNER:                    {types.TaskContextSummary, types.TaskDatasetSummary, types.TaskOntologyUpload, types.TaskOntologyEmbedding},
		types.TaskRE:                     {types.TaskContextSummary, types.TaskDatasetSummary, types.TaskOntologyUpload, types.TaskOntologyEmbedding, types.TaskNER},
		types.TaskOntologyMapping:        {types.TaskContextSummary, types.TaskDatasetSummary, types.TaskOntologyUpload, types.TaskOntologyEmbedding, types.TaskNER, types.TaskRE},
		types.TaskKnowledgeGraphCreation: {types.TaskContextSummary, types.TaskDatasetSummary, types.TaskOntologyUpload, types.TaskOntologyEmbedding, types.TaskNER, types.TaskRE, types.TaskOntologyMapping},
	}

	functions = map[types.TaskType]TaskFunc{
		types.TaskUpload: func(sessionID string, a *app.App) error {
			contextFiles, err := a.GetContextFiles(sessionID)
			if err != nil {
				return err
			}
			datasetFile, err := a.GetDatasetFile(sessionID)
			if err != nil {
				return err
			}

			if len(contextFiles) == 0 {
				return fmt.Errorf("no uploaded context files found")
			}
			if datasetFile == "" {
				return fmt.Errorf("no uploaded dataset file found")
			}
			return nil
		},
		types.TaskContextSummary: func(sessionID string, a *app.App) error {
			return PerformContextSummary(sessionID, a)
		},
		types.TaskDatasetSummary: func(sessionID string, a *app.App) error {
			return PerformDatasetSummary(sessionID, a)
		},
		types.TaskOntologyUpload: func(sessionID string, a *app.App) error {
			return PerformOntologyUpload(sessionID, a)
		},
		types.TaskOntologyEmbedding: func(sessionID string, a *app.App) error {
			progress := func(progress float64, message string) {
				UpdateTaskProgress(sessionID, types.TaskOntologyEmbedding, progress, message)
			}
			return PerformOntologyEmbedding(sessionID, a, progress)
		},
		types.TaskNER: func(sessionID string, a *app.App) error {
			return PerformNER(sessionID, a)
		},
		types.TaskRE: func(sessionID string, a *app.App) error {
			return PerformRE(sessionID, a)
		},
		types.TaskOntologyMapping: func(sessionID string, a *app.App) error {
			progress := func(progress float64, message string) {
				UpdateTaskProgress(sessionID, types.TaskOntologyMapping, progress, message)
			}
			return PerformOntologyMapping(sessionID, a, progress)
		},
		types.TaskKnowledgeGraphCreation: func(sessionID string, a *app.App) error {
			return PerformKnowledgeGraphCreation(sessionID, a)
		},
	}
)

func InitializeTaskManager(s store.Store) error {
	return s.ForEachField(store.TasksField, func(sessionID string, data []byte) error {
		var tasks map[types.TaskType]*types.Task
		if err := json.Unmarshal(data, &tasks); err != nil {
			slog.Warn("Skipping init for session, failed to unmarshal tasks", "session", sessionID, "details", err)
			return nil // Skip faulty entries
		}

		manager.mu.Lock()
		defer manager.mu.Unlock()

		for taskType, task := range tasks {
			key := types.TaskKey{
				SessionID: sessionID,
				Type:      taskType,
			}
			if t, exists := manager.tasks[key]; !exists || t == nil {
				if task.Status == types.Success || task.Status == types.Error || task.Status == types.Canceled {
					manager.tasks[key] = task
				}
			}
		}

		return nil
	})
}

func StartTask(sessionID string, a *app.App, taskType types.TaskType) *types.Task {
	dependsOn := getDependencies(taskType)
	execFunc := getTaskFunction(taskType)
	key := types.TaskKey{SessionID: sessionID, Type: taskType}

	// Cancel previous first
	CancelTask(sessionID, a, taskType)
	for _, t := range getAllDependents(taskType) {
		CancelTask(sessionID, a, t)
	}

	// Create new task
	taskCtx, cancel := context.WithCancel(context.Background())
	task := &types.Task{
		Key:        key,
		CancelFunc: cancel,
	}
	if areDependenciesValid(sessionID, dependsOn) {
		task.Status = types.Queued
	} else {
		task.Status = types.Blocked
	}

	manager.mu.Lock()
	manager.tasks[key] = task
	manager.mu.Unlock()

	go func() {
		// Wait until dependencies are done or task canceled
		for {
			if taskCtx.Err() != nil {
				updateTaskStatus(sessionID, a, task, types.Canceled, nil)
				return
			}
			if areDependenciesDone(sessionID, dependsOn) {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}

		updateTaskStatus(sessionID, a, task, types.Running, nil)
		err := execFunc(sessionID, a)
		if taskCtx.Err() == context.Canceled {
			updateTaskStatus(sessionID, a, task, types.Canceled, err)
		} else if err != nil {
			updateTaskStatus(sessionID, a, task, types.Error, err)
		} else {
			updateTaskStatus(sessionID, a, task, types.Success, nil)
		}
	}()

	return task
}

func StartTaskOnce(sessionID string, a *app.App, taskType types.TaskType) *types.Task {
	prevFinished, _ := a.WasTaskFinished(sessionID, taskType)
	if prevFinished {
		return nil
	}

	task := getTask(sessionID, taskType)
	if task == nil || task.Status == types.NotStarted || task.Status == types.Error || task.Status == types.Canceled || task.Status == types.Stale {
		return StartTask(sessionID, a, taskType)
	}
	return nil
}

func updateTaskStatus(sessionID string, a *app.App, task *types.Task, status types.TaskStatus, err error) {
	var dependentsToStale []*types.Task

	manager.mu.Lock()
	taskInMap, ok := manager.tasks[task.Key]
	if ok {
		taskInMap.Err = ErrorToString(err)
		taskInMap.Status = status
		taskInMap.Dirty = true
		taskInMap.Message = "" // reset message
	}

	// Collect dependents that need to be marked stale
	if status == types.Running || status == types.Success || status == types.Error || status == types.Canceled {
		dependentsToStale = getValidDependentsToStaleLocked(sessionID, task.Key.Type)
	}
	for _, dep := range dependentsToStale {
		depInMap, ok := manager.tasks[dep.Key]
		if ok {
			depInMap.Status = types.Stale
			depInMap.Dirty = true
		}
	}
	manager.mu.Unlock()

	// Also save the tasks to the store
	err = a.UpdateTasks(sessionID, func(tasks map[types.TaskType]*types.Task) map[types.TaskType]*types.Task {
		// Mark all stale dependents
		for _, dep := range dependentsToStale {
			if t, exists := tasks[dep.Key.Type]; exists {
				t.Status = types.Stale
				t.Dirty = true
			}
		}

		// Update or insert the current task
		if t, exists := tasks[task.Key.Type]; exists {
			t.Status = status
			t.Dirty = true
			t.Err = ErrorToString(err)
			t.Message = ""
		} else {
			tasks[task.Key.Type] = task
		}
		return tasks
	})
	if err != nil {
		slog.Error("Error updating task state", "details", err)
	}
}

func UpdateTaskProgress(sessionID string, taskType types.TaskType, progress float64, message string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	key := types.TaskKey{SessionID: sessionID, Type: taskType}
	if task, ok := manager.tasks[key]; ok && (task.Status == types.Running || task.Status == types.Queued) {
		task.Progress = progress
		task.Message = message
	}
}

func getValidDependentsToStaleLocked(sessionID string, taskType types.TaskType) []*types.Task {
	var staleTasks []*types.Task
	dependents := getAllDependents(taskType)
	for _, dep := range dependents {
		key := types.TaskKey{SessionID: sessionID, Type: dep}
		if task, ok := manager.tasks[key]; ok &&
			(task.Status == types.Success || task.Status == types.Error || task.Status == types.Canceled) {
			staleTasks = append(staleTasks, task)
		}
	}
	return staleTasks
}

func CancelTask(sessionID string, a *app.App, taskType types.TaskType) {
	var taskToCancel *types.Task

	manager.mu.Lock()
	key := types.TaskKey{SessionID: sessionID, Type: taskType}
	if task, ok := manager.tasks[key]; ok {
		taskToCancel = task
		if task.CancelFunc != nil && (task.Status == types.Running || task.Status == types.Queued) {
			task.CancelFunc()
		}
	}
	manager.mu.Unlock()

	if taskToCancel != nil {
		updateTaskStatus(sessionID, a, taskToCancel, types.Canceled, nil)
	}
}

func TasksDisplayed(sessionID string, a *app.App) {
	manager.mu.Lock()
	var dirtyTasks []types.TaskType
	for key, task := range manager.tasks {
		if key.SessionID == sessionID && task.Dirty {
			task.Dirty = false
			dirtyTasks = append(dirtyTasks, key.Type)
		}
	}
	manager.mu.Unlock()

	if err := a.UpdateTasks(sessionID, func(tasks map[types.TaskType]*types.Task) map[types.TaskType]*types.Task {
		for _, t := range dirtyTasks {
			if task, exists := tasks[t]; exists {
				task.Dirty = false
			}
		}
		return tasks
	}); err != nil {
		slog.Error("Error clearing dirty flag for tasks", "details", err)
	}
}

func getTaskFunction(t types.TaskType) TaskFunc {
	if f, ok := functions[t]; ok {
		return f
	}
	return nil
}

func getTask(sessionID string, taskType types.TaskType) *types.Task {
	key := types.TaskKey{SessionID: sessionID, Type: taskType}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.tasks[key]
}

func ListTasks(sessionID string) []*types.Task {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	var result []*types.Task
	for key, t := range manager.tasks {
		if key.SessionID == sessionID {
			result = append(result, t)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return types.TaskOrder[result[i].Key.Type] < types.TaskOrder[result[j].Key.Type]
	})

	return result
}

func getDependencies(t types.TaskType) []types.TaskType {
	if deps, ok := dependencies[t]; ok {
		return deps
	}
	return nil
}

func getDependents(taskType types.TaskType) []types.TaskType {
	var dependents []types.TaskType
	for t, deps := range dependencies {
		for _, dep := range deps {
			if dep == taskType {
				dependents = append(dependents, t)
			}
		}
	}
	return dependents
}

func getAllDependents(taskType types.TaskType) []types.TaskType {
	visited := make(map[types.TaskType]bool)
	var result []types.TaskType

	var dfs func(t types.TaskType)
	dfs = func(t types.TaskType) {
		for _, dep := range getDependents(t) {
			if !visited[dep] {
				visited[dep] = true
				result = append(result, dep)
				dfs(dep)
			}
		}
	}

	dfs(taskType)
	return result
}

func areDependenciesDone(sessionID string, deps []types.TaskType) bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	for _, dep := range deps {
		key := types.TaskKey{SessionID: sessionID, Type: dep}
		if task, ok := manager.tasks[key]; !ok || task.Status != types.Success {
			return false
		}
	}
	return true
}

func areDependenciesValid(sessionID string, deps []types.TaskType) bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	for _, dep := range deps {
		key := types.TaskKey{SessionID: sessionID, Type: dep}
		if task, ok := manager.tasks[key]; !ok || (task.Status != types.Running && task.Status != types.Queued && task.Status != types.Success) {
			return false
		}
	}
	return true
}

func ErrorToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func IsValidTaskType(t types.TaskType) bool {
	_, ok := dependencies[t]
	return ok
}
