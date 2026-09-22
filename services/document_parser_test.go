package services

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// TEST 1: Horizontal table
func Test1_HorizontalTable(t *testing.T) {
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

// TEST 2: Vertical 2-column DOCX table (Exact user reproduction)
func Test2_Vertical2ColumnDocxTable(t *testing.T) {
	parser := NewDocumentParser()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	xmlContent := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:tbl>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Lead Name</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Rahul Sharma</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Company Name</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>TechNova Solutions</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Contact Number</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>+91 9876543210</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Email Address</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>rahul.sharma@technova.example</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Request Type</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>IT Service</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Request Details</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>The client needs a custom CRM solution with Microsoft 365 integration, automated follow-ups, and role-based access for their sales team.</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Lead Owner</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Sahil Derekar</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Priority</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>High</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Lead Status</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Open</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>Estimated Req. Date</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>15-10-2026</w:t></w:r></w:p></w:tc>
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

	leads, err := parser.ExtractLeadsFromDocument(buf.Bytes(), "dummy_lead_test.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if err != nil {
		t.Fatalf("unexpected error parsing vertical docx table: %v", err)
	}
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead, got %d", len(leads))
	}

	lead := leads[0]
	if lead.Contact != "Rahul Sharma" {
		t.Errorf("expected Contact 'Rahul Sharma', got '%s'", lead.Contact)
	}
	if lead.Company != "TechNova Solutions" {
		t.Errorf("expected Company 'TechNova Solutions', got '%s'", lead.Company)
	}
	if lead.Phone != "9876543210" || lead.CountryCode != "IN|+91" {
		t.Errorf("expected Phone '9876543210' with Country 'IN|+91', got '%s' / '%s'", lead.Phone, lead.CountryCode)
	}
	if lead.Email != "rahul.sharma@technova.example" {
		t.Errorf("expected Email 'rahul.sharma@technova.example', got '%s'", lead.Email)
	}
	if lead.RequestType != "IT Service" {
		t.Errorf("expected RequestType 'IT Service', got '%s'", lead.RequestType)
	}
	if !strings.Contains(lead.RequestDetails, "custom CRM solution") {
		t.Errorf("expected RequestDetails containing 'custom CRM solution', got '%s'", lead.RequestDetails)
	}
	if lead.Owner != "Sahil Derekar" {
		t.Errorf("expected Owner 'Sahil Derekar', got '%s'", lead.Owner)
	}
	if lead.Priority != "High" {
		t.Errorf("expected Priority 'High', got '%s'", lead.Priority)
	}
	if lead.Status != "Open" {
		t.Errorf("expected Status 'Open', got '%s'", lead.Status)
	}
	if lead.EstimatedRequirementDate != "2026-10-15" {
		t.Errorf("expected EstimatedRequirementDate '2026-10-15', got '%s'", lead.EstimatedRequirementDate)
	}
}

// TEST 3: Key: Value text
func Test3_KeyValueText(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Lead Name: Rahul Sharma
Company Name: TechNova Solutions
Contact Number: +91 9876543210
Email Address: rahul.sharma@example.com
Lead Owner: Sahil Derekar
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead, got %d", len(leads))
	}
	l := leads[0]
	if l.Contact != "Rahul Sharma" || l.Company != "TechNova Solutions" || l.Email != "rahul.sharma@example.com" {
		t.Errorf("lead mapping mismatch: %+v", l)
	}
	if l.Owner != "Sahil Derekar" {
		t.Errorf("expected Owner 'Sahil Derekar', got '%s'", l.Owner)
	}
}

