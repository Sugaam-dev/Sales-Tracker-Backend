package services

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
	"crm-auth-service/models"
)

// DocumentParser handles parsing leads from PDF, DOCX, and DOC files.
type DocumentParser struct{}

func NewDocumentParser() *DocumentParser {
	return &DocumentParser{}
}

// ExtractLeadsFromDocument parses the file bytes based on file extension / MIME type and extracts leads.
func (p *DocumentParser) ExtractLeadsFromDocument(fileBytes []byte, filename string, contentType string) ([]models.CreateLeadRequest, error) {
	if len(fileBytes) == 0 {
		return nil, errors.New("The uploaded document is empty.")
	}

	// 10MB limit check
	if len(fileBytes) > 10*1024*1024 {
		return nil, errors.New("File size exceeds maximum allowed limit of 10MB.")
	}

	ext := strings.ToLower(filepath.Ext(filename))
	
	// Security check on extension
	unsupportedExts := map[string]bool{
		".exe": true, ".bat": true, ".cmd": true, ".sh": true, ".msi": true,
		".js": true, ".vbs": true, ".zip": true, ".tar": true, ".gz": true,
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
		".xlsx": true, ".xls": true, ".ppt": true, ".pptx": true,
	}
	if unsupportedExts[ext] {
		return nil, fmt.Errorf("Unsupported file format '%s'. Please upload CSV, PDF, DOC, or DOCX.", ext)
	}

	var rawText string
	var tables [][]string
	var err error

	switch ext {
	case ".pdf":
		rawText, tables, err = p.parsePDF(fileBytes)
		if err != nil {
			return nil, err
		}
	case ".docx":
		rawText, tables, err = p.parseDOCX(fileBytes)
		if err != nil {
			return nil, err
		}
	case ".doc":
		rawText, err = p.parseDOC(fileBytes)
		if err != nil {
			return nil, err
		}
	default:
		// Attempt magic byte inspection
		sniffed := http.DetectContentType(fileBytes[:min(512, len(fileBytes))])
		if strings.Contains(sniffed, "pdf") {
			rawText, tables, err = p.parsePDF(fileBytes)
		} else if strings.Contains(sniffed, "zip") || strings.Contains(sniffed, "officedocument") {
			rawText, tables, err = p.parseDOCX(fileBytes)
		} else {
			return nil, errors.New("Unsupported file format. Please upload CSV, PDF, DOC, or DOCX.")
		}
		if err != nil {
			return nil, err
		}
	}

	// 1. If explicit structured tables were extracted (e.g. from DOCX)
	if len(tables) > 0 {
		// A. Try Horizontal Table Parser (e.g. multi-column headers on Row 0)
		horizontalLeads := p.parseTableRows(tables)
		if len(horizontalLeads) > 0 {
			return horizontalLeads, nil
		}

		// B. Try Vertical 2-Column Key-Value Table Parser
		verticalLeads := p.parseVerticalTable(tables)
		if len(verticalLeads) > 0 {
			return verticalLeads, nil
		}
	}

	// 2. Check if text is extractable
	trimmedText := strings.TrimSpace(rawText)
	if trimmedText == "" {
		if ext == ".pdf" {
			return nil, errors.New("This PDF does not contain extractable text. Please upload a text-based PDF or use a supported OCR workflow.")
		}
		return nil, errors.New("The uploaded document does not contain readable lead data.")
	}

	// 3. Try parsing text-based horizontal tables (e.g. pipe '|' separated or tab separated)
	textTableLeads := p.parseTextTable(trimmedText)
	if len(textTableLeads) > 0 {
		return textTableLeads, nil
	}

	// 4. Try parsing multi-lead / label-value blocks (supports "Key: Value" and alternating "Label \n Value")
	labelValueLeads := p.parseLabelValueBlocks(trimmedText)
	if len(labelValueLeads) > 0 {
		return labelValueLeads, nil
	}

	return nil, errors.New("We couldn't identify lead information from this document.")
}

// -------------------------------------------------------------
// PDF Parser
// -------------------------------------------------------------

