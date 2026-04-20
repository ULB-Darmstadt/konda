package handler

import (
	"log"
	"log/slog"
	"net/http"
	"runtime/debug"

	"git.rwth-aachen.de/dsma/publications/software/konda/app"
	"git.rwth-aachen.de/dsma/publications/software/konda/store"
	"git.rwth-aachen.de/dsma/publications/software/konda/web"
)

type Handlers struct {
	App *app.App
}

// SetupRoutes sets up all HTTP handlers with appropriate middleware.
func SetupRoutes(app *app.App) http.Handler {
	handlers := Handlers{App: app}

	// Inner mux for app routes
	mux := http.NewServeMux()
	registerRoutes(mux, handlers)

	// Apply middleware to app routes only
	wrappedMux := applyMiddleware(mux, app)

	// Top-level mux to separate static from dynamic routes
	topMux := http.NewServeMux()
	topMux.Handle("GET /static/", staticFileServerHandler(http.FS(web.StaticDir)))
	topMux.Handle("/", wrappedMux)

	return topMux
}

func registerRoutes(mux *http.ServeMux, h Handlers) {
	// Public routes
	mux.HandleFunc("GET /", h.IndexHandler)

	// Tool routes
	toolMux := http.NewServeMux()

	// Upload Dataset
	toolMux.HandleFunc("GET /", h.IndexHandler) // TODO: this is using the index handler for not found
	toolMux.HandleFunc("GET /tool/upload-dataset", h.GetUploadDatasetHandler)
	toolMux.HandleFunc("POST /tool/upload-dataset", h.PostUploadDatasetHandler)
	toolMux.HandleFunc("GET /tool/upload-dataset/domains", h.GetDomainsHandler)
	toolMux.HandleFunc("GET /tool/upload-dataset/uploaded-context-files", h.GetUploadedContextFilesHandler)
	toolMux.HandleFunc("DELETE /tool/upload-dataset/uploaded-context-files/{id}", h.DeleteUploadedContextFilesHandler)
	toolMux.HandleFunc("GET /tool/upload-dataset/uploaded-dataset-files", h.GetUploadedDatasetFilesHandler)
	toolMux.HandleFunc("DELETE /tool/upload-dataset/uploaded-dataset-files/{id}", h.DeleteUploadedDatasetFilesHandler)

	// Find Ontology
	toolMux.HandleFunc("GET /tool/find-ontology", h.GetFindOntologyHandler)
	toolMux.HandleFunc("POST /tool/find-ontology", h.PostFindOntologyHandler)
	toolMux.HandleFunc("POST /tool/find-ontology/search-ontology", h.SearchOntologyHandler)
	toolMux.HandleFunc("GET /tool/find-ontology/selected-ontologies", h.GetSelectedOntologiesHandler)
	toolMux.HandleFunc("POST /tool/find-ontology/selected-ontologies", h.AddSelectedOntologiesHandler)
	toolMux.HandleFunc("DELETE /tool/find-ontology/selected-ontologies/{id}", h.DeleteSelectedOntologiesHandler)

	// Entity Recognition
	toolMux.HandleFunc("GET /tool/entity-recognition", h.GetEntityRecognitionHandler)
	toolMux.HandleFunc("POST /tool/entity-recognition", h.PostEntityRecognitionHandler)
	toolMux.HandleFunc("GET /tool/entity-recognition/entities", h.GetEntitiesHandler)
	toolMux.HandleFunc("GET /tool/entity-recognition/entities/add", h.AddEntityHandler)
	toolMux.HandleFunc("DELETE /tool/entity-recognition/entities/{id}", h.DeleteEntityHandler)
	toolMux.HandleFunc("PUT /tool/entity-recognition/entities/{id}", h.UpdateEntityHandler)

	// Relation Extraction
	toolMux.HandleFunc("GET /tool/relation-extraction", h.GetRelationExtractionHandler)
	toolMux.HandleFunc("POST /tool/relation-extraction", h.PostRelationExtractionHandler)
	toolMux.HandleFunc("GET /tool/relation-extraction/relations", h.GetRelationsHandler)
	toolMux.HandleFunc("GET /tool/relation-extraction/relations/add", h.AddRelationHandler)
	toolMux.HandleFunc("DELETE /tool/relation-extraction/relations/{id}", h.DeleteRelationHandler)
	toolMux.HandleFunc("PUT /tool/relation-extraction/relations/{id}", h.UpdateRelationHandler)

	// Ontology Mapping
	toolMux.HandleFunc("GET /tool/ontology-mapping", h.GetOntologyMappingHandler)
	toolMux.HandleFunc("POST /tool/ontology-mapping", h.PostOntologyMappingHandler)
	toolMux.HandleFunc("GET /tool/ontology-mapping/mappings", h.GetMappingsHandler)
	toolMux.HandleFunc("POST /tool/ontology-mapping/mappings", h.PostMappingHandler)
	toolMux.HandleFunc("DELETE /tool/ontology-mapping/mappings/{id}", h.DeleteMappingHandler)
	toolMux.HandleFunc("PUT /tool/ontology-mapping/mappings/{id}", h.UpdateMappingHandler)
	toolMux.HandleFunc("GET /tool/ontology-mapping/mappings/add", h.GetAddMappingHandler)
	toolMux.HandleFunc("PUT /tool/ontology-mapping/mappings/add", h.UpdateAddMappingHandler)
	toolMux.HandleFunc("DELETE /tool/ontology-mapping/mappings/add", h.DeleteAddMappingHandler)
	toolMux.HandleFunc("POST /tool/ontology-mapping/search-terms", h.SearchOntologyTermsHandler)

	// Knowledge Graph
	toolMux.HandleFunc("GET /tool/knowledge-graph", h.GetKnowledgeGraphHandler)
	toolMux.HandleFunc("GET /tool/knowledge-graph/graph-data", h.GetNeo4jGraphHandler)
	toolMux.HandleFunc("GET /tool/knowledge-graph/rdf-data", h.GetRDFData)
	toolMux.HandleFunc("GET /tool/knowledge-graph/onto-data", h.GetOntoData)
	toolMux.HandleFunc("GET /tool/knowledge-graph/export-graph", h.GetRDFExport)

	// Tasks
	toolMux.HandleFunc("GET /tool/running-tasks", h.GetRunningTasksHandler)
	toolMux.HandleFunc("PUT /tool/running-tasks/restart", h.RestartTaskHandler)
	toolMux.HandleFunc("DELETE /tool/running-tasks/cancel", h.CancelTaskHandler)

	// Expose /tool/ for all supported methods explicitly
	mux.Handle("GET /tool/", toolMux)
	mux.Handle("POST /tool/", toolMux)
	mux.Handle("PUT /tool/", toolMux)
	mux.Handle("DELETE /tool/", toolMux)
}