// TEST 4: Alternating lines
func Test4_AlternatingLines(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Lead Name
Rahul Sharma
Company Name
TechNova Solutions
Contact Number
+91 9876543210
Email Address
rahul.sharma@example.com
Request Type
IT Service
Request Details
The client needs a custom CRM solution with Microsoft 365 integration.
Lead Owner
Sahil Derekar
Priority
High
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead from alternating lines, got %d", len(leads))
	}
	l := leads[0]
	if l.Contact != "Rahul Sharma" {
		t.Errorf("expected Contact 'Rahul Sharma', got '%s'", l.Contact)
	}
	if l.Company != "TechNova Solutions" {
		t.Errorf("expected Company 'TechNova Solutions', got '%s'", l.Company)
	}
	if l.Email != "rahul.sharma@example.com" {
		t.Errorf("expected Email 'rahul.sharma@example.com', got '%s'", l.Email)
	}
	if l.Phone != "9876543210" || l.CountryCode != "IN|+91" {
		t.Errorf("expected Phone '9876543210' IN|+91, got '%s' / '%s'", l.Phone, l.CountryCode)
	}
	if l.RequestType != "IT Service" {
		t.Errorf("expected RequestType 'IT Service', got '%s'", l.RequestType)
	}
	if l.Owner != "Sahil Derekar" {
		t.Errorf("expected Owner 'Sahil Derekar', got '%s'", l.Owner)
	}
	if l.Priority != "High" {
		t.Errorf("expected Priority 'High', got '%s'", l.Priority)
	}
}

// TEST 5: US phone (+1 415 555 0186)
func Test5_USPhoneNumber(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Company: Apple Inc
Contact: Tim Cook
Email: tim@apple.com
Phone: +1 415 555 0186
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead, got %d", len(leads))
	}
	if leads[0].Phone != "4155550186" || leads[0].CountryCode != "US|+1" {
		t.Errorf("US phone extraction mismatch: phone=%s, cc=%s", leads[0].Phone, leads[0].CountryCode)
	}
}

// TEST 6: UK phone (+44 20 7946 0312)
func Test6_UKPhoneNumber(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Company: London Fintech
Contact: Arthur Dent
Email: arthur@fintech.co.uk
Phone: +44 20 7946 0312
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead, got %d", len(leads))
	}
	if leads[0].Phone != "2079460312" || leads[0].CountryCode != "GB|+44" {
		t.Errorf("UK phone extraction mismatch: phone=%s, cc=%s", leads[0].Phone, leads[0].CountryCode)
	}
}

// TEST 7: Explicit valid owner in alternating text
func Test7_ExplicitValidOwner(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Company
Acme Corp
Contact
John Smith
Email
john@acme.com
Lead Owner
Sahil Derekar
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead, got %d", len(leads))
	}
	if leads[0].Owner != "Sahil Derekar" {
		t.Errorf("expected Owner 'Sahil Derekar', got '%s'", leads[0].Owner)
	}
}

// TEST 8: Explicit invalid owner preserved for validation
func Test8_ExplicitInvalidOwnerPreserved(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Company: Acme Corp
Contact: John Smith
Email: john@acme.com
Lead Owner: Nonexistent Person
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead, got %d", len(leads))
	}
	// Parser must preserve "Nonexistent Person" so frontend/backend flags it as invalid
	if leads[0].Owner != "Nonexistent Person" {
		t.Errorf("expected Owner 'Nonexistent Person' preserved, got '%s'", leads[0].Owner)
	}
}

// TEST 9: Missing owner in document
func Test9_MissingOwner(t *testing.T) {
	parser := NewDocumentParser()
	text := `
Company: Acme Corp
Contact: John Smith
Email: john@acme.com
Phone: 9876543210
`
	leads := parser.parseLabelValueBlocks(text)
	if len(leads) != 1 {
		t.Fatalf("expected 1 lead, got %d", len(leads))
	}
	// Parser leaves owner empty when not in document, allowing frontend to set loggedInUser
	if leads[0].Owner != "" {
		t.Errorf("expected empty Owner from parser when absent, got '%s'", leads[0].Owner)
	}
}

// TEST 10: Existing horizontal multi-lead table
func Test10_HorizontalMultiLeadTable(t *testing.T) {
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

// TEST 11: Multi-lead Key: Value document
func Test11_MultiLeadKeyValue(t *testing.T) {
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

// TEST 12: Empty / Corrupted document rejection
func Test12_EmptyAndCorruptedDocument(t *testing.T) {
	parser := NewDocumentParser()
	_, err := parser.ExtractLeadsFromDocument([]byte{}, "empty.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if err == nil || err.Error() != "The uploaded document is empty." {
		t.Fatalf("expected 'The uploaded document is empty.' error, got: %v", err)
	}

	_, err = parser.ExtractLeadsFromDocument([]byte("corrupt content"), "corrupt.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if err == nil {
		t.Fatalf("expected error for corrupted docx, got nil")
	}
}