func (p *DocumentParser) parsePDF(fileBytes []byte) (string, [][]string, error) {
	readerAt := bytes.NewReader(fileBytes)
	pdfReader, err := pdf.NewReader(readerAt, int64(len(fileBytes)))
	if err != nil {
		return "", nil, errors.New("Corrupted or invalid PDF document.")
	}

	numPages := pdfReader.NumPage()
	if numPages == 0 {
		return "", nil, errors.New("This PDF does not contain any pages.")
	}

	var sb strings.Builder
	for pageIndex := 1; pageIndex <= numPages; pageIndex++ {
		page := pdfReader.Page(pageIndex)
		if page.V.IsNull() {
			continue
		}
		plainText, err := page.GetPlainText(nil)
		if err == nil {
			sb.WriteString(plainText)
			sb.WriteString("\n")
		}
	}

	extracted := sb.String()
	if strings.TrimSpace(extracted) == "" {
		return "", nil, errors.New("This PDF does not contain extractable text. Please upload a text-based PDF or use a supported OCR workflow.")
	}

	return extracted, nil, nil
}

// -------------------------------------------------------------
// DOCX Parser (Pure Go via archive/zip & XML)
// -------------------------------------------------------------

func (p *DocumentParser) parseDOCX(fileBytes []byte) (string, [][]string, error) {
	readerAt := bytes.NewReader(fileBytes)
	zipReader, err := zip.NewReader(readerAt, int64(len(fileBytes)))
	if err != nil {
		return "", nil, errors.New("Corrupted or invalid DOCX document.")
	}

	var docFile *zip.File
	for _, f := range zipReader.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}

	if docFile == nil {
		return "", nil, errors.New("Invalid DOCX format: word/document.xml not found.")
	}

	rc, err := docFile.Open()
	if err != nil {
		return "", nil, errors.New("Failed to read DOCX document contents.")
	}
	defer rc.Close()

	xmlData, err := io.ReadAll(rc)
	if err != nil {
		return "", nil, errors.New("Failed to read DOCX XML stream.")
	}

	return p.extractTextAndTablesFromDocxXML(xmlData)
}

func (p *DocumentParser) extractTextAndTablesFromDocxXML(xmlBytes []byte) (string, [][]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(xmlBytes))

	var fullText strings.Builder
	var allTables [][]string

	var inTable bool
	var inRow bool
	var inCell bool
	var inParagraph bool
	var currentParagraph strings.Builder
	var currentCell strings.Builder
	var currentRow []string

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		switch elem := token.(type) {
		case xml.StartElement:
			name := elem.Name.Local
			switch name {
			case "tbl":
				inTable = true
			case "tr":
				inRow = true
				currentRow = []string{}
			case "tc":
				inCell = true
				currentCell.Reset()
			case "p":
				inParagraph = true
				currentParagraph.Reset()
			}
		case xml.EndElement:
			name := elem.Name.Local
			switch name {
			case "p":
				inParagraph = false
				pText := strings.TrimSpace(currentParagraph.String())
				if pText != "" {
					fullText.WriteString(pText)
					fullText.WriteString("\n")
				}
				if inCell {
					if currentCell.Len() > 0 && pText != "" {
						currentCell.WriteString(" ")
					}
					currentCell.WriteString(pText)
				}
			case "tc":
				inCell = false
				if inRow {
					currentRow = append(currentRow, strings.TrimSpace(currentCell.String()))
				}
			case "tr":
				inRow = false
				if inTable && len(currentRow) > 0 {
					allTables = append(allTables, currentRow)
				}
			case "tbl":
				inTable = false
			}
		case xml.CharData:
			text := string(elem)
			if inParagraph {
				currentParagraph.WriteString(text)
			}
		}
	}

	return fullText.String(), allTables, nil
}

// -------------------------------------------------------------
// DOC Parser (Word 97-2003 Binary format handler)
// -------------------------------------------------------------

func (p *DocumentParser) parseDOC(fileBytes []byte) (string, error) {
	// Inspect OLE header (Magic: 0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1)
	if len(fileBytes) < 8 || fileBytes[0] != 0xD0 || fileBytes[1] != 0xCF {
		// Not a standard OLE2 doc, attempt clean ASCII/Unicode text scan
		extracted := p.extractPrintableStrings(fileBytes)
		if len(extracted) < 20 {
			return "", errors.New("This .doc file is not a valid Microsoft Word 97-2003 document. Please save as .docx or .pdf.")
		}
		return extracted, nil
	}

	extracted := p.extractPrintableStrings(fileBytes)
	if len(strings.TrimSpace(extracted)) < 20 {
		return "", errors.New("Could not extract readable lead data from this .doc file. Please save as .docx or .pdf for full compatibility.")
	}

	return extracted, nil
}

