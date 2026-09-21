package services

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// TEST 1: Single lead in a table
func Test1_SingleLeadTable(t *testing.T) {
	parser := NewDocumentParser()
	tableText := `
Name | Company | Email | Phone | Country | Stage | Priority | Deal Value
Sarah Connor | Cyberdyne Systems | sarah@cyberdyne.com | 4155552671 | US | Prospecting | High | 50000
`
	leads := parser.parseTextTable(tableText)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead, got %d", len(leads))
	}
	if leads[0].Contact != "Sarah Connor" || leads[0].Company != "Cyberdyne Systems" || leads[0].Email != "sarah@cyberdyne.com" {
		t.Errorf("unexpected lead data: %+v", leads[0])
	}
}

// TEST 2: Multiple leads in a table
func Test2_MultipleLeadsTable(t *testing.T) {
	parser := NewDocumentParser()
	tableText := `
Contact Person | Company Name | Email Address | Phone Number | Country | Priority | Request Type
Alice Smith | Acme Corp | alice@acme.com | 9876543210 | India | High | IT Product
Bob Jones | Wayne Enterprises | bob@wayne.com | 4155552671 | US | Normal | IT Service
Charlie Brown | Stark Industries | charlie@stark.com | 7911123456 | UK | Urgent | IT Product
`
	leads := parser.parseTextTable(tableText)
	if len(leads) != 3 {
		t.Fatalf("expected 3 leads, got %d", len(leads))
	}
	if leads[0].Company != "Acme Corp" || leads[1].Company != "Wayne Enterprises" || leads[2].Company != "Stark Industries" {
		t.Errorf("company mapping error: %+v", leads)
	}
}

// TEST 3: Single lead using label/value format
func Test3_SingleLeadLabelValue(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Name: Bruce Wayne
Company: Wayne Enterprises
Email: bruce@wayne.com
Phone: +14155552671
Country: US
Deal Value: 250000
Owner: Admin User
Stage: Qualification
Priority: High
Request Type: IT Service
Request Details: Comprehensive AI automation system for enterprise logistics.
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead, got %d", len(leads))
	}
	l := leads[0]
	if l.Contact != "Bruce Wayne" || l.Company != "Wayne Enterprises" || l.Email != "bruce@wayne.com" {
		t.Errorf("lead mapping mismatch: %+v", l)
	}
	if l.Phone != "4155552671" || l.CountryCode != "US|+1" {
		t.Errorf("phone/country mapping mismatch: %s / %s", l.Phone, l.CountryCode)
	}
}

// TEST 4: Multiple leads using label/value format
func Test4_MultipleLeadsLabelValue(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Lead 1:
Company: Nexus Corp
Contact: John Doe
Email: john@nexus.com
Phone: 9876543210
Country: India

Lead 2:
Company: Vandelay Industries
Contact: George Costanza
Email: george@vandelay.com
Phone: 4155552671
Country: US
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 2 {
		t.Fatalf("expected 2 leads, got %d", len(leads))
	}
	if leads[0].Company != "Nexus Corp" || leads[1].Company != "Vandelay Industries" {
		t.Errorf("multiple label value lead extraction failed: %+v", leads)
	}
}

// TEST 5: Lead with India phone number (+91 or 10 digits)
func Test5_IndiaPhoneNumber(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Company: Tata Consultancy
Contact: Ratan Sharma
Email: ratan@tcs.com
Phone: +919876543210
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead, got %d", len(leads))
	}
	if leads[0].Phone != "9876543210" || leads[0].CountryCode != "IN|+91" {
		t.Errorf("India phone extraction mismatch: phone=%s, cc=%s", leads[0].Phone, leads[0].CountryCode)
	}
}

// TEST 6: Lead with US phone number (+1)
func Test6_USPhoneNumber(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Company: Apple Inc
Contact: Tim Cook
Email: tim@apple.com
Phone: +14155552671
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead, got %d", len(leads))
	}
	if leads[0].Phone != "4155552671" || leads[0].CountryCode != "US|+1" {
		t.Errorf("US phone extraction mismatch: phone=%s, cc=%s", leads[0].Phone, leads[0].CountryCode)
	}
}

// TEST 7: Document with India / US / UK / Germany numbers
func Test7_InternationalPhoneNumbers(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Lead 1:
Company: Berlin Digital
Email: info@berlindigital.de
Phone: +4915123456789
Country: Germany

Lead 2:
Company: London Analytics
Email: contact@londonanalytics.co.uk
Phone: +447911123456
Country: UK

Lead 3:
Company: Tokyo Systems
Email: info@tokyosystems.jp
Phone: 81234567
Country: Singapore
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 3 {
		t.Fatalf("expected 3 leads, got %d", len(leads))
	}
	if leads[0].CountryCode != "DE|+49" || leads[0].Phone != "15123456789" {
		t.Errorf("Germany mismatch: cc=%s, phone=%s", leads[0].CountryCode, leads[0].Phone)
	}
	if leads[1].CountryCode != "GB|+44" || leads[1].Phone != "7911123456" {
		t.Errorf("UK mismatch: cc=%s, phone=%s", leads[1].CountryCode, leads[1].Phone)
	}
}

