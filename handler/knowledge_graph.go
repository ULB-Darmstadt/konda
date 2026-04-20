package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"git.rwth-aachen.de/dsma/publications/software/konda/analyzer"
	"git.rwth-aachen.de/dsma/publications/software/konda/store"
	"git.rwth-aachen.de/dsma/publications/software/konda/types"
	"git.rwth-aachen.de/dsma/publications/software/konda/view"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func (h *Handlers) GetKnowledgeGraphHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	analyzer.StartTaskOnce(sessionID, h.App, types.TaskKnowledgeGraphCreation)

	prefix, err := h.App.GetPrefix(sessionID)
	if err != nil {
		slog.Error("Error getting prefix", "details", err)
		view.Error(w, "Error getting prefix")
		return
	}
	namespace, err := h.App.GetNamespace(sessionID)
	if err != nil {
		slog.Error("Error getting namespace", "details", err)
		view.Error(w, "Error getting namespace")
		return
	}
	kg, err := store.FetchRDFData(store.GetSessionDB(sessionID), prefix, "Turtle")
	if err != nil {
		slog.Error("Error fetching Ontology from DB", "details", err)
		view.Error(w, "Error fetching Ontology from DB")
	}
	onto, err := store.FetchOntoData(store.GetSessionDB(sessionID))
	if err != nil {
		slog.Error("Error fetching Ontology from DB", "details", err)
		view.Error(w, "Error fetching Ontology from DB")
	}

	params := view.KnowledgeGraphParams{
		Prefix:         prefix,
		Namespace:      namespace,
		KnowledgeGraph: string(kg),
		Ontology:       onto,
	}

	if err := view.KnowledgeGraph(w, params); err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) GetNeo4jGraphHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	// TODO: most of this should be in the store?
	dbCtx := store.DbClient.Ctx
	session := store.DbClient.Driver.NewSession(dbCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead, DatabaseName: store.GetSessionDB(sessionID)})
	defer session.Close(dbCtx)

	prefix, err := h.App.GetPrefix(sessionID)
	if err != nil {
		slog.Error("Error getting prefix", "details", err)
		view.Error(w, "Error getting prefix")
		return
	}
	namespace, err := h.App.GetNamespace(sessionID)
	if err != nil {
		slog.Error("Error getting namespace", "details", err)
		view.Error(w, "Error getting namespace")
		return
	}
	datasetFile, err := h.App.GetDatasetFile(sessionID)
	if err != nil {
		slog.Error("Error getting datasetfile", "details", err)
		view.Error(w, "Error getting datasetfile")
		return
	}
	query := fmt.Sprintf(`
		MATCH (n:%s__Dataset {uri: $datasetURI})
		CALL apoc.path.subgraphAll(n, { minLevel: 0, maxLevel: 10 })
		YIELD nodes, relationships
		WITH
			[node IN nodes | {id: id(node), labels: CASE WHEN node.external THEN ["external"] ELSE labels(node) END, prefLabel: coalesce(node.rdfs__label, node.n4sch__prefLabel, node.n4sch__label, node.n4sch__name, node.n4sch__altLabel, ""), uri: node.uri}] AS distinctNodes,
			[rel IN relationships | {id: id(rel), source: id(startNode(rel)), target: id(endNode(rel)), type: type(rel), prefLabel: coalesce(rel.rdfs__label, rel.n4sch__label, type(rel)), uri: rel.uri}] AS distinctLinks
		RETURN DISTINCT distinctNodes AS nodes, distinctLinks AS links
		LIMIT 100;
	`, prefix)
	params := map[string]any{
		"datasetURI": store.CreateRDFURI(namespace, datasetFile),
	}

	result, err := session.Run(dbCtx, query, params)
	if err != nil {
		view.Error(w, "Failed to query graph from DB")
		return
	}

	var nodes, links any
	if result.Next(dbCtx) {
		record := result.Record()
		nodes, _ = record.Get("nodes")
		links, _ = record.Get("links")
	}

	if err = result.Err(); err != nil {
		view.Error(w, "Error retrieving graph data")
		return
	}

	response := map[string]interface{}{
		"nodes": nodes,
		"links": links,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handlers) GetRDFData(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())

	prefix, err := h.App.GetPrefix(sessionID)
	if err != nil {
		slog.Error("Unable to get the prefix", "details", err)
		prefix = "ex"
	}

	kg, err := store.FetchRDFData(store.GetSessionDB(sessionID), prefix, "Turtle")
	if err != nil {
		view.Error(w, "Error fetching RDF data")
	}
	err = view.CodeBlockComp(w, string(kg))
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) GetOntoData(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())

	onto, err := store.FetchOntoData(store.GetSessionDB(sessionID))
	if err != nil {
		view.Error(w, "Error fetching RDF data")
	}
	err = view.CodeBlockComp(w, onto)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) GetRDFExport(w http.ResponseWriter, r *http.Request) {
	sessionID := store.GetSessionID(r.Context())
	format := r.URL.Query().Get("format")

	var neo4jFormat string
	var contentType string

	switch format {
	case "xml":
		neo4jFormat = "RDF/XML"
		contentType = "application/xml"
	case "turtle":
		neo4jFormat = "Turtle"
		contentType = "text/turtle"
	case "jsonld":
		neo4jFormat = "JSON-LD"
		contentType = "application/ld+json"
	default:
		format = "ttl"
		neo4jFormat = "Turtle"
		contentType = "text/turtle"
	}

	prefix, err := h.App.GetPrefix(sessionID)
	if err != nil {
		slog.Error("Unable to get the prefix", "details", err)
		prefix = "ex"
	}

	filename := "export." + format
	data, err := store.FetchRDFData(store.GetSessionDB(sessionID), prefix, neo4jFormat)
	if err != nil {
		slog.Error("Error fetching RDF data", "details", err)
		view.Error(w, "Error fetching RDF data")
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