func (p *DocumentParser) extractPrintableStrings(data []byte) string {
	var sb strings.Builder
	var current strings.Builder

	for _, b := range data {
		if (b >= 32 && b <= 126) || b == '\t' {
			current.WriteByte(b)
		} else if b == '\n' || b == '\r' {
			if current.Len() > 0 {
				sb.WriteString(current.String())
				sb.WriteString("\n")
				current.Reset()
			}
		} else {
			if current.Len() >= 4 {
				sb.WriteString(current.String())
				sb.WriteString("\n")
			}
			current.Reset()
		}
	}
	if current.Len() >= 4 {
		sb.WriteString(current.String())
		sb.WriteString("\n")
	}
	return sb.String()
}

// -------------------------------------------------------------
// Heuristic Field Aliases & Normalization
// -------------------------------------------------------------

var fieldAliases = map[string][]string{
	"company": {
		"company", "company_name", "company name", "organization", "organisation", "account", "client", "client company", "business name",
	},
	"contact": {
		"contact", "contact_person", "contact person", "contact name", "name", "full name", "lead name", "person", "representative",
	},
	"projectName": {
		"project_name", "project name", "project", "deal name",
	},
	"email": {
		"email", "email_address", "email address", "e-mail", "mail", "contact email",
	},
	"phone": {
		"phone", "phone_number", "phone number", "mobile", "mobile_number", "contact_number", "contact number", "telephone", "tel", "cell",
	},
	"countryCode": {
		"country", "country_code", "country code", "countrycode", "region_code", "dial code", "phone country",
	},
	"officePhone": {
		"office_phone", "office phone", "office phone number", "work phone", "office tel", "office",
	},
	"officePhoneCountry": {
		"office_phone_country", "office phone country", "office country",
	},
	"alternatePhone": {
		"alternate_phone", "alternate phone", "secondary phone", "alt phone", "other phone",
	},
	"alternatePhoneCountry": {
		"alternate_phone_country", "alternate phone country",
	},
	"owner": {
		"owner", "lead_owner", "lead owner", "assigned to", "assigned_to", "sales rep", "sales executive", "rep",
	},
	"kamName": {
		"kam_name", "kam name", "kam", "key account manager", "account manager",
	},
	"designation": {
		"designation", "title", "job title", "role", "position",
	},
	"industry": {
		"industry", "sector", "vertical",
	},
	"size": {
		"size", "company_size", "company size", "employees", "headcount",
	},
	"region": {
		"region", "location", "geography", "territory",
	},
	"source": {
		"source", "lead_source", "lead source", "channel",
	},
	"stage": {
		"stage", "lead_stage", "lead stage", "pipeline stage",
	},
	"status": {
		"status", "lead_status", "lead status",
	},
	"sentiment": {
		"sentiment", "lead sentiment",
	},
	"priority": {
		"priority", "urgency",
	},
	"value": {
		"value", "deal_value", "deal value", "amount", "deal amount", "revenue", "opportunity value",
	},
	"requestType": {
		"request_type", "request type", "requesttype", "service type", "offering", "type",
	},
	"requestDetails": {
		"request_details", "request details", "requestdetails", "requirement_details", "requirements", "requirement", "basic_requirements", "basic requirements", "scope", "description",
	},
	"estimatedRequirementDate": {
		"estimated_requirement_date", "estimated requirement date", "est_requirement_date", "est requirement date", "est. requirement date", "estimated req. date", "estimated req date", "target date", "deadline",
	},
	"notes": {
		"notes", "comments", "remarks", "additional info",
	},
}

