package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"slices"

	"git.rwth-aachen.de/dsma/publications/software/konda/search"
	"git.rwth-aachen.de/dsma/publications/software/konda/store"
	"git.rwth-aachen.de/dsma/publications/software/konda/types"
	"git.rwth-aachen.de/dsma/publications/software/konda/workspace"
	"github.com/google/uuid"
)

const (
	DEFAULT_NAMESPACE        = "http://example.org/ontology#"
	DEFAULT_PREFIX           = "ex"
	DEFAULT_EXTRACTION_COUNT = 15
)

type App struct {
	Store store.Store
}

func (a *App) GetWorkspace(sessionID string) (*workspace.Workspace, error) {
	var ws *workspace.Workspace
	err := a.Store.Get(sessionID, store.WorkspaceField, &ws)
	return ws, err
}

func (a *App) SetWorkspace(sessionID string, workspace *workspace.Workspace) error {
	return a.Store.Set(sessionID, store.WorkspaceField, workspace)
}

func (a *App) EnsureWorkspace(sessionID string) (*workspace.Workspace, error) {
	stored, err := a.GetWorkspace(sessionID)
	if err != nil {
		return nil, err
	}
	if stored != nil {
		return stored, nil
	}
	workSpace, err := workspace.CreateWorkspace(sessionID)
	if err != nil {
		return nil, fmt.Errorf("unable to create temporary workspace: %w", err)
	}
	if err := workspace.SaveWorkspaceMetadata(workSpace); err != nil {
		return nil, err
	}
	if err := a.SetWorkspace(sessionID, workSpace); err != nil {
		return nil, err
	}
	return workSpace, nil
}

func (a *App) GetAllTasks(sessionID string) (map[types.TaskType]*types.Task, error) {
	var tasks map[types.TaskType]*types.Task
	err := a.Store.Get(sessionID, store.TasksField, &tasks)
	return tasks, err
}

func (a *App) UpdateTasks(sessionID string, updateFn func(map[types.TaskType]*types.Task) map[types.TaskType]*types.Task) error {
	return a.Store.ModifyField(sessionID, store.TasksField, func(current []byte) ([]byte, error) {
		tasks := make(map[types.TaskType]*types.Task)
		if current != nil {
			if err := json.Unmarshal(current, &tasks); err != nil {
				return nil, fmt.Errorf("failed to unmarshal Tasks: %w", err)
			}
		}
		updated := updateFn(tasks)
		return json.Marshal(updated)
	})
}

func (a *App) WasTaskFinished(sessionID string, taskType types.TaskType) (bool, error) {
	tasks, err := a.GetAllTasks(sessionID)
	if err != nil {
		return false, err
	}
	if task, ok := tasks[taskType]; ok {
		return task.Status == types.Success, nil
	}
	return false, nil
}

func (a *App) GetDomain(sessionID string) (string, error) {
	var d string
	err := a.Store.Get(sessionID, store.DomainField, &d)
	return d, err
}

func (a *App) UpdateDomain(sessionID string, updateFn func(current string) string) error {
	return a.Store.ModifyField(sessionID, store.DomainField, func(current []byte) ([]byte, error) {
		var domain string
		if current != nil {
			if err := json.Unmarshal(current, &domain); err != nil {
				return nil, fmt.Errorf("failed to unmarshal Domain: %w", err)
			}
		}
		updated := updateFn(domain)
		return json.Marshal(updated)
	})
}

func (a *App) GetRemarks(sessionID string) (string, error) {
	var r string
	err := a.Store.Get(sessionID, store.RemarksField, &r)
	return r, err
}

func (a *App) UpdateRemarks(sessionID string, updateFn func(current string) string) error {
	return a.Store.ModifyField(sessionID, store.RemarksField, func(current []byte) ([]byte, error) {
		var r string
		if current != nil {
			if err := json.Unmarshal(current, &r); err != nil {
				return nil, fmt.Errorf("failed to unmarshal Remarks: %w", err)
			}
		}
		updated := updateFn(r)
		return json.Marshal(updated)
	})
}

func (a *App) GetExtractionCount(sessionID string) (int, error) {
	var count int
	err := a.Store.Get(sessionID, store.ExtractionCountField, &count)
	if err == store.ErrNotFound {
		count = DEFAULT_EXTRACTION_COUNT
		if setErr := a.Store.Set(sessionID, store.ExtractionCountField, count); setErr != nil {
			return count, fmt.Errorf("failed to set default extraction count: %w", setErr)
		}
	}
	return count, err
}

