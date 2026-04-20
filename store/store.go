package store

import (
	"errors"
	"time"

	"git.rwth-aachen.de/dsma/publications/software/konda/types"
	"git.rwth-aachen.de/dsma/publications/software/konda/workspace"
)

var ErrNotFound = errors.New("not found")

type FieldKey string

const (
	WorkspaceField               FieldKey = "Workspace"
	TasksField                   FieldKey = "Tasks"
	DomainField                  FieldKey = "Domain"
	RemarksField                 FieldKey = "Remarks"
	FromScratchField             FieldKey = "FromScratch"
	ExtractionCountField         FieldKey = "ExtractionCount"
	PrefixField                  FieldKey = "Prefix"
	NamespaceField               FieldKey = "Namespace"
	ContextFilesField            FieldKey = "ContextFiles"
	DatasetFileField             FieldKey = "DatasetFile"
	ContextSummaryField          FieldKey = "ContextSummary"
	DatasetSummaryField          FieldKey = "DatasetSummary"
	SelectedOntologiesField      FieldKey = "SelectedOntologies"
	SearchedOntologiesField      FieldKey = "SearchedOntologies"
	EntitiesField                FieldKey = "Entities"
	RelationsField               FieldKey = "Relations"
	MappingsMapField             FieldKey = "MappingsMap"
	MappingsDirtyField           FieldKey = "MappingsDirty"
	EntitiesDirtyField           FieldKey = "EntitiesDirty"
	RelationsDirtyField          FieldKey = "RelationsDirty"
	SelectedOntologiesDirtyField FieldKey = "SelectedOntologiesDirty"
	LastAccessedField            FieldKey = "LastAccessed"
)

type AppState struct {
	LastAccessed            time.Time
	Workspace               *workspace.Workspace
	Tasks                   map[types.TaskType]*types.Task
	Domain                  string
	ExtractionCount         int
	Prefix                  string
	Namespace               string
	ContextFiles            []string
	DatasetFile             string
	FromScratch             bool
	Remarks                 string
	ContextSummary          string
	DatasetSummary          string
	SelectedOntologies      []types.Ontology
	SearchedOntologies      []types.Ontology
	Entities                []types.Entity
	Relations               []types.Relation
	SelectedOntologiesDirty bool
	EntitiesDirty           bool
	RelationsDirty          bool
	MappingsMap             types.URIMatchMap
	MappingsDirty           bool
}

type Store interface {
	Set(sessionID string, field FieldKey, value any) error
	Get(sessionID string, field FieldKey, value any) error
	ModifyField(sessionID string, field FieldKey, fn func(current []byte) ([]byte, error)) error
	Delete(sessionID string) error
	ForEachField(field FieldKey, fn func(sessionID string, data []byte) error) error
	Close() error
}