func matchFieldAlias(rawHeader string) string {
	cleaned := strings.ToLower(strings.TrimSpace(rawHeader))
	cleaned = strings.Trim(cleaned, ":- \t\"'*#.")
	if cleaned == "" {
		return ""
	}

	for field, aliases := range fieldAliases {
		for _, alias := range aliases {
			if cleaned == alias || strings.EqualFold(cleaned, alias) {
				return field
			}
		}
	}
	return ""
}

// -------------------------------------------------------------
// Table Parsers (Horizontal Format A & Vertical 2-Column)
// -------------------------------------------------------------

// parseTableRows parses horizontal multi-column tables where row 0 contains headers.
func (p *DocumentParser) parseTableRows(rows [][]string) []models.CreateLeadRequest {
	if len(rows) < 2 {
		return nil
	}

	// Find the horizontal header row (requires >= 2 recognized column headers)
	headerIdx := -1
	var matchedColumns []string

	for rIdx, row := range rows {
		matches := 0
		colMap := make([]string, len(row))
		for cIdx, cell := range row {
			matched := matchFieldAlias(cell)
			colMap[cIdx] = matched
			if matched != "" {
				matches++
			}
		}
		// Require at least 2 recognized headers in this single row
		if matches >= 2 && matches > len(matchedColumns) {
			headerIdx = rIdx
			matchedColumns = colMap
		}
	}

	if headerIdx == -1 {
		return nil
	}

	var leads []models.CreateLeadRequest
	for i := headerIdx + 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) == 0 {
			continue
		}

		hasContent := false
		for _, c := range row {
			if strings.TrimSpace(c) != "" {
				hasContent = true
				break
			}
		}
		if !hasContent {
			continue
		}

		fieldMap := make(map[string]string)
		for cIdx, cellVal := range row {
			if cIdx < len(matchedColumns) && matchedColumns[cIdx] != "" {
				fieldMap[matchedColumns[cIdx]] = strings.TrimSpace(cellVal)
			}
		}

		lead := p.buildLeadFromMap(fieldMap)
		if lead.Email != "" || lead.Company != "" || lead.Contact != "" {
			leads = append(leads, lead)
		}
	}

	return leads
}

// parseVerticalTable parses 2-column key-value tables where column 0 contains field labels and column 1 contains values.
func (p *DocumentParser) parseVerticalTable(rows [][]string) []models.CreateLeadRequest {
	if len(rows) < 2 {
		return nil
	}

	var leads []models.CreateLeadRequest
	fieldMap := make(map[string]string)
	seenKeys := make(map[string]bool)
	recognizedCount := 0

	flushLead := func() {
		if len(fieldMap) == 0 {
			return
		}
		lead := p.buildLeadFromMap(fieldMap)
		if lead.Email != "" || lead.Company != "" || lead.Contact != "" {
			leads = append(leads, lead)
		}
		fieldMap = make(map[string]string)
		seenKeys = make(map[string]bool)
	}

	for _, row := range rows {
		if len(row) < 2 {
			continue
		}

		label := strings.TrimSpace(row[0])
		val := strings.TrimSpace(row[1])

		// Handle table rows with extra empty/merged cells
		if len(row) > 2 && val == "" {
			for c := 2; c < len(row); c++ {
				if strings.TrimSpace(row[c]) != "" {
					val = strings.TrimSpace(row[c])
					break
				}
			}
		}

		alias := matchFieldAlias(label)
		if alias != "" {
			recognizedCount++
			// If we see a duplicate main key and we already have fields, flush current lead
			if (alias == "company" || alias == "email" || alias == "contact") && seenKeys[alias] && len(fieldMap) >= 2 {
				flushLead()
			}
			fieldMap[alias] = val
			seenKeys[alias] = true
		}
	}

	flushLead()

	// Require at least 2 recognized field labels across the vertical table
	if recognizedCount >= 2 && len(leads) > 0 {
		return leads
	}

	return nil
}