func (a *App) UpdateExtractionCount(sessionID string, updateFn func(current int) int) error {
	return a.Store.ModifyField(sessionID, store.ExtractionCountField, func(current []byte) ([]byte, error) {
		var count int
		if current != nil {
			if err := json.Unmarshal(current, &count); err != nil {
				return nil, fmt.Errorf("failed to unmarshal extraction count: %w", err)
			}
		}
		updated := updateFn(count)
		return json.Marshal(updated)
	})
}

func (a *App) GetNamespace(sessionID string) (string, error) {
	var ns string
	err := a.Store.Get(sessionID, store.NamespaceField, &ns)
	if err == store.ErrNotFound || ns == "" {
		ns = DEFAULT_NAMESPACE
		if setErr := a.Store.Set(sessionID, store.NamespaceField, ns); setErr != nil {
			return ns, fmt.Errorf("failed to set default namespace: %w", setErr)
		}
	}
	return ns, err
}

func (a *App) UpdateNamespace(sessionID string, updateFn func(current string) string) error {
	return a.Store.ModifyField(sessionID, store.NamespaceField, func(current []byte) ([]byte, error) {
		var ns string
		if current != nil {
			if err := json.Unmarshal(current, &ns); err != nil {
				return nil, fmt.Errorf("failed to unmarshal Namespace: %w", err)
			}
		}
		updated := updateFn(ns)
		return json.Marshal(updated)
	})
}

func (a *App) GetPrefix(sessionID string) (string, error) {
	var prefix string
	err := a.Store.Get(sessionID, store.PrefixField, &prefix)
	if err == store.ErrNotFound || prefix == "" {
		prefix = DEFAULT_PREFIX
		if setErr := a.Store.Set(sessionID, store.PrefixField, prefix); setErr != nil {
			return prefix, fmt.Errorf("failed to set default prefix: %w", setErr)
		}
	}
	return prefix, err
}

func (a *App) UpdatePrefix(sessionID string, updateFn func(current string) string) error {
	return a.Store.ModifyField(sessionID, store.PrefixField, func(current []byte) ([]byte, error) {
		var prefix string
		if current != nil {
			if err := json.Unmarshal(current, &prefix); err != nil {
				return nil, fmt.Errorf("failed to unmarshal Prefix: %w", err)
			}
		}
		updated := updateFn(prefix)
		return json.Marshal(updated)
	})
}

func (a *App) GetFromScratch(sessionID string) (bool, error) {
	var fs bool
	err := a.Store.Get(sessionID, store.FromScratchField, &fs)
	return fs, err
}

func (a *App) UpdateFromScratch(sessionID string, updateFn func(current bool) bool) error {
	return a.Store.ModifyField(sessionID, store.FromScratchField, func(current []byte) ([]byte, error) {
		var fs bool
		if current != nil {
			if err := json.Unmarshal(current, &fs); err != nil {
				return nil, fmt.Errorf("failed to unmarshal FromScratch: %w", err)
			}
		}
		updated := updateFn(fs)
		return json.Marshal(updated)
	})
}

func (a *App) GetContextFiles(sessionID string) ([]string, error) {
	var files []string
	err := a.Store.Get(sessionID, store.ContextFilesField, &files)
	return files, err
}

func (a *App) SetContextFiles(sessionID string, contextFiles []types.UploadFile) error {
	workDir, err := a.GetWorkspace(sessionID)
	if err != nil {
		return fmt.Errorf("error retrieving workspace: %w", err)
	}

	if err := workspace.CleanUpWorkDir(workDir.ContextDir); err != nil {
		return fmt.Errorf("error cleaning context dir: %w", err)
	}

	if err := workspace.SaveFilesToWorkspace(workDir.ContextDir, contextFiles); err != nil {
		return fmt.Errorf("error saving context files: %w", err)
	}

	var fileNames []string
	for _, f := range contextFiles {
		fileNames = append(fileNames, f.FileName)
	}

	return a.Store.Set(sessionID, store.ContextFilesField, fileNames)
}

