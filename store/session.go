package store

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"git.rwth-aachen.de/dsma/publications/software/konda/types"
	"git.rwth-aachen.de/dsma/publications/software/konda/workspace"
	"github.com/alexedwards/scs/v2"
)

const (
	LIFETIME = 24 * time.Hour
)

var (
	SessionManager *scs.SessionManager
)

// InitSessionManager initializes the SCS session manager with default settings.
func InitSessionManager(store scs.Store) {
	SessionManager = scs.New()
	SessionManager.Lifetime = LIFETIME
	SessionManager.Cookie.Name = "user_session"
	SessionManager.Store = store
}

// EnsureAppState ensures that initial session fields exist for the given session ID.
func EnsureAppState(sessionID string, appStore Store) error {
	var ts time.Time
	err := appStore.Get(sessionID, LastAccessedField, &ts)
	if !errors.Is(err, ErrNotFound) {
		return err
	}

	// Now set all fields
	now := time.Now()
	initFields := map[FieldKey]any{
		LastAccessedField:            now,
		WorkspaceField:               nil,
		TasksField:                   make(map[types.TaskType]*types.Task),
		MappingsMapField:             make(types.URIMatchMap),
		MappingsDirtyField:           false,
		DomainField:                  "",
		PrefixField:                  "",
		NamespaceField:               "",
		RemarksField:                 "",
		ContextFilesField:            []string{},
		DatasetFileField:             "",
		FromScratchField:             false,
		ContextSummaryField:          "",
		DatasetSummaryField:          "",
		SelectedOntologiesField:      []types.Ontology{},
		SearchedOntologiesField:      []types.Ontology{},
		SelectedOntologiesDirtyField: false,
		EntitiesField:                []types.Entity{},
		EntitiesDirtyField:           false,
		RelationsField:               []types.Relation{},
		RelationsDirtyField:          false,
	}

	for field, val := range initFields {
		if err := appStore.Set(sessionID, field, val); err != nil {
			return err
		}
	}

	return nil
}

// GetSessionID retrieves the current session token from the context.
func GetSessionID(ctx context.Context) string {
	return SessionManager.Token(ctx)
}

func TouchSession(ctx context.Context) {
	SessionManager.Put(ctx, "__init", true)
}

func CleanupExpiredSessions(s Store) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		<-ticker.C
		now := time.Now()

		_ = s.ForEachField(LastAccessedField, func(sessionID string, data []byte) error {
			var lastAccess time.Time
			if err := json.Unmarshal(data, &lastAccess); err != nil {
				slog.Warn("Could not parse lastAccess for session", "session", sessionID, "err", err)
				return nil // skip malformed entries
			}

			if lastAccess.Add(LIFETIME).Before(now) {
				slog.Info("Cleaning up expired session", "session", sessionID)

				_ = workspace.CleanUpWorkspace(sessionID)
				_ = DeleteDatabase(sanitizeDbName(sessionID))
				_ = DeleteDatabase(sanitizeDbName(sessionID) + OntoExt)
				_ = s.Delete(sessionID)
			}

			return nil
		})
	}
}