func (p *DocumentParser) parseTextTable(text string) []models.CreateLeadRequest {
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleanedLines = append(cleanedLines, trimmed)
		}
	}

	if len(cleanedLines) < 2 {
		return nil
	}

	// Try Delimiters: '|', '\t', ','
	for _, delim := range []string{"|", "\t", ","} {
		var tableRows [][]string
		for _, line := range cleanedLines {
			if delim == "|" && !strings.Contains(line, "|") {
				continue
			}
			parts := strings.Split(line, delim)
			if len(parts) >= 2 {
				var cleanParts []string
				for _, part := range parts {
					cleanParts = append(cleanParts, strings.TrimSpace(part))
				}
				tableRows = append(tableRows, cleanParts)
			}
		}

		if len(tableRows) >= 2 {
			// Try horizontal table
			leads := p.parseTableRows(tableRows)
			if len(leads) > 0 {
				return leads
			}
			// Try vertical 2-column table
			verticalLeads := p.parseVerticalTable(tableRows)
			if len(verticalLeads) > 0 {
				return verticalLeads
			}
		}
	}

	return nil
}

// -------------------------------------------------------------
// Label/Value and Alternating Lines Parser (Format B, C, D)
// -------------------------------------------------------------

func (p *DocumentParser) parseLabelValueBlocks(text string) []models.CreateLeadRequest {
	rawLines := strings.Split(text, "\n")
	var lines []string
	for _, l := range rawLines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	if len(lines) == 0 {
		return nil
	}

	leadBoundaryRegex := regexp.MustCompile(`(?i)^(?:lead|record|prospect)\s*(?:#|\d+|:|-)\s*\d+.*$|^[-=_*]{3,}$`)
	keyValRegex := regexp.MustCompile(`^([^:\t]{2,35})[:\t]\s*(.+)$`)

	var leads []models.CreateLeadRequest
	fieldMap := make(map[string]string)
	seenKeys := make(map[string]bool)

	flushLead := func() {
		if len(fieldMap) == 0 {
			return
		}
		// Fallback email heuristic if email wasn't explicitly paired
		if fieldMap["email"] == "" {
			emailRegex := regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
			for _, v := range fieldMap {
				if match := emailRegex.FindString(v); match != "" {
					fieldMap["email"] = match
					break
				}
			}
		}

		lead := p.buildLeadFromMap(fieldMap)
		if lead.Email != "" || lead.Company != "" || lead.Contact != "" {
			leads = append(leads, lead)
		}
		fieldMap = make(map[string]string)
		seenKeys = make(map[string]bool)
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// 1. Boundary check (e.g. "Lead 1", "---")
		if leadBoundaryRegex.MatchString(line) {
			flushLead()
			continue
		}

		// 2. Check for single-line "Key: Value" or "Key \t Value"
		if matches := keyValRegex.FindStringSubmatch(line); len(matches) == 3 {
			key := strings.TrimSpace(matches[1])
			val := strings.TrimSpace(matches[2])
			mappedField := matchFieldAlias(key)
			if mappedField != "" {
				if seenKeys[mappedField] && len(fieldMap) >= 2 {
					flushLead()
				}
				fieldMap[mappedField] = val
				seenKeys[mappedField] = true
				continue
			}
		}

		// 3. Check for alternating line "Label \n Value"
		mappedField := matchFieldAlias(line)
		if mappedField != "" {
			val := ""
			// Lookahead to line i+1 for value
			if i+1 < len(lines) {
				nextLine := lines[i+1]
				isNextBoundary := leadBoundaryRegex.MatchString(nextLine)
				isNextLabel := matchFieldAlias(nextLine) != ""
				isNextKeyVal := keyValRegex.MatchString(nextLine) && matchFieldAlias(keyValRegex.FindStringSubmatch(nextLine)[1]) != ""

				if !isNextBoundary && !isNextLabel && !isNextKeyVal {
					val = nextLine
					i++ // Consume next line as value
				}
			}

			if seenKeys[mappedField] && len(fieldMap) >= 2 {
				flushLead()
			}
			fieldMap[mappedField] = val
			seenKeys[mappedField] = true
			continue
		}

		// 4. Standalone plain email heuristic if encountered
		emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
		if emailRegex.MatchString(line) && fieldMap["email"] == "" {
			fieldMap["email"] = line
			seenKeys["email"] = true
		}
	}

	flushLead()
	return leads
}

// -------------------------------------------------------------
// Lead Building & Normalization
// -------------------------------------------------------------

