package store

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"git.rwth-aachen.de/dsma/publications/software/konda/types"
	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

const (
	BASE_DB    = "basedb"
	OntoExt    = "onto"
	BATCH_SIZE = 200
	MAX_LENGTH = 200
)

var (
	DbClient Neo4jClient

	initCyphers = []string{
		`WITH '
			@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
			@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
			@prefix owl: <http://www.w3.org/2002/07/owl#> .
			@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
			@prefix skos: <http://www.w3.org/2004/02/skos/core#> .
			@prefix dc: <http://purl.org/dc/elements/1.1/> .
			@prefix sioc: <http://rdfs.org/sioc/ns#> .
			@prefix geo: <http://www.opengis.net/ont/geosparql#> .
			@prefix foaf: <http://xmlns.com/foaf/0.1/> .
			@prefix prov: <http://www.w3.org/ns/prov#> .
			@prefix cc: <http://creativecommons.org/ns#> .
			@prefix bfo: <http://purl.obolibrary.org/obo/bfo#> .
			@prefix vcard: <http://www.w3.org/2006/vcard/ns#> .
			@prefix sh: <http://www.w3.org/ns/shacl#> .
			@prefix cert: <http://www.w3.org/ns/auth/cert#> .
			@prefix bibo: <http://purl.org/ontology/bibo/> .
			@prefix dcat: <http://www.w3.org/ns/dcat#> .
			@prefix dcterms: <http://purl.org/dc/terms/> .
			@prefix gpo: <https://gpo.ontology.link/> .
			@prefix aeon: <https://gpo.ontology.link> .
			@prefix edam: <http://edamontology.org/> .
			@prefix euroscivoc: <http://data.europa.eu/8mn/euroscivoc/> .
			@prefix fair: <https://w3id.org/fair/principles/terms/FAIR-Vocabulary#> .
			@prefix gfo: <http://www.onto-med.de/ontologies/gfo-basic.owl#> .
			@prefix gist: <https://w3id.org/semanticarts/ns/ontology/gist/> .
			@prefix gistd: <https://w3id.org/semanticarts/ns/data/gist/> .
			@prefix gpo: <https://gpo.ontology.link/> .
			@prefix modsci: <https://w3id.org/skgo/modsci#> .
			@prefix premis3: <http://www.loc.gov/premis/rdf/v3/> .
			@prefix reproduceme: <https://w3id.org/reproduceme#> .
			@prefix ro: <http://purl.obolibrary.org/obo/ro.owl#> .
			@prefix obo: <http://purl.obolibrary.org/obo/> .
			@prefix dpv: <https://w3id.org/dpv#> .
			@prefix sepio: <http://purl.obolibrary.org/obo/sepio.owl#> .
			@prefix stato: <http://purl.obolibrary.org/obo/stato/stato.owl#> .
			@prefix oboe-core: <http://ecoinformatics.org/oboe/oboe.1.2/oboe-core.owl#> .
		' AS txt
		CALL n10s.nsprefixes.addFromText(txt) YIELD prefix, namespace
		RETURN prefix, namespace`,
	}

	initOntoCyphers = append(initCyphers,
		"call n10s.graphconfig.init({handleRDFTypes: \"LABELS_AND_NODES\"})",
		"CREATE CONSTRAINT n10s_unique_uri IF NOT EXISTS FOR (r:Resource) REQUIRE r.uri IS UNIQUE",
		"CREATE VECTOR INDEX resourceEmbeddings IF NOT EXISTS FOR (r:Resource) ON (r.embedding) OPTIONS {indexConfig: { `vector.dimensions`: 3072, `vector.similarity_function`: 'cosine' }}",
		"CREATE VECTOR INDEX classEmbeddings IF NOT EXISTS FOR (r:n4sch__Class) ON (r.embedding) OPTIONS {indexConfig: { `vector.dimensions`: 3072, `vector.similarity_function`: 'cosine' }}",
		"CREATE VECTOR INDEX relationshipEmbeddings IF NOT EXISTS FOR (r:n4sch__Relationship) ON (r.embedding) OPTIONS {indexConfig: { `vector.dimensions`: 3072, `vector.similarity_function`: 'cosine' }}",
	)
)

type Neo4jClient struct {
	Driver      neo4j.DriverWithContext
	Ctx         context.Context
	AIToken     string
	Credentials string

	Neo4jHTTPURL             string
	GenAIProvider            string
	GenAIResource            string
	GenAIEmbeddingDeployment string
}