func (a *App) AddContextFiles(sessionID string, contextFiles []types.UploadFile) error {
	workDir, err := a.GetWorkspace(sessionID)
	if err != nil {
		return fmt.Errorf("error retrieving workspace: %w", err)
	}

	if err := workspace.SaveFilesToWorkspace(workDir.ContextDir, contextFiles); err != nil {
		return fmt.Errorf("error saving context files: %w", err)
	}

	entries, err := workspace.ListFilesInDir(workDir.ContextDir)
	if err != nil {
		return fmt.Errorf("error listing context dir: %w", err)
	}

	return a.Store.Set(sessionID, store.ContextFilesField, entries)
}

func (a *App) DeleteContextFile(sessionID string, index int) error {
	workDir, err := a.GetWorkspace(sessionID)
	if err != nil {
		return fmt.Errorf("error retrieving workspace: %w", err)
	}

	return a.Store.ModifyField(sessionID, store.ContextFilesField, func(current []byte) ([]byte, error) {
		var files []string
		if current != nil {
			if err := json.Unmarshal(current, &files); err != nil {
				return nil, fmt.Errorf("failed to unmarshal ContextFiles: %w", err)
			}
		}

		if index < 0 || index >= len(files) {
			return nil, fmt.Errorf("index out of bounds")
		}

		// Delete file from workspace
		if err := workspace.DeleteFileFromWorkspace(workDir.ContextDir, files[index]); err != nil {
			return nil, fmt.Errorf("error deleting context file: %w", err)
		}

		// Refresh from actual dir to reflect real state
		entries, err := workspace.ListFilesInDir(workDir.ContextDir)
		if err != nil {
			return nil, fmt.Errorf("error listing context dir: %w", err)
		}

		return json.Marshal(entries)
	})
}

func (a *App) GetDatasetFile(sessionID string) (string, error) {
	var file string
	err := a.Store.Get(sessionID, store.DatasetFileField, &file)
	return file, err
}

func (a *App) SetDatasetFile(sessionID string, datasetFile types.UploadFile) error {
	workDir, err := a.GetWorkspace(sessionID)
	if err != nil {
		return fmt.Errorf("error retrieving workspace: %w", err)
	}

	if err := workspace.CleanUpWorkDir(workDir.DatasetDir); err != nil {
		return fmt.Errorf("error cleaning dataset dir: %w", err)
	}

	if !workspace.IsZip(datasetFile.Content) {
		return fmt.Errorf("uploaded file is not a valid ZIP")
	}

	if err := workspace.Unzip(datasetFile.Content, workDir.DatasetDir); err != nil {
		return fmt.Errorf("failed to extract ZIP file: %w", err)
	}

	return a.Store.Set(sessionID, store.DatasetFileField, datasetFile.FileName)
}

func (a *App) DeleteDatasetFile(sessionID string) error {
	workDir, err := a.GetWorkspace(sessionID)
	if err != nil {
		return fmt.Errorf("error retrieving workspace: %w", err)
	}

	if err := workspace.CleanUpWorkDir(workDir.DatasetDir); err != nil {
		return fmt.Errorf("error cleaning dataset dir: %w", err)
	}

	return a.Store.Set(sessionID, store.DatasetFileField, "")
}

func (a *App) GetContextSummary(sessionID string) (string, error) {
	var s string
	err := a.Store.Get(sessionID, store.ContextSummaryField, &s)
	return s, err
}

func (a *App) UpdateContextSummary(sessionID string, updateFn func(string) string) error {
	return a.Store.ModifyField(sessionID, store.ContextSummaryField, func(current []byte) ([]byte, error) {
		var summary string
		if current != nil {
			if err := json.Unmarshal(current, &summary); err != nil {
				return nil, fmt.Errorf("failed to unmarshal ContextSummary: %w", err)
			}
		}
		updated := updateFn(summary)
		return json.Marshal(updated)
	})
}

func (a *App) GetDatasetSummary(sessionID string) (string, error) {
	var s string
	err := a.Store.Get(sessionID, store.DatasetSummaryField, &s)
	return s, err
}

func (a *App) UpdateDatasetSummary(sessionID string, updateFn func(string) string) error {
	return a.Store.ModifyField(sessionID, store.DatasetSummaryField, func(current []byte) ([]byte, error) {
		var summary string
		if current != nil {
			if err := json.Unmarshal(current, &summary); err != nil {
				return nil, fmt.Errorf("failed to unmarshal DatasetSummary: %w", err)
			}
		}
		updated := updateFn(summary)
		return json.Marshal(updated)
	})
}

