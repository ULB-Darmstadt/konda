package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"git.rwth-aachen.de/dsma/publications/software/konda/types"
)

const (
	BASE_URL = "https://api.terminology.tib.eu"
)

// term represents a single term in the TIB search response
type term struct {
	IRI          string   `json:"iri"`
	Label        string   `json:"label"`
	Description  []string `json:"description"`
	OntologyName string   `json:"ontology_name"`
}

// tIBResponse represents the structure of the TIB search API response
type tIBResponse struct {
	Response struct {
		Docs     []term `json:"docs"`
		NumFound int    `json:"numFound"`
	} `json:"response"`
}

// QueryForOntology queries the TIB API for the given search term and returns the most relevant terms
func QueryForOntology(searchTerm string) ([]types.Ontology, error) {
	if searchTerm == "" {
		return nil, nil
	}

	params := url.Values{}
	params.Add("q", searchTerm)
	params.Add("rows", "5") // Limit to top 5 results; adjust as needed

	apiURL := fmt.Sprintf("%s/%s?%s", BASE_URL, "api/search", params.Encode())

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 response: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var tibResponse tIBResponse
	if err := json.Unmarshal(body, &tibResponse); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %v", err)
	}

	if tibResponse.Response.NumFound == 0 {
		return nil, nil // No results found
	}

	var ontologies []types.Ontology
	for _, onto := range tibResponse.Response.Docs {
		description := strings.Join(onto.Description, "\n\n")
		if description == "" {
			description = "<no description available>"
		}

		ontologies = append(ontologies, types.Ontology{
			IRI:          onto.IRI,
			OntologyName: onto.OntologyName,
			Description:  description,
			SearchTerm:   onto.Label,
			Source:       "TIB",
			Content:      "",
		})
	}

	return ontologies, nil
}

type OntoSchema struct {
	OntologyID          string `json:"ontologyId"`
	Config              Config `json:"config"`
	NumberOfTerms       int    `json:"numberOfTerms"`
	NumberOfProperties  int    `json:"numberOfProperties"`
	NumberOfIndividuals int    `json:"numberOfIndividuals"`
}

type Config struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	FileLocation string `json:"fileLocation"`
}

func SetNumberOfTerms(ontology *types.Ontology) error {
	// skipping if not from TIB or already filled
	if ontology.Source != "TIB" || ontology.IRI == "" || ontology.NumberOfItems != 0 {
		return nil
	}

	// Query API to get the FileLocation
	params := url.Values{}
	params.Add("lang", "en")

	apiURL := fmt.Sprintf("%s/%s/%s?%s", BASE_URL, "api/ontologies", ontology.OntologyName, params.Encode())

	resp, err := http.Get(apiURL)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error while getting content location by calling %s, received non-200 response: %d", apiURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	var ontoSchema OntoSchema
	if err := json.Unmarshal(body, &ontoSchema); err != nil {
		return fmt.Errorf("failed to parse JSON response: %v", err)
	}

	ontology.NumberOfItems = ontoSchema.NumberOfTerms + ontoSchema.NumberOfIndividuals + ontoSchema.NumberOfProperties

	return nil
}

func PopulateOntologyContent(ontology *types.Ontology) error {
	if ontology.Source != "TIB" || ontology.Content != "" {
		return nil
	}

	// Try to get the ontology from the API first, if that failed then from the UI link
	ontologyBody, ontoSchema, err := getOntologyContentFromAPI(ontology)
	if err != nil {
		if ontoSchema == nil {
			return err
		}
		ontologyBody, err = getOntologyContentFromUI(ontology)
		if err != nil {
			return err
		}
	}

	name := cleanFilename(ontology.OntologyName, ontoSchema.Config.FileLocation)
	if filepath.Ext(ontoSchema.Config.FileLocation) == "" {
		name = name + guessExtension(ontologyBody)
	}

	ontology.Content = string(ontologyBody)
	ontology.FileName = name

	return nil
}

func getOntologyContentFromAPI(ontology *types.Ontology) ([]byte, *OntoSchema, error) {
	// Query API to get the FileLocation
	params := url.Values{}
	params.Add("lang", "en")

	apiURL := fmt.Sprintf("%s/api/ontologies/%s?%s", BASE_URL, ontology.OntologyName, params.Encode())

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("error while getting content location by calling %s, received non-200 response: %d", apiURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var ontoSchema OntoSchema
	if err := json.Unmarshal(body, &ontoSchema); err != nil {
		return nil, nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Download the Ontology from FileLocation
	resp, err = http.Get(ontoSchema.Config.FileLocation)
	if err != nil {
		return nil, &ontoSchema, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &ontoSchema, fmt.Errorf("error calling %s, received non-200 response: %d", ontoSchema.Config.FileLocation, resp.StatusCode)
	}

	ontologyBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ontoSchema, fmt.Errorf("failed to read response body: %w", err)
	}
	return ontologyBody, &ontoSchema, nil
}

func getOntologyContentFromUI(ontology *types.Ontology) ([]byte, error) {
	params := url.Values{}
	params.Add("lang", "en")

	apiURL := fmt.Sprintf("https://service.tib.eu/ts4tib/api/ontologies/%s/download", ontology.OntologyName)

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error while getting content location by calling %s, received non-200 response: %d", apiURL, resp.StatusCode)
	}

	ontologyContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return ontologyContent, nil
}

func cleanFilename(prefix, input string) string {
	decoded, err := url.QueryUnescape(input)
	if err != nil {
		decoded = input // fallback if decoding fails
	}

	base := filepath.Base(decoded)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// replace non text or digits
	reg := regexp.MustCompile(`[^a-z0-9\-]+`)
	name = reg.ReplaceAllString(name, "")

	// add prefix incase name is empty
	name = prefix + "-" + name
	name = strings.Trim(name, "-")

	return name + ext
}

// guessExtension checks the first non-whitespace character and determines the file extension.
func guessExtension(data []byte) string {
	// Trim leading whitespace
	str := string(bytes.TrimLeftFunc(data, unicode.IsSpace))

	// Check the first non-whitespace character
	switch {
	case strings.HasPrefix(str, "<?xml"):
		return ".owl"
	case strings.HasPrefix(str, "@"), strings.HasPrefix(str, "#"):
		return ".ttl"
	case strings.HasPrefix(str, "<!DOCTYPE html>"):
		return ".html"
	default:
		return ""
	}
}