func staticFileServerHandler(fs http.FileSystem) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check beforehand if the requested file exists.
		_, err := fs.Open(r.URL.Path)
		if err != nil {
			http.NotFound(w, r)
			slog.Error(err.Error(), "method", r.Method, "status", http.StatusNotFound, "path", r.URL.Path)
			return
		}

		// File is found, serve it using the standard http.FileServer.
		http.FileServer(fs).ServeHTTP(w, r)
	})
}

// Middleware

func applyMiddleware(mux http.Handler, app *app.App) http.Handler {
	wrapped := recoveryMiddleware(mux)
	wrapped = sessionMiddleware(app)(wrapped)
	wrapped = store.SessionManager.LoadAndSave(wrapped)
	return wrapped
}

// Middleware to ensure every request has a session ID

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Caught panic: %v, Stack trace: %s", err, string(debug.Stack()))

				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// sessionMiddleware ensures that an AppState exists for the current session.
func sessionMiddleware(a *app.App) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sessionID := store.GetSessionID(r.Context())
			if sessionID == "" {
				store.TouchSession(r.Context())
				store.SessionManager.Commit(r.Context())
				sessionID = store.GetSessionID(r.Context())

				err := store.EnsureAppState(sessionID, a.Store)
				if err != nil {
					slog.Error("Failed to ensure AppState", "error", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