func (p *DocumentParser) buildLeadFromMap(m map[string]string) models.CreateLeadRequest {
	req := models.CreateLeadRequest{
		Company:                  strings.TrimSpace(m["company"]),
		CompanyName:              strings.TrimSpace(m["company"]),
		Contact:                  strings.TrimSpace(m["contact"]),
		LeadName:                 strings.TrimSpace(m["contact"]),
		ProjectName:              strings.TrimSpace(m["projectName"]),
		Email:                    strings.ToLower(strings.TrimSpace(m["email"])),
		Designation:              strings.TrimSpace(m["designation"]),
		Industry:                 strings.TrimSpace(m["industry"]),
		Size:                     strings.TrimSpace(m["size"]),
		Region:                   strings.TrimSpace(m["region"]),
		Source:                   strings.TrimSpace(m["source"]),
		Stage:                    strings.TrimSpace(m["stage"]),
		Status:                   strings.TrimSpace(m["status"]),
		Sentiment:                strings.TrimSpace(m["sentiment"]),
		Priority:                 strings.TrimSpace(m["priority"]),
		Value:                    strings.TrimSpace(m["value"]),
		LostReason:               strings.TrimSpace(m["lostReason"]),
		RequestType:              strings.TrimSpace(m["requestType"]),
		RequestDetails:           strings.TrimSpace(m["requestDetails"]),
		BasicRequirements:        strings.TrimSpace(m["requestDetails"]),
		LifecycleTemplate:        strings.TrimSpace(m["lifecycleTemplate"]),
		KamName:                  strings.TrimSpace(m["kamName"]),
		Owner:                    strings.TrimSpace(m["owner"]),
		LeadOwner:                strings.TrimSpace(m["owner"]),
		BestTimeToConnect:        strings.TrimSpace(m["bestTimeToConnect"]),
		LinkedinProfileUrl:       strings.TrimSpace(m["linkedinProfileUrl"]),
		LinkedinCompanyPageUrl:   strings.TrimSpace(m["linkedinCompanyPageUrl"]),
		EstimatedRequirementDate: strings.TrimSpace(m["estimatedRequirementDate"]),
		Notes:                    strings.TrimSpace(m["notes"]),
	}

	// If KAM Name is missing, default to Contact Person
	if req.KamName == "" && req.Contact != "" {
		req.KamName = req.Contact
	}
	if req.Contact == "" && req.KamName != "" {
		req.Contact = req.KamName
		req.LeadName = req.KamName
	}

	// Normalizing Request Type ("IT Product" or "IT Service")
	if req.RequestType != "" {
		lowerType := strings.ToLower(req.RequestType)
		if strings.Contains(lowerType, "service") {
			req.RequestType = "IT Service"
		} else if strings.Contains(lowerType, "product") {
			req.RequestType = "IT Product"
		}
	} else {
		req.RequestType = "IT Product"
	}

	// Default Status, Stage, Priority, Sentiment
	if req.Status == "" {
		req.Status = "Open"
	}
	if req.Priority == "" {
		req.Priority = "Normal"
	} else {
		pLower := strings.ToLower(req.Priority)
		if pLower == "low" {
			req.Priority = "Low"
		} else if pLower == "high" {
			req.Priority = "High"
		} else if pLower == "urgent" {
			req.Priority = "Urgent"
		} else {
			req.Priority = "Normal"
		}
	}
	if req.Sentiment == "" {
		req.Sentiment = "Neutral"
	}
	if req.Stage == "" {
		req.Stage = "Prospecting"
	}

	// Normalize Phone & Country
	rawPhone := strings.TrimSpace(m["phone"])
	rawCountry := strings.TrimSpace(m["countryCode"])
	parsedPhone, parsedCountry := p.normalizePhoneAndCountry(rawPhone, rawCountry)
	req.Phone = parsedPhone
	req.CountryCode = parsedCountry

	// Office phone
	rawOfficePhone := strings.TrimSpace(m["officePhone"])
	rawOfficeCountry := strings.TrimSpace(m["officePhoneCountry"])
	if rawOfficePhone != "" {
		pOffice, cOffice := p.normalizePhoneAndCountry(rawOfficePhone, rawOfficeCountry)
		req.OfficePhone = pOffice
		req.OfficePhoneCountry = cOffice
	} else {
		req.OfficePhone = parsedPhone
		req.OfficePhoneCountry = parsedCountry
	}

	// Alternate phone
	rawAltPhone := strings.TrimSpace(m["alternatePhone"])
	rawAltCountry := strings.TrimSpace(m["alternatePhoneCountry"])
	if rawAltPhone != "" {
		pAlt, cAlt := p.normalizePhoneAndCountry(rawAltPhone, rawAltCountry)
		req.AlternatePhone = pAlt
		req.AlternatePhoneCountry = cAlt
	}

	// Normalize Date
	if req.EstimatedRequirementDate != "" {
		req.EstimatedRequirementDate = p.normalizeDate(req.EstimatedRequirementDate)
	}

	return req
}

