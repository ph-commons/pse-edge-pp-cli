// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package quotationreport

const (
	ContractID = "pse-edge-quotation-report-v1"
	SourceType = "pse_daily_quotation_report"

	StatusOK                   = "ok"
	StatusListingUnavailable   = "listing_unavailable"
	StatusReportUnavailable    = "report_unavailable"
	StatusAcquisitionFailed    = "acquisition_failed"
	StatusWrongSessionDate     = "wrong_session_date"
	StatusMalformedDocument    = "malformed_document"
	StatusTextExtractorMissing = "text_extractor_missing"
	StatusPartialParse         = "partial_parse"
	StatusMissingSymbols       = "missing_symbols"
	StatusAmbiguousIdentity    = "ambiguous_identity"
	StatusConflictingListing   = "conflicting_listing"

	FieldOK           = "ok"
	FieldReportedDash = "reported_dash"
)

// Error is a distinguishable report failure. Detail is safe to print.
type Error struct {
	Status string
	Detail string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Detail == "" {
		return e.Status
	}
	return e.Status + ": " + e.Detail
}

func statusErr(status, detail string) *Error {
	return &Error{Status: status, Detail: detail}
}

// Measure is one reported number. A dash is a null with FieldReportedDash.
// It is not a zero and it is not a suspension.
type Measure struct {
	Value  *float64 `json:"value"`
	Status string   `json:"status"`
}

// Row is one security line from the quotation table.
type Row struct {
	Symbol        string            `json:"symbol"`
	IssueName     string            `json:"issue_name"`
	Board         string            `json:"board"`
	SecurityClass string            `json:"security_class"`
	Currency      string            `json:"currency"`
	SessionDate   string            `json:"session_date"`
	SourceType    string            `json:"source_type"`
	SourceSHA256  string            `json:"source_sha256"`
	Page          int               `json:"page"`
	RowLocator    string            `json:"row_locator"`
	Bid           *float64          `json:"bid"`
	Ask           *float64          `json:"ask"`
	Open          *float64          `json:"open"`
	High          *float64          `json:"high"`
	Low           *float64          `json:"low"`
	Close         *float64          `json:"close"`
	Volume        *float64          `json:"volume"`
	Value         *float64          `json:"value"`
	NetForeign    *float64          `json:"net_foreign"`
	FieldStatus   map[string]string `json:"field_status"`
}

// AmbiguousSymbol is a symbol the report stated more than once.
type AmbiguousSymbol struct {
	Symbol   string   `json:"symbol"`
	Locators []string `json:"locators"`
}

// Parsed is the admitted table. Ambiguous symbols are not rows.
type Parsed struct {
	SessionDate string            `json:"session_date"`
	Rows        []Row             `json:"rows"`
	Ambiguous   []AmbiguousSymbol `json:"ambiguous"`
}

// Discovery is how the PDF URL was found. Nonces are not stored.
type Discovery struct {
	ListingURL   string `json:"listing_url"`
	AjaxURL      string `json:"ajax_url"`
	TableID      string `json:"table_id"`
	MatchedTitle string `json:"matched_title"`
	MatchedSlug  string `json:"matched_category"`
	UploadDate   string `json:"upload_date"`
	SessionTitle string `json:"document_session_date"`
	PDFURL       string `json:"pdf_url"`
}

// Source is retained evidence for one admitted PDF.
type Source struct {
	Type       string    `json:"type"`
	URL        string    `json:"url"`
	SHA256     string    `json:"sha256"`
	ByteLength int       `json:"byte_length"`
	AcquiredAt string    `json:"acquired_at"`
	Discovery  Discovery `json:"discovery"`
	PDFPath    string    `json:"pdf_path"`
}

// Document is the stored parse plus evidence. Coverage is added at read time.
type Document struct {
	Contract    string            `json:"contract"`
	SessionDate string            `json:"session_date"`
	Source      Source            `json:"source"`
	Rows        []Row             `json:"rows"`
	Ambiguous   []AmbiguousSymbol `json:"ambiguous"`
}

// Coverage is the caller's symbol request against one document.
type Coverage struct {
	Requested        []string `json:"requested"`
	Present          []string `json:"present"`
	Absent           []string `json:"absent"`
	PresentNullClose []string `json:"present_null_close"`
	Ambiguous        []string `json:"ambiguous"`
	RowCount         int      `json:"row_count"`
}

// Report is the command payload.
type Report struct {
	Contract    string            `json:"contract"`
	SessionDate string            `json:"session_date"`
	Status      string            `json:"status"`
	Detail      string            `json:"detail,omitempty"`
	Reused      bool              `json:"reused"`
	Revision    bool              `json:"revision"`
	PreviousSHA string            `json:"previous_sha256,omitempty"`
	Source      *Source           `json:"source,omitempty"`
	Coverage    Coverage          `json:"coverage"`
	Rows        []Row             `json:"rows"`
	Ambiguous   []AmbiguousSymbol `json:"ambiguous"`
	Limits      Limits            `json:"limits"`
	RetainedSHA string            `json:"retained_sha256,omitempty"`
}

// Limits states what this payload does not claim.
type Limits struct {
	PublicationTiming string `json:"publication_timing"`
	DistinctFrom      string `json:"distinct_from"`
	DashMeaning       string `json:"dash_meaning"`
}

func defaultLimits() Limits {
	return Limits{
		PublicationTiming: "acquired_at is the only clock; a dated file does not mean the report was ready at 16:30",
		DistinctFrom:      "pse-edge-export-eod-v1 chart history is a different source and is not backfilled from this report",
		DashMeaning:       "a dash is a null reported_dash; it is not a zero, a suspension, or proof of no trading",
	}
}