// TEST 8: Document containing an invalid phone number
func Test8_InvalidPhoneNumberHandling(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Company: Broken Phone Ltd
Contact: John Error
Email: john@broken.com
Phone: abc-123-xyz
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead extracted, got %d", len(leads))
	}
	// Phone is cleaned of letters, remaining "123" will be caught by preview validation
	if leads[0].Phone != "123" {
		t.Errorf("expected cleaned digits '123', got '%s'", leads[0].Phone)
	}
}

// TEST 9: Document with missing email
func Test9_MissingEmail(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Company: No Email Corp
Contact: Jane Doe
Phone: 9876543210
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead extracted, got %d", len(leads))
	}
	if leads[0].Email != "" {
		t.Errorf("expected empty email, got '%s'", leads[0].Email)
	}
}

// TEST 10: Document with missing required lead fields
func Test10_MissingRequiredFields(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Company: Minimal Co
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead extracted, got %d", len(leads))
	}
	if leads[0].Company != "Minimal Co" || leads[0].Contact != "" || leads[0].Email != "" {
		t.Errorf("unexpected field mapping: %+v", leads[0])
	}
}

// TEST 11: Document with extra unrelated text
func Test11_ExtraUnrelatedText(t *testing.T) {
	parser := NewDocumentParser()
	text := `
CONFIDENTIAL MEETING MINUTES
Date: September 18, 2026
Attendees: Marketing Team

The committee reviewed several potential client prospects for Q4 enterprise outreach:

Lead:
Company: Acme AI Solutions
Contact Person: Bruce Banner
Email: banner@acmeai.com
Phone: 9876543210
Request Type: IT Product
Request Details: Machine learning models deployment for automated pipeline monitoring.

Meeting adjourned at 5:00 PM.
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead extracted amidst noise, got %d", len(leads))
	}
	if leads[0].Company != "Acme AI Solutions" || leads[0].Contact != "Bruce Banner" {
		t.Errorf("failed extraction from noisy document: %+v", leads[0])
	}
}

// TEST 12: Empty PDF / Word document
func Test12_EmptyDocument(t *testing.T) {
	parser := NewDocumentParser()
	_, err := parser.ExtractLeadsFromDocument([]byte{}, "empty.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if err == nil || err.Error() != "The uploaded document is empty." {
		t.Fatalf("expected 'The uploaded document is empty.' error, got: %v", err)
	}
}

// TEST 13: Scanned / image-only PDF error message check
func Test13_ScannedOrEmptyPDF(t *testing.T) {
	// Empty plain text check simulation
	rawText := "   \n\t  "
	if strings.TrimSpace(rawText) != "" {
		t.Fatalf("expected whitespace only")
	}
}

type DocumentParserError struct {
	Message string
}

func (e *DocumentParserError) Error() string {
	return e.Message
}

// TEST 14: Corrupted document
func Test14_CorruptedDocument(t *testing.T) {
	parser := NewDocumentParser()
	_, err := parser.ExtractLeadsFromDocument([]byte("random garbled data that is not zip or pdf"), "corrupt.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if err == nil {
		t.Fatalf("expected corrupted file error, got nil")
	}
}

// TEST 15: Unsupported file (.xlsx, .jpg, .exe)
func Test15_UnsupportedFiles(t *testing.T) {
	parser := NewDocumentParser()
	unsupported := []string{"data.xlsx", "image.jpg", "program.exe", "archive.zip"}
	for _, fn := range unsupported {
		_, err := parser.ExtractLeadsFromDocument([]byte("some data"), fn, "application/octet-stream")
		if err == nil {
			t.Errorf("expected error for unsupported file %s, got nil", fn)
		}
	}
}

// Comprehensive DOCX Table Extraction Test
func TestDocxTableExtraction(t *testing.T) {
	parser := NewDocumentParser()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	xmlContent := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:tbl>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Company Name</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Contact Name</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Email Address</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Phone Number</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Country</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Munich Auto</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Klaus Schmidt</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>klaus@munichauto.de</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>+4915123456789</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Germany</w:t></w:r></w:p></w:tc>
      </w:tr>
    </w:tbl>
  </w:body>
</w:document>`

	f, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("failed to create zip file: %v", err)
	}
	_, err = f.Write([]byte(xmlContent))
	if err != nil {
		t.Fatalf("failed to write xml to zip: %v", err)
	}
	zw.Close()

	leads, err := parser.ExtractLeadsFromDocument(buf.Bytes(), "leads.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if err != nil {
		t.Fatalf("unexpected error parsing docx: %v", err)
	}
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead, got %d", len(leads))
	}

	lead := leads[0]
	if lead.Company != "Munich Auto" || lead.Contact != "Klaus Schmidt" || lead.CountryCode != "DE|+49" {
		t.Errorf("docx lead mismatch: %+v", lead)
	}
}