func (a *App) GetSelectedOntologies(sessionID string) ([]types.Ontology, error) {
	var items []types.Ontology
	err := a.Store.Get(sessionID, store.SelectedOntologiesField, &items)
	return items, err
}

func (a *App) UpdateSelectedOntologies(sessionID string, updateFn func([]types.Ontology) []types.Ontology) error {
	return a.Store.ModifyField(sessionID, store.SelectedOntologiesField, func(current []byte) ([]byte, error) {
		var existing []types.Ontology
		if current != nil {
			if err := json.Unmarshal(current, &existing); err != nil {
				return nil, fmt.Errorf("failed to unmarshal SelectedOntologies: %w", err)
			}
		}
		updated := updateFn(existing)
		for i := range updated {
			search.SetNumberOfTerms(&updated[i])
		}
		return json.Marshal(updated)
	})
}

func (a *App) AddSelectedOntology(sessionID string, ontologies ...types.Ontology) error {
	return a.UpdateSelectedOntologies(sessionID, func(existing []types.Ontology) []types.Ontology {
		for _, onto := range ontologies {
			found := false
			for _, e := range existing {
				if e.OntologyName == onto.OntologyName {
					found = true
					break
				}
			}
			if !found {
				existing = append(existing, onto)
			}
		}
		return existing
	})
}

func (a *App) DeleteSelectedOntology(sessionID string, index int) error {
	ws, err := a.GetWorkspace(sessionID)
	if err != nil {
		return fmt.Errorf("error retrieving workspace: %w", err)
	}

	return a.Store.ModifyField(sessionID, store.SelectedOntologiesField, func(current []byte) ([]byte, error) {
		var ontologies []types.Ontology
		if current != nil {
			if err := json.Unmarshal(current, &ontologies); err != nil {
				return nil, fmt.Errorf("failed to unmarshal SelectedOntologies: %w", err)
			}
		}

		if index < 0 || index >= len(ontologies) {
			return nil, fmt.Errorf("index out of bounds")
		}

		if err := workspace.DeleteFileFromWorkspace(ws.OntologyDir, ontologies[index].FileName); err != nil {
			return nil, fmt.Errorf("failed to delete ontology file from workspace: %w", err)
		}

		updated := append(ontologies[:index], ontologies[index+1:]...)
		return json.Marshal(updated)
	})
}

func (a *App) GetSelectedOntologiesDirty(sessionID string) (bool, error) {
	var dirty bool
	err := a.Store.Get(sessionID, store.SelectedOntologiesDirtyField, &dirty)
	return dirty, err
}

func (a *App) UpdateSelectedOntologiesDirty(sessionID string, updateFn func(bool) bool) error {
	return a.Store.ModifyField(sessionID, store.SelectedOntologiesDirtyField, func(current []byte) ([]byte, error) {
		var dirty bool
		if current != nil {
			if err := json.Unmarshal(current, &dirty); err != nil {
				return nil, fmt.Errorf("failed to unmarshal SelectedOntologiesDirty: %w", err)
			}
		}
		updated := updateFn(dirty)
		return json.Marshal(updated)
	})
}

func (a *App) WriteSelectedOntologies(sessionID string, items []types.Ontology) error {
	if err := store.DeleteSessionDBs(store.GetSessionDB(sessionID)); err != nil {
		return fmt.Errorf("failed to delete session DBs: %w", err)
	}

	prefix, err := a.GetPrefix(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get namespace: %w", err)
	}
	namespace, err := a.GetNamespace(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get namespace: %w", err)
	}
	if err := store.EnsureSessionDatabase(store.GetSessionDB(sessionID), prefix, namespace); err != nil {
		return fmt.Errorf("failed to ensure session database: %w", err)
	}

	workDir, err := a.GetWorkspace(sessionID)
	if err != nil {
		return fmt.Errorf("error retrieving workspace: %w", err)
	}

	if err := workspace.CleanUpWorkDir(workDir.OntologyDir); err != nil {
		return fmt.Errorf("error cleaning ontology dir: %w", err)
	}

	for i := range items {
		if err := search.PopulateOntologyContent(&items[i]); err != nil {
			return fmt.Errorf("failed to populate ontology content: %w", err)
		}
	}

	files := types.ConvOntoToUploadFile(items)
	if err := workspace.SaveFilesToWorkspace(workDir.OntologyDir, files); err != nil {
		return fmt.Errorf("error saving ontology files: %w", err)
	}

	if err := a.UpdateSelectedOntologies(sessionID, func(_ []types.Ontology) []types.Ontology {
		return items
	}); err != nil {
		return fmt.Errorf("failed to update selected ontologies: %w", err)
	}

	for _, f := range files {
		path := filepath.Join(workDir.OntologyDir, f.FileName)
		if err := store.UploadOntology(path, store.GetSessionOntoDB(sessionID)); err != nil {
			return fmt.Errorf("failed to upload ontology: %w", err)
		}
	}

	return nil
}