func init() {
	ctx := context.Background()

	// Load environment variables.
	// Priority:
	// 1) ENV_FILE (explicit)
	// 2) .env (current working directory)
	if envFile := os.Getenv("ENV_FILE"); envFile != "" {
		_ = godotenv.Load(envFile)
	} else if _, err := os.Stat(".env"); err == nil {
		_ = godotenv.Load(".env")
	}

	dbUri := os.Getenv("NEO4J_DB_URL")
	if dbUri == "" {
		dbUri = "neo4j://localhost"
	}
	dbUser := os.Getenv("NEO4J_USERNAME")
	if dbUser == "" {
		dbUser = "neo4j"
	}
	dbPassword := os.Getenv("NEO4J_PASSWORD")
	if dbPassword == "" {
		dbPassword = "neo4j"
	}

	neo4jHTTPURL := getEnv("NEO4J_HTTP_URL", "http://localhost:7474")
	genaiToken := os.Getenv("NEO4J_GENAI_TOKEN")
	if genaiToken == "" {
		genaiToken = os.Getenv("AOAI_CHAT_COMPLETIONS_API_KEY")
	}
	genaiProvider := getEnv("NEO4J_GENAI_PROVIDER", "AzureOpenAI")
	genaiResource := os.Getenv("NEO4J_GENAI_RESOURCE")
	genaiEmbeddingDeployment := getEnv("NEO4J_GENAI_EMBEDDING_DEPLOYMENT", "text-embedding-3-large")

	d, err := neo4j.NewDriverWithContext(
		dbUri,
		neo4j.BasicAuth(dbUser, dbPassword, ""))
	if err != nil {
		slog.Error("Error creating neo4j driver", "details", "err")
	}

	err = d.VerifyConnectivity(ctx)
	if err != nil {
		panic(err)
	}

	DbClient = Neo4jClient{
		Driver:      d,
		Ctx:         ctx,
		AIToken:     genaiToken,
		Credentials: dbUser + ":" + dbPassword,

		Neo4jHTTPURL:             neo4jHTTPURL,
		GenAIProvider:            genaiProvider,
		GenAIResource:            genaiResource,
		GenAIEmbeddingDeployment: genaiEmbeddingDeployment,
	}
}

func UploadOntology(path string, database string) error {
	mimeFormat := map[string]string{
		".xml":    "RDF/XML",
		".owl":    "RDF/XML",
		".rdf":    "RDF/XML",
		".ttl":    "Turtle",
		".json":   "JSON-LD",
		".jsonld": "JSON-LD",
	}
	format, ok := mimeFormat[filepath.Ext(path)]
	if !ok {
		return fmt.Errorf("could not find format for extension: %s", filepath.Ext(path))
	}

	// Open a new session
	session := DbClient.Driver.NewSession(DbClient.Ctx, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(DbClient.Ctx)
	query := "CALL n10s.rdf.import.fetch($filePath, $format, {languageFilter: 'en'})"

	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("could not get the full path: %w", err)
	}

	// Execute the query
	result, err := session.Run(DbClient.Ctx, query, map[string]any{
		"filePath": "file:///" + abs,
		"format":   format,
	})
	if err != nil {
		return fmt.Errorf("failed to execute import query: %w", err)
	}

	// Process the result
	if result.Next(DbClient.Ctx) {
		record := result.Record()
		status, ok := record.Get("terminationStatus")
		if !ok {
			return fmt.Errorf("terminationStatus not found in result")
		}

		if statusStr, ok := status.(string); !ok || statusStr != "OK" {
			info, _ := record.Get("extraInfo")
			info = toString(info)
			return fmt.Errorf("import failed for %s with terminationStatus: %v, %s", filepath.Base(path), status, info)
		}
	} else if err = result.Err(); err != nil {
		return fmt.Errorf("error processing result: %w", err)
	}

	return nil
}

func SearchNodeByURI(database, uri string) (types.URIMatch, error) {
	// First search in OntoDB, then in BaseDB
	domainMatches, err := searchNodeByURI(database, uri)
	if err != nil {
		baseMatches, err := searchNodeByURI(BASE_DB, uri)
		if err != nil {
			return types.URIMatch{}, err
		}
		return baseMatches, err
	}
	return domainMatches, nil
}