// normalizePhoneAndCountry parses phone string, extracting dial codes (e.g. +1, +91, +44, +49)
// and country names/ISOs if provided.
func (p *DocumentParser) normalizePhoneAndCountry(phoneStr, countryStr string) (string, string) {
	trimmedPhone := strings.TrimSpace(phoneStr)
	trimmedCountry := strings.TrimSpace(countryStr)

	// Clean any non-digit chars except leading '+'
	isInternational := strings.HasPrefix(trimmedPhone, "+")
	var digits strings.Builder
	for _, r := range trimmedPhone {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	digitStr := digits.String()

	// Default Country
	countryCode := "IN|+91"

	// Match country code from countryStr if provided
	if trimmedCountry != "" {
		cUpper := strings.ToUpper(trimmedCountry)
		switch {
		case cUpper == "US" || cUpper == "USA" || cUpper == "UNITED STATES" || strings.Contains(trimmedCountry, "+1"):
			countryCode = "US|+1"
		case cUpper == "IN" || cUpper == "INDIA" || strings.Contains(trimmedCountry, "+91"):
			countryCode = "IN|+91"
		case cUpper == "GB" || cUpper == "UK" || cUpper == "UNITED KINGDOM" || strings.Contains(trimmedCountry, "+44"):
			countryCode = "GB|+44"
		case cUpper == "DE" || cUpper == "GERMANY" || strings.Contains(trimmedCountry, "+49"):
			countryCode = "DE|+49"
		case cUpper == "AU" || cUpper == "AUSTRALIA" || strings.Contains(trimmedCountry, "+61"):
			countryCode = "AU|+61"
		case cUpper == "CA" || cUpper == "CANADA":
			countryCode = "CA|+1"
		case cUpper == "AE" || cUpper == "UAE" || strings.Contains(trimmedCountry, "+971"):
			countryCode = "AE|+971"
		case cUpper == "SG" || cUpper == "SINGAPORE" || strings.Contains(trimmedCountry, "+65"):
			countryCode = "SG|+65"
		default:
			if strings.Contains(trimmedCountry, "|") {
				countryCode = trimmedCountry
			}
		}
	}

	// If phone starts with + or contains country prefix
	if isInternational {
		if strings.HasPrefix(digitStr, "91") && len(digitStr) >= 12 {
			countryCode = "IN|+91"
			digitStr = digitStr[2:]
		} else if strings.HasPrefix(digitStr, "1") && len(digitStr) >= 11 {
			countryCode = "US|+1"
			digitStr = digitStr[1:]
		} else if strings.HasPrefix(digitStr, "44") && len(digitStr) >= 11 {
			countryCode = "GB|+44"
			digitStr = digitStr[2:]
		} else if strings.HasPrefix(digitStr, "49") && len(digitStr) >= 11 {
			countryCode = "DE|+49"
			digitStr = digitStr[2:]
		} else if strings.HasPrefix(digitStr, "971") && len(digitStr) >= 11 {
			countryCode = "AE|+971"
			digitStr = digitStr[3:]
		} else if strings.HasPrefix(digitStr, "65") && len(digitStr) >= 10 {
			countryCode = "SG|+65"
			digitStr = digitStr[2:]
		}
	}

	return digitStr, countryCode
}

func (p *DocumentParser) normalizeDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	formats := []string{
		"2006-01-02",
		"02-01-2006",
		"02/01/2006",
		"2006/01/02",
		"02 Jan 2006",
		"02 January 2006",
		"January 02, 2006",
		"Jan 02, 2006",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, raw); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return raw
}
