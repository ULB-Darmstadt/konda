package analyzer

import "os"

var mimeProcessors = map[string]func(*os.File, bool) ([]byte, error){
	"text/plain":         ExtractText,
	"text/markdown":      ExtractText,
	"application/json":   ExtractJSON,
	"application/x-yaml": ExtractYAML,
	"application/toml":   ExtractTOML,
	"text/csv":           ExtractCSV,
	"text/tsv":           ExtractCSV,
	"application/pdf":    ExtractPDF,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   ExtractDocuments,
	"application/vnd.oasis.opendocument.text":                                   ExtractDocuments,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ExtractXLSX,
	"application/vnd.oasis.opendocument.spreadsheet":                            ExtractODS,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ExtractPPTX,
	"application/vnd.oasis.opendocument.presentation":                           ExtractODP,
}