func (a *App) GetSearchedOntologies(sessionID string) ([]types.Ontology, error) {
	var items []types.Ontology
	err := a.Store.Get(sessionID, store.SearchedOntologiesField, &items)
	return items, err
}

func (a *App) UpdateSearchedOntologies(sessionID string, updateFn func([]types.Ontology) []types.Ontology) error {
	return a.Store.ModifyField(sessionID, store.SearchedOntologiesField, func(current []byte) ([]byte, error) {
		var items []types.Ontology
		if current != nil {
			if err := json.Unmarshal(current, &items); err != nil {
				return nil, fmt.Errorf("failed to unmarshal SearchedOntologies: %w", err)
			}
		}
		updated := updateFn(items)
		return json.Marshal(updated)
	})
}

func (a *App) GetEntities(sessionID string) ([]types.Entity, error) {
	var entities []types.Entity
	err := a.Store.Get(sessionID, store.EntitiesField, &entities)
	return entities, err
}

func (a *App) GetAllEntityLabels(sessionID string) ([]string, error) {
	entities, err := a.GetEntities(sessionID)
	if err != nil {
		return nil, fmt.Errorf("error getting entities: %w", err)
	}
	var entityLabels []string
	for _, e := range entities {
		entityLabels = append(entityLabels, e.Entity, e.Type)
	}
	entityLabels = UniqueStrings(entityLabels)
	sort.Slice(entityLabels, func(i, j int) bool {
		return strings.ToLower(entityLabels[i]) < strings.ToLower(entityLabels[j])
	})
	return entityLabels, nil
}

func (a *App) UpdateEntities(sessionID string, updateFn func([]types.Entity) []types.Entity) error {
	return a.Store.ModifyField(sessionID, store.EntitiesField, func(current []byte) ([]byte, error) {
		var entities []types.Entity
		if current != nil {
			if err := json.Unmarshal(current, &entities); err != nil {
				return nil, fmt.Errorf("failed to unmarshal Entities: %w", err)
			}
		}
		updated := updateFn(entities)
		return json.Marshal(updated)
	})
}

func (a *App) DeleteEntity(sessionID string, index int) error {
	return a.UpdateEntities(sessionID, func(entities []types.Entity) []types.Entity {
		if index < 0 || index >= len(entities) {
			return entities
		}
		return append(entities[:index], entities[index+1:]...)
	})
}

func (a *App) GetEntitiesDirty(sessionID string) (bool, error) {
	var dirty bool
	err := a.Store.Get(sessionID, store.EntitiesDirtyField, &dirty)
	return dirty, err
}

func (a *App) UpdateEntitiesDirty(sessionID string, updateFn func(bool) bool) error {
	return a.Store.ModifyField(sessionID, store.EntitiesDirtyField, func(current []byte) ([]byte, error) {
		var dirty bool
		if current != nil {
			if err := json.Unmarshal(current, &dirty); err != nil {
				return nil, fmt.Errorf("failed to unmarshal EntitiesDirty: %w", err)
			}
		}
		updated := updateFn(dirty)
		return json.Marshal(updated)
	})
}

func (a *App) GetRelations(sessionID string) ([]types.Relation, error) {
	var relations []types.Relation
	err := a.Store.Get(sessionID, store.RelationsField, &relations)
	return relations, err
}

