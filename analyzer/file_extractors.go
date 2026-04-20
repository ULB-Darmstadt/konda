package analyzer

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/rudolfoborges/pdf2go"
	"github.com/thedatashed/xlsxreader"
	"gopkg.in/yaml.v3"
)

// ExtractText extracts plain text or markdown files.
func ExtractText(file *os.File, sample bool) ([]byte, error) {
	scanner := bufio.NewScanner(file)
	var buffer bytes.Buffer

	for scanner.Scan() {
		buffer.WriteString(scanner.Text() + "\n")
		if sample && buffer.Len() >= 1000 {
			break
		}
	}

	return buffer.Bytes(), scanner.Err()
}

// ExtractCSV extracts CSV or TSV files.
func ExtractCSV(file *os.File, sample bool) ([]byte, error) {
	scanner := bufio.NewScanner(file)
	var rows []string

	// Read all rows
	for scanner.Scan() {
		rows = append(rows, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var selectedRows []string
	if sample && len(rows) > 10 {
		// Select 10 distinct rows evenly spaced
		step := float64(len(rows)-1) / float64(9) // to get exactly 10 rows
		for i := 0; i < 10; i++ {
			index := int(float64(i) * step)
			selectedRows = append(selectedRows, rows[index])
		}
	} else if sample {
		selectedRows = rows
	} else {
		// If not sampling, take up to 1000 rows or all rows if less
		limit := len(rows)
		if limit > 1000 {
			limit = 1000
		}
		selectedRows = rows[:limit]
	}

	// Combine selected rows into buffer
	var buffer bytes.Buffer
	for _, row := range selectedRows {
		buffer.WriteString(row + "\n")
	}

	return buffer.Bytes(), nil
}

// ExtractJSON extracts JSON files with sampling.
func ExtractJSON(file *os.File, sample bool) ([]byte, error) {
	var data any
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return nil, err
	}

	if sample {
		data = sampleData(data, 10)
	}

	return json.MarshalIndent(data, "", "  ")
}

// ExtractYAML extracts YAML files with sampling.
func ExtractYAML(file *os.File, sample bool) ([]byte, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var content any
	if err := yaml.Unmarshal(data, &content); err != nil {
		return nil, err
	}

	if sample {
		content = sampleData(content, 10)
	}

	return yaml.Marshal(content)
}

// ExtractTOML extracts TOML files with sampling.
func ExtractTOML(file *os.File, sample bool) ([]byte, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var content any
	if err := toml.Unmarshal(data, &content); err != nil {
		return nil, err
	}

	if sample {
		content = sampleData(content, 10)
	}

	return toml.Marshal(content)
}

// ExtractDocuments extracts text from .docx and OpenDocument (.odt) files
func ExtractDocuments(file *os.File, sample bool) ([]byte, error) {
	zipReader, err := zip.OpenReader(file.Name())
	if err != nil {
		return nil, err
	}
	defer zipReader.Close()

	var buffer bytes.Buffer

	for _, zipFile := range zipReader.File {
		if zipFile.Name == "word/document.xml" || strings.HasPrefix(zipFile.Name, "content.xml") {
			rc, err := zipFile.Open()
			if err != nil {
				return nil, err
			}

			decoder := xml.NewDecoder(rc)
			var inTextElement bool

			for {
				t, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					rc.Close()
					return nil, err
				}

				switch elem := t.(type) {
				case xml.StartElement:
					if elem.Name.Local == "t" || elem.Name.Local == "p" {
						inTextElement = true
					}
				case xml.CharData:
					if inTextElement {
						buffer.Write(elem)
					}
				case xml.EndElement:
					if elem.Name.Local == "t" || elem.Name.Local == "p" {
						buffer.WriteString("\n")
						inTextElement = false
					}
				}
			}
			rc.Close()
			break // Exit after processing main content file
		}
	}

	content := buffer.Bytes()
	if sample && len(content) > 2000 {
		return content[:2000], nil
	}
	return content, nil
}

// ExtractPDF extracts PDF file text.
func ExtractPDF(file *os.File, sample bool) ([]byte, error) {
	pdf, err := pdf2go.New(file.Name(), pdf2go.Config{LogLevel: pdf2go.LogLevelError})
	if err != nil {
		return nil, err
	}

	pages, err := pdf.Pages()
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	pageLimit := len(pages)
	if sample && pageLimit > 10 {
		pageLimit = 10
	}

	for i := 0; i < pageLimit; i++ {
		text, err := pages[i].Text()
		if err != nil {
			return nil, err
		}
		buffer.WriteString(text + "\n")
	}

	return buffer.Bytes(), nil
}