func searchNodeByURI(database, uri string) (types.URIMatch, error) {
	session := DbClient.Driver.NewSession(DbClient.Ctx, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(DbClient.Ctx)

	query := `
		MATCH (n)
		WHERE n.uri = $uri
		RETURN 
			coalesce(n.skos__prefLabel, n.rdfs__label, n.skos__altLabel, n.n4sch__prefLabel, n.n4sch__label, n.n4sch__name, n.n4sch__altLabel, "") as label,
			coalesce(n.skos__definition, n.rdfs__comment, n.n4sch__definition, n.n4sch__comment, "") as description,
			n.uri as uri
	`

	params := map[string]any{
		"uri": uri,
	}

	result, err := session.Run(DbClient.Ctx, query, params)
	if err != nil {
		return types.URIMatch{}, fmt.Errorf("query execution failed: %w", err)
	}

	if result.Next(DbClient.Ctx) {
		record := result.Record()
		label, _ := record.Get("label")
		description, _ := record.Get("description")
		uriVal, _ := record.Get("uri")

		return types.URIMatch{
			Label:       toString(label),
			Description: toString(description),
			URI:         toString(uriVal),
			Score:       1.0, // Default score for single match; or omit if not relevant
		}, nil
	}

	if err = result.Err(); err != nil {
		return types.URIMatch{}, fmt.Errorf("result iteration failed: %w", err)
	}

	return types.URIMatch{}, fmt.Errorf("no node found with URI: %s", uri)
}

func SimilaritySearch(searchTerm, database, index string, number int) ([]types.URIMatch, error) {
	// TODO: We are importing now as rdf not as onto. Therefore we are losing the information about Class vs Relation
	domainMatches, err := similaritySearch(searchTerm, database, "resourceEmbeddings", number)
	if err != nil {
		return nil, err
	}

	baseMatches, err := similaritySearch(searchTerm, BASE_DB, index, number)
	if err != nil {
		return nil, err
	}

	return types.MergeMatches(number, domainMatches, baseMatches), nil
}

func similaritySearch(searchTerm, database, index string, number int) ([]types.URIMatch, error) {
	session := DbClient.Driver.NewSession(DbClient.Ctx, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(DbClient.Ctx)

	if DbClient.AIToken == "" || DbClient.GenAIResource == "" {
		return nil, fmt.Errorf("neo4j GenAI is not configured (set NEO4J_GENAI_RESOURCE and NEO4J_GENAI_TOKEN or AOAI_CHAT_COMPLETIONS_API_KEY)")
	}

	// use 100 nearest neighbors because of a bug in neo4j where not the best match was found, then limit to $number
	query := `
		WITH genai.vector.encode($searchTerm, $provider, { token: $token, resource: $resource, deployment: $deployment }) AS queryVector
		CALL db.index.vector.queryNodes($index, 100, queryVector)
		YIELD node, score
		RETURN 
			coalesce(node.skos__prefLabel, node.rdfs__label, node.skos__altLabel, node.n4sch__prefLabel, node.n4sch__label, node.n4sch__name, node.n4sch__altLabel, "") as label,
			coalesce(node.skos__definition, node.rdfs__comment, node.n4sch__definition, node.n4sch__comment, "") as description,
			node.uri as uri,
			score
		Limit $number;
		`
	params := map[string]any{
		"searchTerm": searchTerm,
		"token":      DbClient.AIToken,
		"provider":   DbClient.GenAIProvider,
		"resource":   DbClient.GenAIResource,
		"deployment": DbClient.GenAIEmbeddingDeployment,
		"number":     number,
		"index":      index,
	}

	result, err := session.Run(DbClient.Ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	var matches []types.URIMatch
	for result.Next(DbClient.Ctx) {
		record := result.Record()
		label, _ := record.Get("label")
		description, _ := record.Get("description")
		uri, _ := record.Get("uri")
		score, _ := record.Get("score")

		matches = append(matches, types.URIMatch{
			Label:       toString(label),
			Description: toString(description),
			URI:         toString(uri),
			Score:       score.(float64),
		})

		if err = result.Err(); err != nil {
			return nil, fmt.Errorf("result iteration failed: %w", err)
		}
	}

	// Sort matches for each search term by score in descending order
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	return matches, nil
}

func BulkSimilaritySearch(labels []string, database, index string, minScore float64, maxNumber int) (types.URIMatchMap, error) {
	if len(labels) == 0 {
		empty := make(types.URIMatchMap)
		return empty, nil
	}
	domainMatches, err := bulkSimilaritySearch(labels, minScore, database, "resourceEmbeddings")
	if err != nil {
		return nil, err
	}

	baseMatches, err := bulkSimilaritySearch(labels, minScore, BASE_DB, index)
	if err != nil {
		return nil, err
	}

	matches := types.MergeMatchMaps(maxNumber, domainMatches, baseMatches)

	return matches, nil

}

func bulkSimilaritySearch(labels []string, minScore float64, database, index string) (types.URIMatchMap, error) {
	// Create a session to run transactions
	session := DbClient.Driver.NewSession(DbClient.Ctx, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(DbClient.Ctx)

	if DbClient.AIToken == "" || DbClient.GenAIResource == "" {
		return nil, fmt.Errorf("neo4j GenAI is not configured (set NEO4J_GENAI_RESOURCE and NEO4J_GENAI_TOKEN or AOAI_CHAT_COMPLETIONS_API_KEY)")
	}

	labels = uniqueStrings(labels)

	// Cypher query
	// use 100 nearest neighbors because of a bug where not the best match was found, then limit to 10
	query := `
		WITH $labels AS labels
		CALL genai.vector.encodeBatch(labels, $provider, { token: $token, resource: $resource, deployment: $deployment }) YIELD index, resource, vector
		WITH index, resource, vector
		CALL db.index.vector.queryNodes($index, 100, vector) YIELD node, score
		WITH resource AS searchTerm, node.uri AS uri, coalesce(node.skos__prefLabel, node.rdfs__label, node.skos__altLabel, node.n4sch__prefLabel, node.n4sch__label, node.n4sch__name, node.n4sch__altLabel, "") as label,
			coalesce(node.skos__definition, node.rdfs__comment,node.n4sch__definition, node.n4sch__comment, "") as description, score
		ORDER BY searchTerm, score DESC
		WITH searchTerm, collect({label: label, description: description, uri: uri, score: score})[0..10] AS matches
		RETURN searchTerm, matches;
		`
	params := map[string]any{
		"labels":     labels,
		"index":      index,
		"token":      DbClient.AIToken,
		"provider":   DbClient.GenAIProvider,
		"resource":   DbClient.GenAIResource,
		"deployment": DbClient.GenAIEmbeddingDeployment,
	}

	// Execute the query directly within the session
	result, err := session.Run(DbClient.Ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	matches := make(types.URIMatchMap)
	for result.Next(DbClient.Ctx) {
		record := result.Record()
		label, _ := record.Get("searchTerm")
		topMatches, _ := record.Get("matches")

		var topResults []types.URIMatch
		for _, r := range topMatches.([]any) {
			res := r.(map[string]any)
			topResults = append(topResults, types.URIMatch{
				URI:         toString(res["uri"]),
				Label:       toString(res["label"]),
				Description: toString(res["description"]),
				Score:       res["score"].(float64),
			})
		}

		// Sort matches for each search term by score in descending order
		sort.Slice(topResults, func(i, j int) bool {
			return topResults[i].Score > topResults[j].Score
		})

		// Add the best match
		if len(topResults) > 0 && topResults[0].Score >= minScore {
			matches[toString(label)] = topResults
		}
	}

	if err = result.Err(); err != nil {
		return nil, fmt.Errorf("result iteration failed: %w", err)
	}

	return matches, nil
}

// uniqueStrings returns a new slice containing only unique strings from the input slice.
func uniqueStrings(input []string) []string {
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

func ExecuteQuery(query, database string, params map[string]any) *neo4j.EagerResult {
	result, _ := neo4j.ExecuteQuery(DbClient.Ctx, DbClient.Driver, query, params,
		neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase(database))
	return result
}

func toString(value any) string {
	if str, ok := value.(string); ok {
		return str
	}
	return "" // Return an empty string if the value is nil or not a string
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func CreateBaseEmbeddings() {
	CreateEmbeddings(BASE_DB, nil)
	slog.Info("Finished creating basedb embeddings.")
}

// TODO: invert createEmbedding and fetch Resource so that RAM does not overflow on millions of entities
// Since we are potentially holding all of the resources in the map
// CreateEmbeddings processes batches of resources to generate embeddings
func CreateEmbeddings(database string, reportProgress types.ProgressCallback) error {
	resources, err := fetchResources(database)
	if err != nil {
		return fmt.Errorf("failed to fetch resources: %w", err)
	}

	if DbClient.AIToken == "" || DbClient.GenAIResource == "" {
		return fmt.Errorf("neo4j GenAI is not configured (set NEO4J_GENAI_RESOURCE and NEO4J_GENAI_TOKEN or AOAI_CHAT_COMPLETIONS_API_KEY)")
	}

	if len(resources) == 0 {
		// TODO: do we want an error here? fmt.Errorf("No resources found without embeddings.")
		return nil
	}

	textLen := 0

	session := DbClient.Driver.NewSession(DbClient.Ctx, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(DbClient.Ctx)

	if reportProgress != nil {
		progress := 0.0
		msg := "Starting to create term embeddings"
		reportProgress(progress, msg)
	}

	for i := 0; i < len(resources); i += BATCH_SIZE {
		end := i + BATCH_SIZE
		if end > len(resources) {
			end = len(resources)
		}
		batch := resources[i:end]

		// Create the text inputs for embedding generation
		toEncode := make([]string, len(batch))
		for j, res := range batch {
			encode := trimToMaxLength(res["label"] + ": " + res["description"])
			toEncode[j] = encode
			textLen += len(encode)
		}

		query := `
        CALL genai.vector.encodeBatch($toEncode, $provider, { token: $token, resource: $resource, deployment: $deployment })
        YIELD index, vector
        WITH $batch[index] AS resource, vector
        WHERE vector IS NOT NULL
        MATCH (r:Resource {uri: resource.uri})
        CALL db.create.setNodeVectorProperty(r, 'embedding', vector)
        `
		params := map[string]any{
			"toEncode":   toEncode,
			"batch":      batch,
			"token":      DbClient.AIToken,
			"provider":   DbClient.GenAIProvider,
			"resource":   DbClient.GenAIResource,
			"deployment": DbClient.GenAIEmbeddingDeployment,
		}

		result, err := session.Run(DbClient.Ctx, query, params)
		if err != nil {
			return fmt.Errorf("failed to execute embedding query: %w", err)
		}

		// Process the result
		if result.Next(DbClient.Ctx) {
			_ = result.Record()
		} else if err = result.Err(); err != nil {
			return fmt.Errorf("error processing result: %w", err)
		}

		if reportProgress != nil {
			progress := float64(end) / float64(len(resources))
			msg := fmt.Sprintf("Created embeddings for %d out of %d terms", end, len(resources))
			reportProgress(progress, msg)
		}
	}
	// log.Printf("Embeddings successfully generated, and used %d tokens - so about: %f Euro", textLen/4, float64(textLen)/4/1000*0.000124)
	return nil
}

// fetchResources retrieves Resource nodes without embeddings
func fetchResources(database string) ([]map[string]string, error) {
	session := DbClient.Driver.NewSession(DbClient.Ctx, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(DbClient.Ctx)

	query := `
		MATCH (r:Resource)
		WITH coalesce(r.skos__prefLabel, r.rdfs__label, r.skos__altLabel, r.n4sch__prefLabel, r.n4sch__label, r.n4sch__name, r.n4sch__altLabel, "") as label,
		coalesce(r.skos__definition, r.rdfs__comment,r.n4sch__definition, r.n4sch__comment, "") as description,
		r
		WHERE size(label) > 0 AND r.embedding IS NULL
		RETURN r.uri AS uri, label, description
	`
	result, err := session.Run(DbClient.Ctx, query, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	var resources []map[string]string
	for result.Next(DbClient.Ctx) {
		record := result.Record()

		// TODO: need to fix these? map labels always to the same, what about other ontologies?
		uri, _ := record.Get("uri")
		label, _ := record.Get("label")
		description, _ := record.Get("description")

		resources = append(resources, map[string]string{
			"uri":         toString(uri),
			"label":       toString(label),
			"description": toString(description),
		})
	}
	if err = result.Err(); err != nil {
		return nil, fmt.Errorf("result iteration error: %w", err)
	}
	return resources, nil
}

// trimToMaxLength returns the first non-empty string from the provided arguments,
// ensuring the output does not exceed maxLength.
func trimToMaxLength(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > MAX_LENGTH {
		return trimmed[:MAX_LENGTH]
	}
	return trimmed
}

func EnsureSessionOntoDatabase(database, prefix, namespace string) error {
	database = database + OntoExt
	session := DbClient.Driver.NewSession(DbClient.Ctx, neo4j.SessionConfig{DatabaseName: "system"})

	// Check if database exists
	query := "SHOW DATABASES"
	result, err := session.Run(DbClient.Ctx, query, nil)
	if err != nil {
		return fmt.Errorf("failed to check databases: %v", err)
	}

	dbExists := false
	for result.Next(DbClient.Ctx) {
		record := result.Record()
		if toString(record.Values[0]) == database {
			dbExists = true
			break
		}
	}

	if !dbExists {
		result, err := session.Run(DbClient.Ctx, "CREATE DATABASE $database IF NOT EXISTS", map[string]any{"database": database})
		if err != nil {
			return fmt.Errorf("failed to create database: %v", err)
		}
		// Iterate over the result set
		for result.Next(DbClient.Ctx) {
			_ = result.Record()
		}

		// Initialize database
		initSession := DbClient.Driver.NewSession(DbClient.Ctx, neo4j.SessionConfig{DatabaseName: database})
		defer initSession.Close(DbClient.Ctx)

		for _, cypher := range initOntoCyphers {
			_, err := initSession.Run(DbClient.Ctx, cypher, nil)
			if err != nil {
				return fmt.Errorf("failed to execute Cypher: %v", err)
			}
		}

		_, err = initSession.Run(DbClient.Ctx, "CALL n10s.nsprefixes.add($prefix, $namespace)", map[string]any{"namespace": namespace, "prefix": prefix})
		if err != nil {
			return fmt.Errorf("failed to execute Cypher: %v", err)
		}

		// Wait for transaction to be complete
		for result.Next(DbClient.Ctx) {
			_ = result.Record()
		}
	}

	return nil
}

func EnsureSessionDatabase(database, prefix, namespace string) error {
	EnsureSessionOntoDatabase(database, prefix, namespace)

	session := DbClient.Driver.NewSession(DbClient.Ctx, neo4j.SessionConfig{DatabaseName: "system"})

	// Check if database exists
	query := "SHOW DATABASES"
	result, err := session.Run(DbClient.Ctx, query, nil)
	if err != nil {
		return fmt.Errorf("failed to check databases: %v", err)
	}

	dbExists := false
	for result.Next(DbClient.Ctx) {
		record := result.Record()
		if toString(record.Values[0]) == database {
			dbExists = true
			break
		}
	}

	if !dbExists {
		result, err := session.Run(DbClient.Ctx, "CREATE DATABASE $database IF NOT EXISTS", map[string]any{"database": database})
		if err != nil {
			return fmt.Errorf("failed to create database: %v", err)
		}
		// TODO: this is hacky.. make it a proper transaction?
		// Wait to finish by iterating over the result set
		for result.Next(DbClient.Ctx) {
			_ = result.Record()
		}

		// Initialize database
		initSession := DbClient.Driver.NewSession(DbClient.Ctx, neo4j.SessionConfig{DatabaseName: database})
		defer initSession.Close(DbClient.Ctx)

		for _, cypher := range initCyphers {
			_, err := initSession.Run(DbClient.Ctx, cypher, nil)
			if err != nil {
				return fmt.Errorf("failed to execute Cypher: %v", err)
			}
		}

		_, err = initSession.Run(DbClient.Ctx, fmt.Sprintf("CALL n10s.nsprefixes.addFromText('@prefix %s: <%s> .');", prefix, namespace), nil)
		if err != nil {
			return fmt.Errorf("failed to execute Cypher: %v", err)
		}
	}

	return nil
}

func DeleteSessionDBs(database string) error {
	err := DeleteDatabase(database)
	if err != nil {
		return err
	}
	err = DeleteDatabase(database + OntoExt)
	if err != nil {
		return err
	}
	return nil
}

// TODO: This can be abused i think, since session might be read from client
func DeleteDatabase(database string) error {
	session := DbClient.Driver.NewSession(DbClient.Ctx, neo4j.SessionConfig{DatabaseName: "system"})
	defer session.Close(DbClient.Ctx)

	_, err := session.Run(DbClient.Ctx, fmt.Sprintf("DROP DATABASE %s IF EXISTS", database), nil)
	if err != nil {
		return fmt.Errorf("failed to delete database: %w", err)
	}

	return nil
}

func GetSessionOntoDB(sessionID string) string {
	return sanitizeDbName(sessionID) + OntoExt
}

func GetSessionDB(sessionID string) string {
	return sanitizeDbName(sessionID)
}

func sanitizeDbName(sessionID string) string {
	// Replace special characters
	sanitized := strings.ReplaceAll(sessionID, "_", "")
	sanitized = strings.ReplaceAll(sanitized, "-", "")
	sanitized = strings.ReplaceAll(sanitized, ".", "")
	sanitized = strings.ReplaceAll(sanitized, ",", "")
	sanitized = strings.ToLower(sanitized)

	// Ensure the first character is alphabetic; prepend 'db-' if not
	if len(sanitized) == 0 || !isAlphabetic(sanitized[0]) {
		sanitized = "db" + sanitized
	}

	// Truncate to 63 characters if necessary
	if len(sanitized) > 63 {
		sanitized = sanitized[:63]
	}

	// Check if the sanitized name ends with a dot or dash and remove it
	if strings.HasSuffix(sanitized, ".") || strings.HasSuffix(sanitized, "-") {
		sanitized = sanitized[:len(sanitized)-1]
	}

	return sanitized
}

func isAlphabetic(char byte) bool {
	return (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z')
}

// CreateRDFURI ensures each entity has a proper URI
func CreateRDFURI(baseNamespace, label string) string {
	// Check if label is already a valid URL
	if u, err := url.Parse(label); err == nil && u.Scheme != "" && u.Host != "" {
		// It's a full URL (like a DOI) — use as-is
		return label
	}

	// Normalize label
	normalized := strings.ToLower(strings.TrimSpace(label))

	// Replace non-alphanumeric characters with dashes
	re := regexp.MustCompile(`[^\w]+`)
	slug := re.ReplaceAllString(normalized, "-")
	slug = strings.Trim(slug, "-")

	// Combine into URI
	return fmt.Sprintf("%s%s", baseNamespace, slug)
}

// FetchOntoData fetches data with basic authentication.
func FetchOntoData(database string) (string, error) {
	base := strings.TrimRight(DbClient.Neo4jHTTPURL, "/")
	url := fmt.Sprintf("%s/rdf/%s/onto", base, database)

	// Create the request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add basic auth header
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(DbClient.Credentials))
	req.Header.Add("Authorization", authHeader)

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read the body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(bodyBytes), nil
}

// FetchRDFData executes a Cypher query and retrieves RDF-formatted data.
func FetchRDFData(database, prefix, rdfFormat string) ([]byte, error) {
	// Construct the URL for the RDF endpoint
	base := strings.TrimRight(DbClient.Neo4jHTTPURL, "/")
	url := fmt.Sprintf("%s/rdf/%s/cypher", base, database)

	cypherQuery := fmt.Sprintf(
		`MATCH (n:%s)
		CALL apoc.path.subgraphAll(n, {minLevel: 0, maxLevel: 10}) 
		YIELD nodes, relationships 
		UNWIND nodes AS node
		WITH collect(DISTINCT apoc.map.removeKey(properties(node), 'external')) AS cleanNodes, relationships
		RETURN cleanNodes AS nodes, relationships
		LiMIT 150`,
		neo4jLabel(prefix, "Dataset"))

	// Create the RDFQuery payload
	payload, err := json.Marshal(
		struct {
			Cypher string `json:"cypher"`
			Format string `json:"format"`
		}{
			Cypher: cypherQuery,
			Format: rdfFormat,
		})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON payload: %w", err)
	}

	// Create the HTTP POST request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(DbClient.Credentials)))

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for non-OK status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

func CreateKnowledgeGraph(database, namespace, prefix, datasetName string, contextFiles []string,
	entities []types.Entity, relations []types.Relation, mappings types.URIMatchMap) error {

	session := DbClient.Driver.NewSession(DbClient.Ctx, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(DbClient.Ctx)

	// Clean the entire graph (careful in production)
	result, err := session.Run(DbClient.Ctx, "MATCH (n) DETACH DELETE n", nil)
	if err != nil {
		return err
	}
	// Wait to finish by iterating over the result set
	for result.Next(DbClient.Ctx) {
		_ = result.Record()
	}

	if err := InitNeoSemanticsPrefixes(session, prefix, namespace); err != nil {
		return err
	}

	datasetURI := CreateRDFURI(namespace, datasetName)

	if err := CreateOntologySchema(session, namespace); err != nil {
		return err
	}
	if err := InsertEntitiesAndTypes(session, prefix, namespace, datasetURI, datasetName, contextFiles, entities, mappings); err != nil {
		return err
	}
	if err := InsertRelations(session, prefix, namespace, relations, mappings); err != nil {
		return err
	}

	return nil
}

func InitNeoSemanticsPrefixes(session neo4j.SessionWithContext, prefix, namespace string) error {
	queries := []struct {
		query  string
		params map[string]any
	}{
		{
			query: `CALL n10s.graphconfig.init()`,
		},
		{query: initCyphers[0]},
		{
			query: `CALL n10s.nsprefixes.add($prefix, $namespace)`,
			params: map[string]any{
				"prefix":    prefix,
				"namespace": namespace,
			},
		},
	}

	for _, q := range queries {
		_, err := session.Run(DbClient.Ctx, q.query, q.params)
		if err != nil {
			return err
		}
	}
	return nil
}

func CreateOntologySchema(session neo4j.SessionWithContext, namespace string) error {
	query := `
		UNWIND [
			{uri: $datasetURI, label: "Dataset"},
			{uri: $conceptURI, label: "Concept"},
			{uri: $categoryURI, label: "Category"},
			{uri: $contextFileURI, label: "Context File"}
		] AS cls
		MERGE (c {uri: cls.uri})
		SET c:Resource:owl__Class,
		    c.rdfs__label = cls.label

		WITH *
		UNWIND [
			{uri: $hasConceptURI, label: "has concept"},
			{uri: $hasFileURI, label: "has file"}
		] AS prop
		MERGE (p {uri: prop.uri})
		SET p:Resource:owl__ObjectProperty,
		    p.rdfs__label = prop.label
	`
	params := map[string]any{
		"datasetURI":     CreateRDFURI(namespace, "Dataset"),
		"conceptURI":     CreateRDFURI(namespace, "Concept"),
		"categoryURI":    CreateRDFURI(namespace, "Category"),
		"contextFileURI": CreateRDFURI(namespace, "Context File"),
		"hasConceptURI":  CreateRDFURI(namespace, "has Concept"),
		"hasFileURI":     CreateRDFURI(namespace, "has File"),
	}
	_, err := session.Run(DbClient.Ctx, query, params)
	return err
}

func InsertEntitiesAndTypes(session neo4j.SessionWithContext, prefix, namespace, datasetURI, datasetLabel string, contextFiles []string, entities []types.Entity, mappings types.URIMatchMap) error {
	entityData := []map[string]any{}

	for _, e := range entities {
		entityMatch := mappings.GetBestMatch(e.Entity)
		typeMatch := mappings.GetBestMatch(e.Type)

		entityURI := CreateRDFURI(namespace, e.Entity)
		entityLabel := e.Entity
		entitySeeAlsoURI := entityMatch.URI
		entitySeeAlsoLabel := entityMatch.Label
		if entityMatch.Replace {
			entityURI = entityMatch.URI
			entityLabel = entityMatch.Label
		}

		typeURI := CreateRDFURI(namespace, e.Type)
		typeLabel := e.Type
		typeSeeAlsoURI := typeMatch.URI
		typeSeeAlsoLabel := typeMatch.Label
		if typeMatch.Replace {
			typeURI = typeMatch.URI
			typeLabel = typeMatch.Label
		}

		entityData = append(entityData, map[string]any{
			"entityURI":          entityURI,
			"entityLabel":        entityLabel,
			"entityExternal":     entityMatch.Replace,
			"entitySeeAlsoURI":   entitySeeAlsoURI,
			"entitySeeAlsoLabel": entitySeeAlsoLabel,
			"typeURI":            typeURI,
			"typeLabel":          typeLabel,
			"typeExternal":       typeMatch.Replace,
			"typeSeeAlsoURI":     typeSeeAlsoURI,
			"typeSeeAlsoLabel":   typeSeeAlsoLabel,
			"contextFiles":       contextFiles,
		})
	}

	contextFileData := []map[string]any{}
	for _, file := range contextFiles {
		contextFileData = append(contextFileData, map[string]any{
			"fileURI":  CreateRDFURI(namespace, file),
			"fileName": file,
		})
	}

	query := fmt.Sprintf(`
		//  Dataset
		MERGE (d {uri: $datasetURI})
		SET d:Resource:%s,
		    d.rdfs__label = $datasetLabel

		//  Context Files
		WITH d, $contextFiles AS cFiles
		UNWIND cFiles AS file
		MERGE (f {uri: file.fileURI})
		SET f:Resource:%s,
			f.rdfs__label = file.fileName

		WITH d, f
		CALL apoc.create.relationship(d, $hasFileURI, {uri: $hasContextFileURI, rdfs__label: "has context file"}, f) YIELD rel AS _


		WITH d, $entities AS entities
		UNWIND entities AS e
		MERGE (n {uri: e.entityURI})
		SET n:Resource:%s,
		    n.rdfs__label = e.entityLabel

		// Create has concept relationship with full URI using APOC
		WITH d, n, e
		CALL apoc.create.relationship(d, $hasConceptURI, {uri: $hasConceptURI, rdfs__label: "has concept"}, n) YIELD rel AS _

		WITH e, n
		MERGE (t {uri: e.typeURI})
		SET t:Resource:owl__Class:%s,
		    t.rdfs__label = e.typeLabel

		// SKOS-style hierarchy
		MERGE (n)-[:skos__broader {rdfs__label: "has broader", uri: "http://www.w3.org/2004/02/skos/core#broader"}]->(t)

		// owl:seeAlso for entity (if not replaced)
		FOREACH (_ IN CASE WHEN e.entitySeeAlsoURI <> "" AND e.entityExternal = false THEN [1] ELSE [] END |
			MERGE (m {uri: e.entitySeeAlsoURI})
			SET m:Resource:%s,
			    m.rdfs__label = e.entitySeeAlsoLabel,
				m.external = true
			MERGE (n)-[:skos__relatedMatch {rdfs__label: "has related match", uri: "http://www.w3.org/2004/02/skos/core#relatedMatch"}]->(m)
		)

		// owl:seeAlso for type (if not replaced)
		FOREACH (_ IN CASE WHEN e.typeSeeAlsoURI <> "" AND e.typeExternal = false THEN [1] ELSE [] END |
			MERGE (m {uri: e.typeSeeAlsoURI})
			SET m:Resource:%s,
			    m.rdfs__label = e.typeSeeAlsoLabel,
				m.external = true
			MERGE (t)-[:skos__relatedMatch {rdfs__label: "has related match", uri: "http://www.w3.org/2004/02/skos/core#relatedMatch"}]->(m)
		)
	`,
		neo4jLabel(prefix, "Dataset"),
		neo4jLabel(prefix, "ContextFile"),
		neo4jLabel(prefix, "Concept"),
		neo4jLabel(prefix, "Category"),
		neo4jLabel(prefix, "Concept"),
		neo4jLabel(prefix, "Concept"),
	)

	params := map[string]any{
		"datasetURI":        datasetURI,
		"datasetLabel":      datasetLabel,
		"entities":          entityData,
		"contextFiles":      contextFileData,
		"hasConceptURI":     CreateRDFURI(namespace, "has concept"),
		"hasContextFileURI": CreateRDFURI(namespace, "has Context File"),
		"hasFileURI":        CreateRDFURI(namespace, "has File"),
	}
	_, err := session.Run(DbClient.Ctx, query, params)
	return err
}

func InsertRelations(session neo4j.SessionWithContext, prefix, namespace string, relations []types.Relation, mappings types.URIMatchMap) error {
	relationData := []map[string]any{}

	for _, r := range relations {
		s := mappings.GetBestMatch(r.Subject)
		o := mappings.GetBestMatch(r.Object)
		v := mappings.GetBestMatch(r.Verb)

		subjURI := CreateRDFURI(namespace, r.Subject)
		subjLabel := r.Subject
		if s.Replace {
			subjURI = s.URI
			subjLabel = s.Label
		}

		objURI := CreateRDFURI(namespace, r.Object)
		objLabel := r.Object
		if o.Replace {
			objURI = o.URI
			objLabel = o.Label
		}

		predURI := CreateRDFURI(namespace, r.Verb)
		predLabel := r.Verb
		if v.Label != "" {
			predURI = v.URI
			predLabel = v.Label
		}

		if v.Inverse {
			subjURI, objURI = objURI, subjURI
			subjLabel, objLabel = objLabel, subjLabel
			s, o = o, s
		}

		relationData = append(relationData, map[string]any{
			"subjectURI":      subjURI,
			"subjectLabel":    subjLabel,
			"subjectExternal": s.Replace,
			"objectURI":       objURI,
			"objectLabel":     objLabel,
			"objectExternal":  o.Replace,
			"verbURI":         predURI,
			"verbLabel":       predLabel,
		})
	}

	query := `
		UNWIND $relations AS rel

		MERGE (p {uri: rel.verbURI})
		SET p:Resource:owl__ObjectProperty,
		    p.rdfs__label = rel.verbLabel

		MERGE (s {uri: rel.subjectURI})
		MERGE (o {uri: rel.objectURI})
		WITH rel, s, o
		CALL apoc.create.relationship(s, rel.verbURI, {uri: rel.verbURI, rdfs__label: rel.verbLabel}, o) YIELD rel AS _
		RETURN COUNT(*) AS _
	`

	params := map[string]any{
		"relations": relationData,
	}
	result, err := session.Run(DbClient.Ctx, query, params)
	if result.Next(DbClient.Ctx) {
		_ = result.Record()
	} else if err = result.Err(); err != nil {
		return fmt.Errorf("error processing result: %w", err)
	}
	return err
}

func neo4jLabel(prefix, name string) string {
	return fmt.Sprintf("%s__%s", prefix, name)
}