func (a *App) UpdateRelations(sessionID string, updateFn func([]types.Relation) []types.Relation) error {
	return a.Store.ModifyField(sessionID, store.RelationsField, func(current []byte) ([]byte, error) {
		var relations []types.Relation
		if current != nil {
			if err := json.Unmarshal(current, &relations); err != nil {
				return nil, fmt.Errorf("failed to unmarshal Relations: %w", err)
			}
		}
		updated := updateFn(relations)
		return json.Marshal(updated)
	})
}

func (a *App) DeleteRelation(sessionID string, index int) error {
	return a.UpdateRelations(sessionID, func(relations []types.Relation) []types.Relation {
		if index < 0 || index >= len(relations) {
			return relations
		}
		return append(relations[:index], relations[index+1:]...)
	})
}

func (a *App) GetRelationsDirty(sessionID string) (bool, error) {
	var dirty bool
	err := a.Store.Get(sessionID, store.RelationsDirtyField, &dirty)
	return dirty, err
}

func (a *App) UpdateRelationsDirty(sessionID string, updateFn func(bool) bool) error {
	return a.Store.ModifyField(sessionID, store.RelationsDirtyField, func(current []byte) ([]byte, error) {
		var dirty bool
		if current != nil {
			if err := json.Unmarshal(current, &dirty); err != nil {
				return nil, fmt.Errorf("failed to unmarshal RelationsDirty: %w", err)
			}
		}
		updated := updateFn(dirty)
		return json.Marshal(updated)
	})
}

func (a *App) GetMappings(sessionID string) (types.URIMatchMap, error) {
	var m types.URIMatchMap
	err := a.Store.Get(sessionID, store.MappingsMapField, &m)
	return m, err
}

func (a *App) GetMappingByID(sessionID string, id uuid.UUID) (string, types.URIMatch, error) {
	var m types.URIMatchMap
	err := a.Store.Get(sessionID, store.MappingsMapField, &m)

	for key, matches := range m {
		for _, match := range matches {
			if match.ID == id {
				return key, match, nil
			}
		}
	}
	return "", types.URIMatch{}, fmt.Errorf("could not find a mapping with the given id: %s, %w", id, err)
}

func (a *App) UpdateMappings(sessionID string, updateFn func(m types.URIMatchMap) types.URIMatchMap) error {
	return a.Store.ModifyField(sessionID, store.MappingsMapField, func(current []byte) ([]byte, error) {
		var mm types.URIMatchMap
		if current != nil {
			if err := json.Unmarshal(current, &mm); err != nil {
				return nil, fmt.Errorf("failed to unmarshal MappingsMap: %w", err)
			}
		}
		updated := updateFn(mm)

		for key, matches := range updated {
			for i := range matches {
				// remove invalid matches
				if matches[i].Label == "" || matches[i].URI == "" {
					matches = slices.Delete(matches, i, i+1)
					continue
				}
				// Ensure all match IDs are initialized
				if matches[i].ID == uuid.Nil {
					matches[i].ID = uuid.New()
				}
			}
			updated[key] = matches
		}

		return json.Marshal(updated)
	})
}

func (a *App) DeleteMapping(sessionID string, id string) error {
	return a.UpdateMappings(sessionID, func(m types.URIMatchMap) types.URIMatchMap {
		for k, matches := range m {
			for _, match := range matches {
				if match.ID.String() == id {
					delete(m, k)
					break
				}
			}
		}
		return m
	})
}

func (a *App) GetMappingsDirty(sessionID string) (bool, error) {
	var dirty bool
	err := a.Store.Get(sessionID, store.MappingsDirtyField, &dirty)
	return dirty, err
}

func (a *App) UpdateMappingsDirty(sessionID string, updateFn func(bool) bool) error {
	return a.Store.ModifyField(sessionID, store.MappingsDirtyField, func(current []byte) ([]byte, error) {
		var dirty bool
		if current != nil {
			if err := json.Unmarshal(current, &dirty); err != nil {
				return nil, fmt.Errorf("failed to unmarshal MappingsDirty: %w", err)
			}
		}
		updated := updateFn(dirty)
		return json.Marshal(updated)
	})
}

// uniqueStrings returns a new slice containing only unique strings from the input slice.
func UniqueStrings(input []string) []string {
	uniqueMap := make(map[string]bool)
	uniqueSlice := []string{}
	for _, item := range input {
		if !uniqueMap[item] {
			uniqueMap[item] = true
			uniqueSlice = append(uniqueSlice, item)
		}
	}
	return uniqueSlice
}