// ExtractXLSX reads an Excel .xlsx file and returns tab-separated text.
func ExtractXLSX(file *os.File, sample bool) ([]byte, error) {
	xl, err := xlsxreader.OpenFile(file.Name())
	if err != nil {
		return nil, err
	}
	defer xl.Close()

	var buffer bytes.Buffer
	rowLimit := 10
	if !sample {
		rowLimit = 1000
	}

	for _, sheet := range xl.Sheets {
		rowCount := 0
		for row := range xl.ReadRows(sheet) {
			if row.Error != nil {
				return nil, row.Error
			}
			for _, cell := range row.Cells {
				buffer.WriteString(cell.Value + "\t")
			}
			buffer.WriteString("\n")

			rowCount++
			if rowCount >= rowLimit {
				break
			}
		}
	}

	return buffer.Bytes(), nil
}

// ExtractODS extracts text from an .ods spreadsheet
func ExtractODS(file *os.File, sample bool) ([]byte, error) {
	reader, err := zip.OpenReader(file.Name())
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var buffer bytes.Buffer
	rowLimit := 10
	if !sample {
		rowLimit = 1000
	}

	for _, f := range reader.File {
		if f.Name == "content.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()

			decoder := xml.NewDecoder(rc)
			var rowData []string
			rowCount := 0
			inCell := false

			for {
				tok, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					return nil, err
				}

				switch se := tok.(type) {
				case xml.StartElement:
					if se.Name.Local == "table-cell" {
						inCell = true
						rowData = append(rowData, "") // default for empty cells
					}
					if se.Name.Local == "p" && inCell {
						var cellText string
						if err := decoder.DecodeElement(&cellText, &se); err == nil {
							rowData[len(rowData)-1] = strings.TrimSpace(cellText)
						}
					}
				case xml.EndElement:
					if se.Name.Local == "table-row" {
						buffer.WriteString(strings.Join(rowData, "\t") + "\n")
						rowCount++
						rowData = nil
						if rowCount >= rowLimit {
							return buffer.Bytes(), nil
						}
					}
					if se.Name.Local == "table-cell" {
						inCell = false
					}
				}
			}
		}
	}

	return buffer.Bytes(), nil
}

// ExtractPPTX extracts text from PPTX (Microsoft PowerPoint) presentations.
func ExtractPPTX(file *os.File, sample bool) ([]byte, error) {
	reader, err := zip.OpenReader(file.Name())
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var buffer bytes.Buffer
	slideCount := 0
	slideLimit := 10
	if !sample {
		slideLimit = 100
	}

	for _, f := range reader.File {
		if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			decoder := xml.NewDecoder(rc)
			for {
				t, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					rc.Close()
					return nil, err
				}
				if se, ok := t.(xml.StartElement); ok && se.Name.Local == "t" {
					var text string
					if err := decoder.DecodeElement(&text, &se); err != nil {
						rc.Close()
						return nil, err
					}
					buffer.WriteString(text + "\n")
				}
			}
			rc.Close()
			slideCount++
			if slideCount >= slideLimit {
				break
			}
		}
	}

	return buffer.Bytes(), nil
}

// ExtractODP extracts text from ODP (OpenDocument) presentations.
func ExtractODP(file *os.File, sample bool) ([]byte, error) {
	reader, err := zip.OpenReader(file.Name())
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var buffer bytes.Buffer
	slideLimit := 10
	if !sample {
		slideLimit = 100
	}

	for _, f := range reader.File {
		if f.Name == "content.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			decoder := xml.NewDecoder(rc)
			var inText bool
			slideCount := 0

			for {
				t, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					rc.Close()
					return nil, err
				}

				switch elem := t.(type) {
				case xml.StartElement:
					if elem.Name.Local == "p" || elem.Name.Local == "span" {
						inText = true
					}
				case xml.CharData:
					if inText {
						text := strings.TrimSpace(string(elem))
						if text != "" {
							buffer.WriteString(text + "\n")
						}
					}
				case xml.EndElement:
					if elem.Name.Local == "p" || elem.Name.Local == "span" {
						inText = false
					}
					if elem.Name.Local == "page" { // ODP uses draw:page for slides
						slideCount++
						if slideCount >= slideLimit {
							rc.Close()
							return buffer.Bytes(), nil
						}
					}
				}
			}
			rc.Close()
			break // content.xml is processed fully once
		}
	}

	return buffer.Bytes(), nil
}
